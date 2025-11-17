# wireformat Library Design

## Problem Statement

Services receiving OTLP data need to:
1. Count signals for observability
2. Split batches for parallel processing
3. Extract metadata for routing

Current approaches require full unmarshaling, which is expensive:
- High CPU usage (parsing entire protobuf structure)
- High memory allocation (creating Go objects for all data)
- Garbage collector pressure from thousands of short-lived objects
- Unnecessary overhead when only making routing/sharding decisions

## Goals

1. **Fast signal counting** without unmarshaling
2. **Resource-level batch splitting** for sharding/parallel processing
3. **Metadata extraction** (Resource bytes) for routing decisions
4. **Minimal API** - provide tools, not opinions
5. **Zero-copy where possible** - work on raw bytes

## Non-Goals

1. **Not a complete OTLP parser** - use official libraries for full deserialization
2. **Not attribute-aware** - we don't read/filter by attribute values
3. **Not scope/metric-level splitting** - only resource-level granularity is provided
4. **Not a query language** - no path expressions or complex filters

## Core Principle

This library gives you:
- Raw bytes at different granularity levels
- Methods to count, split, and extract
- Building blocks for your use case

It does not:
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
func (r ResourceMetrics) DataPointCount() (int, error)
// Returns number of metric data points in this resource (zero allocation)

func (r ResourceMetrics) Resource() ([]byte, error)
// Returns raw Resource message bytes (attributes like service.name, host.name)

func (r ResourceMetrics) WriteTo(w io.Writer) (int64, error)
// Writes ResourceMetrics as valid ExportMetricsServiceRequest
```

**Same pattern for Logs and Traces:**
- `ExportLogsServiceRequest.LogRecordCount() (int, error)`
- `ExportLogsServiceRequest.ResourceLogs() (iter.Seq[ResourceLogs], func() error)`
- `ExportTracesServiceRequest.SpanCount() (int, error)`
- `ExportTracesServiceRequest.ResourceSpans() (iter.Seq[ResourceSpans], func() error)`
- `ResourceLogs.LogRecordCount() (int, error)`
- `ResourceSpans.SpanCount() (int, error)`

## Use Cases

### 1. Observability Metrics
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
count, err := data.DataPointCount()
if err != nil {
    return err
}

metrics.RecordDataPointsReceived(count)
metrics.RecordBatchSize(len(otlpBytes))

processMetrics(data)
```

### 2. Per-Service Sharding
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

resources, getErr := data.ResourceMetrics()
for resource := range resources {
    resourceBytes, _ := resource.Resource()
    hash := fnv64a(resourceBytes)
    partition := hash % numPartitions

    resource.WriteTo(kafka.Writer(topic, partition))
}
if err := getErr(); err != nil {
    return err
}
```

### 3. Per-Service Observability Metrics
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

resources, getErr := data.ResourceMetrics()
for resource := range resources {
    // Count signals in this resource
    var buf bytes.Buffer
    resource.WriteTo(&buf)
    count, _ := otlpwire.ExportMetricsServiceRequest(buf.Bytes()).DataPointCount()

    // Extract service name for metrics attribution
    svc := extractServiceName(resource.Resource())

    metrics.RecordServiceDataPoints(svc, count)
    metrics.RecordServiceBatchSize(svc, buf.Len())
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
    resourceBytes, _ := resource.Resource()
    res := unmarshalResource(resourceBytes)

    if res.Attributes["environment"] == "production" {
        var buf bytes.Buffer
        resource.WriteTo(&buf)
        sendToProdPipeline(buf.Bytes())
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
    var buf bytes.Buffer
    resource.WriteTo(&buf)
    hash := xxhash(buf.Bytes())

    if seenHashes[hash] {
        continue // Duplicate - skip
    }

    seenHashes[hash] = true
    process(buf.Bytes())
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

### WriteTo Implementation

Write ResourceMetrics as valid ExportMetricsServiceRequest to io.Writer:
1. Encode field tag (field 1, wire type 2)
2. Encode length prefix
3. Write ResourceMetrics bytes

**Performance**: Included in iterator time (~50ns total for iterate + write)

### Resource Extraction

Extract Resource message from ResourceMetrics:
1. Parse ResourceMetrics to find field 1 (Resource)
2. Extract raw bytes

**Performance**: Negligible (included in split cost)

## Type Composition

Key insight: Symmetric API at batch and resource levels

```go
// Count at batch level
batch := otlpwire.ExportMetricsServiceRequest(otlpBytes)
totalCount, _ := batch.DataPointCount()

