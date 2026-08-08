package otlpwire

// Fixtures use protowire because pdata's Span API cannot emit repeated
// occurrences of a singular field, zero-value encodings, or malformed shapes;
// pdata stays the oracle for meaning. The shared repeated-field machinery is
// covered in resource_test.go and scope_test.go.

import (
	"bytes"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/protobuf/encoding/protowire"
)

// exportTracesWithSpan wraps a raw Span as the single span of a single
// ScopeSpans in a single ResourceSpans.
func exportTracesWithSpan(span []byte) []byte {
	return wrapAsRequest(resourceContainerWithScope(bytesField(2, span)))
}

func pdataSpanFromWire(t *testing.T, span []byte) ptrace.Span {
	t.Helper()
	req := ptraceotlp.NewExportRequest()
	require.NoError(t, req.UnmarshalProto(exportTracesWithSpan(span)))
	scopeSpans := req.Traces().ResourceSpans().At(0).ScopeSpans().At(0)
	require.Equal(t, 1, scopeSpans.Spans().Len())
	return scopeSpans.Spans().At(0)
}

// spanScalarsOf reads the four accessors backed by parseSpanFields, asserting
// they agree about validity. The shared walk makes disagreement impossible by
// construction; this is the test that keeps it that way.
//
// The identifier accessors are deliberately excluded: they scan first-match
// and are not expected to agree with these on malformed input. See
// TestSpanIdentifiers_FirstMatchDivergence.
func spanScalarsOf(t *testing.T, s Span) (spanFields, error) {
	t.Helper()

	name, nameErr := s.Name()
	kind, kindErr := s.Kind()
	start, startErr := s.StartTimeUnixNano()
	end, endErr := s.EndTimeUnixNano()

	for _, err := range []error{kindErr, startErr, endErr} {
		require.Equal(t, nameErr == nil, err == nil,
			"the four shared-walk accessors must agree about whether these bytes are valid")
	}

	return spanFields{
		name:          name,
		kind:          kind,
		startUnixNano: start,
		endUnixNano:   end,
	}, nameErr
}

type spanIdentifiers struct {
	traceID      [16]byte
	spanID       [8]byte
	parentSpanID [8]byte
}

func spanIdentifiersOf(t *testing.T, s Span) spanIdentifiers {
	t.Helper()

	traceID, err := s.TraceID()
	require.NoError(t, err)
	spanID, err := s.SpanID()
	require.NoError(t, err)
	parentSpanID, err := s.ParentSpanID()
	require.NoError(t, err)

	return spanIdentifiers{traceID: traceID, spanID: spanID, parentSpanID: parentSpanID}
}

// Field builders. protowire is used directly because pdata's Span API cannot
// emit repeated occurrences of a singular field, zero-value encodings, or
// malformed shapes.

func bytesField(fieldNum protowire.Number, body []byte) []byte {
	out := protowire.AppendTag(nil, fieldNum, protowire.BytesType)
	return protowire.AppendBytes(out, body)
}

func varintField(fieldNum protowire.Number, value uint64) []byte {
	out := protowire.AppendTag(nil, fieldNum, protowire.VarintType)
	return protowire.AppendVarint(out, value)
}

func idField(fieldNum protowire.Number, id []byte) []byte { return bytesField(fieldNum, id) }

func nameField(name string) []byte { return bytesField(5, []byte(name)) }

func traceStateField(state string) []byte { return bytesField(3, []byte(state)) }

func kindField(kind uint64) []byte { return varintField(6, kind) }

func droppedCountField(fieldNum protowire.Number, count uint64) []byte {
	return varintField(fieldNum, count)
}

func timestampField(fieldNum protowire.Number, value uint64) []byte {
	out := protowire.AppendTag(nil, fieldNum, protowire.Fixed64Type)
	return protowire.AppendFixed64(out, value)
}

func flagsField(flags uint32) []byte {
	out := protowire.AppendTag(nil, 16, protowire.Fixed32Type)
	return protowire.AppendFixed32(out, flags)
}

// spanAttribute builds a Span attributes entry (field 9) holding one
// string-valued KeyValue.
func spanAttribute(key, value string) []byte {
	return bytesField(9, stringKeyValue(key, value))
}

