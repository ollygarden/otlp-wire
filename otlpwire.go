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

// Resource represents a Resource message (raw wire bytes).
//
// Resource values are normally obtained by converting the bytes returned by a
// ResourceMetrics, ResourceLogs, or ResourceSpans Resource method. This keeps
// those established methods source-compatible while providing attribute
// accessors for callers that need resource context.
type Resource []byte

// ScopeLogs represents a single ScopeLogs message (raw wire bytes).
type ScopeLogs []byte

// LogRecord represents a single LogRecord message (raw wire bytes).
type LogRecord []byte

// ResourceSpans represents a single ResourceSpans message.
type ResourceSpans []byte

// ScopeSpans represents a single ScopeSpans message (raw wire bytes).
type ScopeSpans []byte

// Span represents a single Span message (raw wire bytes).
type Span []byte

// ScopeMetrics represents a single ScopeMetrics message (raw wire bytes).
type ScopeMetrics []byte

// Metric represents a single Metric message (raw wire bytes).
type Metric []byte

// MetricType identifies which oneof body a DataPoint came from.
type MetricType int

// Metric oneof body field numbers in the Metric protobuf message.
const (
	MetricTypeGauge                MetricType = 5
	MetricTypeSum                  MetricType = 7
	MetricTypeHistogram            MetricType = 9
	MetricTypeExponentialHistogram MetricType = 10
	MetricTypeSummary              MetricType = 11
)

// DataPoint represents a single datapoint message (raw wire bytes) together
// with the metric type it came from. The type is needed because the
// attributes field number differs between datapoint message types.
type DataPoint struct {
	raw []byte
	typ MetricType
}

// Raw returns the raw datapoint message bytes.
func (d DataPoint) Raw() []byte { return d.raw }

// Type returns the metric type this datapoint came from.
func (d DataPoint) Type() MetricType { return d.typ }

// KeyValue represents a single KeyValue message (raw wire bytes).
type KeyValue []byte

// Key returns the attribute key (field 1) as a view into the underlying
// buffer. Returns nil if the field is not present.
func (kv KeyValue) Key() ([]byte, error) {
	return extractBytesField([]byte(kv), 1)
}

// ValueRaw returns the raw AnyValue message bytes (field 2) as a view into
// the underlying buffer, suitable for type-tagged hashing.
// Returns nil if the field is not present.
func (kv KeyValue) ValueRaw() ([]byte, error) {
	return extractBytesField([]byte(kv), 2)
}

// StringValue returns the string value in the KeyValue's AnyValue, if it has
// one. The bool reports whether a string value was present; an empty string is
// present and is returned as a non-nil, zero-length slice. The returned slice
// aliases the underlying buffer.
func (kv KeyValue) StringValue() ([]byte, bool, error) {
	_, _, value, err := parseKeyValue([]byte(kv))
	if err != nil || value.kind != anyValueString {
		return nil, false, err
	}
	return value.stringValue, true, nil
}

// Attributes returns an iterator over the Resource's attribute KeyValues.
// The returned function should be called after iteration to check for errors.
func (r Resource) Attributes() (iter.Seq[KeyValue], func() error) {
	var iterErr error

	seq := func(yield func(KeyValue) bool) {
		forEachRepeatedField([]byte(r), 1, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(KeyValue(rb))
		})
	}

	return seq, func() error { return iterErr }
}

// AttributesSeq is a zero-allocation alternative to Attributes. It has the
// shape of an iter.Seq2[KeyValue, error] and is meant to be ranged over
// directly. On a parse error it yields a nil KeyValue with a non-nil error and
// stops.
func (r Resource) AttributesSeq(yield func(KeyValue, error) bool) {
	forEachRepeatedField([]byte(r), 1, func(rb []byte, err error) bool {
		if err != nil {
			yield(nil, err)
			return false
		}
		return yield(KeyValue(rb), nil)
	})
}

// StringAttribute returns the string value of the first resource attribute
// named key. The bool reports whether that attribute had a string value; it
// distinguishes an absent attribute from a present-but-empty string. The
// returned slice aliases the underlying buffer.
func (r Resource) StringAttribute(key string) ([]byte, bool, error) {
	var state resourceStringAttributeState
	if err := scanResourceStringAttribute([]byte(r), key, &state); err != nil {
		return nil, false, err
	}
	return state.value, state.found, nil
}

