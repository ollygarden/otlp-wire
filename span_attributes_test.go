package otlpwire

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// pdataSpansWire marshals one span per fill callback through pdata and returns
// the wire Spans in order, plus pdata's own decode of the same bytes as the
// oracle.
func pdataSpansWire(t *testing.T, fills ...func(ptrace.Span)) ([]Span, ptrace.SpanSlice) {
	t.Helper()

	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for _, fill := range fills {
		span := spans.AppendEmpty()
		span.SetName("op")
		fill(span)
	}
	data, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)

	var out []Span
	for rs, err := range ExportTracesServiceRequest(data).ResourceSpansSeq {
		require.NoError(t, err)
		for ss, err := range rs.ScopeSpansSeq {
			require.NoError(t, err)
			seq, errFn := ss.Spans()
			out = slices.AppendSeq(out, seq)
			require.NoError(t, errFn())
		}
	}
	require.Len(t, out, len(fills))

	decoded, err := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(data)
	require.NoError(t, err)
	return out, decoded.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
}

// requireAttributeMatches checks one wire attribute against pdata's decode of
// the same attribute, recursing into key-value lists.
func requireAttributeMatches(t *testing.T, kv KeyValue, key string, want pcommon.Value) {
	t.Helper()

	gotKey, err := kv.Key()
	require.NoError(t, err)
	require.Equal(t, key, string(gotKey))

	got, err := kv.Value()
	require.NoError(t, err)
	requireValueMatches(t, got, want)
}

func requireValueMatches(t *testing.T, got AnyValue, want pcommon.Value) {
	t.Helper()

	switch want.Type() {
	case pcommon.ValueTypeStr:
		require.Equal(t, AnyValueString, got.Kind)
		require.Equal(t, want.Str(), string(got.Str))
	case pcommon.ValueTypeBool:
		require.Equal(t, AnyValueBool, got.Kind)
		require.Equal(t, want.Bool(), got.Bool)
	case pcommon.ValueTypeInt:
		require.Equal(t, AnyValueInt, got.Kind)
		require.Equal(t, want.Int(), got.Int)
	case pcommon.ValueTypeDouble:
		require.Equal(t, AnyValueDouble, got.Kind)
		require.Equal(t, want.Double(), got.Double)
	case pcommon.ValueTypeBytes:
		require.Equal(t, AnyValueBytes, got.Kind)
		require.Equal(t, want.Bytes().AsRaw(), got.Raw)
	case pcommon.ValueTypeSlice:
		require.Equal(t, AnyValueArray, got.Kind)
		i := 0
		forEachRepeatedField(got.Raw, 1, func(raw []byte, err error) bool {
			require.NoError(t, err)
			require.Less(t, i, want.Slice().Len())
			element, err := ParseAnyValue(raw)
			require.NoError(t, err)
			requireValueMatches(t, element, want.Slice().At(i))
			i++
			return true
		})
		require.Equal(t, want.Slice().Len(), i)
	case pcommon.ValueTypeMap:
		require.Equal(t, AnyValueKeyValueList, got.Kind)
		i := 0
		keys := make([]string, 0, want.Map().Len())
		want.Map().Range(func(k string, _ pcommon.Value) bool {
			keys = append(keys, k)
			return true
		})
		for nested, err := range got.KeyValuesSeq {
			require.NoError(t, err)
			require.Less(t, i, len(keys))
			nestedWant, ok := want.Map().Get(keys[i])
			require.True(t, ok)
			requireAttributeMatches(t, nested, keys[i], nestedWant)
			i++
		}
		require.Len(t, keys, i)
	default:
		require.Equal(t, AnyValueEmpty, got.Kind)
	}
}

