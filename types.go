// Package otlpwire provides utilities for working with OTLP wire format data.
package otlpwire

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
// Resource values are normally obtained from a ResourceMetrics, ResourceLogs,
// or ResourceSpans Resource method, which returns exactly one merged Resource
// per container, matching pdata's object model.
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

// KeyValue represents a single KeyValue message (raw wire bytes).
type KeyValue []byte
