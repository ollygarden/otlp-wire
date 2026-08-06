# otlp-wire specification

## Status and authority

This document specifies the product boundary and compatibility contract of
`go.olly.garden/otlp-wire` as of v0.0.4. It is the canonical reference for
what the library is for, which behavior consumers may rely on, and what must
remain true through a refactor.

The other repository documents have narrower roles:

- [README.md](../README.md) is the installation and usage entry point.
- [DESIGN.md](DESIGN.md) explains the current implementation and its trade-offs.
- [BENCHMARKS.md](BENCHMARKS.md) records measured performance and methodology.
- Go documentation and executable examples define the exact exported API.

If prose conflicts with code, tests, or Go documentation, treat that as
documentation drift and reconcile it before changing behavior.

## Product purpose

OllyGarden services often receive complete OTLP protobuf requests but need only
a small part of each request to make an early decision: count records, look up
a service-context cache key, split work by resource, hash metric attributes, or
classify log severity. Fully unmarshaling those requests into pdata creates an
object graph for data the caller may immediately discard.

`otlp-wire` provides typed, zero-copy views over OTLP protobuf bytes so callers
can perform those bounded operations before deciding whether a full decode is
necessary. In production this boundary is used to reduce CPU, allocation and
garbage-collection pressure in detector hot paths, and to keep consumers from
falling behind telemetry ingestion.

The library is intentionally a set of parsing primitives. It does not own
transport, decompression, acknowledgements, retries, caching, hashing,
detector policy, or insight publication.

## Scope

The library supports the following operation families:

| Family | Metrics | Logs | Traces |
| --- | --- | --- | --- |
| Count records in a request or resource container | data points | log records | spans |
| Iterate resource containers | `ResourceMetrics` | `ResourceLogs` | `ResourceSpans` |
| Extract raw Resource bytes | yes | yes | yes |
| Re-wrap one resource container as an export request | yes | yes | yes |
| Iterate below the resource container | scopes, metrics, data points, attributes | scopes, log records | scopes, spans |
| Selected field access | metric name, data-point type/timestamp/attributes, KeyValue fields | severity number, resource string attributes | trace, span and parent-span IDs |

### Non-goals

`otlp-wire` is not:

- a complete OTLP implementation or a replacement for pdata;
- a general attribute query or transformation engine;
- an OTLP validity oracle for bytes a caller does not traverse;
- a routing, sampling, aggregation, cache or detector framework;
- a transport, compression or persistence library;
- a promise to expose every field in the OTLP schema.

New accessors should be added only for demonstrated consumer needs. Callers
that need arbitrary mutation, full semantic interpretation, or whole-message
validation should use the official OpenTelemetry data APIs.

## Data and ownership model

Most exported wire types are named `[]byte` views. Constructing a request view,
iterating it, and returning nested byte slices does not copy the payload.

The caller owns the backing bytes and must keep them alive and unchanged while
any request, resource, scope, record, span, metric, data point, KeyValue, or
returned byte slice is in use. Retaining a nested value retains the original
payload. A caller that needs independent ownership must copy explicitly.

`DataPoint` is the exception to the named-slice representation. It carries both
the raw data-point bytes and the metric body type because OTLP uses different
attribute field numbers for different data-point messages.

## Public hierarchy

```text
ExportMetricsServiceRequest
└── ResourceMetrics
    ├── Resource
    └── ScopeMetrics
        └── Metric
            └── DataPoint
                └── KeyValue

ExportLogsServiceRequest
└── ResourceLogs
    ├── Resource
    └── ScopeLogs
        └── LogRecord

ExportTracesServiceRequest
└── ResourceSpans
    ├── Resource
    └── ScopeSpans
        └── Span
```

The exported methods are listed in [README.md](../README.md) and verified by
[example_test.go](../example_test.go). The contracts below apply across that
surface.

## Behavioral contracts

### Counting

`DataPointCount`, `LogRecordCount`, and `SpanCount` return the number of nested
signal records represented by the request or resource container. Counting
walks the relevant protobuf hierarchy and returns an error for malformed
structure encountered on that walk.

Counting must remain allocation-free for valid inputs. A refactor may improve
speed, but must not silently change which metric bodies or repeated fields are
counted.

### Iteration and errors

The ordinary iterator contract is:

```go
seq, errFunc := value.Children()
for child := range seq {
    // use child
}
if err := errFunc(); err != nil {
    return err
}
```

The error function must be checked after iteration, including after an early
exit. Iteration is lazy: breaking early stops parsing, so corruption later in
the unvisited input cannot be reported.

