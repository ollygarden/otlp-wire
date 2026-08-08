package otlpwire

// Tests for Metric.Metadata and Metric.MetadataSeq (Metric field 12, repeated
// KeyValue). Fixtures are built with protowire because pdata's Map API cannot
// emit duplicate keys or malformed shapes; pdata is the oracle for meaning.
// The shared repeated-field machinery is covered in resource_test.go and
// scope_test.go; what is pinned here is the field number, wire order across
// duplicates, and the two variants' contracts.

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"google.golang.org/protobuf/encoding/protowire"
)

// metadataEntry wraps an encoded KeyValue as one metadata occurrence.
func metadataEntry(kv []byte) []byte {
	out := protowire.AppendTag(nil, 12, protowire.BytesType)
	out = protowire.AppendVarint(out, uint64(len(kv)))
	return append(out, kv...)
}

// metricWithMetadata builds a Metric with a name (field 1) and metadata
// occurrences in order.
func metricWithMetadata(name string, kvs ...[]byte) []byte {
	out := protowire.AppendTag(nil, 1, protowire.BytesType)
	out = protowire.AppendString(out, name)
	for _, kv := range kvs {
		out = append(out, metadataEntry(kv)...)
	}
	return out
}

// metricsPayloadWithMetric wraps a Metric as a one-resource, one-scope request
// so pdata can read the same bytes.
func metricsPayloadWithMetric(metric []byte) []byte {
	scope := protowire.AppendTag(nil, 2, protowire.BytesType)
	scope = protowire.AppendVarint(scope, uint64(len(metric)))
	scope = append(scope, metric...)
	return wrapAsRequest(resourceContainerWithScope(scope))
}

type metadataPair struct{ key, value string }

// collectMetadata drains both variants, requires they agree, and checks the
// capacity clamp on every yielded view.
func collectMetadata(t *testing.T, m Metric) []metadataPair {
	t.Helper()

	read := func(kv KeyValue) metadataPair {
		require.Equal(t, len(kv), cap(kv), "yielded KeyValue must be capacity-clamped")
		key, err := kv.Key()
		require.NoError(t, err)
		value, found, err := kv.StringValue()
		require.NoError(t, err)
		require.True(t, found, "fixture value for %q is not a string", key)
		return metadataPair{string(key), string(value)}
	}

	var viaClosure []metadataPair
	seq, errFunc := m.Metadata()
	for kv := range seq {
		viaClosure = append(viaClosure, read(kv))
	}
	require.NoError(t, errFunc())

	var viaSeq []metadataPair
	for kv, err := range m.MetadataSeq {
		require.NoError(t, err)
		viaSeq = append(viaSeq, read(kv))
	}

	require.Equal(t, viaClosure, viaSeq, "Metadata and MetadataSeq must agree")
	return viaClosure
}

func pdataMetric(t *testing.T, payload []byte) pmetric.Metric {
	t.Helper()
	req := pmetricotlp.NewExportRequest()
	require.NoError(t, req.UnmarshalProto(payload))
	sm := req.Metrics().ResourceMetrics().At(0).ScopeMetrics().At(0)
	require.Equal(t, 1, sm.Metrics().Len())
	return sm.Metrics().At(0)
}