// spanEvent builds a Span events entry (field 11) with a name and timestamp.
func spanEvent(name string, timeUnixNano uint64) []byte {
	event := protowire.AppendTag(nil, 1, protowire.Fixed64Type)
	event = protowire.AppendFixed64(event, timeUnixNano)
	return bytesField(11, append(event, bytesField(2, []byte(name))...))
}

// spanLink builds a Span links entry (field 13) referencing another span.
func spanLink(traceID, spanID []byte) []byte {
	return bytesField(13, slices.Concat(bytesField(1, traceID), bytesField(2, spanID)))
}

// spanStatus builds a Span status field (field 15).
func spanStatus(message string, code uint64) []byte {
	return bytesField(15, slices.Concat(bytesField(2, []byte(message)), varintField(3, code)))
}

var (
	traceIDA = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	traceIDB = []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	spanIDA  = []byte{1, 2, 3, 4, 5, 6, 7, 8}
	spanIDB  = []byte{8, 7, 6, 5, 4, 3, 2, 1}
)

// TestSpanFields_PdataParity is the differential gate: for every fixture, all
// seven accessors must report exactly what pdata reports after a full
// unmarshal of the same bytes.
func TestSpanFields_PdataParity(t *testing.T) {
	tests := []struct {
		name string
		span []byte
	}{
		{
			name: "empty span, every field absent",
			span: nil,
		},
		{
			name: "fully populated",
			span: slices.Concat(
				idField(1, traceIDA),
				idField(2, spanIDA),
				idField(4, spanIDB),
				nameField("GET /orders"),
				kindField(uint64(ptrace.SpanKindServer)),
				timestampField(7, 1_700_000_000_000_000_000),
				timestampField(8, 1_700_000_000_500_000_000),
			),
		},
		{
			name: "zero-value encodings present on the wire",
			span: slices.Concat(
				idField(1, nil),
				idField(2, nil),
				idField(4, nil),
				nameField(""),
				kindField(0),
				timestampField(7, 0),
				timestampField(8, 0),
			),
		},
		{
			name: "fields in descending order",
			span: slices.Concat(
				timestampField(8, 99),
				timestampField(7, 42),
				kindField(uint64(ptrace.SpanKindClient)),
				nameField("reversed"),
				idField(4, spanIDB),
				idField(2, spanIDA),
				idField(1, traceIDA),
			),
		},
		{
			name: "repeated scalars resolve last-value-wins",
			span: slices.Concat(
				nameField("first"),
				kindField(uint64(ptrace.SpanKindInternal)),
				timestampField(7, 1),
				timestampField(8, 2),
				nameField("last"),
				kindField(uint64(ptrace.SpanKindProducer)),
				timestampField(7, 100),
				timestampField(8, 200),
			),
		},
		{
			name: "kind above the defined range",
			span: kindField(9999),
		},
		{
			// int32 truncation of a large varint is negative; representing
			// kind as int32 keeps that distinguishable from the OTLP range.
			name: "kind truncating to a negative int32",
			span: kindField(0xFFFFFFFFFFFFFFFF),
		},
		{
			name: "maximum timestamps",
			span: slices.Concat(
				timestampField(7, 0xFFFFFFFFFFFFFFFF),
				timestampField(8, 0xFFFFFFFFFFFFFFFF),
			),
		},
		{
			name: "invalid UTF-8 name is accepted, matching pdata",
			span: slices.Concat(
				nameField("\xff\xfe invalid"),
				idField(1, traceIDA),
			),
		},
		{
			name: "unknown scalar fields are skipped",
			span: slices.Concat(
				nameField("with-unknowns"),
				unknownScalarFields(),
				kindField(uint64(ptrace.SpanKindConsumer)),
			),
		},
		{
			// trace_state after name: field 3 and field 5 share a case arm, so
			// a walk that dropped the `num == 5` guard would report the
			// trace_state here and stay green on every other fixture.
			name: "trace_state after name",
			span: slices.Concat(
				nameField("GET /orders"),
				traceStateField("vendor=value"),
			),
		},
		{
			// A field after flags (16). pdata marshals descending, so flags
			// precedes the three identifiers on real payloads; a walk that
			// stopped at flags would still pass every ascending fixture.
			name: "fields after flags",
			span: slices.Concat(
				flagsField(1),
				idField(1, traceIDA),
				nameField("after-flags"),
			),
		},
		{
			name: "known but unexposed fields are walked, not read",
			span: slices.Concat(
				idField(1, traceIDA),
				traceStateField("vendor=value"),
				nameField("full-span"),
				kindField(uint64(ptrace.SpanKindServer)),
				timestampField(7, 5),
				timestampField(8, 6),
				spanAttribute("http.method", "GET"),
				droppedCountField(10, 3),
				spanEvent("exception", 7),
				droppedCountField(12, 4),
				spanLink(traceIDB, spanIDB),
				droppedCountField(14, 5),
				spanStatus("boom", 2),
				flagsField(1),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spanScalarsOf(t, Span(tt.span))
			require.NoError(t, err)
			ids := spanIdentifiersOf(t, Span(tt.span))

			want := pdataSpanFromWire(t, tt.span)
			require.Equal(t, [16]byte(want.TraceID()), ids.traceID)
			require.Equal(t, [8]byte(want.SpanID()), ids.spanID)
			require.Equal(t, [8]byte(want.ParentSpanID()), ids.parentSpanID)
			require.Equal(t, want.Name(), string(got.name))
			require.Equal(t, int32(want.Kind()), got.kind)
			require.Equal(t, uint64(want.StartTimestamp()), got.startUnixNano)
			require.Equal(t, uint64(want.EndTimestamp()), got.endUnixNano)
		})
	}
}

