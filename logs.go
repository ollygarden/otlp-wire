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

// ResourceLogs returns an iterator over ResourceLogs in the batch.
// The returned function should be called after iteration to check for errors.
func (l ExportLogsServiceRequest) ResourceLogs() (iter.Seq[ResourceLogs], func() error) {
	return repeatedFieldSeq[ResourceLogs]([]byte(l), 1)
}

// LogRecordCount returns the number of log records in this resource.
func (r ResourceLogs) LogRecordCount() (int, error) {
	return countInResourceLogs([]byte(r))
}

// Resource returns the Resource message for this ResourceLogs. It returns
// (nil, nil) when the field is absent, aliases the input for the single
// occurrence every real producer emits, and merges 2+ occurrences into a new
// buffer. See extractResourceMessage for the full contract.
func (r ResourceLogs) Resource() (Resource, error) {
	raw, err := extractResourceMessage([]byte(r))
	if err != nil {
		return nil, err
	}
	return Resource(raw), nil
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
	return repeatedFieldSeq[ScopeLogs]([]byte(r), 2)
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
