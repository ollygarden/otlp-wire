package otlpwire

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"google.golang.org/protobuf/encoding/protowire"
)

func semanticBytesField(dst []byte, n protowire.Number, value []byte) []byte {
	dst = protowire.AppendTag(dst, n, protowire.BytesType)
	return protowire.AppendBytes(dst, value)
}

func semanticFixed64Field(dst []byte, n protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, n, protowire.Fixed64Type)
	return protowire.AppendFixed64(dst, value)
}

func semanticVarintField(dst []byte, n protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, n, protowire.VarintType)
	return protowire.AppendVarint(dst, value)
}

func semanticPoint(t *testing.T, kind MetricType, dp []byte) SemanticDataPoint {
	t.Helper()
	body := semanticBytesField(nil, 1, dp)
	metric := semanticBytesField(nil, protowire.Number(kind), body)
	m, err := Metric(metric).Semantic()
	require.NoError(t, err)
	seq, done := m.DataPoints()
	var points []SemanticDataPoint
	for point := range seq {
		points = append(points, point)
	}
	require.NoError(t, done())
	require.Len(t, points, 1)
	return points[0]
}

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
				require.Equal(t, "description", string(m.Description))
				require.Equal(t, "1", string(m.Unit))
				kinds = append(kinds, m.Kind)
				points, pointErr := m.DataPoints()
				seen := 0
				for p := range points {
					seen++
					require.Equal(t, m.Kind, p.Kind)
					if p.Kind == MetricTypeGauge {
						require.Equal(t, uint64(1), p.StartTimestamp)
						require.Equal(t, uint64(2), p.Timestamp)
						require.Equal(t, uint32(3), p.Flags)
						require.Equal(t, NumberValueDouble, p.NumberType)
						require.Equal(t, uint64(0x8000000000000000), p.NumberDoubleBits)
						require.Equal(t, "code", string(p.Attributes[0].Key))
						require.Equal(t, int64(200), p.Attributes[0].Value.Int)
					}
					if p.Kind == MetricTypeSum {
						require.Equal(t, NumberValueInt, p.NumberType)
						require.Equal(t, int64(-4), p.NumberInt)
						require.Equal(t, int32(pmetric.AggregationTemporalityCumulative), m.AggregationTemporality)
						require.True(t, m.Monotonic)
					}
					if p.Kind == MetricTypeHistogram {
						require.Equal(t, uint64(3), p.Count)
						require.Equal(t, []uint64{1, 2, 0}, p.BucketCounts)
						require.Equal(t, []uint64{math.Float64bits(1), math.Float64bits(2)}, p.ExplicitBoundsBits)
						require.Equal(t, OptionalFloat64{Present: true, Bits: math.Float64bits(1.5)}, p.Sum)
						require.Equal(t, OptionalFloat64{Present: true, Bits: math.Float64bits(-1)}, p.Min)
						require.Equal(t, OptionalFloat64{Present: true, Bits: math.Float64bits(2)}, p.Max)
						require.Equal(t, int32(pmetric.AggregationTemporalityDelta), m.AggregationTemporality)
					}
					if p.Kind == MetricTypeExponentialHistogram {
						require.Equal(t, uint64(4), p.Count)
						require.Equal(t, int32(-2), p.Scale)
						require.Equal(t, uint64(1), p.ZeroCount)
						require.Equal(t, math.Float64bits(.5), p.ZeroThresholdBits)
						require.Equal(t, int32(-1), p.Positive.Offset)
						require.Equal(t, []uint64{2, 1}, p.Positive.BucketCounts)
						require.Equal(t, int32(3), p.Negative.Offset)
						require.Equal(t, []uint64{1}, p.Negative.BucketCounts)
					}
					if p.Kind == MetricTypeSummary {
						require.Equal(t, uint64(2), p.Count)
						require.Equal(t, math.Float64bits(9), p.SummarySumBits)
						require.Equal(t, []QuantileValue{{math.Float64bits(.5), math.Float64bits(4.5)}}, p.Quantiles)
					}
				}
				require.NoError(t, pointErr())
				require.Equal(t, 1, seen)
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
	seen := 0
	for p := range seq {
		seen++
		require.Equal(t, []uint64{1, 2, 3}, p.BucketCounts)
	}
	require.NoError(t, done())
	require.Equal(t, 1, seen)
	_, err = Metric(append(metric, 0x80)).Semantic()
	require.Error(t, err)
}

