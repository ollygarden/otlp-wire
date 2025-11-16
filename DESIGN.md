# wireformat Library Design

## Problem Statement

When processing OTLP (OpenTelemetry Protocol) telemetry data, common operations include:
1. **Counting signals** for rate limiting/telemetry
2. **Splitting batches** for parallel processing/sharding
3. **Extracting metadata** for routing decisions

Current approaches require full unmarshaling, which is expensive:
- High CPU usage (parsing entire protobuf structure)
- High memory allocation (creating Go objects for all data)
- Unnecessary when you only need counts or batch splitting

## Goals

1. **Fast signal counting** without unmarshaling
2. **Resource-level batch splitting** for sharding/parallel processing
3. **Metadata extraction** (Resource bytes) for routing decisions
4. **Minimal API** - provide tools, not opinions
5. **Zero-copy where possible** - work on raw bytes

## Non-Goals

1. **Not a complete OTLP parser** - use official libraries for that
2. **Not attribute-aware** - we don't read/filter by attribute values
3. **Not scope/metric-level splitting** - resource-level is sufficient for sharding
4. **Not a query language** - no path expressions or complex filters

## Core Principle

**"Provide raw bytes and tools. Users decide what to do with them."**

We give you:
- Raw bytes at different granularity levels
- Methods to count, split, and extract
- Building blocks for your use case

We don't:
- Force hash algorithms on you
- Decide when/how to route
- Unmarshal unless absolutely necessary

## API Design

### Type Hierarchy

```
ExportMetricsServiceRequest (OTLP message bytes)
  └─ ResourceMetrics[] (one per resource)
      └─ ScopeMetrics[] (scopes within resource)
          └─ Metrics[] (metrics within scope)
              └─ DataPoints[] (individual data points)
```

We expose two levels:
- **ExportMetricsServiceRequest** - complete batch (all resources)
- **ResourceMetrics** - single resource (all scopes + data points)

### Types and Methods

```go
// Top-level batch types
type ExportMetricsServiceRequest []byte
type ExportLogsServiceRequest []byte
type ExportTracesServiceRequest []byte

// Resource-level types
type ResourceMetrics []byte
type ResourceLogs []byte
type ResourceSpans []byte
```

**ExportMetricsServiceRequest methods:**
```go
func (m ExportMetricsServiceRequest) DataPointCount() (int, error)
// Returns total number of metric data points in the entire batch

func (m ExportMetricsServiceRequest) ResourceMetrics() (iter.Seq[ResourceMetrics], func() error)
// Returns iterator and error callback
// Check error after iteration completes
```

**ResourceMetrics methods:**
```go
func (r ResourceMetrics) Resource() []byte
// Returns raw Resource message bytes (attributes like service.name, host.name)

func (r ResourceMetrics) AsExportRequest() []byte
// Wraps ResourceMetrics in valid ExportMetricsServiceRequest
```

**Same pattern for Logs and Traces:**
- `ExportLogsServiceRequest.LogRecordCount() (int, error)`
- `ExportLogsServiceRequest.ResourceLogs() (iter.Seq[ResourceLogs], func() error)`
- `ExportTracesServiceRequest.SpanCount() (int, error)`
- `ExportTracesServiceRequest.ResourceSpans() (iter.Seq[ResourceSpans], func() error)`

## Use Cases

### 1. Global Rate Limiting
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
count, err := data.DataPointCount()
if err != nil {
    return err
}

if count > globalLimit {
    return errors.New("rate limit exceeded")
}

processMetrics(data)
```

### 2. Per-Service Sharding (Zero Allocations)
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

resources, getErr := data.ResourceMetrics()
for resource := range resources {
    // Hash resource for consistent routing
    hash := fnv64a(resource.Resource())
    workerID := hash % numWorkers

    // Send valid OTLP message to worker
    sendToWorker(workerID, resource.AsExportRequest())
}
if err := getErr(); err != nil {
    return err
}
```

### 3. Per-Service Rate Limiting with Early Exit
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

resources, getErr := data.ResourceMetrics()
for resource := range resources {
    // Count signals in this resource
    exportBytes := resource.AsExportRequest()
    count, _ := otlpwire.ExportMetricsServiceRequest(exportBytes).DataPointCount()

    // Extract service name for limit lookup
    svc := extractServiceName(resource.Resource())

    if count > serviceLimit[svc] {
        reject(resource)
        break // Early exit - stop processing remaining resources
    }
}
if err := getErr(); err != nil {
    return err
}
```

### 4. Attribute-Based Filtering
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

resources, getErr := data.ResourceMetrics()
for resource := range resources {
    // Unmarshal just the Resource (small, cheap)
    res := unmarshalResource(resource.Resource())

    if res.Attributes["environment"] == "production" {
        sendToProdPipeline(resource.AsExportRequest())
    }
}
if err := getErr(); err != nil {
    return err
}
```