// TestSpanName_AbsentVersusEmpty pins the one distinction pdata cannot make.
// pdata reports "" for both; the wire path reports nil for absent and a
// non-nil zero-length slice for present-but-empty.
func TestSpanName_AbsentVersusEmpty(t *testing.T) {
	absent, err := Span(kindField(1)).Name()
	require.NoError(t, err)
	require.Nil(t, absent)

	empty, err := Span(nameField("")).Name()
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)
}

// TestSpanIdentifiers_FirstMatchDivergence pins the deliberate difference
// between the identifier accessors and the shared walk. The identifiers scan
// first-match and stop, which is cheaper and indistinguishable from pdata on
// conformant OTLP — every producer emits each singular field once. On
// malformed input the two disagree, and that is the accepted cost of not
// walking every span to its end. operations.md records it.
func TestSpanIdentifiers_FirstMatchDivergence(t *testing.T) {
	t.Run("repeated identifier reports the first, pdata the last", func(t *testing.T) {
		span := slices.Concat(idField(1, traceIDA), idField(1, traceIDB))

		got, err := Span(span).TraceID()
		require.NoError(t, err)
		require.Equal(t, [16]byte(traceIDA), got, "wire path reports the first occurrence")

		want := pdataSpanFromWire(t, span)
		require.Equal(t, [16]byte(traceIDB), [16]byte(want.TraceID()), "pdata reports the last")
	})

	t.Run("trailing empty identifier does not clear the earlier value", func(t *testing.T) {
		span := slices.Concat(idField(1, traceIDA), idField(1, nil))

		got, err := Span(span).TraceID()
		require.NoError(t, err)
		require.Equal(t, [16]byte(traceIDA), got)

		want := pdataSpanFromWire(t, span)
		require.Equal(t, [16]byte{}, [16]byte(want.TraceID()), "pdata's empty occurrence resets the ID")
	})

	t.Run("corruption after the identifier is reported by the walk only", func(t *testing.T) {
		// attributes (field 9), length 8, only one byte present.
		span := slices.Concat(idField(1, traceIDA), nameField("ok"), []byte{0x4a, 0x08, 0x01})

		got, err := Span(span).TraceID()
		require.NoError(t, err, "first-match stops at field 1 and never sees the corruption")
		require.Equal(t, [16]byte(traceIDA), got)

		_, err = Span(span).Name()
		require.Error(t, err, "the shared walk runs to the end and reports it")

		_, pdataErr := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(exportTracesWithSpan(span))
		require.Error(t, pdataErr, "pdata agrees with the walk, not with the identifier accessor")
	})
}

