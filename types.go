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

// InstrumentationScope represents an InstrumentationScope message (raw wire
// bytes).
//
// InstrumentationScope values are normally obtained from a ScopeMetrics,
// ScopeLogs, or ScopeSpans Scope method, which returns exactly one merged
// scope per container, matching pdata's object model.
type InstrumentationScope []byte

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

// AnyValueType identifies the selected OTLP AnyValue oneof member.
type AnyValueType uint8

const (
	AnyValueEmpty AnyValueType = iota
	AnyValueString
	AnyValueBool
	AnyValueInt
	AnyValueDouble
	AnyValueArray
	AnyValueKeyValueList
	AnyValueBytes
)

// AnyValue is a fully validated semantic view of an OTLP AnyValue. Byte
// fields and nested message views alias the input. Float64Bits preserves the
// exact IEEE-754 representation.
type AnyValue struct {
	Type        AnyValueType
	String      []byte
	Bool        bool
	Int         int64
	Float64Bits uint64
	Bytes       []byte
	array       []byte
	kvlist      []byte
	depth       int
}

// SemanticKeyValue is a fully validated KeyValue with protobuf singular-field
// resolution. Duplicate entries in a containing list remain in wire order.
type SemanticKeyValue struct {
	Key   []byte
	Value AnyValue
}

// SemanticMetric is the protobuf-resolved semantic header and selected body.
// Header byte views alias the source Metric. Repeated occurrences of the same
// selected oneof message may require an allocated merged body.
type SemanticMetric struct {
	Name, Description, Unit []byte
	Kind                    MetricType
	AggregationTemporality  int32
	Monotonic               bool
	body                    []byte
}

// NumberValueType identifies a NumberDataPoint oneof value.
type NumberValueType uint8

const (
	NumberValueEmpty NumberValueType = iota
	NumberValueInt
	NumberValueDouble
)

// OptionalFloat64 preserves both protobuf optional presence and exact bits.
type OptionalFloat64 struct {
	Present bool
	Bits    uint64
}

// ExponentialHistogramBuckets is one side of an exponential histogram.
type ExponentialHistogramBuckets struct {
	Offset       int32
	BucketCounts []uint64
}

// QuantileValue is an ordered summary quantile/value pair, in exact bits.
type QuantileValue struct{ QuantileBits, ValueBits uint64 }

// SemanticDataPoint contains the fields consumed by semantic metric
// processors. Fields not applicable to Kind remain zero. Repeated primitive
// slices are newly allocated; attributes and nested byte values alias input.
type SemanticDataPoint struct {
	Kind                      MetricType
	Attributes                []SemanticKeyValue
	StartTimestamp, Timestamp uint64
	Flags                     uint32
	NumberType                NumberValueType
	NumberInt                 int64
	NumberDoubleBits          uint64
	Count                     uint64
	Sum, Min, Max             OptionalFloat64
	ExplicitBoundsBits        []uint64
	BucketCounts              []uint64
	Scale                     int32
	ZeroThresholdBits         uint64
	ZeroCount                 uint64
	Positive, Negative        ExponentialHistogramBuckets
	SummarySumBits            uint64
	Quantiles                 []QuantileValue
}
