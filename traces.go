package otlpwire

import (
	"errors"
	"io"
	"iter"

	"google.golang.org/protobuf/encoding/protowire"
)

// SpanCount returns the total number of spans in the batch.
func (t ExportTracesServiceRequest) SpanCount() (int, error) {
	return countSpans([]byte(t))
}

// ResourceSpans returns an iterator over ResourceSpans in the batch. The
// returned function should be called after iteration to check for errors.
// ResourceSpans is a thin adapter over ResourceSpansSeq.
func (t ExportTracesServiceRequest) ResourceSpans() (iter.Seq[ResourceSpans], func() error) {
	var iterErr error
	seq := func(yield func(ResourceSpans) bool) {
		t.ResourceSpansSeq(func(resource ResourceSpans, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(resource)
		})
	}
	return seq, func() error { return iterErr }
}

// ResourceSpansSeq is the zero-allocation alternative to ResourceSpans. It is
// shaped like iter.Seq2[ResourceSpans, error] and meant to be ranged over
// directly. On a parse error it yields a nil ResourceSpans with a non-nil
// error and stops.
func (t ExportTracesServiceRequest) ResourceSpansSeq(yield func(ResourceSpans, error) bool) {
	repeatedFieldSeq2([]byte(t), 1, yield)
}

// SpanCount returns the number of spans in this resource.
func (r ResourceSpans) SpanCount() (int, error) {
	return countInResourceSpans([]byte(r))
}

// Resource returns the Resource message for this ResourceSpans. It returns
// (nil, nil) when the field is absent, aliases the input for the single
// occurrence every real producer emits, and merges 2+ occurrences into a new
// buffer. See extractMergedMessage for the full contract.
func (r ResourceSpans) Resource() (Resource, error) {
	raw, err := extractMergedMessage([]byte(r), 1)
	if err != nil {
		return nil, err
	}
	return Resource(raw), nil
}

// SchemaUrl returns the ResourceSpans schema_url (field 3) as a view into the
// underlying buffer. Returns nil if the field is not present. Repeated
// occurrences resolve to the last one.
func (r ResourceSpans) SchemaUrl() ([]byte, error) {
	return extractLastBytesField([]byte(r), 3)
}

// WriteTo writes the ResourceSpans as a valid ExportTracesServiceRequest to w.
// Implements io.WriterTo interface.
func (r ResourceSpans) WriteTo(w io.Writer) (int64, error) {
	return writeResourceMessage(w, []byte(r))
}

// ScopeSpans returns an iterator over ScopeSpans in this ResourceSpans.
// Field 2 in the ResourceSpans protobuf message.
// The returned function should be called after iteration to check for errors.
// ScopeSpans is a thin adapter over ScopeSpansSeq.
func (r ResourceSpans) ScopeSpans() (iter.Seq[ScopeSpans], func() error) {
	var iterErr error
	seq := func(yield func(ScopeSpans) bool) {
		r.ScopeSpansSeq(func(scope ScopeSpans, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(scope)
		})
	}
	return seq, func() error { return iterErr }
}

// ScopeSpansSeq is the zero-allocation alternative to ScopeSpans. It is shaped
// like iter.Seq2[ScopeSpans, error] and meant to be ranged over directly. On a
// parse error it yields a nil ScopeSpans with a non-nil error and stops.
func (r ResourceSpans) ScopeSpansSeq(yield func(ScopeSpans, error) bool) {
	repeatedFieldSeq2([]byte(r), 2, yield)
}

// SpanCount returns the number of spans in this ScopeSpans.
func (s ScopeSpans) SpanCount() (int, error) {
	return countOccurrences([]byte(s), 2)
}

// Scope returns the InstrumentationScope for this ScopeSpans. It returns
// (nil, nil) when the field is absent, aliases the input for the single
// occurrence every real producer emits, and merges 2+ occurrences into a new
// buffer. See extractMergedMessage for the full contract.
func (s ScopeSpans) Scope() (InstrumentationScope, error) {
	raw, err := extractMergedMessage([]byte(s), 1)
	if err != nil {
		return nil, err
	}
	return InstrumentationScope(raw), nil
}

