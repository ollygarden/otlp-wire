# Performance Benchmarks

Comparison of otlp-wire operations vs traditional unmarshal/marshal approaches.

**Test Setup:**
- Platform: Apple M4
- Data: 5 resources, 100 data points/spans/logs per resource
- Go version: 1.24.5

## Counting Operations

Counting is available at both batch and resource levels with the same performance characteristics.

### Metrics - DataPointCount()

**Batch-level: `ExportMetricsServiceRequest.DataPointCount()`**

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format | 2.3 μs | 0 B | 0 |
| Unmarshal | 81.0 μs | 143 KB | 5,161 |

Speedup: 35.1x faster, zero allocations

**Resource-level: `ResourceMetrics.DataPointCount()`**

Resource-level counting has identical performance characteristics (zero allocations) since it uses the same underlying implementation, just starting from a different entry point in the wire format.

### Traces - SpanCount()

**Batch-level: `ExportTracesServiceRequest.SpanCount()`**

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format | 2.1 μs | 0 B | 0 |
| Unmarshal | 115.3 μs | 217 KB | 5,131 |

Speedup: 55.5x faster, zero allocations

**Resource-level: `ResourceSpans.SpanCount()`** - Same zero-allocation performance.

### Logs - LogRecordCount()

**Batch-level: `ExportLogsServiceRequest.LogRecordCount()`**

| Method | Time | Memory | Allocations |
|--------|------|--------|-------------|
| Wire Format | 2.2 μs | 0 B | 0 |
| Unmarshal | 108.9 μs | 198 KB | 6,131 |

Speedup: 49.2x faster, zero allocations

**Resource-level: `ResourceLogs.LogRecordCount()`** - Same zero-allocation performance.

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

## Resource() absence, aliasing, and merge (E-2941)

`Resource()` on `ResourceMetrics`/`ResourceLogs`/`ResourceSpans` changed in
three ways: it returns `(nil, nil)` instead of an error for an absent
Resource field, it merges repeated Resource occurrences instead of returning
only the first, and it returns the typed `Resource` rather than `[]byte`.
Merging requires scanning the complete container instead of returning as soon
as the first Resource field is found, which is a small but real cost even in
the common single-occurrence case; these benchmarks quantify that cost and
confirm the hard performance gate that the single-occurrence path still
allocates nothing.

**Environment:** 11th Gen Intel(R) Core(TM) i7-11800H @ 2.30GHz, linux/amd64,
Go 1.25.12. Compared with two checkouts on the same machine: this change
(feature branch) and `upstream/main` at commit `8c0e9b4` (pre-E-2941).

**Commands** (medians of 10 runs for the pair below, 5 for the scaling table):

```bash
go test -run '^$' -bench 'BenchmarkResource_(SingleOccurrence|MultipleOccurrences)$' -benchmem -count=10 ./...
go test -run '^$' -bench 'BenchmarkResource_ScanScaling' -benchmem -count=5 ./...
```

The `upstream/main` side was measured in a detached worktree of `8c0e9b4`
with the same fixtures ported verbatim, since the benchmarks themselves are
new in this change.

**Fixture:** a `ResourceMetrics` container built directly with `protowire`
(not through pdata, since pdata's API cannot produce a container with 2+
Resource occurrences) holding either one `service.name` Resource occurrence
(single-occurrence case) or three occurrences with distinct keys
(`service.name`, `deployment.environment`, `host.name`) for the
multi-occurrence case. The pre-E-2941 checkout returns only the first
occurrence regardless of how many are present, so its "multiple occurrences"
number measures the same raw extraction cost on the same bytes, not a merge
(it does not merge).

| Benchmark | Revision | ns/op | B/op | allocs/op |
|---|---|---:|---:|---:|
| `BenchmarkResource_SingleOccurrence` | pre-E-2941 (`upstream/main`) | 6.72 | 0 | 0 |
| `BenchmarkResource_SingleOccurrence` | this change | 17.6 | 0 | 0 |
| `BenchmarkResource_MultipleOccurrences` | pre-E-2941 (`upstream/main`, returns 1st occurrence only) | 6.65 | 0 | 0 |
| `BenchmarkResource_MultipleOccurrences` | this change (merges all occurrences) | ~205 (160–253) | 144 | 2 |