// StringAttribute returns a string resource attribute from all Resource
// messages encoded in this ResourceLogs. Singular Resource messages merge in
// wire order; resource attributes retain pdata's first-value-wins behavior for
// duplicate keys across the merged messages.
func (r ResourceLogs) StringAttribute(key string) ([]byte, bool, error) {
	var state resourceStringAttributeState
	data := []byte(r)
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, false, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		switch fieldNum {
		case 1: // Resource.attributes
			if wireType != protowire.BytesType {
				return nil, false, errors.New("wrong wire type for resource logs resource")
			}
			resource, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, false, errors.New("invalid bytes in resource logs resource")
			}
			pos += n
			if err := scanResourceStringAttribute(resource, key, &state); err != nil {
				return nil, false, err
			}
		case 2: // ResourceLogs.scope_logs
			if wireType != protowire.BytesType {
				return nil, false, errors.New("wrong wire type for resource logs scope logs")
			}
			_, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, false, errors.New("invalid bytes in resource logs scope logs")
			}
			pos += n
		case 3: // ResourceLogs.schema_url
			if wireType != protowire.BytesType {
				return nil, false, errors.New("wrong wire type for resource logs schema url")
			}
			_, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, false, errors.New("invalid bytes in resource logs schema url")
			}
			pos += n
		default:
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				return nil, false, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return state.value, state.found, nil
}

// attributesFieldNum returns the field number of the repeated KeyValue
// attributes for each datapoint message type.
func (d DataPoint) attributesFieldNum() protowire.Number {
	switch d.typ {
	case MetricTypeHistogram:
		return 9
	case MetricTypeExponentialHistogram:
		return 1
	default: // NumberDataPoint (gauge, sum) and SummaryDataPoint
		return 7
	}
}

// Timestamp returns the datapoint's time_unix_nano (field 3, fixed64).
// Returns 0 if the field is not present.
func (d DataPoint) Timestamp() (uint64, error) {
	return extractFixed64Field(d.raw, 3)
}

