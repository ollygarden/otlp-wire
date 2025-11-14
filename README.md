# otlp-wire

Fast OTLP wire format utilities for Go, designed to improve telemetry pipelines for sharding and metadata extraction.

[![Go Reference](https://pkg.go.dev/badge/go.olly.garden/otlp-wire.svg)](https://pkg.go.dev/go.olly.garden/otlp-wire)
[![Go Report Card](https://goreportcard.com/badge/go.olly.garden/otlp-wire)](https://goreportcard.com/report/go.olly.garden/otlp-wire)

## What It Does

- **Count signals** (metrics/logs/traces) without unmarshaling
- **Split batches** by resource for parallel processing and sharding
- **Extract metadata** for routing decisions with minimal overhead

## Why Use This

When processing high-volume OTLP data, full protobuf unmarshaling is expensive.
otlp-wire operates directly on wire format bytes, providing:

- 🚀 **35-52x faster** counting than unmarshaling (zero allocations)
- 🧮 **~1000x faster** splitting than unmarshal+remarshal
- 🔧 **Simple API** - Type-based design that composes naturally
- 📦 **Zero dependencies** - Only stdlib + protowire

See [BENCHMARKS.md](BENCHMARKS.md) for detailed comparison.

## Perfect For

- Rate limiting OTLP ingestion
- Sharding batches across workers
- Per-service/tenant routing
- High-throughput telemetry pipelines

## Installation

```bash
go get go.olly.garden/otlp-wire
```

## Quick Start

```go
import "go.olly.garden/otlp-wire"

// Count signals for rate limiting
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
count := data.DataPointCount()
if count > limit {
    return errors.New("rate limit exceeded")
}

// Split batches for sharding
for _, resource := range data.SplitByResource() {
    hash := fnv64a(resource.Resource())
    workerID := hash % numWorkers
    sendToWorker(workerID, resource.AsExportRequest())
}
```

See [example_test.go](example_test.go) for complete working examples.

## API Overview

### Type Hierarchy

```
ExportMetricsServiceRequest (OTLP message bytes)
  └─ ResourceMetrics[] (one per resource)

ExportLogsServiceRequest (OTLP message bytes)
  └─ ResourceLogs[] (one per resource)

ExportTracesServiceRequest (OTLP message bytes)
  └─ ResourceSpans[] (one per resource)
```

### Methods

**Batch-level operations:**
```go
type ExportMetricsServiceRequest []byte
func (m ExportMetricsServiceRequest) DataPointCount() int
func (m ExportMetricsServiceRequest) SplitByResource() []ResourceMetrics

type ExportLogsServiceRequest []byte
func (l ExportLogsServiceRequest) LogRecordCount() int
func (l ExportLogsServiceRequest) SplitByResource() []ResourceLogs

type ExportTracesServiceRequest []byte
func (t ExportTracesServiceRequest) SpanCount() int
func (t ExportTracesServiceRequest) SplitByResource() []ResourceSpans
```

**Resource-level operations:**
```go
type ResourceMetrics []byte
func (r ResourceMetrics) Resource() []byte
func (r ResourceMetrics) AsExportRequest() []byte

// Same pattern for ResourceLogs and ResourceSpans
```

## Design Philosophy

**"Provide raw bytes and tools. Users decide what to do with them."**

We give you:
- Raw bytes at different granularity levels
- Methods to count, split, and extract
- Building blocks for your use case

We don't:
- Force hash algorithms on you
- Decide when/how to route
- Unmarshal unless absolutely necessary

## Performance

Benchmarks on Apple M4 (5 resources, 100 data points each):

### Wire Format Performance

| Operation | Time | Allocations |
|-----------|------|-------------|
| DataPointCount() | ~2μs | 0 allocs |
| SplitByResource() | ~140ns | 5 allocs |
| AsExportRequest() | ~160ns | 1 alloc |

### Comparison vs Unmarshal

| Operation | Wire Format | Unmarshal | Speedup |
|-----------|-------------|-----------|---------|
| Count Metrics | 2.2 μs | 77 μs | **35x faster** |
| Count Traces | 1.9 μs | 99 μs | **52x faster** |
| Count Logs | 2.1 μs | 100 μs | **48x faster** |
| Split Metrics | 134 ns | 133 μs | **~1000x faster** |
| Split Traces | 136 ns | 169 μs | **~1200x faster** |
| Split Logs | 134 ns | 175 μs | **~1300x faster** |

**Key advantage:** No unmarshaling required - works directly on protobuf wire format.

For detailed benchmarks and real-world impact analysis, see [BENCHMARKS.md](BENCHMARKS.md).

## Documentation

- **[DESIGN.md](DESIGN.md)** - Architecture, design decisions, and implementation details
- **[BENCHMARKS.md](BENCHMARKS.md)** - Performance comparison and real-world impact analysis
- **[example_test.go](example_test.go)** - Complete working examples (rate limiting, sharding, filtering)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

[Apache License 2.0](LICENSE)

## Related Projects

- [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) - Full-featured OTLP processing
- [protowire](https://pkg.go.dev/google.golang.org/protobuf/encoding/protowire) - Low-level protobuf wire format utilities