// SchemaUrl returns the ScopeSpans schema_url (field 3) as a view into the
// underlying buffer. Returns nil if the field is not present. Repeated
// occurrences resolve to the last one.
func (s ScopeSpans) SchemaUrl() ([]byte, error) {
	return extractLastBytesField([]byte(s), 3)
}

// Spans returns an iterator over Spans in this ScopeSpans.
// Field 2 in the ScopeSpans protobuf message.
// The returned function should be called after iteration to check for errors.
func (s ScopeSpans) Spans() (iter.Seq[Span], func() error) {
	return repeatedFieldSeq[Span]([]byte(s), 2)
}

// TraceID extracts the trace ID from the Span.
// Returns the raw 16 bytes from field 1.
// Returns zero value if the field is not present.
func (s Span) TraceID() ([16]byte, error) {
	raw, err := extractFixedBytesField([]byte(s), 1, 16)
	if err != nil {
		return [16]byte{}, err
	}
	var id [16]byte
	copy(id[:], raw)
	return id, nil
}

// SpanID extracts the span ID from the Span.
// Returns the raw 8 bytes from field 2.
// Returns zero value if the field is not present.
func (s Span) SpanID() ([8]byte, error) {
	raw, err := extractFixedBytesField([]byte(s), 2, 8)
	if err != nil {
		return [8]byte{}, err
	}
	var id [8]byte
	copy(id[:], raw)
	return id, nil
}

// ParentSpanID extracts the parent span ID from the Span.
// Returns the raw 8 bytes from field 4.
// Returns zero value if the field is not present (root span).
func (s Span) ParentSpanID() ([8]byte, error) {
	raw, err := extractFixedBytesField([]byte(s), 4, 8)
	if err != nil {
		return [8]byte{}, err
	}
	var id [8]byte
	copy(id[:], raw)
	return id, nil
}

// Name returns the Span name (field 5) as a view into the underlying buffer.
// It returns nil when the field is absent and a non-nil zero-length slice when
// it is present but empty, which pdata cannot distinguish. Repeated
// occurrences resolve to the last one. UTF-8 validity is consumer policy and
// is not checked here; docs/specification.md records why.
func (s Span) Name() ([]byte, error) {
	fields, err := parseSpanFields([]byte(s))
	return fields.name, err
}

// Kind returns the Span kind enum (field 6). It returns 0
// (SPAN_KIND_UNSPECIFIED) when the field is absent. Values are represented as
// int32 so that unexpected negative protobuf enum values remain
// distinguishable from the defined OTLP range.
func (s Span) Kind() (int32, error) {
	fields, err := parseSpanFields([]byte(s))
	return fields.kind, err
}

// StartTimeUnixNano returns the Span start_time_unix_nano (field 7). It
// returns 0 when the field is absent, which OTLP also uses to mean "unknown".
func (s Span) StartTimeUnixNano() (uint64, error) {
	fields, err := parseSpanFields([]byte(s))
	return fields.startUnixNano, err
}

// EndTimeUnixNano returns the Span end_time_unix_nano (field 8). It returns 0
// when the field is absent, which OTLP also uses to mean "unknown".
func (s Span) EndTimeUnixNano() (uint64, error) {
	fields, err := parseSpanFields([]byte(s))
	return fields.endUnixNano, err
}

// spanFields holds the Span scalars that resolve last-value-wins. It is
// returned by value and never escapes, so the accessors stay allocation-free.
type spanFields struct {
	name          []byte
	startUnixNano uint64
	endUnixNano   uint64
	kind          int32
}