// Attributes returns an iterator over the datapoint's attribute KeyValues.
// The returned function should be called after iteration to check for errors.
func (d DataPoint) Attributes() (iter.Seq[KeyValue], func() error) {
	var iterErr error
	fieldNum := d.attributesFieldNum()

	seq := func(yield func(KeyValue) bool) {
		forEachRepeatedField(d.raw, fieldNum, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(KeyValue(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// AttributesSeq is a zero-allocation alternative to Attributes. It has the
// shape of an iter.Seq2[KeyValue, error] and is meant to be ranged over
// directly:
//
//	for kv, err := range dp.AttributesSeq {
//		if err != nil { ... }
//	}
//
// On a parse error it yields a nil KeyValue with a non-nil error and stops.
func (d DataPoint) AttributesSeq(yield func(KeyValue, error) bool) {
	forEachRepeatedField(d.raw, d.attributesFieldNum(), func(rb []byte, err error) bool {
		if err != nil {
			yield(nil, err)
			return false
		}
		return yield(KeyValue(rb), nil)
	})
}

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

// ScopeMetrics returns an iterator over ScopeMetrics in this ResourceMetrics.
// Field 2 in the ResourceMetrics protobuf message.
// The returned function should be called after iteration to check for errors.
func (r ResourceMetrics) ScopeMetrics() (iter.Seq[ScopeMetrics], func() error) {
	var iterErr error

	seq := func(yield func(ScopeMetrics) bool) {
		forEachRepeatedField([]byte(r), 2, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(ScopeMetrics(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// Metrics returns an iterator over Metrics in this ScopeMetrics.
// Field 2 in the ScopeMetrics protobuf message.
// The returned function should be called after iteration to check for errors.
func (s ScopeMetrics) Metrics() (iter.Seq[Metric], func() error) {
	var iterErr error

	seq := func(yield func(Metric) bool) {
		forEachRepeatedField([]byte(s), 2, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(Metric(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// Name returns the metric name (field 1) as a view into the underlying
// buffer. Returns nil if the field is not present.
func (m Metric) Name() ([]byte, error) {
	return extractBytesField([]byte(m), 1)
}

// DataPoints returns an iterator over datapoints in this Metric, descending
// whichever oneof body is present (gauge 5, sum 7, histogram 9,
// exponential_histogram 10, summary 11). Each body holds its datapoints in
// field 1. If a malformed metric carries more than one oneof body,
// datapoints from each are yielded, each tagged with its own type.
// The returned function should be called after iteration to check for errors.
// DataPoints is a thin adapter over DataPointsSeq.
func (m Metric) DataPoints() (iter.Seq[DataPoint], func() error) {
	var iterErr error

	seq := func(yield func(DataPoint) bool) {
		m.DataPointsSeq(func(dp DataPoint, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(dp)
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// DataPointsSeq is a zero-allocation alternative to DataPoints. It has the
// shape of an iter.Seq2[DataPoint, error] and is meant to be ranged over
// directly:
//
//	for dp, err := range m.DataPointsSeq {
//		if err != nil { ... }
//	}
//
// On a parse error it yields a zero DataPoint with a non-nil error and
// stops. Unlike DataPoints, no closures escape, so iterating allocates
// nothing. If a malformed metric carries more than one oneof body,
// datapoints from each are yielded, each tagged with its own type.
func (m Metric) DataPointsSeq(yield func(DataPoint, error) bool) {
	data := []byte(m)
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			yield(DataPoint{}, errors.New("malformed protobuf tag in metric"))
			return
		}
		pos += tagLen

		typ := MetricType(fieldNum)
		isBody := typ == MetricTypeGauge || typ == MetricTypeSum ||
			typ == MetricTypeHistogram || typ == MetricTypeExponentialHistogram ||
			typ == MetricTypeSummary
		if isBody && wireType != protowire.BytesType {
			yield(DataPoint{}, errors.New("wrong wire type for metric data"))
			return
		}
		if isBody {
			body, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				yield(DataPoint{}, errors.New("invalid bytes in metric data"))
				return
			}
			pos += n

			done := false
			forEachRepeatedField(body, 1, func(dpBytes []byte, err error) bool {
				if err != nil {
					done = true
					yield(DataPoint{}, err)
					return false
				}
				if !yield(DataPoint{raw: dpBytes, typ: typ}, nil) {
					done = true
					return false
				}
				return true
			})
			if done {
				return
			}
		} else {
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				yield(DataPoint{}, errors.New("failed to skip field"))
				return
			}
			pos += n
		}
	}
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

// ScopeLogs returns an iterator over ScopeLogs in this ResourceLogs.
// Field 2 in the ResourceLogs protobuf message.
// The returned function should be called after iteration to check for errors.
func (r ResourceLogs) ScopeLogs() (iter.Seq[ScopeLogs], func() error) {
	var iterErr error

	seq := func(yield func(ScopeLogs) bool) {
		forEachRepeatedField([]byte(r), 2, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(ScopeLogs(rb))
		})
	}

	return seq, func() error { return iterErr }
}

// LogRecords returns an iterator over LogRecords in this ScopeLogs.
// Field 2 in the ScopeLogs protobuf message. The returned function should be
// called after iteration to check for errors. LogRecords is a thin adapter over
// LogRecordsSeq.
func (s ScopeLogs) LogRecords() (iter.Seq[LogRecord], func() error) {
	var iterErr error

	seq := func(yield func(LogRecord) bool) {
		s.LogRecordsSeq(func(record LogRecord, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(record)
		})
	}

	return seq, func() error { return iterErr }
}

// LogRecordsSeq is a zero-allocation alternative to LogRecords. It has the
// shape of an iter.Seq2[LogRecord, error] and is meant to be ranged over
// directly. On a parse error it yields a nil LogRecord with a non-nil error and
// stops.
func (s ScopeLogs) LogRecordsSeq(yield func(LogRecord, error) bool) {
	forEachRepeatedField([]byte(s), 2, func(rb []byte, err error) bool {
		if err != nil {
			yield(nil, err)
			return false
		}
		return yield(LogRecord(rb), nil)
	})
}

// SeverityNumber returns the LogRecord severity_number enum (field 2).
// It returns 0 when the field is absent. Values are represented as int32 so
// that unexpected negative protobuf enum values remain distinguishable from
// the positive OTLP severity ranges.
func (r LogRecord) SeverityNumber() (int32, error) {
	return parseLogRecordSeverity([]byte(r))
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
			n := skipField(data[pos:], fieldNum, wireType)
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

// skipField skips a field value after its tag has been consumed. The field
// number is required for groups so ConsumeFieldValue can verify the matching
// end-group marker and recursively validate its contents.
func skipField(data []byte, fieldNum protowire.Number, wireType protowire.Type) int {
	return protowire.ConsumeFieldValue(fieldNum, wireType, data)
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
			n := skipField(data[pos:], num, wireType)
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
			n := skipField(data[pos:], num, wireType)
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
			n := skipField(data[pos:], num, wireType)
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
		n := skipField(data[pos:], fieldNum, wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	return nil, errors.New("resource field not found")
}

// extractBytesField extracts the first occurrence of a length-delimited
// field from protobuf data. Returns nil (not an error) if absent.
// The returned slice aliases data; no copy is made.
func extractBytesField(data []byte, fieldNum protowire.Number) ([]byte, error) {
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
			return msgBytes, nil
		}

		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	return nil, nil
}

// extractFixed64Field extracts the first occurrence of a fixed64 field from
// protobuf data. Returns 0 (not an error) if absent.
func extractFixed64Field(data []byte, fieldNum protowire.Number) (uint64, error) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.Fixed64Type {
				return 0, errors.New("wrong wire type for field")
			}
			v, n := protowire.ConsumeFixed64(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid fixed64 in field")
			}
			return v, nil
		}

		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return 0, errors.New("failed to skip field")
		}
		pos += n
	}

	return 0, nil
}

// parseLogRecordSeverity validates every known LogRecord field in pdata v1.64.0
// while extracting severity_number with protobuf last-scalar-value semantics.
// This is intentionally schema-aware: an early valid severity cannot conceal a
// malformed body, attribute, timestamp, identifier, or trailing known field.
func parseLogRecordSeverity(data []byte) (int32, error) {
	pos := 0
	var result int32

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		switch num {
		case 1, 11: // time_unix_nano, observed_time_unix_nano
			if wireType != protowire.Fixed64Type {
				return 0, errors.New("wrong wire type for log record timestamp")
			}
			_, n := protowire.ConsumeFixed64(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid fixed64 in log record timestamp")
			}
			pos += n
		case 2: // severity_number
			if wireType != protowire.VarintType {
				return 0, errors.New("wrong wire type for log record severity number")
			}
			value, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid varint in log record severity number")
			}
			result = int32(value)
			pos += n
		case 3, 12: // severity_text, event_name
			if wireType != protowire.BytesType {
				return 0, errors.New("wrong wire type for log record string")
			}
			_, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in log record string")
			}
			pos += n
		case 5: // body
			if wireType != protowire.BytesType {
				return 0, errors.New("wrong wire type for log record body")
			}
			body, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in log record body")
			}
			if err := parseAnyValue(body, &parsedAnyValue{}); err != nil {
				return 0, err
			}
			pos += n
		case 6: // attributes
			if wireType != protowire.BytesType {
				return 0, errors.New("wrong wire type for log record attributes")
			}
			attribute, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in log record attributes")
			}
			if _, _, _, err := parseKeyValue(attribute); err != nil {
				return 0, err
			}
			pos += n
		case 7: // dropped_attributes_count
			if wireType != protowire.VarintType {
				return 0, errors.New("wrong wire type for log record dropped attributes count")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid varint in log record dropped attributes count")
			}
			pos += n
		case 8: // flags
			if wireType != protowire.Fixed32Type {
				return 0, errors.New("wrong wire type for log record flags")
			}
			_, n := protowire.ConsumeFixed32(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid fixed32 in log record flags")
			}
			pos += n
		case 9, 10: // trace_id, span_id
			if wireType != protowire.BytesType {
				return 0, errors.New("wrong wire type for log record identifier")
			}
			identifier, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in log record identifier")
			}
			expectedSize := 16
			if num == 10 {
				expectedSize = 8
			}
			if len(identifier) != 0 && len(identifier) != expectedSize {
				return 0, errors.New("log record identifier has unexpected size")
			}
			pos += n
		default:
			n := skipField(data[pos:], num, wireType)
			if n < 0 {
				return 0, errors.New("failed to skip field in log record")
			}
			pos += n
		}
	}

	return result, nil
}

type anyValueKind uint8

const (
	anyValueUnset anyValueKind = iota
	anyValueString
	anyValueBool
	anyValueInt
	anyValueDouble
	anyValueArray
	anyValueKeyValueList
	anyValueBytes
	anyValueStringIndex
)

type parsedAnyValue struct {
	kind        anyValueKind
	stringValue []byte
}

const semanticParseMaxDepth = 64

var errSemanticParseDepth = errors.New("protobuf semantic parse nesting limit exceeded")

// parseKeyValue parses the complete KeyValue message using pdata's singular
// field behavior: scalar key fields and AnyValue oneofs are last-value-wins,
// while repeated Value messages merge by processing their contents in order.
func parseKeyValue(data []byte) ([]byte, []byte, parsedAnyValue, error) {
	return parseKeyValueDepth(data, 0)
}

func parseKeyValueDepth(data []byte, depth int) ([]byte, []byte, parsedAnyValue, error) {
	if depth > semanticParseMaxDepth {
		return nil, nil, parsedAnyValue{}, errSemanticParseDepth
	}
	var key []byte
	var valueRaw []byte
	var value parsedAnyValue
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, nil, parsedAnyValue{}, errors.New("malformed protobuf tag in key value")
		}
		pos += tagLen

		switch fieldNum {
		case 1: // KeyValue.key
			if wireType != protowire.BytesType {
				return nil, nil, parsedAnyValue{}, errors.New("wrong wire type for key value key")
			}
			var n int
			key, n = protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, nil, parsedAnyValue{}, errors.New("invalid bytes in key value key")
			}
			pos += n
		case 2: // KeyValue.value
			if wireType != protowire.BytesType {
				return nil, nil, parsedAnyValue{}, errors.New("wrong wire type for key value value")
			}
			var n int
			valueRaw, n = protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, nil, parsedAnyValue{}, errors.New("invalid bytes in key value value")
			}
			pos += n
			if err := parseAnyValueDepth(valueRaw, &value, depth); err != nil {
				return nil, nil, parsedAnyValue{}, err
			}
		case 3: // KeyValue.key_strindex
			if wireType != protowire.VarintType {
				return nil, nil, parsedAnyValue{}, errors.New("wrong wire type for key value key string index")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return nil, nil, parsedAnyValue{}, errors.New("invalid varint in key value key string index")
			}
			pos += n
		default:
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				return nil, nil, parsedAnyValue{}, errors.New("failed to skip field in key value")
			}
			pos += n
		}
	}

	return key, valueRaw, value, nil
}

// parseAnyValue applies the AnyValue oneof in wire order. It parses every
// field, including fields superseded by a later oneof member, so malformed
// trailing data is never hidden by an earlier string value.
func parseAnyValue(data []byte, value *parsedAnyValue) error {
	return parseAnyValueDepth(data, value, 0)
}

func parseAnyValueDepth(data []byte, value *parsedAnyValue, depth int) error {
	if depth > semanticParseMaxDepth {
		return errSemanticParseDepth
	}
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in any value")
		}
		pos += tagLen

		switch fieldNum {
		case 1: // string_value
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for any value string")
			}
			stringValue, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in any value string")
			}
			value.kind = anyValueString
			value.stringValue = stringValue
			pos += n
		case 2, 3, 8: // bool_value, int_value, string_value_strindex
			if wireType != protowire.VarintType {
				return errors.New("wrong wire type for any value varint")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return errors.New("invalid varint in any value")
			}
			switch fieldNum {
			case 2:
				value.kind = anyValueBool
			case 3:
				value.kind = anyValueInt
			case 8:
				value.kind = anyValueStringIndex
			}
			value.stringValue = nil
			pos += n
		case 4: // double_value
			if wireType != protowire.Fixed64Type {
				return errors.New("wrong wire type for any value double")
			}
			_, n := protowire.ConsumeFixed64(data[pos:])
			if n < 0 {
				return errors.New("invalid fixed64 in any value double")
			}
			value.kind = anyValueDouble
			value.stringValue = nil
			pos += n
		case 5, 6, 7: // array_value, kvlist_value, bytes_value
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for any value bytes")
			}
			message, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in any value")
			}
			if fieldNum == 5 {
				if err := validateArrayValueDepth(message, depth+1); err != nil {
					return err
				}
				value.kind = anyValueArray
			} else if fieldNum == 6 {
				if err := validateKeyValueListDepth(message, depth+1); err != nil {
					return err
				}
				value.kind = anyValueKeyValueList
			} else {
				value.kind = anyValueBytes
			}
			value.stringValue = nil
			pos += n
		default:
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				return errors.New("failed to skip field in any value")
			}
			pos += n
		}
	}
	return nil
}

