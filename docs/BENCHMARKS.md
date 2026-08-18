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
  implementation returned at the first Resource field, so its cost was
  proportional to the number of fields preceding that field — O(1) only for a
  Resource-first container like this fixture, which is the ordering real
  producers emit but not one the wire format guarantees. This one is
  O(number of top-level fields) unconditionally, roughly 8 ns per additional
  scope entry on this machine.
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

## Scope and schema_url (E-2942)

E-2942 added `Scope()` to `ScopeMetrics`, `ScopeLogs` and `ScopeSpans`,
`SchemaUrl()` to all six containers, and the `InstrumentationScope`
accessors. Both new resolutions must scan the whole enclosing message, so
these benchmarks quantify that scan and confirm the zero-allocation gates
still hold. (`Metric.Name` was moved to last-value-wins here too and moved
back in E-2985; its current figures are in "Metric.Name resolution" below.)

**Environment:** 11th Gen Intel(R) Core(TM) i7-11800H @ 2.30GHz, linux/amd64,
Go 1.25.12.

**Commands:**

```bash
go test -run '^$' -bench 'BenchmarkScope_(SingleOccurrence|MultipleOccurrences|NameVersion_WireFormat|NameVersion_Unmarshal)$' -benchmem -count=10 ./...
go test -run '^$' -bench 'ScanScaling' -benchmem -count=5 ./...
```

### Scope access versus full decode

