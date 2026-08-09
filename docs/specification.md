# otlp-wire specification

## Status and authority

This document specifies the product boundary and compatibility contract of
`go.olly.garden/otlp-wire` as of the latest release, v0.1.0, plus changes
merged to `main` and not yet released. Sections that describe behavior a
release does not yet carry say so. This document is the canonical reference
for what the library is for, which behavior consumers may rely on, and what
must remain true through a refactor.

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
| Selected field access | metric name, metric metadata, data-point type/timestamp/attributes, KeyValue fields | severity number and text, resource string attributes | span identifiers, name, kind and timings |

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
        ├── InstrumentationScope
        └── Metric
            └── DataPoint
                └── KeyValue

ExportLogsServiceRequest
└── ResourceLogs
    ├── Resource
    └── ScopeLogs
        ├── InstrumentationScope
        └── LogRecord

ExportTracesServiceRequest
└── ResourceSpans
    ├── Resource
    └── ScopeSpans
        ├── InstrumentationScope
        └── Span
```

This hierarchy mirrors pdata's object model deliberately: a container holds
exactly one Resource and exactly one InstrumentationScope, and each level's
accessor is named after pdata's. The mirror cannot be exact in one respect —
otlp-wire parses lazily, so every accessor returns `(T, error)` and cannot
chain the way pdata's validated-at-unmarshal accessors do. A flattened
convenience that collapses two error checks is admitted only when a measured
hot path justifies it.

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
`DataPointsSeq`, `MetadataSeq`, `AttributesSeq`, and `LogRecordsSeq`. They
yield `(value, error)` inline, stop after the first error, and avoid the
escaping closures of the ordinary form. Both forms preserve wire order.

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

The unreleased E-2941 change narrows this operation-scoping for one accessor:
`Resource()` must scan the complete container (not just up to the first
Resource field) to find every occurrence to merge, so a malformed field after
the last Resource occurrence is now reported as an error where it previously
was not. See "Resources and attributes" below for the full contract.

### Resources and attributes

`ResourceMetrics.Resource()`, `ResourceLogs.Resource()`, and
`ResourceSpans.Resource()` each return `(Resource, error)`: exactly one typed
Resource per container, matching pdata's object model.

`Resource` is declared as `[]byte` (`type Resource []byte`), so a returned
value assigns to a `[]byte` variable, passes to a `[]byte` parameter, and
appends to a `[][]byte` without conversion. That covers how callers use the
method directly, and every consumer surveyed for E-2941 compiles unchanged.
It is not general source compatibility, though: the *method signature* changed,
so an interface declaring `Resource() ([]byte, error)` is no longer satisfied,
and a method value cannot be assigned to a `func() ([]byte, error)` variable.
Code doing either must be updated.

**Absence is not an error (unreleased, E-2941, v0.1.0).** OTLP declares the
Resource field optional (`ResourceSpans.resource`, `ResourceLogs.resource`,
`ResourceMetrics.resource`: "If this field is not set then no resource info is
known"), and pdata accepts such a container, reporting an empty Resource.
`Resource()` returns `(nil, nil)` for an absent field, matching the existing
convention for other optional fields in this library. A malformed *framing* of
the Resource field — wrong wire type, or a length that overruns the container —
still returns an error.

Be precise about how far that error checking reaches. With a single occurrence,
`Resource()` returns a view of the field's bytes without inspecting their
contents, so a lone occurrence whose interior is corrupt is returned without an
error and fails later, when a semantic accessor such as `StringAttribute` reads
it. That is the same lightweight-view behavior the field had before v0.1.0. With
two or more occurrences the contents of each are checked structurally before
merging, for the correctness reason given below, so the identical corrupt bytes
are rejected there. The asymmetry is deliberate: the library never promised to
validate bytes a caller has not traversed, but it must not assemble a valid
message out of invalid parts.

**Repeated occurrences merge (unreleased, E-2941, v0.1.0).** Protobuf merges
repeated occurrences of a singular message field, and pdata performs this
merge; `Resource()` does too, rather than returning only the first occurrence.
A single occurrence — what every real producer emits — returns a slice
aliasing the container, with no allocation. Two or more occurrences are merged
by concatenating their encoded bodies into one new buffer, which has been
verified byte-equivalent to protobuf's field-by-field merge for singular
message fields. This is the one allocating case in an otherwise zero-copy
accessor; see the amended performance contract below.

**Each occurrence must stand alone.** Before concatenating, every occurrence is
checked to be a structurally well-formed protobuf message on its own. This is a
correctness requirement, not a convenience: a Resource can be split across two
occurrences so that neither half parses independently while their
concatenation does. pdata parses each occurrence separately and rejects such a
payload, so concatenating unchecked would let the wire path accept input the
pdata fallback rejects — exactly the parity consumers running a wire fast path
beside a pdata fallback depend on. The check is structural only (tags parse,
values are contained, the walk ends exactly at the end); it does not
semantically validate the Resource, consistent with the lightweight-view versus
semantic-accessor split described above. It runs only on the multi-occurrence
path, so the single-occurrence hot path is unaffected in both time and
allocations.

**Validation-scope change (unreleased, E-2941, v0.1.0).** Finding every
Resource occurrence to merge correctly requires scanning the complete
container instead of returning as soon as the first Resource field is found.
Consequently, a malformed field located *after* the last Resource occurrence
in the container is now reported as an error, where the previous
first-match-and-return implementation never reached it. This is a narrow,
intentional expansion of validation scope for this one accessor: other
single-field extractors in this library (used for fields that do not merge)
keep the shallow, first-match behavior described under "Parsing and protobuf
behavior" above.

`Resource.Attributes` and `AttributesSeq` expose KeyValues from one Resource
message. `Resource.StringAttribute` parses one Resource and distinguishes a
missing or non-string attribute from a present empty string with its `found`
result. Because `Resource()` already performs the merge described above,
`Resource.StringAttribute` sees every merged occurrence's attributes and
naturally preserves pdata's first-value-wins behavior for duplicate keys
without any merge-specific logic of its own.

**`ResourceLogs.StringAttribute` is removed (unreleased, E-2941, v0.1.0,
breaking).** It predated `Resource()` returning a merged, typed `Resource` and
skipped a level (`ResourceLogs` straight to attribute) with no pdata
analogue. Callers migrate by calling `Resource()` first:

```go
// before
value, found, err := resourceLogs.StringAttribute("service.name")