Single-occurrence figures are medians of 10 runs and are stable to within
about 0.7 ns. The multi-occurrence figure varies run to run because it
allocates; the range observed across 10 runs is given rather than a single
median, since quoting one number there would overstate the precision.

### Scan cost scales with the container (`BenchmarkResource_ScanScaling`)

The single-occurrence table above uses a container with one scope entry, which
undersells the change. Merging requires scanning every top-level field, so the
cost grows with the container's scope count where the previous
first-match-and-return implementation was flat. Measured on the same machine
with `-count=5`, one Resource followed by *n* 64-byte scope entries:

| scope entries | pre-E-2941 | this change |
|---:|---:|---:|
| 1 | 6.8 ns | 17.4 ns |
| 10 | 6.8 ns | 88.5 ns |
| 50 | 6.8 ns | 412 ns |

Findings:

- **The hard performance gate holds:** the single-occurrence path — what
  every real producer emits — remains zero-allocation after this change, at
  every container size measured.
- **This is a complexity change, not a constant-factor one.** The previous
  implementation returned at the first Resource field, which in practice is
  the container's first field, making it O(1) in container size. This one is
  O(number of top-level fields), roughly 8 ns per additional scope entry on
  this machine.
- Each skipped field is still cheap: `protowire.ConsumeFieldValue` on a
  length-delimited field reads the length prefix and steps over the body
  without descending into it, so the walk covers the container's tags and
  never its payload. The growth is in the number of tags, not their size.
- Scanning fully is what parity with pdata costs — pdata parses the whole
  message too — and there is no way to prove a second occurrence is absent
  without looking. The absolute numbers stay well under a microsecond for
  realistic containers, but consumers calling `Resource()` once per container
  on every message should be aware the cost now tracks container shape.
- Merging 2+ occurrences allocates (144 B, 2 allocs in this fixture: one
  slice growth for the second and later occurrences, one for the
  concatenated buffer) where the pre-E-2941 code allocated nothing — because
  it silently returned only the first occurrence instead of merging. This is
  the documented, intentional exception to the zero-copy accessor contract;
  see [DESIGN.md](DESIGN.md) and the "Resources and attributes" section of
  [specification.md](specification.md).

### Log severity classification (E-2892) — unaffected by E-2941

`classifyWireLogSeverities` was updated to call the new `ResourceLogs.Resource()`
once per resource and `Resource.StringAttribute` per key, replacing calls to
the removed `ResourceLogs.StringAttribute` per key. Re-running
`BenchmarkLogs_SeverityClassification_WireFormat` on the same machine (11th
Gen Intel i7-11800H, linux/amd64, Go 1.25.12; command:
`go test -run '^$' -bench 'BenchmarkLogs_SeverityClassification' -benchmem -count=5 ./...`)
shows no meaningful change: pre-E-2941 and this change both measure
approximately 80,000 ns/op, 488 B/op, 15 allocs/op (median of 5 runs on this
machine; this machine's absolute numbers differ from the Apple M4 figures
recorded for this benchmark above since they are different hardware, but the
before/after comparison on identical hardware is what matters here).

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

---

## Deep iteration (metrics depth, E-2608)

**Test Setup:**
- Platform: Apple M4
- Go version: go1.26.1 darwin/arm64
- `go test -run '^$' -bench 'BenchmarkMetrics_(Scrape)?DeepIteration' -benchmem -count=5 ./...`

These benchmarks drive the full metrics-depth API (`ResourceMetrics.ScopeMetrics()` →
`ScopeMetrics.Metrics()` → `Metric.DataPoints()` → `DataPoint.Attributes()`) all the way
down to individual attribute key/value bytes, and compare it against a full pdata
unmarshal doing the equivalent walk. This is the "marigold" workload from E-2601:
for every data point, read the timestamp and consume every attribute's key and value
bytes (a stand-in for feeding them into a hash function).

Two fixtures are used:

- **Scrape-shaped** (`createScrapeShapedMetrics`): 1 resource, 1 scope, 4,800 metrics,
  1 data point each, 4 attributes per data point — mirrors real Prometheus-receiver
  scrape traffic (the E-2601 shape).
- **Continuity** (`createBenchMetrics`, reused from the existing suite): 5 resources,
  1 scope each, 1 metric each, 100 data points per metric (500 data points total) —
  kept for continuity with the other benchmarks in this file.

