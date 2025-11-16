# Performance Benchmarks

Comparison of otlp-wire operations vs traditional unmarshal/marshal approaches.

**Test Setup:**
- Platform: Apple M4
- Data: 5 resources, 100 data points/spans/logs per resource
- Go version: 1.24.5

## Counting Operations

### Metrics - DataPointCount()

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format | 2.3 μs | 0 B | 0 |
| Unmarshal | 81.0 μs | 143 KB | 5,161 |

Speedup: 35.1x faster, zero allocations

### Traces - SpanCount()

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format | 2.1 μs | 0 B | 0 |
| Unmarshal | 115.3 μs | 217 KB | 5,131 |

Speedup: 55.5x faster, zero allocations

### Logs - LogRecordCount()

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format | 2.2 μs | 0 B | 0 |
| Unmarshal | 108.9 μs | 198 KB | 6,131 |

Speedup: 49.2x faster, zero allocations

---

## Iterator Operations

### Metrics - ResourceMetrics()

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format Iterator | 56.4 ns | 24 B | 2 |
| Unmarshal + Iterate | 158.2 μs | 143 KB | 5,161 |

Speedup: 2,805x faster (iteration only)

### Traces - ResourceSpans()

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format Iterator | 60.7 ns | 24 B | 2 |
| Unmarshal + Iterate | 100.5 μs | 217 KB | 5,131 |

Speedup: 1,655x faster (iteration only)

### Logs - ResourceLogs()

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format Iterator | 93.3 ns | 24 B | 2 |
| Unmarshal + Iterate | 106.0 μs | 198 KB | 6,131 |

Speedup: 1,136x faster (iteration only)

**Note:** The 2 allocations (24 bytes) are from the iterator error handling pattern (closure capture mechanism).

---

## Split Operations (Iterate + WriteTo)

### Metrics

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format Split | 50.1 ns | 24 B | 2 |
| Unmarshal + Remarshal | 143.2 μs | 281 KB | 7,742 |

Speedup: 2,858x faster

### Traces

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format Split | 51.2 ns | 24 B | 2 |
| Unmarshal + Remarshal | 191.9 μs | 432 KB | 7,192 |

Speedup: 3,748x faster

### Logs

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format Split | 51.0 ns | 24 B | 2 |
| Unmarshal + Remarshal | 178.2 μs | 386 KB | 8,692 |

Speedup: 3,494x faster

---

## Resource Extraction

### Metrics - Resource()

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format | 113.6 ns | 24 B | 2 |
| Unmarshal | 99.9 μs | 143 KB | 5,161 |

Speedup: 879x faster

---

## Implementation Details

### Counting Performance

Wire format counting avoids:
- Unmarshaling protobuf to Go structs
- Allocating memory for intermediate objects
- Creating thousands of struct instances (ResourceMetrics, ScopeMetrics, Metric, DataPoint objects)
- Allocating maps for attributes at each level
- Garbage collector pressure from short-lived objects

The implementation reads protobuf tags directly and counts occurrences without full deserialization.

**GC Impact:** Unmarshaling a 500-datapoint batch creates 5,000+ objects. Wire format creates zero objects for counting.

### Iterator Performance

Wire format iteration provides:
- Direct byte slice references (zero-copy)
- Minimal heap allocations (2 per batch for error handling)
- Early exit capability when processing subset of data
- No garbage collector pressure from OTLP object allocation

The 2 allocations per iterator are from Go's closure capture mechanism for error handling.

**GC Impact:** Unmarshaling creates the full OTLP object graph (5,000+ objects per batch). Wire format iteration creates 2 small objects (24 bytes total) for error handling.

### Split Performance

Wire format splitting combines iteration with WriteTo:
- Iteration: ~50-60 ns (2 allocs)
- WriteTo adds tag/length prefix (no additional allocations)
- Total: ~50 ns per resource batch

---

## Use Cases

### Counting
Suitable for edge ingester services:
- Monitoring ingestion volume
- Observability metrics about incoming batches
- Batch size validation

### Iteration
Suitable for edge ingester services:
- Sharding batches across workers
- Per-resource routing decisions
- Parallel processing pipelines

### Resource Extraction
Suitable for edge ingester services:
- Service-based routing
- Metadata extraction for routing decisions
- Resource attribute hashing for load balancing

---

## Theoretical CPU Impact

**Disclaimer:** The following calculations extrapolate benchmark results to theoretical workloads. Actual production performance depends on many factors including network I/O, disk access, and system architecture.

### Counting Operations at 10,000 req/s

**Unmarshal approach:**
- 81 μs × 10,000 = 810 ms CPU/second
- Single-core CPU usage: 81%

**Wire format approach:**
- 2.3 μs × 10,000 = 23 ms CPU/second
- Single-core CPU usage: 2.3%

Difference: 787 ms CPU/second saved

### Sharding at 10,000 req/s

**Unmarshal + remarshal approach:**
- 143 μs × 10,000 = 1,430 ms CPU/second
- Multi-core CPU usage: >100% (requires multiple cores)

**Wire format approach:**
- 50 ns × 10,000 = 0.5 ms CPU/second
- Single-core CPU usage: 0.05%

Difference: 1,429 ms CPU/second saved

---

## Running Benchmarks

To reproduce these results:

```bash
# Run all comparison benchmarks
go test -bench='Count|Iterator|Split|ResourceExtraction' -benchmem

# Run specific signal type
go test -bench='BenchmarkMetrics' -benchmem

# Extended run for more stable results
go test -bench=. -benchmem -benchtime=3s
```

---

## Test Data Characteristics

All benchmarks use realistic OTLP data:

- 5 resources with full resource attributes
- 100 data points/spans/logs per resource (500 total)
- Full scope information (instrumentation library)
- Complete metadata (timestamps, attributes)
- Realistic attribute cardinality

This represents typical production telemetry batch sizes.