**Fixture:** an `ExportLogsServiceRequest` holding one resource container, one
scope container and one populated scope (name, version, one attribute), built
with `protowire`. The wire side opens both closure-based iterators and reads
name and version; the baseline unmarshals the same bytes with
`plogotlp.ExportRequest` and reads the same two fields. Medians of 10 runs.

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkScope_NameVersion_WireFormat` | 194 | 72 | 4 |
| `BenchmarkScope_NameVersion_Unmarshal` | 441 | 360 | 10 |
| `BenchmarkScope_SingleOccurrence` (accessor alone) | 9.5 | 0 | 0 |
| `BenchmarkScope_MultipleOccurrences` (3 occurrences, merges) | 141 | 112 | 2 |

The four allocations on the wire side are the two closure-based iterators
being opened, not `Scope()` — the accessor itself is zero-allocation, as the
single-occurrence row shows.

**What this comparison does and does not prove.** The baseline is a full pdata
decode of the same bytes. It stands in for the consumer code this API replaces
— dibber unmarshals a generated `InstrumentationScope` per scope, sage
strict-parses one per occurrence — but it is not either implementation
verbatim. Reproducing them exactly would mean adding `go.opentelemetry.io/proto/otlp`
as a dependency of a public library purely for a benchmark, which was judged
not worth the supply-chain surface. Treat these numbers as the order of
magnitude, and validate the real saving in the consumer migrations, which are
tracked separately.

### Scan cost scales with the container

Merging must find every occurrence and last-value-wins must reach the final
one, so neither accessor can stop early. Cost therefore grows with the number
of sibling fields. This matters more here than for `Resource()`: a resource
container holds a handful of scopes, whereas a scope container holds every
record. One scope, then *n* 64-byte log records, then a `schema_url`; medians
of 5 runs.

| records | `ScopeLogs.Scope()` | `ScopeLogs.SchemaUrl()` |
|---:|---:|---:|
| 1 | 26.3 ns | 26.2 ns |
| 100 | 847 ns | 766 ns |
| 1000 | 8146 ns | 7545 ns |

Both remain 0 B/op, 0 allocs/op at every size.

Findings:

- **Roughly 8 ns per skipped record**, allocation-free. A consumer that reads
  the scope and then iterates the records pays this as a constant factor on a
  walk it was already doing.
- **A consumer that reads only the scope and stops early pays the most.**
  dibber's `scopeFromContainer` returns at the first scope occurrence with a
  non-empty name and never touches the records; `Scope()` cannot, because
  proving no later occurrence exists requires reaching the end. That is the
  price of pdata parity, and it is the same trade already accepted for
  `Resource()` in E-2941.
- Each skipped field is still cheap: `protowire.ConsumeFieldValue` reads the
  length prefix and steps over the body without descending. Growth is in the
  number of tags, not their size.

### Gates unchanged

`BenchmarkResource_SingleOccurrence`, `BenchmarkResource_MultipleOccurrences`,
`BenchmarkResource_ScanScaling` and `BenchmarkMetrics_Count_WireFormat` were
re-run on this branch against `main` in a worktree on the same machine.
Counting stays 0 B/op, 0 allocs/op; `Resource()` keeps its zero-allocation
single-occurrence path and its 144 B / 2 allocs merge path. Timing
differences were within run-to-run noise on this machine, which is why no
before/after table is given for them — generalizing `extractResourceMessage`
into `extractMergedMessage` added a field-number parameter and no work.

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

## Allocation-free container iteration (E-3008)

`BenchmarkContainerSeq` walks every resource, scope and log record using either
the ordinary closure/error-function adapters or the `ResourceLogsSeq` and
`ScopeLogsSeq` method values. The two fixtures isolate different production
shapes: record-heavy is 5 resources × 2 scopes × 100 records; resource-heavy
is 20 resources × 1 scope × 5 records. Both arms use `LogRecordsSeq` at the
record level, so the difference is only the two container levels.

**Environment-specific result:** 11th Gen Intel Core i7-11800H, linux/amd64,
Go 1.25.12:

```bash
go test -run 'TestContainerSeq' -bench '^BenchmarkContainerSeq$' -benchmem -count=1
```

Time varied with machine load and is not claimed here. Allocation counts are
toolchain-specific; zero allocations on the supported toolchain is the gate.

| Shape | Iterator | B/op | allocs/op |
|---|---|---:|---:|
| record-heavy | ordinary | 424 | 15 |
| record-heavy | sequence | 0 | 0 |
| resource-heavy | ordinary | 1,264 | 45 |
| resource-heavy | sequence | 0 | 0 |

The sequence form removes every container-iterator allocation in both shapes.
The ordinary API remains available as an adapter with unchanged wire order,
lazy error timing and early-stop behavior. Metrics and traces expose the same
sequence forms because their resource/scope container layout and escape
behavior are identical; `TestContainerSeq_ZeroAllocations` pins all three.

E-3051 measured whether those ordinary adapter bodies could be replaced by
direct calls to the existing generic `repeatedFieldSeq` helper. On the same
i7-11800H / linux-amd64 / Go 1.25.12 environment, 15 alternating invocations
of prebuilt before/after test binaries kept the counts at 15 and 45 allocs/op
but added exactly 24 B/op to both shapes (424 → 448 and 1,264 → 1,288). A
generic method-value adapter was worse at 27/87 allocs and 720/2,281 B/op.
The explicit ordinary adapters are therefore performance-bearing under the
supported compiler and remain intentionally duplicated.

The same issue removed the type-specific `keyValueSeq` wrapper and routed the
four allocation-free KeyValue APIs directly through `repeatedFieldSeq2`.
Fifteen alternating before/after invocations (`-benchtime=200x`) of
`BenchmarkMetric_Metadata_Seq` and
`BenchmarkMetrics_ScrapeDeepIterationSeq_WireFormat` were indistinguishable:
the after arm was slower in 7/15 and 6/15 pairs respectively, with median
paired deltas of about +0.4% and -1.2%. The dedicated allocation tests for
resource, scope, metadata and datapoint attribute sequences remain the
zero-allocation gate.

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

## `Metric.Metadata`

`MetadataSeq` against the hand-rolled field-12 walk it replaces in marigold.

**Environment:** i7-11800H, linux/amd64, Go 1.25.12, developer desktop under
load. **Fixture:** `createScrapeShapedMetricsWithMetadata` — 4,800 metrics,
three metadata entries each.

`go test -count=N` runs a benchmark's iterations consecutively, so it does not
interleave arms. Alternate single-arm invocations of a prebuilt binary and take
the median of the paired per-round deltas:

```bash
go test -c -o /tmp/otlpwire.test .
for round in $(seq 1 12); do
  for arm in HandRolled Seq; do
    /tmp/otlpwire.test -test.run '^$' -test.benchmem -test.benchtime=800x \
      -test.bench "^BenchmarkMetric_Metadata_${arm}\$"
  done
