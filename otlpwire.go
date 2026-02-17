// Package otlpwire provides utilities for working with OTLP wire format data.
package otlpwire

import (
	"errors"
	"io"
	"iter"

	"google.golang.org/protobuf/encoding/protowire"
)

// ExportMetricsServiceRequest represents an OTLP ExportMetricsServiceRequest message.
type ExportMetricsServiceRequest []byte

// ExportLogsServiceRequest represents an OTLP ExportLogsServiceRequest message.
type ExportLogsServiceRequest []byte

// ExportTracesServiceRequest represents an OTLP ExportTracesServiceRequest message.
type ExportTracesServiceRequest []byte

// ResourceMetrics represents a single ResourceMetrics message.
type ResourceMetrics []byte

// ResourceLogs represents a single ResourceLogs message.
type ResourceLogs []byte

// ResourceSpans represents a single ResourceSpans message.
type ResourceSpans []byte

// ScopeSpans represents a single ScopeSpans message (raw wire bytes).
type ScopeSpans []byte

// Span represents a single Span message (raw wire bytes).
type Span []byte

// DataPointCount returns the total number of metric data points in the batch.
func (m ExportMetricsServiceRequest) DataPointCount() (int, error) {
	return countMetricDataPoints([]byte(m))
}

// ResourceMetrics returns an iterator over ResourceMetrics in the batch.
// The returned function should be called after iteration to check for errors.
func (m ExportMetricsServiceRequest) ResourceMetrics() (iter.Seq[ResourceMetrics], func() error) {
	var iterErr error

	seq := func(yield func(ResourceMetrics) bool) {
		forEachResourceMetrics([]byte(m), func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(ResourceMetrics(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// DataPointCount returns the number of metric data points in this resource.
func (r ResourceMetrics) DataPointCount() (int, error) {
	return countInResourceMetrics([]byte(r))
}

// Resource returns the raw Resource message bytes.
func (r ResourceMetrics) Resource() ([]byte, error) {
	return extractResourceMessage([]byte(r))
}

// WriteTo writes the ResourceMetrics as a valid ExportMetricsServiceRequest to w.
// Implements io.WriterTo interface.
func (r ResourceMetrics) WriteTo(w io.Writer) (int64, error) {
	return writeResourceMessage(w, []byte(r))
}

// LogRecordCount returns the total number of log records in the batch.
func (l ExportLogsServiceRequest) LogRecordCount() (int, error) {
	return countLogRecords([]byte(l))
}

// ResourceLogs returns an iterator over ResourceLogs in the batch.
// The returned function should be called after iteration to check for errors.
func (l ExportLogsServiceRequest) ResourceLogs() (iter.Seq[ResourceLogs], func() error) {
	var iterErr error

	seq := func(yield func(ResourceLogs) bool) {
		forEachResourceLogs([]byte(l), func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(ResourceLogs(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// LogRecordCount returns the number of log records in this resource.
func (r ResourceLogs) LogRecordCount() (int, error) {
	return countInResourceLogs([]byte(r))
}

// Resource returns the raw Resource message bytes.
func (r ResourceLogs) Resource() ([]byte, error) {
	return extractResourceMessage([]byte(r))
}

// WriteTo writes the ResourceLogs as a valid ExportLogsServiceRequest to w.
// Implements io.WriterTo interface.
func (r ResourceLogs) WriteTo(w io.Writer) (int64, error) {
	return writeResourceMessage(w, []byte(r))
}

// SpanCount returns the total number of spans in the batch.
func (t ExportTracesServiceRequest) SpanCount() (int, error) {
	return countSpans([]byte(t))
}

// ResourceSpans returns an iterator over ResourceSpans in the batch.
// The returned function should be called after iteration to check for errors.
func (t ExportTracesServiceRequest) ResourceSpans() (iter.Seq[ResourceSpans], func() error) {
	var iterErr error

	seq := func(yield func(ResourceSpans) bool) {
		forEachResourceSpans([]byte(t), func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(ResourceSpans(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// SpanCount returns the number of spans in this resource.
func (r ResourceSpans) SpanCount() (int, error) {
	return countInResourceSpans([]byte(r))
}

// Resource returns the raw Resource message bytes.
func (r ResourceSpans) Resource() ([]byte, error) {
	return extractResourceMessage([]byte(r))
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
	var iterErr error

	seq := func(yield func(ScopeSpans) bool) {
		forEachRepeatedField([]byte(r), 2, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(ScopeSpans(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// SpanCount returns the number of spans in this ScopeSpans.
func (s ScopeSpans) SpanCount() (int, error) {
	return countOccurrences([]byte(s), 2)
}

// Spans returns an iterator over Spans in this ScopeSpans.
// Field 2 in the ScopeSpans protobuf message.
// The returned function should be called after iteration to check for errors.
func (s ScopeSpans) Spans() (iter.Seq[Span], func() error) {
	var iterErr error

	seq := func(yield func(Span) bool) {
		forEachRepeatedField([]byte(s), 2, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(Span(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
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

// countMetricDataPoints counts the number of metric data points in an OTLP
// ExportMetricsServiceRequest protobuf message without unmarshaling it.
//
// Wire format structure:
//
//	ExportMetricsServiceRequest
//	  └─ field 1: ResourceMetrics[] (repeated message)
//	      └─ field 2: ScopeMetrics[] (repeated message)
//	          └─ field 2: Metric[] (repeated message)
//	              └─ field 5: Gauge | field 7: Sum | field 9: Histogram | etc.
//	                  └─ field 1: DataPoints[] (repeated message) ← count these
func countMetricDataPoints(data []byte) (int, error) {
	return countRepeatedField(data, 1, countInResourceMetrics)
}

// countLogRecords counts the number of log records in an OTLP
// ExportLogsServiceRequest protobuf message without unmarshaling it.
//
// Wire format structure:
//
//	ExportLogsServiceRequest
//	  └─ field 1: ResourceLogs[] (repeated message)
//	      └─ field 2: ScopeLogs[] (repeated message)
//	          └─ field 2: LogRecord[] (repeated message) ← count these
func countLogRecords(data []byte) (int, error) {
	return countRepeatedField(data, 1, countInResourceLogs)
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

func countInResourceMetrics(data []byte) (int, error) {
	return countRepeatedField(data, 2, countInScopeMetrics)
}

func countInResourceLogs(data []byte) (int, error) {
	return countRepeatedField(data, 2, countInScopeLogs)
}

func countInResourceSpans(data []byte) (int, error) {
	return countRepeatedField(data, 2, countInScopeSpans)
}

func countInScopeMetrics(data []byte) (int, error) {
	return countRepeatedField(data, 2, countInMetric)
}

func countInScopeLogs(data []byte) (int, error) {
	return countOccurrences(data, 2)
}

func countInScopeSpans(data []byte) (int, error) {
	return countOccurrences(data, 2)
}

func countInMetric(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in metric")
		}
		pos += tagLen

		// Metric types: field 5=Gauge, 7=Sum, 9=Histogram, 10=ExponentialHistogram, 11=Summary
		if (fieldNum == 5 || fieldNum == 7 || fieldNum == 9 || fieldNum == 10 || fieldNum == 11) && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in metric data")
			}
			pos += n

			c, err := countDataPoints(msgBytes)
			if err != nil {
				return 0, err
			}
			count += c
		} else {
			n := skipField(data[pos:], wireType)
			if n < 0 {
				return 0, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return count, nil
}

func countDataPoints(data []byte) (int, error) {
	return countOccurrences(data, 1)
}

// skipField skips a field based on its wire type.
// Returns the number of bytes skipped. Returns negative value on error.
func skipField(data []byte, wireType protowire.Type) int {
	switch wireType {
	case protowire.VarintType:
		_, n := protowire.ConsumeVarint(data)
		return n
	case protowire.Fixed64Type:
		_, n := protowire.ConsumeFixed64(data)
		return n
	case protowire.BytesType:
		_, n := protowire.ConsumeBytes(data)
		return n
	case protowire.Fixed32Type:
		_, n := protowire.ConsumeFixed32(data)
		return n
	default:
		return -1
	}
}

// countRepeatedField counts items in a repeated field by delegating to countFunc
// for each occurrence of the specified field.
func countRepeatedField(data []byte, fieldNum protowire.Number, countFunc func([]byte) (int, error)) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return 0, errors.New("wrong wire type for field")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in repeated field")
			}
			pos += n

			c, err := countFunc(msgBytes)
			if err != nil {
				return 0, err
			}
			count += c
		} else {
			n := skipField(data[pos:], wireType)
			if n < 0 {
				return 0, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return count, nil
}

// countOccurrences counts direct occurrences of a specific field.
func countOccurrences(data []byte, fieldNum protowire.Number) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return 0, errors.New("wrong wire type for field")
			}
			_, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in field")
			}
			pos += n
			count++
		} else {
			n := skipField(data[pos:], wireType)
			if n < 0 {
				return 0, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return count, nil
}

// forEachRepeatedField iterates over a repeated field, calling fn for each occurrence.
// The callback receives field bytes or an error. Return false to stop iteration.
func forEachRepeatedField(data []byte, fieldNum protowire.Number, fn func([]byte, error) bool) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			fn(nil, errors.New("malformed protobuf tag"))
			return
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				fn(nil, errors.New("wrong wire type for field"))
				return
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				fn(nil, errors.New("invalid bytes in repeated field"))
				return
			}
			pos += n

			if !fn(msgBytes, nil) {
				return
			}
		} else {
			n := skipField(data[pos:], wireType)
			if n < 0 {
				fn(nil, errors.New("failed to skip field"))
				return
			}
			pos += n
		}
	}
}

// forEachResourceMetrics iterates over ResourceMetrics messages, calling fn for each.
// The callback receives resource bytes or an error. Return false to stop iteration.
func forEachResourceMetrics(data []byte, fn func([]byte, error) bool) {
	forEachRepeatedField(data, 1, fn)
}

// forEachResourceLogs iterates over ResourceLogs messages, calling fn for each.
// The callback receives resource bytes or an error. Return false to stop iteration.
func forEachResourceLogs(data []byte, fn func([]byte, error) bool) {
	forEachRepeatedField(data, 1, fn)
}

// forEachResourceSpans iterates over ResourceSpans messages, calling fn for each.
// The callback receives resource bytes or an error. Return false to stop iteration.
func forEachResourceSpans(data []byte, fn func([]byte, error) bool) {
	forEachRepeatedField(data, 1, fn)
}

// extractResourceMessage extracts the Resource message (field 1) from
// ResourceMetrics/ResourceLogs/ResourceSpans messages.
func extractResourceMessage(data []byte) ([]byte, error) {
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		// Field 1 = Resource (message)
		if fieldNum == 1 {
			if wireType != protowire.BytesType {
				return nil, errors.New("resource field has wrong wire type")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in resource field")
			}
			return msgBytes, nil
		}

		// Skip other fields
		n := skipField(data[pos:], wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	return nil, errors.New("resource field not found")
}

// writeResourceMessage writes resource data as a valid OTLP export request message.
// Wraps the resource bytes with field tag 1 and length prefix.
func writeResourceMessage(w io.Writer, data []byte) (int64, error) {
	buf := make([]byte, 0, 11) // tag + length varint
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(len(data)))

	n1, err := w.Write(buf)
	if err != nil {
		return int64(n1), err
	}

	n2, err := w.Write(data)
	return int64(n1 + n2), err
}

// extractFixedBytesField extracts a bytes field of known size from protobuf data.
// Returns nil (not an error) if the field is not present.
func extractFixedBytesField(data []byte, fieldNum protowire.Number, size int) ([]byte, error) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return nil, errors.New("wrong wire type for field")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in field")
			}
			if len(msgBytes) == 0 {
				return nil, nil // proto3 zero-value encoding, treat as absent
			}
			if len(msgBytes) != size {
				return nil, errors.New("field has unexpected size")
			}
			return msgBytes, nil
		}

		n := skipField(data[pos:], wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	return nil, nil
}