// after
resource, err := resourceLogs.Resource()
if err != nil { /* ... */ }
value, found, err := resource.StringAttribute("service.name")
```

`KeyValue.Key` and `ValueRaw` are lightweight views of the first matching
encoded field. `KeyValue.StringValue` is the semantic string accessor: it
validates the complete KeyValue and AnyValue structure and applies protobuf
oneof ordering. These two classes of accessor are deliberately different.

### Instrumentation scope and schema URL

**Added in E-2942 (unreleased, v0.1.0).** `ScopeMetrics`, `ScopeLogs`, and
`ScopeSpans` each expose `Scope() (InstrumentationScope, error)`, and all six
containers expose `SchemaUrl() ([]byte, error)`.

`Scope()` carries the identical contract to `Resource()` above — exactly one
per container, absence returns `(nil, nil)`, a single occurrence aliases the
input with no allocation, 2+ occurrences are validated individually and then
merged by concatenation, and the merge requires scanning the whole container.
Every clause in the "Resources and attributes" section applies unchanged,
because pdata treats both fields the same way: it unmarshals each occurrence
into the same object.

`InstrumentationScope.Name`, `Version`, `Attributes`, and `AttributesSeq` read
that message. **Scope attributes are field 3**, where Resource attributes are
field 1; the accessors absorb the difference, but anyone writing new wire code
against this hierarchy must not assume they match.

**Singular scalars resolve to the last occurrence (E-2942).** `SchemaUrl`,
`InstrumentationScope.Name`, `InstrumentationScope.Version`, and `Metric.Name`
return the *last* occurrence when a field is repeated, which is what protobuf
and pdata do. The distinction from the merge rule for singular messages is a
behavioral contract, not an implementation detail: a consumer relying on
first-match resolution would diverge from its pdata fallback on the same
bytes. [DESIGN.md](DESIGN.md) records why.

Two consequences follow, both mirroring the `Resource()` change: these
accessors scan the whole enclosing message, so a malformed field located
*after* the last occurrence is reported as an error; and cost grows with the
enclosing message's field count while staying allocation-free.

**Where the widened scan is stricter than pdata.** Skipping a field applies
this library's group validation, which requires a start-group wire type to
have its matching end-group marker. pdata's unknown-field skip does not: it
returns as soon as it consumes a non-group wire type, even inside an unclosed
group. A payload carrying an unclosed group in an unknown field *after* the
last occurrence of a scanned field is therefore an error here and accepted by
pdata. This is the one direction in which the wire path and the pdata
fallback disagree, it is the safe direction, and it is deliberate: `AGENTS.md`
requires that corruption never be silently accepted to keep a walk moving.
Groups are a proto2 construct that OTLP never emits, so the shapes affected
are malformed or adversarial rather than anything a conformant producer
emits. `Metric.Name` is newly subject to this because it now scans; before
E-2942 it returned at the first `name` field and never reached such a field.
`TestMetricName_UnclosedGroupIsStricterThanPdata` pins the divergence so it
stays a decision rather than an accident.

**`Metric.Name` behavior change (unreleased, E-2942, v0.1.0).** It previously
returned the first occurrence. That diverged from pdata in exactly the way
`Resource()` did before E-2941, so it was corrected rather than left
inconsistent with the accessors added beside it. The signature is unchanged
and no consumer surveyed depends on the old resolution; a producer emitting a
repeated `Metric.name` was already outside what its pdata fallback would agree
with. `KeyValue.Key` and `ValueRaw` keep first-match semantics deliberately,
as documented above — they are hashing-oriented views, not parity accessors.

### Metrics depth

Metrics traversal supports gauge, sum, histogram, exponential histogram, and
summary bodies. A yielded `DataPoint` retains its `MetricType`; callers must
not infer the data-point wire layout independently.

`Metric.Name`, `Metadata`, `MetadataSeq`, `DataPoint.Raw`, `Timestamp`,
`Attributes`, `AttributesSeq`, `KeyValue.Key`, and `ValueRaw` return views or
scalar values without a full metric decode. If a Metric encodes multiple
recognized body fields, the current traversal yields data points from each body
in wire order and tags each with its body type.

`Metric.Metadata` and `MetadataSeq` iterate `Metric.metadata`, field 12. OTLP
declares it `repeated`, so the singular-field resolution rules below do not
apply: every occurrence is yielded in wire order, duplicate keys included.

### Log depth

Logs traversal preserves resource, scope and record order.
`LogRecord.SeverityNumber` returns zero when the field is absent, represents
the enum as `int32`, and applies protobuf last-scalar-value behavior. It scans
and validates the complete LogRecord fields known to the pinned pdata schema,
including nested body and attributes, while skipping well-formed unknown
fields.

`LogRecord.SeverityText` returns `severity_text` (field 3) as raw bytes
aliasing the request buffer, with the capacity clamped so a caller's `append`
cannot overwrite adjacent record fields. It returns `nil` when the field is
absent and a non-nil zero-length slice when it is present but empty, a
distinction pdata cannot represent; both compare equal to `""`. Repeated
occurrences resolve to the last one, as protobuf and pdata do for a singular
scalar.

`LogRecord.Severity` returns both fields at once, each carrying the contract
its single-field accessor states. It is the accessor for consumers reading both
per record: every accessor runs the walk once, so the single-field pair costs
two walks where `Severity` costs one.

All three severity accessors read from one schema-aware walk of the whole
LogRecord, so they accept and reject exactly the same bytes.

The library does not classify severity bands, and `Severity` returning the two
fields together does not rank them: which one wins when the number and the text
disagree remains consumer policy.

### Trace depth

Trace traversal exposes scopes and spans. `TraceID`, `SpanID`, and
`ParentSpanID` return fixed-width arrays, return the zero value when the field
is absent, and reject malformed or incorrectly sized identifiers. They select
the **first** matching occurrence and stop there, so an empty occurrence yields
the zero value only when it is that first one; later occurrences are never
examined. On conformant OTLP, which carries each singular field exactly once,
this is indistinguishable from pdata.

`Span.Name` returns `name` (field 5) as raw bytes aliasing the request buffer,
with the capacity clamped so a caller's `append` cannot overwrite adjacent
span fields. It returns `nil` when the field is absent and a non-nil
zero-length slice when it is present but empty, a distinction pdata cannot
represent. The name is not checked for UTF-8 validity: pdata accepts an
invalid-UTF-8 span name, so rejecting one here would make the wire path
stricter than the path it mirrors, and that check remains consumer policy.

`Span.Kind` returns the `kind` enum (field 6) as `int32`, keeping an
unexpected negative value distinguishable from the defined OTLP range, as
`SeverityNumber` does. `Span.StartTimeUnixNano` and `Span.EndTimeUnixNano`
return the `fixed64` timestamps in fields 7 and 8. All four return the zero
value when absent and resolve repeated occurrences to the last one.

Those four read from one schema-aware walk of the whole Span, so they accept
and reject exactly the same bytes. Each call runs that walk once, so a consumer
reading all four pays for four. Unlike the LogRecord walk, this one is
schema-aware at the Span level and framing-only below it: `attributes`,
`events`, `links` and `status` are checked for wire type and containment, but
their contents are not parsed. Cost therefore tracks a span's field count
rather than its payload size.

The identifier accessors scan first-match and stop, so they do not walk to the
end of the span and do not report corruption located after the identifier they
read. On conformant OTLP that is indistinguishable from the scalar accessors'
resolution, because each singular field occurs exactly once. On malformed input
the two differ; [operations.md](../operations.md) tabulates exactly how.

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
- `DataPointsSeq`, `MetadataSeq`, `AttributesSeq` (on both `Resource` and
  `InstrumentationScope`), and `LogRecordsSeq` remain zero-allocation on their
  per-element paths.
- Accessors return aliased slices rather than copying payload data, with one
  documented exception: `Resource()` and `Scope()` alias the input when a
  container holds a single occurrence of that field (the case every real
  producer emits) and allocate a concatenated buffer only when merging 2+
  occurrences (unreleased, E-2941 and E-2942, v0.1.0). See "Resources and
  attributes" and "Instrumentation scope and schema URL" above, and the
  `SingleOccurrence` / `MultipleOccurrences` benchmarks in
  [BENCHMARKS.md](BENCHMARKS.md).
- Accessors that must scan a whole message to resolve a singular field
  (`Resource()`, `Scope()`, and the last-value-wins scalars) cost time
  proportional to the enclosing message's field count, not its payload size,
  and remain allocation-free.
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
| Marigold | Deep metrics traversal and hashing | Metric names, metric metadata, all five metric types, data-point timestamps, raw KeyValue bytes, zero-allocation inner iterators |
| Loam | Per-resource cache short-circuit for all signals | Resource-container order, raw Resource extraction, re-wrapping selected containers, safe fallback on parse errors |
| Mulch | Wire-level log severity gate before selective pdata decode | Resource string semantics, scope/record order, severity parsing, retained raw record bytes |
| Bindweed | Log-severity distribution without full pdata decode | Complete log traversal, service-context strings, severity parity, malformed-input behavior |
| Sage | Missing-severity detection with selective fallback | Severity scalar semantics, resource attributes, full traversal/fallback boundary |
| Chaff | Resource cache short-circuit and metric-name extraction | Resource bytes, scope/metric traversal, metric names, error callback behavior |
| Dibber, Gaps, Overstory | Partial trace processing | Resource/scope/span traversal and the Span field accessors |
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
| v0.1.0 (released 2026-08-07) | Typed, absence-tolerant, merged `Resource()`; removes `ResourceLogs.StringAttribute` (E-2941) | Breaking: `Resource()` now returns `(Resource, error)` (direct calls stay compatible since `Resource` assigns to `[]byte`, but interfaces and method values typed on the old signature must be updated) and no longer errors on an absent Resource field; `rl.StringAttribute(k)` callers migrate to `rl.Resource()` then `res.StringAttribute(k)`. Release the primitive, then coordinate consumer releases per the acceptance gates below before broad upgrades |

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
