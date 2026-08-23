package otlpwire

import (
	"errors"
	"io"
	"iter"

	"google.golang.org/protobuf/encoding/protowire"
)

// LogRecordCount returns the total number of log records in the batch.
func (l ExportLogsServiceRequest) LogRecordCount() (int, error) {
	return countLogRecords([]byte(l))
}

// ResourceLogs returns an iterator over ResourceLogs in the batch. The
// returned function should be called after iteration to check for errors.
// ResourceLogs is a thin adapter over ResourceLogsSeq.
func (l ExportLogsServiceRequest) ResourceLogs() (iter.Seq[ResourceLogs], func() error) {
	var iterErr error
	seq := func(yield func(ResourceLogs) bool) {
		l.ResourceLogsSeq(func(resource ResourceLogs, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(resource)
		})
	}
	return seq, func() error { return iterErr }
}

// ResourceLogsSeq is the zero-allocation alternative to ResourceLogs. It is
// shaped like iter.Seq2[ResourceLogs, error] and meant to be ranged over
// directly. On a parse error it yields a nil ResourceLogs with a non-nil error
// and stops.
func (l ExportLogsServiceRequest) ResourceLogsSeq(yield func(ResourceLogs, error) bool) {
	repeatedFieldSeq2([]byte(l), 1, yield)
}

// LogRecordCount returns the number of log records in this resource.
func (r ResourceLogs) LogRecordCount() (int, error) {
	return countInResourceLogs([]byte(r))
}

// Resource returns the Resource message for this ResourceLogs. It returns
// (nil, nil) when the field is absent, aliases the input for the single
// occurrence every real producer emits, and merges 2+ occurrences into a new
// buffer. See extractMergedMessage for the full contract.
func (r ResourceLogs) Resource() (Resource, error) {
	raw, err := extractMergedMessage([]byte(r), 1)
	if err != nil {
		return nil, err
	}
	return Resource(raw), nil
}

// SchemaUrl returns the ResourceLogs schema_url (field 3) as a view into the
// underlying buffer. Returns nil if the field is not present. Repeated
// occurrences resolve to the last one.
func (r ResourceLogs) SchemaUrl() ([]byte, error) {
	return extractLastBytesField([]byte(r), 3)
}

// WriteTo writes the ResourceLogs as a valid ExportLogsServiceRequest to w.
// Implements io.WriterTo interface.
func (r ResourceLogs) WriteTo(w io.Writer) (int64, error) {
	return writeResourceMessage(w, []byte(r))
}

// ScopeLogs returns an iterator over ScopeLogs in this ResourceLogs.
// Field 2 in the ResourceLogs protobuf message.
// The returned function should be called after iteration to check for errors.
// ScopeLogs is a thin adapter over ScopeLogsSeq.
func (r ResourceLogs) ScopeLogs() (iter.Seq[ScopeLogs], func() error) {
	var iterErr error
	seq := func(yield func(ScopeLogs) bool) {
		r.ScopeLogsSeq(func(scope ScopeLogs, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(scope)
		})
	}
	return seq, func() error { return iterErr }
}

// ScopeLogsSeq is the zero-allocation alternative to ScopeLogs. It is shaped
// like iter.Seq2[ScopeLogs, error] and meant to be ranged over directly. On a
// parse error it yields a nil ScopeLogs with a non-nil error and stops.
func (r ResourceLogs) ScopeLogsSeq(yield func(ScopeLogs, error) bool) {
	repeatedFieldSeq2([]byte(r), 2, yield)
}

// Scope returns the InstrumentationScope for this ScopeLogs. It returns
// (nil, nil) when the field is absent, aliases the input for the single
// occurrence every real producer emits, and merges 2+ occurrences into a new
// buffer. See extractMergedMessage for the full contract.
func (s ScopeLogs) Scope() (InstrumentationScope, error) {
	raw, err := extractMergedMessage([]byte(s), 1)
	if err != nil {
		return nil, err
	}
	return InstrumentationScope(raw), nil
}

// SchemaUrl returns the ScopeLogs schema_url (field 3) as a view into the
// underlying buffer. Returns nil if the field is not present. Repeated
// occurrences resolve to the last one.
func (s ScopeLogs) SchemaUrl() ([]byte, error) {
	return extractLastBytesField([]byte(s), 3)
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
	repeatedFieldSeq2([]byte(s), 2, yield)
}

// SeverityNumber returns the LogRecord severity_number enum (field 2).
// It returns 0 when the field is absent. Values are represented as int32 so
// that unexpected negative protobuf enum values remain distinguishable from
// the positive OTLP severity ranges. Use Severity to read both severity fields
// from one walk.
func (r LogRecord) SeverityNumber() (int32, error) {
	number, _, err := parseLogRecordSeverity([]byte(r), true)
	return number, err
}

