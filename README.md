# otlp-wire

OTLP wire format utilities for Go, designed for edge ingester services that need to count, shard, and route telemetry data without full unmarshaling overhead.

[![Go Reference](https://pkg.go.dev/badge/go.olly.garden/otlp-wire.svg)](https://pkg.go.dev/go.olly.garden/otlp-wire)
[![Go Report Card](https://goreportcard.com/badge/go.olly.garden/otlp-wire)](https://goreportcard.com/report/go.olly.garden/otlp-wire)

## What It Does

- Count signals (metrics/logs/traces) without unmarshaling
- Iterate over resources with minimal allocations for parallel processing
- Extract resource metadata for routing decisions

## Performance Characteristics

Edge ingester services receiving OTLP data often need to make routing decisions, enforce rate limits, or shard across workers without processing the full telemetry payload. Full protobuf unmarshaling is expensive for these operations:
- Allocates thousands of Go objects (structs, slices, maps for attributes)
- Creates significant garbage collector pressure
- High CPU overhead parsing entire protobuf structure

otlp-wire operates directly on wire format bytes:

- 35-55x faster counting than unmarshaling (zero allocations)
- 1,100-2,800x faster iteration than unmarshal+iterate (2 allocations)
- 2,800-3,700x faster splitting than unmarshal+remarshal (2 allocations)
- Minimal GC pressure (only 24 bytes per batch for error handling)
- Zero dependencies (only stdlib + protowire)

See [BENCHMARKS.md](BENCHMARKS.md) for detailed comparison.

## Use Cases

Designed for edge ingester services:

- **Observability**: Count signals in batches for monitoring ingestion volume
- **Sharding**: Split batches by resource for parallel processing across workers
- **Routing**: Extract resource attributes to route batches to appropriate backends

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
if count > limit {
    return errors.New("rate limit exceeded")
}

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
func (r ResourceMetrics) Resource() ([]byte, error)
func (r ResourceMetrics) WriteTo(w io.Writer) (int64, error)

// Same pattern for ResourceLogs and ResourceSpans
```

## Design Philosophy

**"Provide raw bytes and tools. Users decide what to do with them."**

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

For detailed benchmarks and methodology, see [BENCHMARKS.md](BENCHMARKS.md).

## Documentation

- **[DESIGN.md](DESIGN.md)** - Architecture, design decisions, and implementation details
- **[BENCHMARKS.md](BENCHMARKS.md)** - Performance comparison and methodology
- **[example_test.go](example_test.go)** - Complete working examples (rate limiting, sharding, filtering)

## Requirements

- Go 1.23+ (for `iter.Seq` iterator support)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

[Apache License 2.0](LICENSE)

## Related Projects

- [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) - Full-featured OTLP processing
- [protowire](https://pkg.go.dev/google.golang.org/protobuf/encoding/protowire) - Low-level protobuf wire format utilities