// Iterate and count at resource level (zero allocation)
resources, getErr := batch.ResourceMetrics()
for resource := range resources {
    // Count this resource directly (no buffer needed)
    count, _ := resource.DataPointCount()

    // Or serialize and send to worker
    var buf bytes.Buffer
    resource.WriteTo(&buf)
    sendToWorker(buf.Bytes())
}
if err := getErr(); err != nil {
    return err
}
```

The API provides the same counting methods at both levels, enabling zero-allocation patterns for per-resource observability.

## Performance Characteristics

Benchmarks (Apple M4, 5 resources, 100 data points each):

| Operation | Time | Memory | Allocations |
|-----------|------|--------|-------------|
| DataPointCount() | ~2.3 μs | 0 B | 0 allocs |
| ResourceMetrics() iterator | ~56 ns | 24 B | 2 allocs |
| WriteTo() | included in iterator time | 0 B | 0 allocs |

**vs. Full Unmarshaling:**
- Count: 35-55x faster, zero allocations, no GC pressure
- Iteration: 1,100-2,800x faster, minimal allocations (2 per batch for error handling)
- Split (Iterate + WriteTo): 2,800-3,700x faster than unmarshal+remarshal

**Garbage Collector Impact:**
- Unmarshaling a 500-datapoint batch creates 5,000+ objects (ResourceMetrics, ScopeMetrics, Metric, DataPoint, attribute maps)
- Wire format operations create 0-2 objects per batch
- Significantly reduced GC pause frequency and duration in high-throughput scenarios

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
1. **Minimal allocations** - No `[]ResourceMetrics` container allocation
2. **Lazy evaluation** - Resources parsed on-demand during iteration
3. **Early exit** - Break stops parsing remaining resources
4. **Streaming** - Process resources immediately without buffering

**Trade-off:** Requires Go 1.23+ for iterator support.

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

1. Matches Go stdlib patterns (`bufio.Scanner.Err()`, `sql.Rows.Err()`)
2. Works with stdlib utilities like `slices.Collect()`
3. Errors stop iteration completely (not per-item errors)
4. Enables all-or-nothing pattern (collect first, then process)
5. `iter.Seq2` is for key-value pairs, not value-error pairs

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

### Why resource-level counting?

**Use case:** Per-service observability metrics (cardinality tracking, billing attribution)

```go
resources, getErr := data.ResourceMetrics()
for resource := range resources {
    count, _ := resource.DataPointCount()  // Zero allocation
    svc := extractServiceName(resource.Resource())
    metrics.RecordServiceCardinality(svc, count)
}
```

**Decision:** Add resource-level counting because:
1. **Zero allocation** - No buffer needed for per-resource counts
2. **Symmetric API** - Same methods available at both batch and resource levels
3. **Code reuse** - Just exposes existing `countInResource*` helper functions
4. **Real use case** - Observability/billing metrics per service

**Alternative considered:** Force users to use `WriteTo` → cast back to batch type
- Rejected: Requires allocation, defeats zero-copy principle

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

### Why WriteTo()?

**Alternatives considered:**
1. Return raw ResourceMetrics bytes (not valid OTLP)
2. Return `[]byte` wrapping method like AsExportRequest()
3. Auto-wrap in iterator

**Decision:** WriteTo implements io.WriterTo because:
1. Standard Go interface - works with any io.Writer
2. Avoids unnecessary allocations - write directly to network/buffer
3. Clear intent: write this resource as valid OTLP message
4. On-demand (pay only if you need it)

## Future Considerations

### Potential Additions (if demand emerges)

1. **Scope-level iteration**
   ```go
   func (r ResourceMetrics) ScopeMetrics() (iter.Seq[ScopeMetrics], func() error)
   ```

2. **Resource count** (for pre-allocation)
   ```go
   func (m ExportMetricsServiceRequest) ResourceCount() (int, error)
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