Hot per-element paths additionally expose yield-based methods such as
`DataPointsSeq`, `AttributesSeq`, and `LogRecordsSeq`. They yield `(value,
error)` inline, stop after the first error, and avoid the escaping closures of
the ordinary form. Both forms preserve wire order.

Changing the ordinary `(iter.Seq[T], func() error)` shape or the inline-error
shape of the hot variants is a breaking API change.

### Parsing and protobuf behavior

Methods consume protobuf tags directly and must:

- reject malformed tags, lengths, groups and consumed values;
- reject an unexpected wire type when a typed accessor or iterator selects a
  known field;
- skip well-formed unknown fields for forward compatibility;
- preserve repeated-field wire order;
- stop rather than continue after a parsing error;
- avoid panics and unbounded recursion on adversarial input.

Validation is operation-scoped. A shallow resource iterator validates enough
of the top-level request to locate resource containers, but it does not
semantically validate every nested field inside each returned container.
Consumers that require full-request validity must traverse the required fields
or fall back to pdata.

In v0.0.4, `DataPointCount` skips a recognized metric-body field with a
non-length-delimited wire type while `Metric.DataPoints` and `DataPointsSeq`
reject it. The unreleased E-2928 change aligns the count path with the
iterators, preventing corrupt input from being reported as a valid
zero-data-point metric. Update this document's version boundary when that
change is released.

### Resources and attributes

`Resource()` returns the raw embedded Resource message without copying it.
The current API reports an error when that field is absent or malformed.

`Resource.Attributes` and `AttributesSeq` expose KeyValues from one Resource
message. `Resource.StringAttribute` parses one Resource and distinguishes a
missing or non-string attribute from a present empty string with its `found`
result.

`ResourceLogs.StringAttribute` operates on a complete `ResourceLogs` message.
It merges repeated singular Resource fields in wire order and preserves the
first value for duplicate attribute keys, matching the pdata behavior required
by log consumers. Later Resource occurrences are still validated.

`KeyValue.Key` and `ValueRaw` are lightweight views of the first matching
encoded field. `KeyValue.StringValue` is the semantic string accessor: it
validates the complete KeyValue and AnyValue structure and applies protobuf
oneof ordering. These two classes of accessor are deliberately different.

### Metrics depth

Metrics traversal supports gauge, sum, histogram, exponential histogram, and
summary bodies. A yielded `DataPoint` retains its `MetricType`; callers must
not infer the data-point wire layout independently.

`Metric.Name`, `DataPoint.Raw`, `Timestamp`, `Attributes`, `AttributesSeq`,
`KeyValue.Key`, and `ValueRaw` return views or scalar values without a full
metric decode. If a Metric encodes multiple recognized body fields, the
current traversal yields data points from each body in wire order and tags
each with its body type.

The metric metadata field is not currently exported as a dedicated accessor.
Consumers needing it either parse that bounded field themselves or use pdata.

### Log depth

Logs traversal preserves resource, scope and record order.
`LogRecord.SeverityNumber` returns zero when the field is absent, represents
the enum as `int32`, and applies protobuf last-scalar-value behavior. It scans
and validates the complete LogRecord fields known to the pinned pdata schema,
including nested body and attributes, while skipping well-formed unknown
fields.

The library does not classify severity bands or combine severity number with
severity text. That remains consumer policy.

### Trace depth

Trace traversal exposes scopes and spans. `TraceID`, `SpanID`, and
`ParentSpanID` return fixed-width arrays, return the zero value when the field
is absent, and reject malformed or incorrectly sized identifiers.

### Re-wrapping resource containers

Each resource-container type implements `io.WriterTo`. `WriteTo` writes a valid
export request containing that one resource container by adding only the
top-level repeated-field tag and length. The container bytes themselves remain
unchanged. The method returns the byte count reported by the writer and
propagates writer errors. v0.0.4 does not synthesize `io.ErrShortWrite` when a
writer incorrectly returns a short count with a nil error.

This is the supported building block for per-resource sharding and selective
full decoding. It does not preserve sibling resource containers from the
original request.

## Performance contract

Correctness takes precedence over a benchmark result, but allocation behavior
is part of this library's purpose and compatibility surface.

- Counting valid requests is zero-allocation.
- Ordinary iterator opens have the documented small closure cost.
- `DataPointsSeq`, `AttributesSeq`, and `LogRecordsSeq` remain zero-allocation
  on their per-element paths.
- Accessors return aliased slices rather than copying payload data.
- Production code must not introduce pdata or generated-message unmarshaling.

