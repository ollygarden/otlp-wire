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
func (m ExportMetricsServiceRequest) Count() int
// Returns total number of metric data points in the entire batch
// Use case: Rate limiting entire batch

func (m ExportMetricsServiceRequest) SplitByResource() []ResourceMetrics
// Splits batch into separate resources for sharding/parallel processing
// Use case: Fan out to workers, route by service
```

**ResourceMetrics methods:**
```go
func (r ResourceMetrics) Resource() []byte
// Returns raw Resource message bytes (attributes like service.name, host.name)
// Use case: Hash for routing, unmarshal for filtering

func (r ResourceMetrics) AsExportRequest() []byte
// Wraps ResourceMetrics in valid ExportMetricsServiceRequest
// Use case: Send to OTLP endpoint, count signals in this resource
```

**Same pattern for Logs and Traces** (ExportLogsServiceRequest, ResourceLogs, ExportTracesServiceRequest, ResourceSpans)

## Use Cases

### 1. Global Rate Limiting
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
count := data.Count()

if count > globalLimit {
    return errors.New("rate limit exceeded")
}

processMetrics(data)
```

### 2. Per-Service Sharding
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

for _, resource := range data.SplitByResource() {
    // Hash resource for consistent routing
    hash := fnv64a(resource.Resource())
    workerID := hash % numWorkers

    // Send valid OTLP message to worker
    sendToWorker(workerID, resource.AsExportRequest())
}
```

### 3. Per-Service Rate Limiting
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

for _, resource := range data.SplitByResource() {
    // Count signals in this resource
    count := otlpwire.ExportMetricsServiceRequest(resource.AsExportRequest()).Count()

    // Extract service name for limit lookup
    svc := extractServiceName(resource.Resource())

    if count > serviceLimit[svc] {
        reject(resource)
    }
}
```

### 4. Attribute-Based Filtering
```go
data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

for _, resource := range data.SplitByResource() {
    // Unmarshal just the Resource (small, cheap)
    res := unmarshalResource(resource.Resource())

    if res.Attributes["environment"] == "production" {
        sendToProdPipeline(resource.AsExportRequest())
    }
}
```

### 5. Deduplication
```go
seenHashes := make(map[uint64]bool)

for _, resource := range data.SplitByResource() {
    // Hash entire resource data for deduplication
    hash := xxhash(resource.AsExportRequest())

    if seenHashes[hash] {
        continue // Duplicate - skip
    }

    seenHashes[hash] = true
    process(resource.AsExportRequest())
}
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

### Split Implementation

Extract ResourceMetrics as raw bytes:
1. Parse top-level to find each ResourceMetrics field
2. Extract raw bytes for each (using protowire.ConsumeBytes)
3. Return as []ResourceMetrics

**Performance**: ~800ns for 5 resources

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

// Split returns []ResourceMetrics
resources := batch.SplitByResource()

// AsExportRequest returns []byte (valid OTLP message)
exportBytes := resources[0].AsExportRequest()

// Cast back to ExportMetricsServiceRequest to use same methods
singleResourceBatch := otlpwire.ExportMetricsServiceRequest(exportBytes)
count := singleResourceBatch.Count()  // Count this resource only
```

No need for duplicate methods - the API composes!

## Performance Characteristics

Benchmarks (Apple M4, 5 resources, 100 data points each):

| Operation | Time | Memory | Allocations |
|-----------|------|--------|-------------|
| Count() | ~800 ns | 0 B | 0 allocs |
| SplitByResource() | ~800 ns | 7 KB | 10 allocs |
| AsExportRequest() | ~90 ns | ~100 B | 1 alloc |

**vs. Full Unmarshaling:**
- Count: 90% faster, 98% fewer allocations
- Split: Still faster than unmarshal + re-marshal

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

### Why only Resource-level splitting?

**Considered:**
- Scope-level splitting
- Metric-level splitting

**Decision:** Resource-level is sufficient because:
1. Sharding is primarily by service/container (= resource)
2. Scope/metric-level splitting has unclear use cases
3. Users can split by resource, then unmarshal for finer filtering
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

1. **Scope-level splitting**
   ```go
   func (r ResourceMetrics) SplitByScope() []ScopeMetrics
   ```

2. **Batch merging**
   ```go
   func MergeMetrics(batches []ExportMetricsServiceRequest) ExportMetricsServiceRequest
   ```

3. **Filtering helpers**
   ```go
   func (m ExportMetricsServiceRequest) FilterResources(fn func([]byte) bool) ExportMetricsServiceRequest
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