// TestSpanAttributesSeq_PdataParity reads the same marshalled spans through
// the accessor and through pdata and requires identical attributes in
// identical order.
func TestSpanAttributesSeq_PdataParity(t *testing.T) {
	tests := []struct {
		name string
		fill func(ptrace.Span)
	}{
		{name: "zero attributes", fill: func(ptrace.Span) {}},
		{name: "one attribute", fill: func(s ptrace.Span) {
			s.Attributes().PutStr("http.method", "GET")
		}},
		{name: "many attributes", fill: func(s ptrace.Span) {
			for i := range 50 {
				s.Attributes().PutInt("attr."+string(rune('a'+i%26))+string(rune('a'+i/26)), int64(i))
			}
		}},
		{name: "every scalar kind", fill: func(s ptrace.Span) {
			s.Attributes().PutStr("s", "v")
			s.Attributes().PutStr("empty", "")
			s.Attributes().PutBool("b", true)
			s.Attributes().PutInt("i", -42)
			s.Attributes().PutDouble("d", 1.5)
			s.Attributes().PutEmptyBytes("raw").FromRaw([]byte{0, 1, 2})
			s.Attributes().PutEmpty("unset")
		}},
		{name: "nested values", fill: func(s ptrace.Span) {
			m := s.Attributes().PutEmptyMap("map")
			m.PutStr("inner", "x")
			m.PutEmptyMap("deeper").PutInt("n", 7)
			sl := s.Attributes().PutEmptySlice("slice")
			sl.AppendEmpty().SetStr("a")
			sl.AppendEmpty().SetInt(2)
		}},
		{name: "large value", fill: func(s ptrace.Span) {
			s.Attributes().PutStr("blob", string(make([]byte, 70_000)))
			s.Attributes().PutStr("after", "still reachable")
		}},
		{name: "attributes beside other span fields", fill: func(s ptrace.Span) {
			s.SetTraceID(pcommon.TraceID(traceIDA))
			s.SetSpanID(pcommon.SpanID(spanIDA))
			s.SetKind(ptrace.SpanKindServer)
			s.Attributes().PutStr("k1", "v1")
			s.Events().AppendEmpty().Attributes().PutStr("event.attr", "not a span attribute")
			s.Links().AppendEmpty().Attributes().PutStr("link.attr", "not a span attribute")
			s.Status().SetMessage("boom")
			s.SetDroppedAttributesCount(3)
			s.Attributes().PutStr("k2", "v2")
		}},
	}

	fills := make([]func(ptrace.Span), len(tests))
	for i, tt := range tests {
		fills[i] = tt.fill
	}
	wireSpans, spans := pdataSpansWire(t, fills...)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := spans.At(i).Attributes()
			keys := make([]string, 0, want.Len())
			want.Range(func(k string, _ pcommon.Value) bool {
				keys = append(keys, k)
				return true
			})

			n := 0
			for kv, err := range wireSpans[i].AttributesSeq {
				require.NoError(t, err)
				require.Less(t, n, len(keys))
				wantValue, ok := want.Get(keys[n])
				require.True(t, ok)
				requireAttributeMatches(t, kv, keys[n], wantValue)
				n++
			}
			require.Equal(t, want.Len(), n)
		})
	}
}

func TestSpanAttributesSeq_Wire(t *testing.T) {
	kv1 := stringKeyValue("a", "1")
	kv2 := stringKeyValue("b", "2")

	tests := []struct {
		name     string
		span     []byte
		wantKeys []string
	}{
		{name: "empty span", span: nil},
		{name: "unknown fields interleaved", span: slices.Concat(
			bytesField(9, kv1),
			varintField(99, 7),
			bytesField(100, []byte("opaque")),
			nameField("op"),
			bytesField(9, kv2),
		), wantKeys: []string{"a", "b"}},
		{name: "duplicate keys all yielded", span: slices.Concat(
			bytesField(9, kv1), bytesField(9, stringKeyValue("a", "other")),
		), wantKeys: []string{"a", "a"}},
		{name: "events and links are not span attributes", span: slices.Concat(
			bytesField(11, bytesField(3, kv1)),
			bytesField(13, bytesField(4, kv1)),
			bytesField(9, kv2),
		), wantKeys: []string{"b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for kv, err := range Span(tt.span).AttributesSeq {
				require.NoError(t, err)
				key, err := kv.Key()
				require.NoError(t, err)
				got = append(got, string(key))
			}
			require.Equal(t, tt.wantKeys, got)

			// The oracle accepts the same bytes.
			_, err := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(exportTracesWithSpan(tt.span))
			require.NoError(t, err)
		})
	}
}