func TestMetricMetadata_MatchesPdata(t *testing.T) {
	// Occurrences need not be contiguous or follow canonical field order.
	interleaved := metadataEntry(stringKeyValue("a", "1"))
	interleaved = protowire.AppendTag(interleaved, 1, protowire.BytesType)
	interleaved = protowire.AppendString(interleaved, "requests.total")
	interleaved = append(interleaved, metadataEntry(stringKeyValue("b", "2"))...)
	// Field 99 is unmodeled by both otlp-wire and pdata; it must be skipped.
	interleaved = protowire.AppendTag(interleaved, 99, protowire.VarintType)
	interleaved = protowire.AppendVarint(interleaved, 7)
	interleaved = append(interleaved, metadataEntry(stringKeyValue("c", "3"))...)

	tests := []struct {
		name   string
		metric []byte
		want   []metadataPair
	}{
		{"nil metric", nil, nil},
		{"empty metric", []byte{}, nil},
		{"absent", metricWithMetadata("m"), nil},
		{"single", metricWithMetadata("m", stringKeyValue("unit", "ms")), []metadataPair{{"unit", "ms"}}},
		{"empty string value is present", metricWithMetadata("m", stringKeyValue("k", "")), []metadataPair{{"k", ""}}},
		{
			"duplicate keys keep every occurrence in wire order",
			metricWithMetadata("m",
				stringKeyValue("k", "first"),
				stringKeyValue("other", "x"),
				stringKeyValue("k", "second")),
			[]metadataPair{{"k", "first"}, {"other", "x"}, {"k", "second"}},
		},
		{"out of order, non-contiguous, unknown field", interleaved,
			[]metadataPair{{"a", "1"}, {"b", "2"}, {"c", "3"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var oracle []metadataPair
			for key, value := range pdataMetric(t, metricsPayloadWithMetric(tt.metric)).Metadata().All() {
				require.Equal(t, pcommon.ValueTypeStr, value.Type())
				oracle = append(oracle, metadataPair{key, value.Str()})
			}
			require.Equal(t, tt.want, oracle, "expectation disagrees with pdata")
			require.Equal(t, tt.want, collectMetadata(t, Metric(tt.metric)))
		})
	}
}

// TestMetricMetadata_PresentButNotAString covers entries StringValue declines:
// a zero-length occurrence is a present entry, not an absent field. Both
// variants must yield it.
func TestMetricMetadata_PresentButNotAString(t *testing.T) {
	tests := []struct {
		name       string
		metric     []byte
		key        string
		hasValue   bool
		pdataValue pcommon.ValueType
	}{
		{"empty entry", metricWithMetadata("m", nil), "", false, pcommon.ValueTypeEmpty},
		{"int value", metricWithMetadata("m", intKeyValue("days", 30)), "days", true, pcommon.ValueTypeInt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := pdataMetric(t, metricsPayloadWithMetric(tt.metric)).Metadata()
			value, ok := metadata.Get(tt.key)
			require.True(t, ok)
			require.Equal(t, tt.pdataValue, value.Type(), "pdata must see the entry too")

			check := func(kv KeyValue) {
				key, err := kv.Key()
				require.NoError(t, err)
				require.Equal(t, tt.key, string(key))
				_, found, err := kv.StringValue()
				require.NoError(t, err)
				require.False(t, found)
				raw, err := kv.ValueRaw()
				require.NoError(t, err)
				require.Equal(t, tt.hasValue, raw != nil)
			}

			m := Metric(tt.metric)
			seqCount := 0
			for kv, err := range m.MetadataSeq {
				require.NoError(t, err)
				seqCount++
				check(kv)
			}
			closureCount := 0
			seq, errFunc := m.Metadata()
			for kv := range seq {
				closureCount++
				check(kv)
			}
			require.NoError(t, errFunc())
			require.Equal(t, 1, seqCount, "MetadataSeq must yield the present entry")
			require.Equal(t, 1, closureCount, "Metadata must yield the present entry")
		})
	}
}

// TestMetricMetadata_AlongsideDataPoints is the metric-specific case:
// DataPointsSeq hand-rolls its own walk instead of using the shared helper, so
// the two must not disturb each other.
func TestMetricMetadata_AlongsideDataPoints(t *testing.T) {
	gauge := protowire.AppendTag(nil, 1, protowire.BytesType)
	gauge = protowire.AppendVarint(gauge, 0)

	metric := protowire.AppendTag(nil, 5, protowire.BytesType) // gauge body
	metric = protowire.AppendVarint(metric, uint64(len(gauge)))
	metric = append(metric, gauge...)
	metric = append(metric, metadataEntry(stringKeyValue("k", "v"))...)

	m := Metric(metric)
	want := []metadataPair{{"k", "v"}}
	require.Equal(t, want, collectMetadata(t, m))

	dataPoints := 0
	for dp, err := range m.DataPointsSeq {
		require.NoError(t, err)
		require.Equal(t, MetricTypeGauge, dp.Type())
		dataPoints++
	}
	require.Equal(t, 1, dataPoints)
	require.Equal(t, want, collectMetadata(t, m))
}