// SeverityText returns the LogRecord severity_text (field 3) as a view into
// the underlying buffer. It returns nil when the field is absent and a
// non-nil zero-length slice when it is present but empty, which pdata cannot
// distinguish. Repeated occurrences resolve to the last one.
//
// Validation matches SeverityNumber: both read the same schema-aware walk of
// the whole LogRecord, so an early severity_text cannot conceal corruption
// that follows it. Use Severity to read both severity fields from one walk.
func (r LogRecord) SeverityText() ([]byte, error) {
	_, text, err := parseLogRecordSeverity([]byte(r), true)
	return text, err
}

// Severity returns severity_number (field 2) and severity_text (field 3) from
// one walk of the record. SeverityNumber and SeverityText each run that walk
// once, so a consumer reading both fields separately pays for two.
//
// Each value carries the contract its single-field accessor documents.
//
// These are the raw wire fields. Classifying them into severity bands, and
// deciding which one wins when they disagree, stay consumer policy.
func (r LogRecord) Severity() (int32, []byte, error) {
	return parseLogRecordSeverity([]byte(r), true)
}

// SeverityFields returns severity_number (field 2) and severity_text (field 3)
// while validating the complete top-level LogRecord framing. Unlike Severity,
// it does not validate the contents of body or attribute messages. Use this
// operation when those unrelated nested values are not consumed next.
//
// Repeated severity fields resolve to the last occurrence. The returned text
// has the same aliasing, nil, empty, and capacity contracts as SeverityText.
func (r LogRecord) SeverityFields() (int32, []byte, error) {
	return parseLogRecordSeverity([]byte(r), false)
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

func countInResourceLogs(data []byte) (int, error) {
	return countRepeatedField(data, 2, countInScopeLogs)
}

func countInScopeLogs(data []byte) (int, error) {
	return countOccurrences(data, 2)
}

// parseLogRecordSeverity walks every known top-level LogRecord field while
// extracting severity_number and severity_text with protobuf last-value-wins
// scalar semantics. validateNested additionally validates body and attribute
// contents for the strict severity accessors.
func parseLogRecordSeverity(data []byte, validateNested bool) (int32, []byte, error) {
	pos := 0
	var number int32
	var text []byte

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, nil, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		switch num {
		case 1, 11: // time_unix_nano, observed_time_unix_nano
			if wireType != protowire.Fixed64Type {
				return 0, nil, errors.New("wrong wire type for log record timestamp")
			}
			_, n := protowire.ConsumeFixed64(data[pos:])
			if n < 0 {
				return 0, nil, errors.New("invalid fixed64 in log record timestamp")
			}
			pos += n
		case 2: // severity_number
			if wireType != protowire.VarintType {
				return 0, nil, errors.New("wrong wire type for log record severity number")
			}
			value, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return 0, nil, errors.New("invalid varint in log record severity number")
			}
			number = int32(value)
			pos += n
		case 3, 12: // severity_text, event_name
			if wireType != protowire.BytesType {
				return 0, nil, errors.New("wrong wire type for log record string")
			}
			value, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, nil, errors.New("invalid bytes in log record string")
			}
			if num == 3 {
				// Clamped so a caller's append reallocates instead of
				// overwriting the record fields that follow this one.
				text = value[:len(value):len(value)]
			}
			pos += n
		case 5: // body
			if wireType != protowire.BytesType {
				return 0, nil, errors.New("wrong wire type for log record body")
			}
			body, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, nil, errors.New("invalid bytes in log record body")
			}
			if validateNested {
				if err := parseAnyValue(body, &parsedAnyValue{}); err != nil {
					return 0, nil, err
				}
			}
			pos += n
		case 6: // attributes
			if wireType != protowire.BytesType {
				return 0, nil, errors.New("wrong wire type for log record attributes")
			}
			attribute, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, nil, errors.New("invalid bytes in log record attributes")
			}
			if validateNested {
				if _, _, _, err := parseKeyValue(attribute); err != nil {
					return 0, nil, err
				}
			}
			pos += n
		case 7: // dropped_attributes_count
			if wireType != protowire.VarintType {
				return 0, nil, errors.New("wrong wire type for log record dropped attributes count")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return 0, nil, errors.New("invalid varint in log record dropped attributes count")
			}
			pos += n
		case 8: // flags
			if wireType != protowire.Fixed32Type {
				return 0, nil, errors.New("wrong wire type for log record flags")
			}
			_, n := protowire.ConsumeFixed32(data[pos:])
			if n < 0 {
				return 0, nil, errors.New("invalid fixed32 in log record flags")
			}
			pos += n
		case 9, 10: // trace_id, span_id
			if wireType != protowire.BytesType {
				return 0, nil, errors.New("wrong wire type for log record identifier")
			}
			identifier, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, nil, errors.New("invalid bytes in log record identifier")
			}
			expectedSize := 16
			if num == 10 {
				expectedSize = 8
			}
			if len(identifier) != 0 && len(identifier) != expectedSize {
				return 0, nil, errors.New("log record identifier has unexpected size")
			}
			pos += n
		default:
			n := skipField(data[pos:], num, wireType)
			if n < 0 {
				return 0, nil, errors.New("failed to skip field in log record")
			}
			pos += n
		}
	}

	return number, text, nil
}