done
```

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkMetric_Metadata_HandRolled` | 618,413 | 168 | 7 |
| `BenchmarkMetric_Metadata_Seq` | 661,764 | 168 | 7 |

Allocations match exactly; the per-element path allocates nothing. `MetadataSeq`
is slower by single-digit percent — the ranges overlap, so the evidence is the
paired median (between +3% and +6.5% across runs) and the sign, which held in
12 of 12 rounds above. The cost is `forEachRepeatedField`'s capacity clamp plus
one indirect call per element; the clamp is the guarantee a caller's `append`
cannot reach the neighbouring entry, which the hand-rolled walk lacks.

## LogRecord severity accessors (E-2944, E-2957)

`SeverityNumber`, `SeverityText` and `Severity` all read from one schema-aware
walk of the whole LogRecord rather than getting their own extractors. Two
measurements matter: what threading `severity_text` out of that walk costs the
`SeverityNumber` hot path, and what the combined accessor saves a consumer
reading both fields.

**Environment:** 11th Gen Intel(R) Core(TM) i7-11800H @ 2.30GHz, linux/amd64,
Go 1.25.12, developer desktop. Both comparisons alternate single-arm
invocations of prebuilt binaries and report the median of the paired per-round
deltas; `go test -count=N` runs a benchmark's iterations consecutively and
does not interleave arms.

### Cost to the existing `SeverityNumber` path

**Fixture:** `createSeverityClassificationBenchLogs` — 5 resources × 120
records, no `severity_text` set, so this measures only the walk's added
branching and return, not reading the new field.

The "before" arm must be built from a checkout of the merge base, not from a
stash — `git stash` on a clean worktree is a no-op and would build the same
binary twice, measuring 0%:

```bash
go test -c -o /tmp/after.test .                    # this branch
mkdir /tmp/base && git archive <merge-base> | tar -x -C /tmp/base
(cd /tmp/base && go test -c -o /tmp/before.test .) # merge base

for round in $(seq 1 15); do
  for arm in before after; do
    /tmp/$arm.test -test.run '^$' -test.benchtime 500ms \
      -test.bench '^BenchmarkLogs_SeverityClassification_WireFormat$'
  done
done
```

| Revision | ns/op (median of 15) | B/op | allocs/op |
|---|---:|---:|---:|
| merge base | 78,744 | 488 | 15 |
| this change | 80,749 | 488 | 15 |

**Median paired delta +2.67%, slower in 14 of 15 rounds**, allocations
unchanged. A second independent 15-round session on the same machine measured
+2.00%, slower in 12 of 15. Take the cost as roughly **+2 to +3%** rather than
either figure exactly: single rounds range from −8.7% to +5.8% under desktop
load, so the sign consistency across rounds is the evidence and the magnitude
is only stable to about a percentage point. Nine rounds is not enough for a
delta this size on this machine — it understated the median by roughly a
percentage point against both 15-round sessions. Use 15 or more.

This is the price of one walk instead of two implementations, and it is
deliberate: see [DESIGN.md](DESIGN.md) for why drift between the severity
accessors is the failure being bought out.

### Reading both fields: `Severity` against the single-field pair

**Fixture:** `createBenchLogs` — 5 resources × 100 records, each with a body,
a timestamp, two attributes, a severity number and `severity_text`.