// parseSpanFields validates every known Span field in pdata v1.64.0 while
// extracting name, kind and the two timestamps with protobuf last-value-wins
// scalar semantics. Those four accessors share this walk, so they can never
// disagree with each other about whether the same bytes are valid.
//
// Reaching the last occurrence of a scalar means walking to the end of the
// span, so this reports corruption located after the field it read. The
// identifier accessors scan first-match instead and stop early, which is
// cheaper and indistinguishable on conformant OTLP; docs/DESIGN.md records
// that decision and operations.md pins where the two behaviors differ.
//
// The walk is schema-aware at the Span level and framing-only below it: the
// attributes, events, links and status fields are checked for wire type and
// containment, but their contents are not parsed. That bounds the cost at the
// span's own field count rather than its payload size, unlike the LogRecord
// walk. docs/DESIGN.md records why the depths differ.
//
// The result is named so the walk writes straight into the caller's result
// slot instead of zeroing and copying a second local.
func parseSpanFields(data []byte) (fields spanFields, err error) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return spanFields{}, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		switch num {
		case 1, 2, 4: // trace_id, span_id, parent_span_id
			if wireType != protowire.BytesType {
				return spanFields{}, errors.New("wrong wire type for span identifier")
			}
			identifier, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return spanFields{}, errors.New("invalid bytes in span identifier")
			}
			// Validated but not captured: the identifier accessors read these
			// fields themselves. Rejecting a wrongly sized identifier here
			// keeps this walk's verdict aligned with both pdata and
			// extractFixedBytesField.
			expectedSize := 8
			if num == 1 {
				expectedSize = 16
			}
			if len(identifier) != 0 && len(identifier) != expectedSize {
				return spanFields{}, errors.New("span identifier has unexpected size")
			}
			pos += n
		case 3, 5: // trace_state, name
			if wireType != protowire.BytesType {
				return spanFields{}, errors.New("wrong wire type for span string")
			}
			value, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return spanFields{}, errors.New("invalid bytes in span string")
			}
			if num == 5 {
				// Clamped so a caller's append reallocates instead of
				// overwriting the span fields that follow this one.
				fields.name = value[:len(value):len(value)]
			}
			pos += n
		case 6: // kind
			if wireType != protowire.VarintType {
				return spanFields{}, errors.New("wrong wire type for span kind")
			}
			value, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return spanFields{}, errors.New("invalid varint in span kind")
			}
			fields.kind = int32(value)
			pos += n
		case 7, 8: // start_time_unix_nano, end_time_unix_nano
			if wireType != protowire.Fixed64Type {
				return spanFields{}, errors.New("wrong wire type for span timestamp")
			}
			value, n := protowire.ConsumeFixed64(data[pos:])
			if n < 0 {
				return spanFields{}, errors.New("invalid fixed64 in span timestamp")
			}
			if num == 7 {
				fields.startUnixNano = value
			} else {
				fields.endUnixNano = value
			}
			pos += n
		case 9, 11, 13, 15: // attributes, events, links, status
			if wireType != protowire.BytesType {
				return spanFields{}, errors.New("wrong wire type for span message field")
			}
			_, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return spanFields{}, errors.New("invalid bytes in span message field")
			}
			pos += n
		case 10, 12, 14: // dropped_attributes_count, dropped_events_count, dropped_links_count
			if wireType != protowire.VarintType {
				return spanFields{}, errors.New("wrong wire type for span dropped count")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return spanFields{}, errors.New("invalid varint in span dropped count")
			}
			pos += n
		case 16: // flags
			if wireType != protowire.Fixed32Type {
				return spanFields{}, errors.New("wrong wire type for span flags")
			}
			_, n := protowire.ConsumeFixed32(data[pos:])
			if n < 0 {
				return spanFields{}, errors.New("invalid fixed32 in span flags")
			}
			pos += n
		default:
			n := skipField(data[pos:], num, wireType)
			if n < 0 {
				return spanFields{}, errors.New("failed to skip field in span")
			}
			pos += n
		}
	}

	return fields, nil
}

// countSpans counts the number of spans in an OTLP
// ExportTracesServiceRequest protobuf message without unmarshaling it.
//
// Wire format structure:
//
//	ExportTracesServiceRequest
//	  └─ field 1: ResourceSpans[] (repeated message)
//	      └─ field 2: ScopeSpans[] (repeated message)
//	          └─ field 2: Span[] (repeated message) ← count these
func countSpans(data []byte) (int, error) {
	return countRepeatedField(data, 1, countInResourceSpans)
}

func countInResourceSpans(data []byte) (int, error) {
	return countRepeatedField(data, 2, countInScopeSpans)
}

func countInScopeSpans(data []byte) (int, error) {
	return countOccurrences(data, 2)
}
