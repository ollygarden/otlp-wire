// Package otlpwire provides utilities for working with OTLP wire format data.
package otlpwire

import (
	"errors"
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

// Resource returns the raw Resource message bytes.
func (r ResourceMetrics) Resource() []byte {
	resourceBytes, _ := extractResourceFromResourceMetrics([]byte(r))
	return resourceBytes
}

// AsExportRequest wraps the ResourceMetrics into a valid ExportMetricsServiceRequest.
func (r ResourceMetrics) AsExportRequest() []byte {
	return wrapResourceMetrics([]byte(r))
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

// Resource returns the raw Resource message bytes.
func (r ResourceLogs) Resource() []byte {
	resourceBytes, _ := extractResourceFromResourceLogs([]byte(r))
	return resourceBytes
}

// AsExportRequest wraps the ResourceLogs into a valid ExportLogsServiceRequest.
func (r ResourceLogs) AsExportRequest() []byte {
	return wrapResourceLogs([]byte(r))
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

// Resource returns the raw Resource message bytes.
func (r ResourceSpans) Resource() []byte {
	resourceBytes, _ := extractResourceFromResourceSpans([]byte(r))
	return resourceBytes
}

// AsExportRequest wraps the ResourceSpans into a valid ExportTracesServiceRequest.
func (r ResourceSpans) AsExportRequest() []byte {
	return wrapResourceSpans([]byte(r))
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
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ExportMetricsServiceRequest")
		}
		pos += tagLen

		// Field 1 = ResourceMetrics (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in ResourceMetrics")
			}
			pos += n

			c, err := countInResourceMetrics(msgBytes)
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
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ExportLogsServiceRequest")
		}
		pos += tagLen

		// Field 1 = ResourceLogs (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in ResourceLogs")
			}
			pos += n

			c, err := countInResourceLogs(msgBytes)
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
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ExportTracesServiceRequest")
		}
		pos += tagLen

		// Field 1 = ResourceSpans (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in ResourceSpans")
			}
			pos += n

			c, err := countInResourceSpans(msgBytes)
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

func countInResourceMetrics(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ResourceMetrics")
		}
		pos += tagLen

		// Field 2 = ScopeMetrics (repeated message)
		if fieldNum == 2 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in ScopeMetrics")
			}
			pos += n

			c, err := countInScopeMetrics(msgBytes)
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

func countInScopeMetrics(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ScopeMetrics")
		}
		pos += tagLen

		// Field 2 = Metrics (repeated message)
		if fieldNum == 2 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in Metrics")
			}
			pos += n

			c, err := countInMetric(msgBytes)
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

func countInMetric(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in Metric")
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
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in metric data points")
		}
		pos += tagLen

		// Field 1 = DataPoints (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in DataPoints")
			}
			pos += n

			count++      // Each occurrence of field 1 is one data point
			_ = msgBytes // Skip the data point content
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

func countInResourceLogs(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ResourceLogs")
		}
		pos += tagLen

		// Field 2 = ScopeLogs (repeated message)
		if fieldNum == 2 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in ScopeLogs")
			}
			pos += n

			c, err := countInScopeLogs(msgBytes)
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

func countInScopeLogs(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ScopeLogs")
		}
		pos += tagLen

		// Field 2 = LogRecords (repeated message)
		if fieldNum == 2 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in LogRecords")
			}
			pos += n

			count++      // Each occurrence is one log record
			_ = msgBytes // Skip the log record content
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

func countInResourceSpans(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ResourceSpans")
		}
		pos += tagLen

		// Field 2 = ScopeSpans (repeated message)
		if fieldNum == 2 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in ScopeSpans")
			}
			pos += n

			c, err := countInScopeSpans(msgBytes)
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

func countInScopeSpans(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in ScopeSpans")
		}
		pos += tagLen

		// Field 2 = Spans (repeated message)
		if fieldNum == 2 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in Spans")
			}
			pos += n

			count++      // Each occurrence is one span
			_ = msgBytes // Skip the span content
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

// forEachResourceMetrics iterates over ResourceMetrics messages, calling fn for each.
// The callback receives resource bytes or an error. Return false to stop iteration.
func forEachResourceMetrics(data []byte, fn func([]byte, error) bool) {
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			fn(nil, errors.New("malformed protobuf tag in ExportMetricsServiceRequest"))
			return
		}
		pos += tagLen

		// Field 1 = ResourceMetrics (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				fn(nil, errors.New("invalid bytes in ResourceMetrics"))
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