Exact timings are environment-specific, not API guarantees. Any performance
claim must identify the machine, Go version, fixture, command and comparison
method. See [BENCHMARKS.md](BENCHMARKS.md) for the recorded measurements.

## Prominent consumers

The following consumers define the most important compatibility constraints.
The list is not an ownership boundary; repository-wide GitHub search should be
repeated before a breaking change.

| Consumer | Main use of otlp-wire | Compatibility-sensitive behavior |
| --- | --- | --- |
| Marigold | Deep metrics traversal and hashing | Metric names, all five metric types, data-point timestamps, raw KeyValue bytes, zero-allocation inner iterators |
| Loam | Per-resource cache short-circuit for all signals | Resource-container order, raw Resource extraction, re-wrapping selected containers, safe fallback on parse errors |
| Mulch | Wire-level log severity gate before selective pdata decode | Resource string semantics, scope/record order, severity parsing, retained raw record bytes |
| Bindweed | Log-severity distribution without full pdata decode | Complete log traversal, service-context strings, severity parity, malformed-input behavior |
| Sage | Missing-severity detection with selective fallback | Severity scalar semantics, resource attributes, full traversal/fallback boundary |
| Chaff | Resource cache short-circuit and metric-name extraction | Resource bytes, scope/metric traversal, metric names, error callback behavior |
| Dibber, Gaps, Overstory | Partial trace processing | Resource/scope/span traversal and fixed-width trace identifiers |
| Fig, Nameplate, Seedtray | Generic resource extraction, counting or splitting | Symmetry across all three signals, raw-byte ownership, `WriteTo` output |

At the time of this audit, repository-wide GitHub search found direct source
imports in 12 OllyGarden services: Bindweed, Chaff, Dibber, Fig, Gaps, Loam,
Marigold, Mulch, Nameplate, Overstory, Sage and Seedtray. A source-compatible
change can still be behaviorally breaking for these services.

## Release and rollout model

Past feature rollouts established the following sequence:

| Release | Library capability | Consumer rollout pattern |
| --- | --- | --- |
| v0.0.2 | Span-level trace access (PR #2) | Adopt partial trace processing in detector services |
| v0.0.3 | Metrics-depth traversal and zero-allocation variants (E-2608, PR #18) | Release the primitive first, then migrate metrics consumers such as Marigold with parity and production evidence |
| v0.0.4 | Log traversal, severity and resource strings (E-2892, PR #22) | Release the primitive first, then use separate Bindweed, Mulch and Sage adoption issues (E-2900, E-2905, E-2906) |

Future capabilities and refactors should follow the same staged model:

1. Start from a measured consumer problem and identify the smallest wire
   contract that solves it.
2. Add or refactor the library with pdata-built differential fixtures,
   malformed-wire tests, allocation checks and paired `-benchmem` benchmarks.
3. Merge and tag the library independently. Do not couple a library release to
   several production service changes.
4. Upgrade one consumer in a separate change. Preserve transport,
   decompression, acknowledgement, retry, telemetry and publication behavior.
5. Compare old and new paths with parity tests and representative benchmarks.
6. Record a production baseline, canary the consumer, and compare throughput,
   backlog slope, CPU throttling, allocation/GC behavior, errors and
   redeliveries.
7. Roll back or fall back to pdata if correctness, delivery semantics, or
   production health regresses. Do not claim rollout success from library
   microbenchmarks alone.

## Refactor acceptance gates

A refactor is acceptable only if it preserves the contracts above or is
explicitly released as a breaking change with consumer migrations.

At minimum:

1. Inventory all direct consumers and the exact methods they call at current
   default-branch heads.
2. Preserve exported names, signatures, iterator shapes, error timing,
   raw-byte aliasing, wire order and `WriteTo` output.
3. Run differential fixtures against pdata for valid, duplicate, reordered,
   omitted, unknown and malformed fields across metrics, logs and traces.
4. Cover early iterator termination and deferred error checks.
5. Keep counting and hot iterator allocation gates, and compare paired
   benchmarks under one environment.
6. Run this repository's race tests and vet plus focused tests in Marigold,
   Loam, Mulch, Bindweed, Sage, Chaff and the trace/resource consumers.
7. Ship through a tagged prerelease or release and canary a prominent consumer
   before broad upgrades when behavior or hot-path performance could change.

Breaking changes require a migration plan, a Conventional Commit and pull
request marked as breaking, and coordinated consumer releases. A major version
is the default boundary for incompatible exported API changes.