Three arms of one binary, alternated. `SeverityText` is the single-field
baseline — one walk per record — and `SeverityNumberAndText` calls both
single-field accessors, which runs that walk twice:

```bash
go test -c -o /tmp/otlpwire.test .
for round in $(seq 1 15); do
  for arm in SeverityText SeverityNumberAndText Severity; do
    /tmp/otlpwire.test -test.run '^$' -test.benchmem -test.benchtime=3000x \
      -test.bench "^BenchmarkLogRecord_${arm}\$"
  done
done
```

| Benchmark | fields / walks | ns/op (median of 15) | B/op | allocs/op |
|---|---|---:|---:|---:|
| `BenchmarkLogRecord_SeverityText` | one / one | 89,783 | 368 | 14 |
| `BenchmarkLogRecord_SeverityNumberAndText` | two / two | 170,990 | 368 | 14 |
| `BenchmarkLogRecord_Severity` | two / one | 89,669 | 368 | 14 |

**Median paired saving 48% against the two-accessor pair, cheaper in 15 of 15
rounds**, allocations unchanged. Put the other way round, reading both fields
through the single-field accessors costs roughly **double**: the pair is 92%
more expensive at the median, per-round deltas spanning +76% to +110%. An
earlier 11-round session on the same machine measured a 44% saving, so take
the effect as "about half the cost, give or take a few points" rather than
either figure exactly.

The combined arm lands on the single-field one — median +1.1%, per-round
deltas from −7.4% to +29.0%, slower in only 9 of 15 rounds, so the sign does
not hold and the two are indistinguishable here. That is the property being
claimed: reading both fields costs one walk, not two.

The 14 allocations are the resource and scope iterator openings, not the
severity read; every arm pays the same. A consumer that wants a
`string` rather than a view pays for that conversion at its own call site,
which is where the allocation belongs.

## Span field accessors (E-2945)

**Environment for every measurement in this section:** 11th Gen Intel Core
i7-11800H @ 2.30GHz, 16 threads, linux/amd64, Go 1.25.12, on a desktop with
other work running. Paired comparisons are alternating invocations of prebuilt
test binaries, reported as the median of the paired per-round deltas plus how
many rounds carried the sign. Never `go test -count=N`: it runs a benchmark's
iterations consecutively rather than interleaving the arms.

**Fixture:** `createInternalSpansBenchTraces` — 5 resources × 100 spans, each
with the three identifiers, a name, a kind cycling through five values, start
and end timestamps and five attributes; every tenth span also carries an
event. The attributes and events matter: they are what both walks step over to
reach `start_time_unix_nano` and `end_time_unix_nano`.

### The identifier accessors are unchanged

`TraceID`, `SpanID` and `ParentSpanID` still scan first-match over
`extractFixedBytesField` and stop at their field. `BenchmarkSpanIteration`
against a merge-base checkout measures −4.3%, slower in 6 of 21 rounds — noise
around zero, which is the expected result for byte-identical code.

That is a deliberate asymmetry with the four scalar accessors, and the reason
is cost, not principle. Resolving a scalar last-value-wins means walking to the
end of the span, because nothing else proves a later occurrence does not exist.
An identifier accessor that did the same would pay that on **every conformant
span** to change behavior observable only on **malformed** ones, since a
conformant producer emits each singular field exactly once.

How much it would have cost depends on the producer, which is why measuring it
against pdata-marshalled fixtures alone is misleading. pdata's
`MarshalProto` writes fields back-to-front, so a collector-marshalled span
carries `trace_id` **last** — a first-match scan already walks nearly the whole
span and a full walk looks nearly free. Stock protobuf (protobuf-go, Java,
Python — the OTLP SDK exporters) emits ascending order with `trace_id` first.
Measured, `Span.TraceID()` on one ascending-order span against the first-match
scan:

| attributes on the span | first-match | full walk | ratio |
|---|---:|---:|---:|
| 5 | 13.0 ns | 133.8 ns | 10x |
| 20 | 10.8 ns | 286.4 ns | 26x |
| 50 | 10.7 ns | 619.0 ns | 58x |
| 200 | 11.6 ns | 1,948 ns | 167x |

Keep this in mind before moving any first-match accessor onto a full walk, and
before quoting a Span benchmark built only from pdata-marshalled fixtures.

### The four scalar accessors, against overstory's hand-rolled walk

`readSpanFieldsHandRolled` is overstory's `internal/detector/wire.go` walk
copied verbatim so the comparison reproduces from this repository alone. The
arms are **not** equivalent work: overstory's reads all five fields in a single
pass and checks the name for UTF-8 validity; here `TraceID` scans first-match
and each of the four scalars walks the span once. Both arms run the same
reduction — count internal spans, total their name lengths and durations.

```bash
go test -c -o /tmp/ow-after.test .
for round in $(seq 1 15); do
  for arm in _HandRolled _Accessors; do
    /tmp/ow-after.test -test.run '^$' -test.benchmem -test.benchtime=300x \
      -test.bench "^BenchmarkSpan_InternalSpans${arm}\$"
  done
done
```

| Benchmark | ns/op (median) | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkSpan_InternalSpans_HandRolled` | 86,236 | 552 | 24 |
| `BenchmarkSpan_InternalSpans_Accessors` | 213,335 | 552 | 24 |
| `BenchmarkSpan_InternalSpans_Unmarshal` (pdata) | 698,032 | 558,816 | 10,456 |
| `BenchmarkSpan_TraceIDOnly` (first-match, control) | 68,486 | 528 | 24 |

**About 151% slower than the hand-rolled walk** (median paired delta +151.4%),
slower in 15 of 15 rounds, at **allocation parity**. The gap is the walk count,
and it is conditional: the reduction calls `Kind()` on every span, then `Name()`
and the two timestamps only on the spans that pass the internal-span filter.
The fixture's kinds cycle through five values of which two are `Internal`, so
the arm averages 1 + 0.4 x 3 = 2.2 scalar walks per span — plus one first-match
`TraceID` scan — against the hand-rolled arm's single pass. A consumer reading
all four scalars unconditionally would pay four.

Read the comparison against the right baseline. overstory runs **pdata
unmarshal** on `main` today; its wire walk lives on an unmerged branch.
Migrating from pdata to these accessors is a **3.3x improvement** (698 µs to
213 µs) and drops 10,456 allocations per batch to 24. Migrating from its
hand-rolled walk is a 2.5x cost. Which number applies depends on which branch
overstory migrates from, and that decision belongs on the migration issue.

**No combined accessor exists yet, and these numbers are the argument for one.**
`parseSpanFields` already produces all four scalars in a single walk, so
exposing them together would need no new parsing machinery — only a decision
about the exported shape. Per the flattened-convenience rule in E-2940 that
needs a committed consumer on a measured hot path, and overstory reading four
scalars per span is exactly that once its migration is decided. Until then the
choice stays open, because the right shape depends on which fields the consumer
actually takes.

Drop the hand-rolled arm and this subsection once overstory moves to the Span
accessors.

## Metric.Name resolution (E-2985)

E-2985 moved `Metric.Name` back to first-match (`extractBytesField`) from the
last-value-wins scan E-2942 gave it. The accessor is called once per metric, so
the cost scales with metric count; marigold reads it across ~4,800-metric
scrape payloads.

**The measurement only works on a fixture built by hand.** pdata's
`ProtoMarshaler` writes fields back-to-front — a marshalled `Metric` here comes
out as `[5 1]`, body then `name` — so `name` lands *last* and a first-match
scan has to walk the whole metric to reach it anyway. Against that fixture the
two resolutions are nearly indistinguishable, which is why
`BenchmarkMetric_Name` did not catch the regression it was added to guard.
Stock protobuf runtimes, and therefore the OTLP SDK exporters, emit ascending
field numbers with `name` first. `createScrapeShapedMetricsSDKOrder` builds
that order with `protowire` because pdata cannot produce it. Both arms are
kept: the pdata one reflects collector-relayed traffic, the SDK one reflects
direct exporters.

**Environment:** Apple M5, darwin/arm64, Go 1.26.6.

**Method:** two prebuilt binaries differing only in `Metric.Name`'s body,
alternated for 6 rounds rather than run as consecutive blocks. Medians of 6.

```bash
go test -c -o after.test .
# revert Metric.Name to extractLastBytesField
go test -c -o before.test .
for round in 1 2 3 4 5 6; do
  for arm in before after; do
    ./$arm.test -test.run '^$' -test.bench 'BenchmarkMetric_Name' -test.benchmem
  done