func validateArrayValue(data []byte) error {
	return validateArrayValueDepth(data, 0)
}

func validateArrayValueDepth(data []byte, depth int) error {
	if depth > semanticParseMaxDepth {
		return errSemanticParseDepth
	}
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in array value")
		}
		pos += tagLen
		if fieldNum == 1 {
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for array value values")
			}
			item, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in array value values")
			}
			if err := parseAnyValueDepth(item, &parsedAnyValue{}, depth); err != nil {
				return err
			}
			pos += n
			continue
		}
		n := skipField(data[pos:], fieldNum, wireType)
		if n < 0 {
			return errors.New("failed to skip field in array value")
		}
		pos += n
	}
	return nil
}

func validateKeyValueList(data []byte) error {
	return validateKeyValueListDepth(data, 0)
}

func validateKeyValueListDepth(data []byte, depth int) error {
	if depth > semanticParseMaxDepth {
		return errSemanticParseDepth
	}
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in key value list")
		}
		pos += tagLen
		if fieldNum == 1 {
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for key value list values")
			}
			item, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in key value list values")
			}
			if _, _, _, err := parseKeyValueDepth(item, depth); err != nil {
				return err
			}
			pos += n
			continue
		}
		n := skipField(data[pos:], fieldNum, wireType)
		if n < 0 {
			return errors.New("failed to skip field in key value list")
		}
		pos += n
	}
	return nil
}

