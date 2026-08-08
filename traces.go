package otlpwire

import (
	"io"
	"iter"
)

// SpanCount returns the total number of spans in the batch.
func (t ExportTracesServiceRequest) SpanCount() (int, error) {
	return countSpans([]byte(t))
}

// ResourceSpans returns an iterator over ResourceSpans in the batch.
// The returned function should be called after iteration to check for errors.
func (t ExportTracesServiceRequest) ResourceSpans() (iter.Seq[ResourceSpans], func() error) {
	return repeatedFieldSeq[ResourceSpans]([]byte(t), 1)
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
func (r ResourceSpans) ScopeSpans() (iter.Seq[ScopeSpans], func() error) {
	return repeatedFieldSeq[ScopeSpans]([]byte(r), 2)
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