// TestSpanFields_MalformedParity keeps the seven accessors from drifting
// apart: every malformed span fails all of them, and pdata rejects the same
// bytes. It is the guard for the accessor-divergence risk in operations.md, so
// it covers the whole known Span schema rather than only the new fields.
func TestSpanFields_MalformedParity(t *testing.T) {
	tests := []struct {
		name string
		span []byte
	}{
		// The identifier fields, whose wire-type check cannot be exercised
		// by an aligned payload: any bytes ConsumeBytes would accept here
		// also desynchronize the walk. Every other field's check is proved
		// on its own terms by TestSpanFields_WireTypeChecksAreLoadBearing.
		{name: "trace id wrong wire type", span: []byte{0x08, 0x01}},
		{name: "span id wrong wire type", span: []byte{0x10, 0x01}},
		{name: "parent span id wrong wire type", span: []byte{0x20, 0x01}},

		{name: "trace id truncated length", span: []byte{0x0a, 0x10, 0x01, 0x02}},
		{name: "name truncated length", span: []byte{0x2a, 0x05, 'a'}},
		{name: "start time truncated", span: []byte{0x39, 0x01, 0x02}},
		{name: "attributes truncated length", span: []byte{0x4a, 0x08, 0x01}},
		{name: "truncated tag", span: []byte{0x80}},
		// Corruption in a field the walk does not read must still be an
		// error: the walk skips unknown fields, it does not ignore them.
		{name: "unknown field truncated length", span: []byte{0xd2, 0x05, 0x08, 0x01}},
		{name: "unknown field truncated varint", span: []byte{0xd0, 0x05, 0x80}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := spanScalarsOf(t, Span(tt.span))
			require.Error(t, err)

			_, pdataErr := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(exportTracesWithSpan(tt.span))
			require.Error(t, pdataErr, "pdata must reject the same bytes")
		})
	}
}

// TestSpanFields_WireTypeChecksAreLoadBearing proves each wire-type check
// rejects on its own rather than being backstopped by framing. Every fixture
// here encodes a known field with the wrong wire type but a payload the
// field's *expected* reader would consume whole, so the walk stays aligned and
// the trailing name parses. Drop the check and the span parses successfully
// with a wrong value; the plain wrong-wire-type fixtures above would not
// notice, because they happen to desynchronize the walk.
func TestSpanFields_WireTypeChecksAreLoadBearing(t *testing.T) {
	// An 8-byte payload that reads as a length-delimited 7-byte string.
	asBytesPayload := append([]byte{0x07}, []byte("payload")...)
	// An 8-byte payload that reads as a single 8-byte varint.
	asVarintPayload := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}

	fixed64Carrying := func(fieldNum protowire.Number, payload []byte) []byte {
		out := protowire.AppendTag(nil, fieldNum, protowire.Fixed64Type)
		return append(out, payload...)
	}
	// A length-delimited field whose tag, length byte and body together span
	// exactly the eight bytes a fixed64 reader would consume.
	bytesCarryingEightBytes := func(fieldNum protowire.Number) []byte {
		out := protowire.AppendTag(nil, fieldNum, protowire.BytesType)
		return protowire.AppendBytes(out, []byte("7 bytes"))
	}

	tests := []struct {
		name  string
		field []byte
	}{
		{name: "trace_state as fixed64", field: fixed64Carrying(3, asBytesPayload)},
		{name: "name as fixed64", field: fixed64Carrying(5, asBytesPayload)},
		{name: "attributes as fixed64", field: fixed64Carrying(9, asBytesPayload)},
		{name: "events as fixed64", field: fixed64Carrying(11, asBytesPayload)},
		{name: "links as fixed64", field: fixed64Carrying(13, asBytesPayload)},
		{name: "status as fixed64", field: fixed64Carrying(15, asBytesPayload)},

		{name: "kind as fixed64", field: fixed64Carrying(6, asVarintPayload)},
		{name: "dropped_attributes_count as fixed64", field: fixed64Carrying(10, asVarintPayload)},
		{name: "dropped_events_count as fixed64", field: fixed64Carrying(12, asVarintPayload)},
		{name: "dropped_links_count as fixed64", field: fixed64Carrying(14, asVarintPayload)},

		{name: "start_time_unix_nano as bytes", field: bytesCarryingEightBytes(7)},
		{name: "end_time_unix_nano as bytes", field: bytesCarryingEightBytes(8)},

		{name: "flags as varint", field: slices.Concat(
			protowire.AppendTag(nil, 16, protowire.VarintType),
			[]byte{0x80, 0x80, 0x80, 0x01},
		)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := slices.Concat(tt.field, nameField("trailing"))

			_, err := spanScalarsOf(t, Span(span))
			require.Error(t, err, "the wire-type check, not framing, must reject this")
			require.ErrorContains(t, err, "wrong wire type")

			_, pdataErr := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(exportTracesWithSpan(span))
			require.Error(t, pdataErr, "pdata must reject the same bytes")
		})
	}
}

