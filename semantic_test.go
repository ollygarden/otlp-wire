package otlpwire

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"google.golang.org/protobuf/encoding/protowire"
)

func semanticFixture(t testing.TB, points int) []byte {
	t.Helper()
	m := pmetric.NewMetrics()
	rm := m.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "semantic-test")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("scope")

	addCommon := func(metric pmetric.Metric) {
		metric.SetName("requests")
		metric.SetDescription("description")
		metric.SetUnit("1")
	}
	gauge := sm.Metrics().AppendEmpty()
	addCommon(gauge)
	g := gauge.SetEmptyGauge()
	for range points {
		p := g.DataPoints().AppendEmpty()
		p.SetStartTimestamp(1)
		p.SetTimestamp(2)
		p.SetDoubleValue(math.Float64frombits(0x8000000000000000))
		p.SetFlags(pmetric.DataPointFlags(3))
		p.Attributes().PutInt("code", 200)
	}
	sum := sm.Metrics().AppendEmpty()
	addCommon(sum)
	s := sum.SetEmptySum()
	s.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	s.SetIsMonotonic(true)
	pn := s.DataPoints().AppendEmpty()
	pn.SetIntValue(-4)
	hm := sm.Metrics().AppendEmpty()
	addCommon(hm)
	h := hm.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	hp := h.DataPoints().AppendEmpty()
	hp.SetCount(3)
	hp.SetSum(1.5)
	hp.SetMin(-1)
	hp.SetMax(2)
	hp.ExplicitBounds().FromRaw([]float64{1, 2})
	hp.BucketCounts().FromRaw([]uint64{1, 2, 0})
	em := sm.Metrics().AppendEmpty()
	addCommon(em)
	e := em.SetEmptyExponentialHistogram()
	e.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	ep := e.DataPoints().AppendEmpty()
	ep.SetCount(4)
	ep.SetScale(-2)
	ep.SetZeroThreshold(.5)
	ep.SetZeroCount(1)
	ep.Positive().SetOffset(-1)
	ep.Positive().BucketCounts().FromRaw([]uint64{2, 1})
	ep.Negative().SetOffset(3)
	ep.Negative().BucketCounts().FromRaw([]uint64{1})
	qm := sm.Metrics().AppendEmpty()
	addCommon(qm)
	qp := qm.SetEmptySummary().DataPoints().AppendEmpty()
	qp.SetCount(2)
	qp.SetSum(9)
	q := qp.QuantileValues().AppendEmpty()
	q.SetQuantile(.5)
	q.SetValue(4.5)
	b, err := (&pmetric.ProtoMarshaler{}).MarshalMetrics(m)
	require.NoError(t, err)
	return b
}

func TestSemanticMetricAllKindsMatchPdataFixture(t *testing.T) {
	b := semanticFixture(t, 1)
	require.NoError(t, ExportMetricsServiceRequest(b).ValidateSemantic())
	var kinds []MetricType
	for rm, err := range ExportMetricsServiceRequest(b).ResourceMetricsSeq {
		require.NoError(t, err)
		for sm, err := range rm.ScopeMetricsSeq {
			require.NoError(t, err)
			metrics, done := sm.Metrics()
			for raw := range metrics {
				m, err := raw.Semantic()
				require.NoError(t, err)
				require.Equal(t, "requests", string(m.Name))
				kinds = append(kinds, m.Kind)
				points, pointErr := m.DataPoints()
				for p := range points {
					require.Equal(t, m.Kind, p.Kind)
					if p.Kind == MetricTypeGauge {
						require.Equal(t, uint64(0x8000000000000000), p.NumberDoubleBits)
						require.Equal(t, int64(200), p.Attributes[0].Value.Int)
					}
					if p.Kind == MetricTypeHistogram {
						require.Equal(t, []uint64{1, 2, 0}, p.BucketCounts)
						require.True(t, p.Sum.Present)
					}
					if p.Kind == MetricTypeExponentialHistogram {
						require.Equal(t, int32(-2), p.Scale)
						require.Equal(t, []uint64{2, 1}, p.Positive.BucketCounts)
					}
					if p.Kind == MetricTypeSummary {
						require.Len(t, p.Quantiles, 1)
					}
				}
				require.NoError(t, pointErr())
			}
			require.NoError(t, done())
		}
	}
	require.Equal(t, []MetricType{5, 7, 9, 10, 11}, kinds)
}

func TestSemanticResolutionPackedMixedAndMalformed(t *testing.T) {
	metric := protowire.AppendTag(nil, 1, protowire.BytesType)
	metric = protowire.AppendString(metric, "first")
	metric = protowire.AppendTag(metric, 5, protowire.BytesType)
	metric = protowire.AppendBytes(metric, nil)
	metric = protowire.AppendTag(metric, 1, protowire.BytesType)
	metric = protowire.AppendString(metric, "last")
	metric = protowire.AppendTag(metric, 9, protowire.BytesType)
	dp := protowire.AppendTag(nil, 6, protowire.Fixed64Type)
	dp = protowire.AppendFixed64(dp, 1)
	packed := protowire.AppendFixed64(nil, 2)
	packed = protowire.AppendFixed64(packed, 3)
	dp = protowire.AppendTag(dp, 6, protowire.BytesType)
	dp = protowire.AppendBytes(dp, packed)
	body := protowire.AppendTag(nil, 1, protowire.BytesType)
	body = protowire.AppendBytes(body, dp)
	metric = protowire.AppendBytes(metric, body)
	m, err := Metric(metric).Semantic()
	require.NoError(t, err)
	require.Equal(t, "last", string(m.Name))
	require.Equal(t, MetricTypeHistogram, m.Kind)
	seq, done := m.DataPoints()
	for p := range seq {
		require.Equal(t, []uint64{1, 2, 3}, p.BucketCounts)
	}
	require.NoError(t, done())
	_, err = Metric(append(metric, 0x80)).Semantic()
	require.Error(t, err)
}

func TestAnyValueRecursiveAndEarlyStop(t *testing.T) {
	item := protowire.AppendTag(nil, 3, protowire.VarintType)
	item = protowire.AppendVarint(item, 7)
	array := protowire.AppendTag(nil, 1, protowire.BytesType)
	array = protowire.AppendBytes(array, item)
	value := protowire.AppendTag(nil, 5, protowire.BytesType)
	value = protowire.AppendBytes(value, array)
	kv := protowire.AppendTag(nil, 2, protowire.BytesType)
	kv = protowire.AppendBytes(kv, value)
	got, err := KeyValue(kv).Semantic()
	require.NoError(t, err)
	values, done := got.Value.Values()
	for v := range values {
		require.Equal(t, int64(7), v.Int)
		break
	}
	require.NoError(t, done())
}

func FuzzSemanticMetric(f *testing.F) {
	f.Add(semanticFixture(f, 1))
	f.Fuzz(func(t *testing.T, b []byte) { _ = ExportMetricsServiceRequest(b).ValidateSemantic() })
}

func BenchmarkSemanticMetricTraversal(b *testing.B) {
	for _, n := range []int{1, 1000} {
		data := semanticFixture(b, n)
		b.Run(string(rune('0'+min(n, 9)))+"-wire", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if err := ExportMetricsServiceRequest(data).ValidateSemantic(); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(string(rune('0'+min(n, 9)))+"-pdata", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := (&pmetric.ProtoUnmarshaler{}).UnmarshalMetrics(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