func TestSemanticExponentialHistogramPinnedFieldMapping(t *testing.T) {
	dp := semanticFixed64Field(nil, 5, math.Float64bits(1.25))
	dp = semanticBytesField(dp, 11, nil) // exemplar: repeated message, not min
	dp = semanticFixed64Field(dp, 12, math.Float64bits(-2.5))
	dp = semanticFixed64Field(dp, 13, math.Float64bits(9.5))
	dp = semanticFixed64Field(dp, 14, math.Float64bits(.125))

	point := semanticPoint(t, MetricTypeExponentialHistogram, dp)
	require.Equal(t, OptionalFloat64{Present: true, Bits: math.Float64bits(1.25)}, point.Sum)
	require.Equal(t, OptionalFloat64{Present: true, Bits: math.Float64bits(-2.5)}, point.Min)
	require.Equal(t, OptionalFloat64{Present: true, Bits: math.Float64bits(9.5)}, point.Max)
	require.Equal(t, math.Float64bits(.125), point.ZeroThresholdBits)

	request := semanticBytesField(nil, 1, semanticBytesField(nil, 2, semanticBytesField(nil, 2,
		semanticBytesField(nil, 10, semanticBytesField(nil, 1, dp)))))
	oracle, err := (&pmetric.ProtoUnmarshaler{}).UnmarshalMetrics(request)
	require.NoError(t, err)
	op := oracle.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
		ExponentialHistogram().DataPoints().At(0)
	require.Equal(t, math.Float64frombits(point.Sum.Bits), op.Sum())
	require.Equal(t, math.Float64frombits(point.Min.Bits), op.Min())
	require.Equal(t, math.Float64frombits(point.Max.Bits), op.Max())
	require.Equal(t, math.Float64frombits(point.ZeroThresholdBits), op.ZeroThreshold())
	require.Equal(t, 1, op.Exemplars().Len())
}

func TestSemanticExponentialBucketsMergeOccurrences(t *testing.T) {
	first := semanticVarintField(nil, 1, protowire.EncodeZigZag(-3))
	first = semanticVarintField(first, 2, 1)                             // unpacked
	first = semanticBytesField(first, 2, protowire.AppendVarint(nil, 2)) // packed
	second := semanticBytesField(nil, 2, append(protowire.AppendVarint(nil, 3), protowire.AppendVarint(nil, 4)...))
	second = semanticVarintField(second, 2, 5) // mixed
	second = semanticVarintField(second, 1, protowire.EncodeZigZag(7))
	dp := semanticBytesField(nil, 8, first)
	dp = semanticBytesField(dp, 8, second)
	dp = semanticBytesField(dp, 9, first)
	dp = semanticBytesField(dp, 9, second)

	point := semanticPoint(t, MetricTypeExponentialHistogram, dp)
	require.Equal(t, int32(7), point.Positive.Offset)
	require.Equal(t, []uint64{1, 2, 3, 4, 5}, point.Positive.BucketCounts)
	require.Equal(t, point.Positive, point.Negative)

	request := semanticBytesField(nil, 1, semanticBytesField(nil, 2, semanticBytesField(nil, 2,
		semanticBytesField(nil, 10, semanticBytesField(nil, 1, dp)))))
	oracle, err := (&pmetric.ProtoUnmarshaler{}).UnmarshalMetrics(request)
	require.NoError(t, err)
	op := oracle.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
		ExponentialHistogram().DataPoints().At(0)
	require.Equal(t, point.Positive.Offset, op.Positive().Offset())
	require.Equal(t, point.Positive.BucketCounts, op.Positive().BucketCounts().AsRaw())
}

func alternatingAnyValue(depth int) []byte {
	value := semanticVarintField(nil, 3, 1)
	for i := range depth {
		if i%2 == 0 {
			value = semanticBytesField(nil, 5, semanticBytesField(nil, 1, value))
		} else {
			kv := semanticBytesField(nil, 2, value)
			value = semanticBytesField(nil, 6, semanticBytesField(nil, 1, kv))
		}
	}
	return value
}

func TestSemanticAnyValueAlternatingDepthLimit(t *testing.T) {
	atLimit := semanticBytesField(nil, 2, alternatingAnyValue(semanticParseMaxDepth))
	_, err := KeyValue(atLimit).Semantic()
	require.NoError(t, err)

	beyond := semanticBytesField(nil, 2, alternatingAnyValue(semanticParseMaxDepth+1))
	_, err = KeyValue(beyond).Semantic()
	require.ErrorIs(t, err, errSemanticParseDepth)

	var legacy parsedAnyValue
	require.NoError(t, parseAnyValue(alternatingAnyValue(semanticParseMaxDepth), &legacy))
	require.ErrorIs(t, parseAnyValue(alternatingAnyValue(semanticParseMaxDepth+1), &legacy), errSemanticParseDepth)
}