### 5. Deduplication
```go
seenHashes := make(map[uint64]bool)

resources, getErr := data.ResourceMetrics()
for resource := range resources {
    // Hash entire resource data for deduplication
    hash := xxhash(resource.AsExportRequest())

    if seenHashes[hash] {
        continue // Duplicate - skip
    }

    seenHashes[hash] = true
    process(resource.AsExportRequest())
}
if err := getErr(); err != nil {
    return err
}
```

### 6. All-or-Nothing Processing
```go
// Collect all resources first (validates during collection)
resources, getErr := data.ResourceMetrics()
all := slices.Collect(resources)
if err := getErr(); err != nil {
    return err // Error during iteration - nothing processed yet
}

// Now process atomically
tx.Begin()
for _, resource := range all {
    process(resource)
}
tx.Commit()
```

## Implementation Details

### Wire Format Structure

OTLP protobuf structure (metrics example):
```
ExportMetricsServiceRequest (message)
  └─ resource_metrics: field 1 (repeated message)
      └─ resource: field 1 (message)
      └─ scope_metrics: field 2 (repeated message)
          └─ scope: field 1 (message)
          └─ metrics: field 2 (repeated message)
              └─ [various metric types]
                  └─ data_points: field 1 (repeated message)
```

### Count Implementation

Navigate wire format to count data points without unmarshaling:
1. Parse top-level tags to find ResourceMetrics fields
2. For each ResourceMetrics, find ScopeMetrics fields
3. For each ScopeMetrics, find Metrics fields
4. For each Metric, find the metric type (Gauge/Sum/Histogram/etc.)
5. Count data_points field occurrences

**Performance**: ~90% CPU reduction vs unmarshaling, 98% fewer allocations

### Iterator Implementation

Iterate over ResourceMetrics with zero allocations:
1. Use callback-based internal function `forEachResourceMetrics`
2. Parse protobuf tags on-demand during iteration
3. Extract raw bytes for each ResourceMetrics as we iterate
4. Support early exit (break stops parsing remaining resources)

**Performance**: ~30ns per iteration, 0 allocations (no slice container)

### AsExportRequest Implementation

Wrap ResourceMetrics in valid ExportMetricsServiceRequest:
1. Encode field tag (field 1, wire type 2)
2. Encode length prefix
3. Append ResourceMetrics bytes

**Performance**: ~90ns overhead

### Resource Extraction

Extract Resource message from ResourceMetrics:
1. Parse ResourceMetrics to find field 1 (Resource)
2. Extract raw bytes

**Performance**: Negligible (included in split cost)

## Type Composition

Key insight: Types compose naturally

```go
// ExportMetricsServiceRequest wraps complete OTLP message
batch := otlpwire.ExportMetricsServiceRequest(otlpBytes)

// Iterate over resources
resources, getErr := batch.ResourceMetrics()
for resource := range resources {
    // AsExportRequest returns []byte (valid OTLP message)
    exportBytes := resource.AsExportRequest()

    // Cast back to ExportMetricsServiceRequest to use same methods
    singleResourceBatch := otlpwire.ExportMetricsServiceRequest(exportBytes)
    count, _ := singleResourceBatch.DataPointCount()  // Count this resource only
}
if err := getErr(); err != nil {
    return err
}
```

No need for duplicate methods - the API composes!

## Performance Characteristics

Benchmarks (Apple M4, 5 resources, 100 data points each):

| Operation | Time | Memory | Allocations |
|-----------|------|--------|-------------|
| DataPointCount() | ~2 μs | 0 B | 0 allocs |
| ResourceMetrics() iterator | ~32 ns | 0 B | 0 allocs |
| AsExportRequest() | ~160 ns | ~100 B | 1 alloc |

**vs. Full Unmarshaling:**
- Count: 35-52x faster, 100% fewer allocations
- Iteration: ~4000x faster, 100% fewer allocations (no slice container)
- AsExportRequest: ~800x faster than unmarshal+remarshal

## Design Decisions

### Why type aliases, not structs?

```go
// Type alias (chosen)
type ExportMetricsServiceRequest []byte

// vs struct (rejected)
type ExportMetricsServiceRequest struct {
    data []byte
}
```

**Reasons:**
1. Zero overhead - just a name for []byte
2. Easy casting: `ExportMetricsServiceRequest(bytes)`
3. Passes efficiently (just a slice header)
4. Clear it's just bytes underneath

### Why iterators instead of slices?

**Alternatives considered:**
```go
// Slice-based (rejected)
func (m ExportMetricsServiceRequest) SplitByResource() ([]ResourceMetrics, error)

// Iterator with error callback (chosen)
func (m ExportMetricsServiceRequest) ResourceMetrics() (iter.Seq[ResourceMetrics], func() error)
```