// TestSpanIdentifier_UnexpectedSize covers the one malformed shape the
// identifier accessors reject on their own terms rather than through framing.
func TestSpanIdentifier_UnexpectedSize(t *testing.T) {
	tests := []struct {
		name string
		span []byte
	}{
		{name: "trace id too short", span: idField(1, spanIDA)},
		{name: "trace id too long", span: idField(1, append(append([]byte{}, traceIDA...), 0xFF))},
		{name: "span id too long", span: idField(2, traceIDA)},
		{name: "parent span id too short", span: idField(4, []byte{1, 2, 3})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := spanScalarsOf(t, Span(tt.span))
			require.ErrorContains(t, err, "unexpected size")

			_, pdataErr := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(exportTracesWithSpan(tt.span))
			require.Error(t, pdataErr, "pdata must reject the same bytes")
		})
	}
}

// TestSpanFields_TrailingCorruption is the observable consequence of sharing
// one walk: an accessor reports corruption that sits after the field it reads,
// which a first-match scan would have returned a value for. It is what makes
// the wire path agree with pdata, which unmarshals the whole span.
func TestSpanFields_TrailingCorruption(t *testing.T) {
	span := slices.Concat(
		idField(1, traceIDA),
		nameField("valid-prefix"),
		kindField(uint64(ptrace.SpanKindServer)),
		timestampField(7, 1),
		timestampField(8, 2),
		[]byte{0x4a, 0x08, 0x01}, // attributes, length 8, only one byte present
	)

	_, err := spanScalarsOf(t, Span(span))
	require.Error(t, err)

	_, pdataErr := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(exportTracesWithSpan(span))
	require.Error(t, pdataErr)
}