type resourceStringAttributeState struct {
	value   []byte
	found   bool
	matched bool
}

// scanResourceStringAttribute validates a complete Resource message and
// records the first matching attribute only. pdata's resource map keeps that
// first duplicate key, including when multiple singular Resource messages are
// merged by ResourceLogs.StringAttribute.
func scanResourceStringAttribute(data []byte, key string, state *resourceStringAttributeState) error {
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in resource")
		}
		pos += tagLen

		switch fieldNum {
		case 1: // Resource.attributes
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for resource attributes")
			}
			attribute, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in resource attributes")
			}
			attributeKey, _, anyValue, err := parseKeyValue(attribute)
			if err != nil {
				return err
			}
			if !state.matched && equalBytesString(attributeKey, key) {
				state.matched = true
				state.found = anyValue.kind == anyValueString
				state.value = nil
				if state.found {
					state.value = anyValue.stringValue
				}
			}
			pos += n
		case 2: // Resource.dropped_attributes_count
			if wireType != protowire.VarintType {
				return errors.New("wrong wire type for resource dropped attributes count")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return errors.New("invalid varint in resource dropped attributes count")
			}
			pos += n
		case 3: // Resource.entity_refs
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for resource entity refs")
			}
			entityRef, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in resource entity refs")
			}
			if err := validateEntityRef(entityRef); err != nil {
				return err
			}
			pos += n
		default:
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				return errors.New("failed to skip field in resource")
			}
			pos += n
		}
	}
	return nil
}

// validateEntityRef matches pdata v1.64.0's EntityRef wire contract. Its four
// known fields (schema_url, type, id_keys, and description_keys) are all
// length-delimited strings; unknown fields remain forward-compatible and are
// skipped with full group validation.
func validateEntityRef(data []byte) error {
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in entity reference")
		}
		pos += tagLen

		if fieldNum >= 1 && fieldNum <= 4 {
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for entity reference field")
			}
			_, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in entity reference field")
			}
			pos += n
			continue
		}

		n := skipField(data[pos:], fieldNum, wireType)
		if n < 0 {
			return errors.New("failed to skip field in entity reference")
		}
		pos += n
	}
	return nil
}

// equalBytesString reports whether b contains exactly s without converting b
// to a string or allocating a temporary byte slice for s.
func equalBytesString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if b[i] != s[i] {
			return false
		}
	}
	return true
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

		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	return nil, nil
}