// forEachResourceLogs iterates over ResourceLogs messages, calling fn for each.
// The callback receives resource bytes or an error. Return false to stop iteration.
func forEachResourceLogs(data []byte, fn func([]byte, error) bool) {
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			fn(nil, errors.New("malformed protobuf tag in ExportLogsServiceRequest"))
			return
		}
		pos += tagLen

		// Field 1 = ResourceLogs (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				fn(nil, errors.New("invalid bytes in ResourceLogs"))
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

// forEachResourceSpans iterates over ResourceSpans messages, calling fn for each.
// The callback receives resource bytes or an error. Return false to stop iteration.
func forEachResourceSpans(data []byte, fn func([]byte, error) bool) {
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			fn(nil, errors.New("malformed protobuf tag in ExportTracesServiceRequest"))
			return
		}
		pos += tagLen

		// Field 1 = ResourceSpans (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				fn(nil, errors.New("invalid bytes in ResourceSpans"))
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

// wrapResourceMetrics wraps a ResourceMetrics message bytes into a new
// ExportMetricsServiceRequest protobuf message.
//
// Wire format structure:
//
//	ExportMetricsServiceRequest
//	  └─ field 1: ResourceMetrics (message)
func wrapResourceMetrics(resourceMetricsBytes []byte) []byte {
	// Calculate total size needed
	tagSize := protowire.SizeTag(1) // field 1, wire type 2
	lengthSize := protowire.SizeBytes(len(resourceMetricsBytes))
	totalSize := tagSize + lengthSize

	// Allocate buffer
	buf := make([]byte, 0, totalSize)

	// Encode: field 1 (ResourceMetrics), wire type 2 (length-delimited)
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)

	// Encode the ResourceMetrics bytes
	buf = protowire.AppendBytes(buf, resourceMetricsBytes)

	return buf
}

// wrapResourceLogs wraps a ResourceLogs message bytes into a new
// ExportLogsServiceRequest protobuf message.
func wrapResourceLogs(resourceLogsBytes []byte) []byte {
	// Calculate total size needed
	tagSize := protowire.SizeTag(1)
	lengthSize := protowire.SizeBytes(len(resourceLogsBytes))
	totalSize := tagSize + lengthSize

	// Allocate buffer
	buf := make([]byte, 0, totalSize)

	// Encode: field 1 (ResourceLogs), wire type 2 (length-delimited)
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)

	// Encode the ResourceLogs bytes
	buf = protowire.AppendBytes(buf, resourceLogsBytes)

	return buf
}

// wrapResourceSpans wraps a ResourceSpans message bytes into a new
// ExportTracesServiceRequest protobuf message.
func wrapResourceSpans(resourceSpansBytes []byte) []byte {
	// Calculate total size needed
	tagSize := protowire.SizeTag(1)
	lengthSize := protowire.SizeBytes(len(resourceSpansBytes))
	totalSize := tagSize + lengthSize

	// Allocate buffer
	buf := make([]byte, 0, totalSize)

	// Encode: field 1 (ResourceSpans), wire type 2 (length-delimited)
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)

	// Encode the ResourceSpans bytes
	buf = protowire.AppendBytes(buf, resourceSpansBytes)

	return buf
}

// extractResourceFromResourceMetrics extracts the Resource message bytes
// from a ResourceMetrics message. The Resource is field 1.
//
// Wire format structure:
//
//	ResourceMetrics
//	  └─ field 1: Resource (message)
func extractResourceFromResourceMetrics(data []byte) ([]byte, error) {
	return extractResourceMessage(data, "ResourceMetrics")
}

// extractResourceFromResourceLogs extracts the Resource message bytes
// from a ResourceLogs message. The Resource is field 1.
func extractResourceFromResourceLogs(data []byte) ([]byte, error) {
	return extractResourceMessage(data, "ResourceLogs")
}

// extractResourceFromResourceSpans extracts the Resource message bytes
// from a ResourceSpans message. The Resource is field 1.
func extractResourceFromResourceSpans(data []byte) ([]byte, error) {
	return extractResourceMessage(data, "ResourceSpans")
}

// extractResourceMessage extracts the Resource message (field 1) from
// ResourceMetrics/ResourceLogs/ResourceSpans messages.
func extractResourceMessage(data []byte, messageType string) ([]byte, error) {
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag in " + messageType)
		}
		pos += tagLen

		// Field 1 = Resource (message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in Resource")
			}
			return msgBytes, nil
		}

		// Skip other fields
		n := skipField(data[pos:], wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field in " + messageType)
		}
		pos += n
	}

	// Resource field not found - return empty bytes (resource might be optional)
	return []byte{}, nil
}