func TestSemanticAnyValueMergesRepeatedMessageOneof(t *testing.T) {
	first := semanticBytesField(nil, 5, semanticBytesField(nil, 1, semanticVarintField(nil, 3, 1)))
	second := semanticBytesField(nil, 5, semanticBytesField(nil, 1, semanticVarintField(nil, 3, 2)))
	kv, err := KeyValue(semanticBytesField(nil, 2, append(first, second...))).Semantic()
	require.NoError(t, err)
	values, done := kv.Value.Values()
	var got []int64
	for value := range values {
		got = append(got, value.Int)
	}
	require.NoError(t, done())
	require.Equal(t, []int64{1, 2}, got)

	reset := append(first, semanticVarintField(nil, 3, 9)...)
	reset = append(reset, second...)
	kv, err = KeyValue(semanticBytesField(nil, 2, reset)).Semantic()
	require.NoError(t, err)
	values, done = kv.Value.Values()
	got = nil
	for value := range values {
		got = append(got, value.Int)
	}
	require.NoError(t, done())
	require.Equal(t, []int64{2}, got, "intervening oneof member replaces the first array")
}

func TestValidateSemanticCoversPreMutationInputs(t *testing.T) {
	validMetric := semanticBytesField(nil, 5, nil)
	cases := map[string][]byte{
		"resource schema URL":    semanticVarintField(nil, 3, 1),
		"scope schema URL":       semanticBytesField(nil, 2, semanticVarintField(nil, 3, 1)),
		"merged resource":        semanticBytesField(semanticBytesField(nil, 1, nil), 1, semanticVarintField(nil, 1, 1)),
		"merged scope name":      semanticBytesField(nil, 2, semanticBytesField(semanticBytesField(nil, 1, nil), 1, semanticVarintField(nil, 1, 1))),
		"merged scope version":   semanticBytesField(nil, 2, semanticBytesField(semanticBytesField(nil, 1, nil), 1, semanticVarintField(nil, 2, 1))),
		"merged scope attribute": semanticBytesField(nil, 2, semanticBytesField(semanticBytesField(nil, 1, nil), 1, semanticVarintField(nil, 3, 1))),
		"superseded body": semanticBytesField(nil, 2, semanticBytesField(nil, 2,
			append(semanticBytesField(nil, 5, semanticBytesField(nil, 1, semanticVarintField(nil, 3, 1))), validMetric...))),
	}
	for name, resource := range cases {
		t.Run(name, func(t *testing.T) {
			request := semanticBytesField(nil, 1, resource)
			require.Error(t, ExportMetricsServiceRequest(request).ValidateSemantic())
		})
	}
}

func TestSemanticExplicitDefaultsUnknownFieldsAndOneofResolution(t *testing.T) {
	dp := semanticFixed64Field(nil, 4, 0)
	dp = semanticFixed64Field(dp, 6, ^uint64(6))
	dp = semanticVarintField(dp, 99, 42)
	negativeZero := math.Copysign(0, -1)
	dp = semanticFixed64Field(dp, 4, math.Float64bits(negativeZero))
	point := semanticPoint(t, MetricTypeGauge, dp)
	require.Equal(t, NumberValueDouble, point.NumberType)
	require.Equal(t, math.Float64bits(negativeZero), point.NumberDoubleBits)

	metric := semanticBytesField(nil, 5, semanticBytesField(nil, 1, append(dp, 0x80)))
	metric = semanticBytesField(metric, 7, nil)
	_, err := Metric(metric).Semantic()
	require.Error(t, err, "malformed superseded gauge must be parsed")
}

func TestSemanticIteratorEarlyStopAndDeferredCorruption(t *testing.T) {
	item := semanticVarintField(nil, 3, 1)
	array := semanticBytesField(semanticBytesField(nil, 1, item), 1, item)
	v := AnyValue{Type: AnyValueArray, array: append(array, 0x80)}
	seq, done := v.Values()
	for range seq {
		break
	}
	require.NoError(t, done(), "early stop does not inspect later bytes")

	seq, done = v.Values()
	for range seq {
	}
	require.Error(t, done(), "complete iteration reports trailing corruption")
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