### Results (median of 5 runs)

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

Wire format still wins on both time and memory (roughly 7.6x less memory on both
fixtures — 3,507,250/460,987 and 159,361/20,912 — and ~5.0-5.5x fewer allocations —
105,631/19,207 and 5,161/1,033 — for the closure-based pair), but the margin is far
narrower than the order-of-magnitude speedups seen
for counting, shallow iteration, and resource extraction elsewhere in this document.
The reason is structural rather than a benchmark artifact: a memory profile of
`BenchmarkMetrics_ScrapeDeepIteration_WireFormat` shows essentially all allocations
(19,207 of them, ~460 KB) coming from the two per-element iterator closures —
`Metric.DataPoints()` opened once per metric (4,800 times) and `DataPoint.Attributes()`
opened once per data point (4,800 times), each paying the documented "2 allocations for
iteration" cost. Shallow operations (counting, single top-level iteration, resource
extraction) open an iterator once per batch, so that fixed cost is amortized; deep
iteration opens a fresh iterator at every level for every element, so the allocation
count scales with the number of metrics/data points rather than staying constant. The
per-element cost is still tiny (~48 bytes, i.e. 2 allocations of ~24 bytes, per
iterator open, matching the existing "2 allocations, 24 bytes" pattern documented
above) and wire format remains faster and
lighter than a full unmarshal at every data point count tested, but it no longer
benefits from the zero-copy, zero-allocation properties that make the shallow
operations near-free.

### Zero-alloc Seq variants (`DataPointsSeq` / `AttributesSeq`)

The library exposes two APIs at the two hot levels of deep iteration:

- **`(iter.Seq[T], func() error)`** — the original, ergonomic pattern used everywhere
  else in this library (`ResourceMetrics()`, `ScopeMetrics()`, `Metrics()`, `DataPoints()`,
  `Attributes()`). Two allocations per iterator open; fine when the level is opened once
  per batch/scope, but costly when opened once per metric or per data point.
- **`Metric.DataPointsSeq` / `DataPoint.AttributesSeq`** — additive, zero-allocation
  variants shaped as `iter.Seq2[T, error]` methods, meant to be ranged over directly
  (`for dp, err := range m.DataPointsSeq`). Because the method value never escapes to
  the heap, the compiler keeps the walk entirely on the stack. Errors are yielded
  inline as the second range value instead of via a separate `func() error`.

`BenchmarkMetrics_ScrapeDeepIterationSeq_WireFormat` confirms the effect: allocations on
the 4,800-metric scrape fixture drop from 19,207 to 7 (only the three outer
`ResourceMetrics()`/`ScopeMetrics()`/`Metrics()` opens remain, one per batch/scope, not
per element), B/op drops from ~461 KB to 184 B, and the speedup vs. a full pdata
unmarshal improves from 2.74x to 3.58x. Use the closure-based pattern for outer,
amortized levels and general iteration; use the Seq variants specifically for
`DataPoints()`/`Attributes()` in code paths that iterate every metric or every data
point in a batch, such as scrape-shaped or high-cardinality workloads.

---

## Log severity classification (E-2892)

This benchmark models an insight consumer that reads resource context through
`ResourceLogs.StringAttribute`, walks every LogRecord, and classifies severity
without needing log bodies or record attributes. `createSeverityClassificationBenchLogs`
contains 5 resources and 600 records, covers unspecified/trace/debug/info/warn/error
severities, and includes present, empty, non-string, and absent `service.name`
and `deployment.environment` values. Each benchmark first computes the complete
classification through both wire and pdata paths outside the timed section and
fails if the results differ.

**Environment-specific result:** Apple M4, darwin/arm64, Go 1.26.3;
`go test -run '^$' -bench 'BenchmarkLogs_SeverityClassification' -benchmem ./...`.
Measurements vary by machine and toolchain; do not treat these values as a
portable performance guarantee.

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkLogs_SeverityClassification_WireFormat` | 49,044 | 488 | 15 |
| `BenchmarkLogs_SeverityClassification_Unmarshal` | 117,241 | 257,906 | 6,690 |

The wire path is approximately 2.4x faster in this environment and avoids the
full pdata object graph. Its 15 allocations are the established outer iterator
error-closure cost; `ScopeLogs.LogRecordsSeq` itself remains zero-allocation on
the per-record path.
