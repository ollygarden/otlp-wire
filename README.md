# otlp-wire

OTLP wire format utilities for Go. Count, shard, and route telemetry data without unmarshaling.

[![CI](https://github.com/ollygarden/otlp-wire/workflows/CI/badge.svg)](https://github.com/ollygarden/otlp-wire/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/go.olly.garden/otlp-wire.svg)](https://pkg.go.dev/go.olly.garden/otlp-wire)
[![Go Report Card](https://goreportcard.com/badge/go.olly.garden/otlp-wire)](https://goreportcard.com/report/go.olly.garden/otlp-wire)

## What It Does

- Count signals (metrics/logs/traces) without unmarshaling
- Iterate over resources with minimal allocations for parallel processing
- Extract resource metadata for routing decisions
- Access individual span fields (TraceID, SpanID, ParentSpanID) with zero allocations

## Performance Characteristics

Full protobuf unmarshaling is expensive:
- Allocates thousands of Go objects
- High garbage collector pressure
- High CPU overhead

otlp-wire operates on wire format bytes:

- 35-55x faster counting than unmarshaling (zero allocations)
- 1,100-2,800x faster iteration than unmarshal+iterate (2 allocations)
- 2,800-3,700x faster splitting than unmarshal+remarshal (2 allocations)
- Minimal GC pressure (only 24 bytes per batch for error handling)
- Zero dependencies (only stdlib + protowire)

See [BENCHMARKS.md](docs/BENCHMARKS.md) for detailed comparison.

## Use Cases

- **Observability**: Count signals for monitoring ingestion volume
- **Sharding**: Split batches by resource for parallel processing
- **Routing**: Extract resource attributes for routing decisions
- **Span Processing**: Extract trace/span IDs without full unmarshal

## Installation

```bash
go get go.olly.garden/otlp-wire
```

## Quick Start

```go
import "go.olly.garden/otlp-wire"

// Count signals for observability
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
count, err := data.DataPointCount()
if err != nil {
    return err
}
metrics.RecordDataPointsReceived(count)

// Iterate over resources for sharding
resources, getErr := data.ResourceMetrics()
for resource := range resources {
    resourceBytes, _ := resource.Resource()
    hash := fnv64a(resourceBytes)
    workerID := hash % numWorkers

    var buf bytes.Buffer
    resource.WriteTo(&buf)
    sendToWorker(workerID, buf.Bytes())
}
if err := getErr(); err != nil {
    return err
}
```

```go
// Access individual span fields without full unmarshal
wire := otlpwire.ExportTracesServiceRequest(otlpBytes)
rsIter, rsErr := wire.ResourceSpans()
for rs := range rsIter {
    ssIter, ssErr := rs.ScopeSpans()
    for ss := range ssIter {
        spanIter, spanErr := ss.Spans()
        for s := range spanIter {
            traceID, _ := s.TraceID()       // [16]byte, zero allocs
            spanID, _ := s.SpanID()          // [8]byte, zero allocs
            parentID, _ := s.ParentSpanID()  // [8]byte, zero allocs
            // ... use IDs for bloom filters, trace assembly, etc.
        }
        if err := spanErr(); err != nil { return err }
    }
    if err := ssErr(); err != nil { return err }
}
if err := rsErr(); err != nil { return err }
```

See [example_test.go](example_test.go) for complete working examples.

## API Overview

### Type Hierarchy

```
ExportMetricsServiceRequest (OTLP message bytes)
  └─ ResourceMetrics[] (one per resource)
       ├─ Resource()   (exactly one, merged)
       ├─ SchemaUrl()
       └─ ScopeMetrics[] (one per instrumentation scope)
            ├─ Scope()      (exactly one, merged)
            ├─ SchemaUrl()
            └─ Metric[] (individual metrics)
                 ├─ Name()
                 └─ DataPoint[] (one per data point, any metric type)
                      ├─ Type()          (Gauge/Sum/Histogram/ExponentialHistogram/Summary)
                      ├─ Timestamp()
                      └─ KeyValue[] (attributes)
                           ├─ Key()
                           └─ ValueRaw()

ExportLogsServiceRequest (OTLP message bytes)
  └─ ResourceLogs[] (one per resource)
       ├─ Resource()   (exactly one, merged)
       ├─ SchemaUrl()
       └─ ScopeLogs[] (one per instrumentation scope)
            ├─ Scope()      (exactly one, merged)
            ├─ SchemaUrl()
            └─ LogRecord[]
                 └─ SeverityNumber()

ExportTracesServiceRequest (OTLP message bytes)
  └─ ResourceSpans[] (one per resource)
       ├─ Resource()   (exactly one, merged)
       ├─ SchemaUrl()
       └─ ScopeSpans[] (one per instrumentation scope)
            ├─ Scope()      (exactly one, merged)
            ├─ SchemaUrl()
            └─ Span[] (individual spans)
                 ├─ TraceID()
                 ├─ SpanID()
                 └─ ParentSpanID()

InstrumentationScope (from any Scope())
  ├─ Name()
  ├─ Version()
  └─ KeyValue[] (attributes)
```

### Methods

**Batch-level operations:**
```go
type ExportMetricsServiceRequest []byte
func (m ExportMetricsServiceRequest) DataPointCount() (int, error)
func (m ExportMetricsServiceRequest) ResourceMetrics() (iter.Seq[ResourceMetrics], func() error)

type ExportLogsServiceRequest []byte
func (l ExportLogsServiceRequest) LogRecordCount() (int, error)
func (l ExportLogsServiceRequest) ResourceLogs() (iter.Seq[ResourceLogs], func() error)

type ExportTracesServiceRequest []byte
func (t ExportTracesServiceRequest) SpanCount() (int, error)
func (t ExportTracesServiceRequest) ResourceSpans() (iter.Seq[ResourceSpans], func() error)
```

**Resource-level operations:**
```go
type ResourceMetrics []byte
func (r ResourceMetrics) DataPointCount() (int, error)
func (r ResourceMetrics) Resource() (Resource, error)
func (r ResourceMetrics) SchemaUrl() ([]byte, error)
func (r ResourceMetrics) WriteTo(w io.Writer) (int64, error)

type ResourceLogs []byte
func (r ResourceLogs) LogRecordCount() (int, error)
func (r ResourceLogs) Resource() (Resource, error)
func (r ResourceLogs) SchemaUrl() ([]byte, error)
func (r ResourceLogs) WriteTo(w io.Writer) (int64, error)
func (r ResourceLogs) ScopeLogs() (iter.Seq[ScopeLogs], func() error)

type ResourceSpans []byte
func (r ResourceSpans) SpanCount() (int, error)
func (r ResourceSpans) Resource() (Resource, error)
func (r ResourceSpans) SchemaUrl() ([]byte, error)
func (r ResourceSpans) WriteTo(w io.Writer) (int64, error)
func (r ResourceSpans) ScopeSpans() (iter.Seq[ScopeSpans], func() error)
```

**Instrumentation scope (all three signals):**
```go
type InstrumentationScope []byte
func (s InstrumentationScope) Name() ([]byte, error)
func (s InstrumentationScope) Version() ([]byte, error)
func (s InstrumentationScope) Attributes() (iter.Seq[KeyValue], func() error)
func (s InstrumentationScope) AttributesSeq(yield func(KeyValue, error) bool)
```

**Scope-level operations (traces):**
```go
type ScopeSpans []byte
func (s ScopeSpans) SpanCount() (int, error)
func (s ScopeSpans) Scope() (InstrumentationScope, error)
func (s ScopeSpans) SchemaUrl() ([]byte, error)
func (s ScopeSpans) Spans() (iter.Seq[Span], func() error)
```

**Span-level field accessors:**
```go
type Span []byte
func (s Span) TraceID() ([16]byte, error)
func (s Span) SpanID() ([8]byte, error)
func (s Span) ParentSpanID() ([8]byte, error)
```

**Scope- and metric-level operations (metrics depth):**
```go
type ScopeMetrics []byte
func (s ScopeMetrics) Scope() (InstrumentationScope, error)
func (s ScopeMetrics) SchemaUrl() ([]byte, error)
func (s ScopeMetrics) Metrics() (iter.Seq[Metric], func() error)

type Metric []byte
func (m Metric) Name() ([]byte, error)
func (m Metric) Metadata() (iter.Seq[KeyValue], func() error)        // ergonomic, 2 allocs per open
func (m Metric) MetadataSeq(yield func(KeyValue, error) bool)        // zero-alloc, range directly
func (m Metric) DataPoints() (iter.Seq[DataPoint], func() error)     // ergonomic, 2 allocs per open
func (m Metric) DataPointsSeq(yield func(DataPoint, error) bool)     // zero-alloc, range directly

type DataPoint struct{ /* unexported */ }
func (d DataPoint) Raw() []byte
func (d DataPoint) Type() MetricType
func (d DataPoint) Timestamp() (uint64, error)
func (d DataPoint) Attributes() (iter.Seq[KeyValue], func() error)   // ergonomic, 2 allocs per open
func (d DataPoint) AttributesSeq(yield func(KeyValue, error) bool)   // zero-alloc, range directly

type KeyValue []byte
func (kv KeyValue) Key() ([]byte, error)
func (kv KeyValue) ValueRaw() ([]byte, error)
func (kv KeyValue) StringValue() ([]byte, bool, error)

type MetricType int
const (
	MetricTypeGauge                MetricType = 5
	MetricTypeSum                  MetricType = 7
	MetricTypeHistogram            MetricType = 9
	MetricTypeExponentialHistogram MetricType = 10
	MetricTypeSummary              MetricType = 11
)
```

**Log-level operations and resource attributes:**
```go
type ScopeLogs []byte
func (s ScopeLogs) Scope() (InstrumentationScope, error)
func (s ScopeLogs) SchemaUrl() ([]byte, error)
func (s ScopeLogs) LogRecords() (iter.Seq[LogRecord], func() error) // ergonomic, 2 allocs per open
func (s ScopeLogs) LogRecordsSeq(yield func(LogRecord, error) bool) // zero-alloc, range directly

type LogRecord []byte
func (r LogRecord) SeverityNumber() (int32, error)

type Resource []byte
func (r Resource) Attributes() (iter.Seq[KeyValue], func() error)
func (r Resource) AttributesSeq(yield func(KeyValue, error) bool)
func (r Resource) StringAttribute(key string) ([]byte, bool, error)
```

`Resource.StringAttribute` is zero-copy and returns a separate `found` value,
so a missing resource attribute can be distinguished from a present empty
string. It inspects one Resource message obtained from a `ResourceMetrics`,
`ResourceLogs`, or `ResourceSpans` `Resource()` method.

Each container type — `ResourceMetrics`, `ResourceLogs`, `ResourceSpans` — has
exactly one `Resource`, matching pdata's object model. `Resource()` returns
`(nil, nil)` when the field is absent (OTLP declares it optional), aliases the
input with no copy for the single occurrence every real producer emits, and
merges 2+ occurrences by concatenating their encoded bodies into a new buffer
— byte-equivalent to protobuf's merge for singular message fields. To read a
resource string attribute starting from a `ResourceLogs`/`ResourceMetrics`/`ResourceSpans`,
call `Resource()` then `Resource.StringAttribute`:

```go
resource, err := resourceLogs.Resource()
if err != nil {
    return err
}
value, found, err := resource.StringAttribute("service.name")
```

Scope containers work the same way: `ScopeMetrics`, `ScopeLogs`, and
`ScopeSpans` each have exactly one `InstrumentationScope`, reached with
`Scope()`. It is absence-tolerant, zero-copy for the single occurrence real
producers emit, and merges 2+ occurrences, exactly as `Resource()` does. Note
that scope attributes are field 3 of `InstrumentationScope`, whereas resource
attributes are field 1 — the accessors hide that difference:

```go
scope, err := scopeLogs.Scope()
if err != nil {
    return err
}
name, err := scope.Name()
```

**Singular fields resolve two different ways**, matching protobuf and pdata.
Fields OTLP declares `repeated` — the attribute fields and `Metric.Metadata` —
yield every occurrence in wire order instead. Singular *messages*
(`Resource()`, `Scope()`) merge, while singular *scalars* (`SchemaUrl()`,
`InstrumentationScope.Name()` and `Version()`, `Metric.Name()`)
resolve to the last occurrence. `KeyValue.Key` and `KeyValue.ValueRaw` are
deliberately exempt and stay first-match for hashing hot paths.
[docs/DESIGN.md](docs/DESIGN.md) explains why.

`KeyValue.ValueRaw` remains a lightweight view of the first encoded AnyValue
field for hashing-oriented hot paths. `KeyValue.StringValue` fully parses
AnyValue and follows protobuf oneof behavior, so a later non-string oneof
member makes `StringValue` report `found=false` even if an earlier encoded
member was a string.

`DataPoint` carries its `MetricType` because the attribute field number differs
per data point wire type (histograms and exponential histograms encode
attributes on a different field than gauges, sums, and summaries) — `Attributes()`
and `AttributesSeq()` use `Type()` internally to pick the right field.

Every level in this chain has both a closure-based iterator
(`Metrics()`, `DataPoints()`, `Attributes()` — return `(iter.Seq[T], func() error)`,
2 allocations per call to open) and, at the hottest, deepest levels
(`Metric.MetadataSeq`, `Metric.DataPointsSeq`, `DataPoint.AttributesSeq`), a
zero-allocation `iter.Seq2`-style variant you range over directly, e.g.
`for dp, err := range m.DataPointsSeq { ... }`.
Prefer the closure-based API for ordinary code — it reads naturally and the error
check is explicit. Reach for the `Seq` variants on a per-element hot path, such as
hashing every data point's attributes across thousands of metrics per scrape,
where the allocations from opening a closure-based iterator per metric add up.

## Design Philosophy

This library provides:
- Raw bytes at different granularity levels
- Methods to count, iterate, and extract
- Building blocks for custom use cases

This library does not:
- Force specific hash algorithms
- Make routing decisions
- Unmarshal unless absolutely necessary

## Performance

Benchmarks on Apple M4 (5 resources, 100 signals per resource):

### Counting Performance

| Operation | Wire Format | Unmarshal | Speedup |
|-----------|-------------|-----------|---------|
| DataPointCount() | 2.3 μs, 0 allocs | 81.0 μs, 5,161 allocs | 35x |
| SpanCount() | 2.1 μs, 0 allocs | 115.3 μs, 5,131 allocs | 55x |
| LogRecordCount() | 2.2 μs, 0 allocs | 108.9 μs, 6,131 allocs | 49x |

### Iteration Performance

| Operation | Wire Format | Unmarshal | Speedup |
|-----------|-------------|-----------|---------|
| ResourceMetrics() | 56 ns, 2 allocs | 158 μs, 5,161 allocs | 2,800x |
| ResourceSpans() | 61 ns, 2 allocs | 100 μs, 5,131 allocs | 1,650x |
| ResourceLogs() | 93 ns, 2 allocs | 106 μs, 6,131 allocs | 1,140x |

### Split Performance (Iterate + WriteTo)

| Operation | Wire Format | Unmarshal+Remarshal | Speedup |
|-----------|-------------|---------------------|---------|
| Metrics | 50 ns, 2 allocs | 143 μs, 7,742 allocs | 2,860x |
| Traces | 51 ns, 2 allocs | 192 μs, 7,192 allocs | 3,750x |
| Logs | 51 ns, 2 allocs | 178 μs, 8,692 allocs | 3,490x |

**Note:** The 2 allocations (24 bytes) in iteration are from the iterator error handling pattern (closure capture mechanism).

### Deep Iteration Performance (metrics depth)

Walking `ResourceMetrics.ScopeMetrics()` → `ScopeMetrics.Metrics()` → `Metric.DataPoints()` →
`DataPoint.Attributes()` all the way down to individual attribute key/value bytes, compared
against a full pdata unmarshal doing the equivalent walk (median of 5 runs, Apple M4):

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `BenchmarkMetrics_ScrapeDeepIteration_WireFormat` | 827,544 | 460,987 | 19,207 |
| `BenchmarkMetrics_ScrapeDeepIterationSeq_WireFormat` | 634,474 | 184 | 7 |
| `BenchmarkMetrics_ScrapeDeepIteration_Unmarshal` | 2,268,745 | 3,507,250 | 105,631 |
| `BenchmarkMetrics_DeepIteration_WireFormat` | 42,548 | 20,912 | 1,033 |
| `BenchmarkMetrics_DeepIteration_Unmarshal` | 87,263 | 159,361 | 5,161 |

Speedup (wire format vs. unmarshal, by ns/op):

| Fixture | Speedup |
|---|---|
| Scrape-shaped, closure-based (4,800 metrics × 1 dp × 4 attrs) | 2.74x |
| Scrape-shaped, Seq variants (4,800 metrics × 1 dp × 4 attrs) | 3.58x |
| Continuity (5 × 1 × 1 × 100 dp) | 2.05x |

The Seq variants cut allocations from thousands per batch to 7 — this is the API to
reach for when hashing or otherwise touching every data point's attributes across a
large scrape.

For detailed benchmarks and methodology, see [BENCHMARKS.md](docs/BENCHMARKS.md).

## Documentation

- **[specification.md](docs/specification.md)** - Product boundary, compatibility contracts, consumers, and rollout gates
- **[DESIGN.md](docs/DESIGN.md)** - Architecture, design decisions, and implementation details
- **[BENCHMARKS.md](docs/BENCHMARKS.md)** - Performance comparison and methodology
- **[example_test.go](example_test.go)** - Complete working examples (observability metrics, sharding, sampling)
- **[AGENTS.md](AGENTS.md)** - Repository map, parser guardrails, and validation matrix
- **[CONTRIBUTING.md](CONTRIBUTING.md)** - Contribution and pull-request expectations

## Requirements

- Go 1.25+

## License

[Apache License 2.0](LICENSE)

## Related Projects

- [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) - Full-featured OTLP processing
- [protowire](https://pkg.go.dev/google.golang.org/protobuf/encoding/protowire) - Low-level protobuf wire format utilities