func TestSpanAttributesSeq_Malformed(t *testing.T) {
	kv := stringKeyValue("a", "1")

	tests := []struct {
		name string
		span []byte
		// wantYielded is how many attributes arrive before the error.
		wantYielded int
	}{
		{name: "truncated tag", span: []byte{0x80}},
		{name: "attribute truncated length", span: []byte{0x4a, 0x08, 0x01}},
		{name: "attribute wrong wire type", span: []byte{0x48, 0x01}},
		{name: "unknown field truncated length", span: []byte{0xd2, 0x05, 0x08, 0x01}},
		{name: "unknown field truncated varint", span: []byte{0xd0, 0x05, 0x80}},
		{name: "corruption after a valid attribute", span: slices.Concat(bytesField(9, kv), []byte{0x4a, 0x08, 0x01}), wantYielded: 1},
		{name: "corruption before a valid attribute", span: slices.Concat([]byte{0xd2, 0x05, 0x7f}, bytesField(9, kv))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yielded, errs := 0, 0
			Span(tt.span).AttributesSeq(func(kv KeyValue, err error) bool {
				if err != nil {
					require.Nil(t, kv)
					errs++
					return true // keep asking: the iterator must stop on its own
				}
				yielded++
				return true
			})
			require.Equal(t, tt.wantYielded, yielded)
			require.Equal(t, 1, errs, "exactly one error, then the iterator stops")

			_, err := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(exportTracesWithSpan(tt.span))
			require.Error(t, err, "pdata must reject the same bytes")
		})
	}
}

// TestSpanAttributesSeq_FramingOnlyDivergence pins the narrower validity
// claim: the walk checks the span's framing, not its other fields or nested
// bodies. A wrongly sized trace id that Name rejects, and a corrupt event body,
// do not stop attributes. A corrupt KeyValue body is still handed out as a
// view and surfaces when the caller reads it.
func TestSpanAttributesSeq_FramingOnlyDivergence(t *testing.T) {
	t.Run("wrong sized trace id", func(t *testing.T) {
		span := Span(slices.Concat(idField(1, []byte{1, 2, 3}), bytesField(9, stringKeyValue("a", "1"))))

		_, nameErr := span.Name()
		require.Error(t, nameErr)

		n := 0
		for _, err := range span.AttributesSeq {
			require.NoError(t, err)
			n++
		}
		require.Equal(t, 1, n)
	})

	t.Run("corrupt event body", func(t *testing.T) {
		span := Span(slices.Concat(
			bytesField(11, []byte{0x80}), // framed event whose inner tag is truncated
			bytesField(9, stringKeyValue("a", "1")),
		))

		n := 0
		for kv, err := range span.AttributesSeq {
			require.NoError(t, err)
			_, _, err = kv.Fields()
			require.NoError(t, err)
			n++
		}
		require.Equal(t, 1, n)
	})

	t.Run("corrupt key value body", func(t *testing.T) {
		span := Span(bytesField(9, []byte{0x80})) // framed attribute whose inner tag is truncated

		n := 0
		for kv, err := range span.AttributesSeq {
			require.NoError(t, err)
			_, _, err = kv.Fields()
			require.Error(t, err)
			n++
		}
		require.Equal(t, 1, n)
	})
}

func TestSpanAttributesSeq_EarlyStopLeavesLaterCorruptionUnvisited(t *testing.T) {
	span := Span(slices.Concat(bytesField(9, stringKeyValue("a", "1")), []byte{0x4a, 0x08, 0x01}))

	n := 0
	span.AttributesSeq(func(_ KeyValue, err error) bool {
		require.NoError(t, err)
		n++
		return false
	})
	require.Equal(t, 1, n)
}

func TestSpanAttributesSeq_ViewsAreCapacityClamped(t *testing.T) {
	span := Span(slices.Concat(
		bytesField(9, stringKeyValue("a", "1")),
		bytesField(9, stringKeyValue("b", "2")),
	))

	n := 0
	for kv, err := range span.AttributesSeq {
		require.NoError(t, err)
		require.Equal(t, len(kv), cap(kv))
		n++
	}
	require.Equal(t, 2, n)
}

func TestSpanAttributesSeq_ZeroAllocations(t *testing.T) {
	span := Span(slices.Concat(
		idField(1, traceIDA),
		bytesField(9, stringKeyValue("http.method", "GET")),
		bytesField(9, stringKeyValue("http.route", "/orders")),
		nameField("GET /orders"),
		bytesField(9, intKeyValue("http.status_code", 200)),
	))

	allocs := testing.AllocsPerRun(1000, func() {
		count := 0
		for kv, err := range span.AttributesSeq {
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := kv.Fields(); err != nil {
				t.Fatal(err)
			}
			count++
		}
		if count != 3 {
			t.Fatalf("expected 3 attributes, got %d", count)
		}
	})
	require.Zero(t, allocs)
}