// TestMetricMetadata_MalformedWire pins each error branch by identity, so the
// table cannot silently exercise one branch four times.
func TestMetricMetadata_MalformedWire(t *testing.T) {
	validKV := stringKeyValue("k", "v")

	tests := []struct {
		name    string
		metric  []byte
		wantErr string
	}{
		{"length past the end", func() []byte {
			out := protowire.AppendTag(nil, 12, protowire.BytesType)
			out = protowire.AppendVarint(out, uint64(len(validKV)+8))
			return append(out, validKV...)
		}(), "invalid bytes in repeated field"},
		{"varint instead of a message", func() []byte {
			out := protowire.AppendTag(nil, 12, protowire.VarintType)
			return protowire.AppendVarint(out, 42)
		}(), "wrong wire type for field"},
		{"truncated tag after a valid entry",
			append(metadataEntry(validKV), 0xFF), "malformed protobuf tag"},
		{"truncated unknown field after the last entry", func() []byte {
			out := protowire.AppendTag(metadataEntry(validKV), 98, protowire.BytesType)
			return protowire.AppendVarint(out, 64)
		}(), "failed to skip field"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Metric(tt.metric)

			seq, errFunc := m.Metadata()
			for range seq { //nolint:revive // draining is the point
			}
			require.ErrorContains(t, errFunc(), tt.wantErr)

			var seqErr error
			for kv, err := range m.MetadataSeq {
				if err != nil {
					require.Nil(t, kv, "an error must come with a nil KeyValue")
					seqErr = err
					break
				}
			}
			require.ErrorContains(t, seqErr, tt.wantErr)
		})
	}
}

// TestMetricMetadata_MalformedKeyValueSurfacesOnAccess separates framing from
// contents: a well-framed entry whose contents are corrupt fails on read.
func TestMetricMetadata_MalformedKeyValueSurfacesOnAccess(t *testing.T) {
	seq, errFunc := Metric(metadataEntry([]byte{0xFF})).Metadata()
	count := 0
	for kv := range seq {
		count++
		_, err := kv.Key()
		require.Error(t, err)
	}
	require.NoError(t, errFunc(), "framing is well formed; only the contents are not")
	require.Equal(t, 1, count)
}

// TestMetricMetadata_EarlyStop covers both halves of the lazy-iteration
// contract: an early exit still requires the error closure to be checked, and
// it leaves later bytes unvisited, so trailing corruption surfaces only on a
// full drain.
func TestMetricMetadata_EarlyStop(t *testing.T) {
	metric := metricWithMetadata("m",
		stringKeyValue("a", "1"), stringKeyValue("b", "2"), stringKeyValue("c", "3"))
	m := Metric(append(metric, 0xFF))

	seq, errFunc := m.Metadata()
	stopped := 0
	for range seq {
		stopped++
		break
	}
	require.Equal(t, 1, stopped)
	require.NoError(t, errFunc(), "unreached corruption is not reported")

	seq, errFunc = m.Metadata()
	drained := 0
	for range seq {
		drained++
	}
	require.Equal(t, 3, drained)
	require.Error(t, errFunc(), "draining reaches the corruption")
}

// TestMetricMetadata_ViewAndAllocationContract pins what the accessors promise
// beyond their values: yielded KeyValues alias the caller's buffer rather than
// copying it, the closure form may be ranged more than once, and neither form
// allocates per element.
func TestMetricMetadata_ViewAndAllocationContract(t *testing.T) {
	m := Metric(metricWithMetadata("m",
		stringKeyValue("a", "1"), stringKeyValue("b", "2")))

	seq, errFunc := m.Metadata()

	// A single-shot sequence would silently yield nothing on a second pass.
	var passes [2]int
	for i := range passes {
		for range seq {
			passes[i]++
		}
		require.NoError(t, errFunc())
	}
	require.Equal(t, [2]int{2, 2}, passes, "ranging the same seq twice must agree")

	// Views must alias the metric buffer, not copy it: writing through the
	// buffer is visible in an already-yielded view.
	var first KeyValue
	seq, errFunc = m.Metadata()
	for kv := range seq {
		first = kv
		break
	}
	require.NoError(t, errFunc())
	offset := bytes.Index([]byte(m), []byte(first))
	require.GreaterOrEqual(t, offset, 0, "view must live inside the metric buffer")
	m[offset] ^= 0xFF
	require.Equal(t, m[offset], first[0], "view must alias, not copy")
	m[offset] ^= 0xFF

	require.Zero(t, testing.AllocsPerRun(100, func() {
		for kv, err := range m.MetadataSeq {
			if err != nil {
				t.Fatal(err)
			}
			_, _ = kv.Key()
		}
	}), "MetadataSeq must not allocate")

	require.LessOrEqual(t, testing.AllocsPerRun(100, func() {
		s, e := m.Metadata()
		for range s { //nolint:revive // draining is the point
		}
		if err := e(); err != nil {
			t.Fatal(err)
		}
	}), 3.0, "Metadata must not allocate per element")
}