**Decision:** Iterators because:
1. **Zero allocations** - No `[]ResourceMetrics` container allocation
2. **Lazy evaluation** - Resources parsed on-demand during iteration
3. **Early exit** - Break stops parsing remaining resources (critical for rate limiting)
4. **Streaming** - Process resources immediately without buffering

**Trade-off:** Requires Go 1.23+, but the performance benefits are worth it for high-throughput pipelines.

### Why error callback instead of iter.Seq2?

**Alternatives considered:**
```go
// iter.Seq2 (rejected)
func (m ExportMetricsServiceRequest) ResourceMetrics() iter.Seq2[ResourceMetrics, error]
for resource, err := range data.ResourceMetrics() {
    if err != nil { return err }
    // process
}

// Error callback (chosen)
func (m ExportMetricsServiceRequest) ResourceMetrics() (iter.Seq[ResourceMetrics], func() error)
resources, getErr := data.ResourceMetrics()
for resource := range resources {
    // process
}
if err := getErr(); err != nil { return err }
```

**Decision:** Error callback because:

1. **Matches Go stdlib patterns** - Same as `bufio.Scanner.Err()` and `sql.Rows.Err()`
2. **Standard `iter.Seq`** - Works with stdlib utilities like `slices.Collect()`
3. **Errors stop iteration** - In practice, errors aren't per-item but "stream stopped"
4. **Enables all-or-nothing** - Can collect all resources first, then process
5. **Semantic clarity** - `iter.Seq2` designed for key-value pairs, not value-error pairs

**Error behavior:**
- Errors occur at protobuf parsing failures (malformed tags, truncated data)
- When error occurs, iteration stops completely
- Can't skip bad resources and continue (data structure corrupted)
- Similar to `Scanner.Scan()` returning false on error

**Usage patterns enabled:**
```go
// Streaming (partial processing OK)
resources, getErr := data.ResourceMetrics()
for resource := range resources {
    sendToWorker(resource) // Process as you go
}
if err := getErr(); err != nil { return err }

// All-or-nothing (no partial processing)
resources, getErr := data.ResourceMetrics()
all := slices.Collect(resources) // Collect ALL first
if err := getErr(); err != nil { return err } // No processing happened yet
for _, resource := range all {
    process(resource) // Process after validation
}
```

### Why only Resource-level iteration?

**Considered:**
- Scope-level iteration
- Metric-level iteration

**Decision:** Resource-level is sufficient because:
1. Sharding is primarily by service/container (= resource)
2. Scope/metric-level iteration has unclear use cases
3. Users can iterate by resource, then unmarshal for finer filtering
4. YAGNI - add later if demand emerges

### Why no built-in hashing?

**Considered:**
```go
func (r ResourceMetrics) ResourceHash() uint64
func (r ResourceMetrics) ContentHash() uint64
```

**Decision:** Let users hash because:
1. Choice of algorithm (FNV? xxHash? CityHash?)
2. Choice of what to hash (Resource? entire data? custom?)
3. Choice of when to hash (routing? deduplication? never?)
4. Avoids 7x performance penalty for users who don't need it

### Why AsExportRequest()?

**Alternatives considered:**
1. Return raw ResourceMetrics bytes (not valid OTLP)
2. Provide separate Wrap() function
3. Auto-wrap in SplitByResource()

**Decision:** Method on ResourceMetrics because:
1. Clear intent: "give me valid OTLP message"
2. On-demand (pay only if you need it)
3. Composes with ExportMetricsServiceRequest casting for counting

## Future Considerations

### Potential Additions (if demand emerges)

1. **Scope-level iteration**
   ```go
   func (r ResourceMetrics) ScopeMetrics() iter.Seq2[ScopeMetrics, error]
   ```

2. **Resource count** (for pre-allocation if needed)
   ```go
   func (m ExportMetricsServiceRequest) ResourceCount() (int, error)
   ```

3. **Filtering helpers**
   ```go
   func FilterResources(data ExportMetricsServiceRequest, fn func(ResourceMetrics) bool) iter.Seq2[ResourceMetrics, error]
   ```

4. **Size estimation**
   ```go
   func (m ExportMetricsServiceRequest) EstimatedSize() int
   ```

### Backward Compatibility

API is designed to be additive:
- New methods can be added to types
- New types can be added (e.g., ScopeMetrics)
- No breaking changes to existing methods

## Testing Strategy

1. **Unit tests** - Test each method independently
2. **Composition tests** - Test type casting and composition
3. **Property tests** - Verify count preservation after split
4. **Benchmark tests** - Performance regression detection
5. **Example tests** - Verify documented usage patterns

## Documentation

1. **README.md** - Overview, installation, quick start
2. **DESIGN.md** - This document (architecture, decisions)
3. **example_test.go** - Runnable examples for godoc
4. **Inline docs** - Method documentation with use cases

## Open Questions

None - design is ready for implementation.

## Approval

- [ ] Reviewed by: @kuklyy
- [ ] Ready to implement: Yes/No