done
```

### The accessor alone

One metric carrying what a Prometheus receiver sets: name, unit, a datapoint
body and three metadata entries. The two arms are the same 168 bytes in
different field order.

| Field order | last-value-wins | first-match (current) | change |
|---|---:|---:|---:|
| ascending (SDK exporters, `name` first) | 26.9 ns | 4.2 ns | **−84%** |
| back-to-front (pdata, `name` last) | 25.9 ns | 25.7 ns | none |

Both remain 0 B/op, 0 allocs/op. The second row is the point: where `name` is
emitted last, first-match buys nothing, because proving it is the first
occurrence and reaching the last one are the same walk.

### Clamping the first-match view

Moving `Metric.Name` onto `extractBytesField` exposed that the helper returned
`protowire.ConsumeBytes`'s slice unclamped, so a caller appending to the
returned name could overwrite the fields behind it. The helper now clamps, the
way `forEachRepeatedField` and `extractMergedMessage` already did. That also
clamps `KeyValue.Key` and `KeyValue.ValueRaw`, the library's hottest path, so
it was measured: paired binaries differing only in the slice expression,
4 alternating rounds.

`BenchmarkMetrics_DeepIteration_WireFormat` medians 36.0 µs unclamped against
37.1 µs clamped, and `BenchmarkMetrics_ScrapeDeepIteration_WireFormat` 701.8 µs
against 713.1 µs. Both pairs of ranges overlap (35.4–36.4 against 35.9–38.4;
686.9–709.4 against 695.9–735.7), so the difference is not separable from noise
at this sample size. Allocations are byte-identical on both arms
(20912 B/op, 1033 allocs/op and 460986 B/op, 19207 allocs/op), which is the
guardrail that matters here.

### Across a scrape payload

The E-2608 scrape shape — one resource, one scope, 4800 metrics with one
datapoint each — iterated in full, reading every name.

| Fixture | last-value-wins | first-match (current) | change |
|---|---:|---:|---:|
| `BenchmarkMetric_Name` (pdata order) | 69.4 µs | 63.0 µs | −9% |
| `BenchmarkMetric_Name_SDKOrder` | 70.8 µs | 41.7 µs | **−41%** |

Both stay at 112 B/op, 6 allocs/op — the allocations are the iterators being
opened, not the accessor.

Read the two rows differently. The SDK-order ranges are far apart
(67.8–72.2 µs against 40.2–45.9 µs) and the gap is the early return. The
pdata-order ranges **overlap** (67.2–71.0 µs against 61.2–67.3 µs), so treat
that −9% as suggestive rather than established: on that fixture both
resolutions traverse identical bytes, and the accessor-level arm above shows no
change at all. Any real residual there is `extractLastBytesField` reaching its
occurrence through a `forEachRepeatedField` closure where `extractBytesField`
runs a flat loop — a per-call cost the remaining last-value-wins accessors
still pay. Isolating it needs its own paired benchmark, which this change does
not add.
