// Package otlpwire provides fast, zero-allocation utilities for working with
// OTLP (OpenTelemetry Protocol) wire format data.
//
// This package operates directly on protobuf wire format bytes, enabling operations
// like signal counting and batch splitting without the overhead of full unmarshaling.
//
// # Basic Usage
//
//	// Count signals in a batch
//	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
//	count := data.DataPointCount()
//
//	// Split batch by resource for sharding
//	for _, resource := range data.SplitByResource() {
//	    hash := fnv64a(resource.Resource())
//	    sendToWorker(hash % numWorkers, resource.AsExportRequest())
//	}
//
// For more details, see the DESIGN.md and example_test.go.
package otlpwire

import (
	"errors"

	"google.golang.org/protobuf/encoding/protowire"
)

// ExportMetricsServiceRequest represents an OTLP ExportMetricsServiceRequest message.
// Use this type to count metric data points or split batches by resource.
type ExportMetricsServiceRequest []byte

// ExportLogsServiceRequest represents an OTLP ExportLogsServiceRequest message.
// Use this type to count log records or split batches by resource.
type ExportLogsServiceRequest []byte

// ExportTracesServiceRequest represents an OTLP ExportTracesServiceRequest message.
// Use this type to count spans or split batches by resource.
type ExportTracesServiceRequest []byte

// ResourceMetrics represents a single ResourceMetrics message extracted from
// an OTLP batch. Use Resource() to get resource attributes, or AsExportRequest()
// to wrap it back into a valid OTLP message.
type ResourceMetrics []byte

// ResourceLogs represents a single ResourceLogs message extracted from
// an OTLP batch. Use Resource() to get resource attributes, or AsExportRequest()
// to wrap it back into a valid OTLP message.
type ResourceLogs []byte

// ResourceSpans represents a single ResourceSpans message extracted from
// an OTLP batch. Use Resource() to get resource attributes, or AsExportRequest()
// to wrap it back into a valid OTLP message.
type ResourceSpans []byte

// DataPointCount returns the total number of metric data points in the batch.
// Counts without unmarshaling by parsing the protobuf wire format.
//
// Use case: Rate limiting entire batch
//
// Example:
//
//	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
//	if data.DataPointCount() > limit {
//	    return errors.New("rate limit exceeded")
//	}
func (m ExportMetricsServiceRequest) DataPointCount() int {
	count, _ := countMetricDataPoints([]byte(m))
	return count
}

// SplitByResource splits the batch into separate ResourceMetrics, one per resource.
// Each ResourceMetrics can be independently routed, counted, or processed.
//
// Use case: Sharding batches across workers, per-service routing
//
// Example:
//
//	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
//	for _, resource := range data.SplitByResource() {
//	    hash := fnv64a(resource.Resource())
//	    sendToWorker(hash % numWorkers, resource.AsExportRequest())
//	}
func (m ExportMetricsServiceRequest) SplitByResource() []ResourceMetrics {
	resourceBytes, err := extractResourceMetrics([]byte(m))
	if err != nil {
		return nil
	}

	result := make([]ResourceMetrics, len(resourceBytes))
	for i, rb := range resourceBytes {
		result[i] = ResourceMetrics(rb)
	}
	return result
}

// Resource returns the raw Resource message bytes.
// The Resource contains attributes like service.name, host.name, etc.
//
// Use case: Hash for routing, unmarshal for attribute filtering
//
// Example:
//
//	resourceBytes := rm.Resource()
//	hash := xxhash.Sum64(resourceBytes)  // Hash for consistent routing
func (r ResourceMetrics) Resource() []byte {
	resourceBytes, _ := extractResourceFromResourceMetrics([]byte(r))
	return resourceBytes
}

// AsExportRequest wraps the ResourceMetrics into a valid ExportMetricsServiceRequest.
// The returned bytes can be sent to OTLP endpoints or cast back to MetricsData.
//
// Use case: Send to worker, count signals in this resource
//
// Example:
//
//	exportBytes := rm.AsExportRequest()
//
//	// Send to OTLP endpoint
//	sendToEndpoint(exportBytes)
//
//	// Or count signals in this resource
//	count := otlpwire.ExportMetricsServiceRequest(exportBytes).DataPointCount()
func (r ResourceMetrics) AsExportRequest() []byte {
	return wrapResourceMetrics([]byte(r))
}

// LogRecordCount returns the total number of log records in the batch.
// Counts without unmarshaling by parsing the protobuf wire format.
//
// Use case: Rate limiting entire batch
func (l ExportLogsServiceRequest) LogRecordCount() int {
	count, _ := countLogRecords([]byte(l))
	return count
}