// TestSpanFields_PdataDivergence pins the inputs where the shared Span walk
// and pdata disagree about validity. They are the exception to the parity the
// rest of the suite asserts, they are all reachable only on malformed or
// adversarial input, and operations.md documents them for consumers that pair
// the wire path with a pdata fallback. The point is to notice when one of them
// changes, in either direction.
func TestSpanFields_PdataDivergence(t *testing.T) {
	tests := []struct {
		name        string
		span        []byte
		wireRejects bool
	}{
		{
			// appendUnknownGroup closes the group at the very end of the span,
			// where pdata's ConsumeUnknown loop exits before processing the
			// final EndGroup. A matched group followed by another field is
			// accepted by both, so this is narrower than "groups diverge".
			name:        "unknown group closing at the end of the span",
			span:        appendUnknownGroup(nameField("named"), 90),
			wireRejects: false,
		},
		{
			// protowire requires the EndGroup number to match its StartGroup;
			// pdata keeps only a depth counter and never compares them. This
			// is the one divergence that runs the other way.
			name:        "unknown group closed with a mismatched field number",
			span:        []byte{0xd3, 0x05, 0xdc, 0x05, 0x2a, 0x04, 't', 'a', 'i', 'l'},
			wireRejects: true,
		},
		// The walk is framing-only below the Span level, so corruption in the
		// contents of a nested message is invisible to it. These four are the
		// systematic consequence of that depth choice, not four accidents.
		{
			name:        "malformed KeyValue inside attributes",
			span:        bytesField(9, protowire.AppendVarint(protowire.AppendTag(nil, 1, protowire.VarintType), 7)),
			wireRejects: false,
		},
		{
			name:        "malformed SpanEvent inside events",
			span:        bytesField(11, protowire.AppendVarint(protowire.AppendTag(nil, 2, protowire.VarintType), 7)),
			wireRejects: false,
		},
		{
			name:        "wrongly sized trace_id inside links",
			span:        bytesField(13, protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), []byte{1, 2, 3})),
			wireRejects: false,
		},
		{
			name:        "malformed Status message",
			span:        bytesField(15, protowire.AppendVarint(protowire.AppendTag(nil, 2, protowire.VarintType), 7)),
			wireRejects: false,
		},
		{
			// A 10-byte varint carries at most bit 63 in its final byte, so a
			// final byte >= 2 sets bits the uint64 cannot hold. protowire
			// rejects the overflow; pdata's generated loop stops shifting at
			// 64 bits and keeps the truncated value.
			name:        "overflowing varint in kind",
			span:        []byte{0x30, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x02},
			wireRejects: true,
		},
		{
			name:        "overflowing varint as the name length",
			span:        []byte{0x2a, 0x81, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x02, 'X'},
			wireRejects: true,
		},
		{
			// Field number 206695283200 truncates to a positive int32, so
			// pdata skips it where protowire rejects the tag outright.
			name:        "field number above MaxInt32 truncating positive",
			span:        []byte{0x90, 0x90, 0x90, 0x90, 0x90, 0x30, 0x30},
			wireRejects: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, wireErr := spanScalarsOf(t, Span(tt.span))
			_, pdataErr := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(exportTracesWithSpan(tt.span))

			if tt.wireRejects {
				require.Error(t, wireErr, "wire path must reject")
				require.NoError(t, pdataErr, "pdata is documented as accepting this")
				return
			}
			require.NoError(t, wireErr, "wire path must accept")
			require.Error(t, pdataErr, "pdata is documented as rejecting this")
		})
	}
}

// TestSpanName_ViewAndAllocationContract pins what Name promises beyond its
// value: the result aliases the caller's buffer with a clamped capacity, and
// reading it allocates nothing.
func TestSpanName_ViewAndAllocationContract(t *testing.T) {
	span := Span(slices.Concat(idField(1, traceIDA), nameField("GET /orders")))

	name, err := span.Name()
	require.NoError(t, err)
	require.Equal(t, len(name), cap(name), "capacity must be clamped so a caller's append reallocates")

	// A copy would keep the old byte.
	index := bytes.Index(span, []byte("GET /orders"))
	require.GreaterOrEqual(t, index, 0)
	span[index] = 'P'
	require.Equal(t, []byte("PET /orders"), name)
}

// TestSpanFields_ZeroAllocation is a hard gate: every accessor reads out of a
// by-value struct that must not escape.
func TestSpanFields_ZeroAllocation(t *testing.T) {
	span := Span(slices.Concat(
		idField(1, traceIDA),
		idField(2, spanIDA),
		idField(4, spanIDB),
		nameField("GET /orders"),
		kindField(uint64(ptrace.SpanKindServer)),
		timestampField(7, 1),
		timestampField(8, 2),
		spanAttribute("http.method", "GET"),
	))

	accessors := map[string]func() error{
		"TraceID":           func() error { _, err := span.TraceID(); return err },
		"SpanID":            func() error { _, err := span.SpanID(); return err },
		"ParentSpanID":      func() error { _, err := span.ParentSpanID(); return err },
		"Name":              func() error { _, err := span.Name(); return err },
		"Kind":              func() error { _, err := span.Kind(); return err },
		"StartTimeUnixNano": func() error { _, err := span.StartTimeUnixNano(); return err },
		"EndTimeUnixNano":   func() error { _, err := span.EndTimeUnixNano(); return err },
	}

	for name, read := range accessors {
		t.Run(name, func(t *testing.T) {
			allocations := testing.AllocsPerRun(100, func() {
				if err := read(); err != nil {
					t.Fatalf("unexpected error from %s: %v", name, err)
				}
			})
			require.Zero(t, allocations)
		})
	}
}