// SplitByResource splits the batch into separate ResourceLogs, one per resource.
// Each ResourceLogs can be independently routed, counted, or processed.
//
// Use case: Sharding batches across workers, per-service routing
func (l ExportLogsServiceRequest) SplitByResource() []ResourceLogs {
	resourceBytes, err := extractResourceLogs([]byte(l))
	if err != nil {
		return nil
	}

	result := make([]ResourceLogs, len(resourceBytes))
	for i, rb := range resourceBytes {
		result[i] = ResourceLogs(rb)
	}
	return result
}

// Resource returns the raw Resource message bytes.
// The Resource contains attributes like service.name, host.name, etc.
//
// Use case: Hash for routing, unmarshal for attribute filtering
func (r ResourceLogs) Resource() []byte {
	resourceBytes, _ := extractResourceFromResourceLogs([]byte(r))
	return resourceBytes
}

// AsExportRequest wraps the ResourceLogs into a valid ExportLogsServiceRequest.
// The returned bytes can be sent to OTLP endpoints or cast back to LogsData.
//
// Use case: Send to worker, count signals in this resource
func (r ResourceLogs) AsExportRequest() []byte {
	return wrapResourceLogs([]byte(r))
}

// SpanCount returns the total number of spans in the batch.
// Counts without unmarshaling by parsing the protobuf wire format.
//
// Use case: Rate limiting entire batch
func (t ExportTracesServiceRequest) SpanCount() int {
	count, _ := countSpans([]byte(t))
	return count
}

// SplitByResource splits the batch into separate ResourceSpans, one per resource.
// Each ResourceSpans can be independently routed, counted, or processed.
//
// Use case: Sharding batches across workers, per-service routing
func (t ExportTracesServiceRequest) SplitByResource() []ResourceSpans {
	resourceBytes, err := extractResourceSpans([]byte(t))
	if err != nil {
		return nil
	}

	result := make([]ResourceSpans, len(resourceBytes))
	for i, rb := range resourceBytes {
		result[i] = ResourceSpans(rb)
	}
	return result
}

// Resource returns the raw Resource message bytes.
// The Resource contains attributes like service.name, host.name, etc.
//
// Use case: Hash for routing, unmarshal for attribute filtering
func (r ResourceSpans) Resource() []byte {
	resourceBytes, _ := extractResourceFromResourceSpans([]byte(r))
	return resourceBytes
}

// AsExportRequest wraps the ResourceSpans into a valid ExportTracesServiceRequest.
// The returned bytes can be sent to OTLP endpoints or cast back to TracesData.
//
// Use case: Send to worker, count signals in this resource
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

// extractResourceMetrics extracts all ResourceMetrics messages from an
// ExportMetricsServiceRequest as raw byte slices.
func extractResourceMetrics(data []byte) ([][]byte, error) {
	var resourceMetricsBytes [][]byte
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag in ExportMetricsServiceRequest")
		}
		pos += tagLen

		// Field 1 = ResourceMetrics (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in ResourceMetrics")
			}
			resourceMetricsBytes = append(resourceMetricsBytes, msgBytes)
			pos += n
		} else {
			n := skipField(data[pos:], wireType)
			if n < 0 {
				return nil, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return resourceMetricsBytes, nil
}

// extractResourceLogs extracts all ResourceLogs messages from an
// ExportLogsServiceRequest as raw byte slices.
func extractResourceLogs(data []byte) ([][]byte, error) {
	var resourceLogsBytes [][]byte
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag in ExportLogsServiceRequest")
		}
		pos += tagLen

		// Field 1 = ResourceLogs (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in ResourceLogs")
			}
			resourceLogsBytes = append(resourceLogsBytes, msgBytes)
			pos += n
		} else {
			n := skipField(data[pos:], wireType)
			if n < 0 {
				return nil, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return resourceLogsBytes, nil
}

// extractResourceSpans extracts all ResourceSpans messages from an
// ExportTracesServiceRequest as raw byte slices.
func extractResourceSpans(data []byte) ([][]byte, error) {
	var resourceSpansBytes [][]byte
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag in ExportTracesServiceRequest")
		}
		pos += tagLen

		// Field 1 = ResourceSpans (repeated message)
		if fieldNum == 1 && wireType == protowire.BytesType {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in ResourceSpans")
			}
			resourceSpansBytes = append(resourceSpansBytes, msgBytes)
			pos += n
		} else {
			n := skipField(data[pos:], wireType)
			if n < 0 {
				return nil, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return resourceSpansBytes, nil
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
