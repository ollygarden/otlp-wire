# otlp-wire design

This document describes the current implementation and its design decisions.
The durable product and compatibility contract lives in
[specification.md](specification.md).

## Design objective

Many OTLP consumers need a bounded subset of a protobuf request before they
know whether a full decode is worthwhile. The implementation therefore walks
protobuf wire bytes directly, returning typed views into the caller's buffer
and allocating only where the Go iterator contract requires it.

The central constraint is that performance may not weaken correctness. Every
operation validates the structure it consumes and stops on malformed input.
Typed accessors and iterators reject wrong wire types for fields they select;
unknown well-formed fields are skipped so newer OTLP producers remain
compatible.

## Package shape

The repository is one Go module and one package. Production implementation is
split by domain: public wire types, metrics, logs, traces, shared attribute
semantics, and low-level wire helpers. The file boundary is organizational,
not an abstraction layer; all code remains in package `otlpwire`. pdata is used
only in tests and benchmarks as a fixture builder and semantic oracle.

The public wire hierarchy is:

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

Request, resource, scope, record, span, metric, Resource, InstrumentationScope
and KeyValue types are named byte slices. They are cheap to construct and make zero-copy ownership
visible. `DataPoint` is a small struct because it must retain the metric body
type that identifies the body and determines its attribute layout.

## Wire walking

Parsing composes a small set of helpers around
`google.golang.org/protobuf/encoding/protowire`:

- repeated-field counting;
- repeated-field iteration;
- length-delimited, fixed-width and scalar extraction, in first-match and
  last-value-wins forms;
- merged singular-message extraction and resource re-wrapping;
- field skipping with matching-group validation;
- bounded semantic parsing for KeyValue, AnyValue, Resource and LogRecord.

Field numbers and expected wire types come from the upstream OTLP protobuf
schema. A refactor should centralize those facts where useful, but must keep
wrong-wire-type handling explicit at the point a known field is consumed.

Parsing is deliberately operation-scoped. For example, top-level resource
iteration locates length-delimited resource containers but does not validate
every nested message within them. A consumer may progressively traverse the
fields it needs or hand selected bytes to pdata for authoritative full decode.

## Iterators

### Ordinary form

Most public iterators return `(iter.Seq[T], func() error)`. The sequence keeps
the range syntax simple; the error closure captures a parse error discovered
during lazy traversal. It must be checked after the range.

This form costs two small allocations when opened because its closures escape.
At outer levels that cost is paid once per request, resource or scope and is
small compared with materializing a pdata object graph.

### Hot-path form

Opening an ordinary iterator once per metric or log record would multiply the
closure cost by thousands of elements. The deepest hot paths therefore expose
yield methods shaped like `iter.Seq2[T, error]`:

- `Metric.DataPointsSeq`;
- `Metric.MetadataSeq`;
- `DataPoint.AttributesSeq`;
- `Resource.AttributesSeq`;
- `ScopeLogs.LogRecordsSeq`.

These yield errors inline and keep the per-element walk on the stack. The
ordinary methods provide the same external traversal semantics;
`Metric.DataPoints` and `ScopeLogs.LogRecords` are adapters over their hot
forms.

Both forms stop on consumer cancellation or the first parse error. Because
iteration is lazy, an early stop intentionally leaves later bytes unvisited.

## Signal-specific choices

### Metrics

A Metric uses a protobuf oneof for gauge, sum, histogram, exponential
histogram and summary bodies. Every body stores data points in field 1, but the
data-point message types do not place attributes on the same field. The
iterator therefore tags every `DataPoint` with a `MetricType`; attribute access
uses that tag rather than guessing from bytes.

Metric names and KeyValue components are returned as slices of the original
payload. `ValueRaw` exposes the encoded AnyValue for hashing-oriented callers;
`StringValue` performs the more expensive semantic parse when string meaning
is required.

`Metric.Metadata` reads field 12. Cardinality consumers combine it with every
data point's attributes across a whole scrape, so it gets the hot-path
`MetadataSeq` form alongside the ordinary one.

### Logs

Log consumers need both a very cheap per-record path and pdata-compatible
resource context. `LogRecordsSeq` supplies the former. `SeverityNumber` scans
the whole known LogRecord structure so an early severity field cannot hide
trailing corruption, and it implements protobuf last-value-wins scalar
semantics.

Resource context comes from `ResourceLogs.Resource()`, which merges every
Resource occurrence for this container (see "Resource extraction" below) so
callers get pdata-compatible resource attributes without a bespoke
per-container-type accessor. `ResourceLogs.StringAttribute` (removed in
E-2941) previously played that role directly; its replacement is
`rl.Resource()` followed by `Resource.StringAttribute`.

### Traces

Trace access is intentionally narrow. Consumers can iterate scopes and spans,
count spans, and extract the three fixed-width identifiers used by partial
trace detectors. Identifier accessors return arrays to make the required width
part of the Go type and reject incorrectly sized wire values.

## Singular field resolution

This section governs only fields the OTLP protobuf declares *singular*. A
`repeated` field needs no resolution decision: pdata appends each occurrence to
the same slice, which a plain repeated-field walk reproduces. Check the
declaration before reaching for either helper below.

All six containers share one outer protobuf shape. In a resource container
(`ResourceMetrics`/`ResourceLogs`/`ResourceSpans`) field 1 is the Resource;
in a scope container (`ScopeMetrics`/`ScopeLogs`/`ScopeSpans`) field 1 is the
InstrumentationScope. In both, field 2 repeats the children and field 3 is
`schema_url`.

Protobuf resolves a repeated occurrence of a *singular* field two different
ways, and pdata implements both, so otlp-wire must distinguish them:

- a singular **message** merges — pdata unmarshals every occurrence into the
  same object, so contents accumulate;
- a singular **scalar** is replaced — pdata assigns
  (`orig.SchemaUrl = string(...)`), so the last occurrence wins.

Getting this wrong is invisible on well-formed input from a normal producer
and wrong on everything else, which is exactly the class of divergence E-2940
exists to close.

### Singular messages: merged (`Resource`, `InstrumentationScope`)

The three `Resource()` and three `Scope()` methods share one extractor,
`extractMergedMessage`. Its contract, decided for E-2941 and generalized to
scope in E-2942:

- **Absence is not an error.** OTLP declares both fields optional, and pdata
  accepts a container without them, reporting an empty message. The
  extractor returns `(nil, nil)` for absence, matching the convention already
  used by `extractBytesField`, `extractFixed64Field`, and
  `extractFixedBytesField` elsewhere in `wire.go`. A malformed occurrence
  (wrong wire type, bad length) is still an error.
- **Exactly one, merged.** Each container has exactly one pdata-visible
  Resource or scope, matching pdata's object model. Protobuf merges repeated
  occurrences of a singular message field, and pdata performs that merge, so
  the accessors do too instead of returning only the first occurrence (the
  pre-E-2941 behavior).
- **Zero-copy for the case that matters.** A single occurrence is what every
  real producer emits. That case returns a slice aliasing the
  input, with no allocation — this is a hard performance gate, verified by
  `testing.AllocsPerRun` in `resource_test.go` and `scope_test.go` and by
  `BenchmarkResource_SingleOccurrence` and `BenchmarkScope_SingleOccurrence`.
  Two or more occurrences are merged by
  concatenating their encoded bodies into one new buffer. Concatenation was
  verified byte-equivalent to protobuf's field-by-field merge for singular
  message fields (distinct keys, duplicate keys, 3+ occurrences), so it
  reproduces pdata's result without a general recursive merge implementation.
  This is the one documented exception to "accessors never allocate for
  valid, real-world input": multi-occurrence Resource is a real but marginal
  wire shape, and the alternative (a recursive field-by-field merge to stay
  zero-copy) would add real complexity for a case no known producer emits.
- **Occurrences are validated before they are joined.** Concatenation must not
  manufacture validity that was never on the wire. A Resource split across two
  occurrences — the first declaring a length it does not carry, the second
  supplying the remainder — has two halves that each fail to parse alone but
  concatenate into a valid message. pdata parses occurrences independently and
  rejects that payload; joining unchecked would make the wire path accept what
  the pdata fallback refuses. `validateMessageFraming` therefore walks each
  occurrence first. It is structural only and allocation-free, and it runs
  exclusively on the multi-occurrence path, so the single-occurrence hot path
  keeps its measured cost. `TestResource_SplicedOccurrencesRejected` and
  `TestScope_SplicedOccurrencesRejected` pin the behavior against pdata;
  the `ValidOccurrencesStillMerge` tests guard against it becoming
  over-strict.

### Singular scalars: last occurrence wins

`SchemaUrl`, `InstrumentationScope.Name`, `InstrumentationScope.Version` and
`Metric.Name` share `extractLastBytesField`. Because a later occurrence
replaces an earlier one, the extractor cannot stop at the first match; it
records the most recent occurrence and returns after the walk completes.

`Metric.Name` returned the first occurrence before E-2942. That was a
divergence from pdata of the same kind E-2941 fixed for `Resource()`, so it
was corrected here rather than left to differ from the new accessors beside
it.

`KeyValue.Key` and `KeyValue.ValueRaw` keep first-match semantics via
`extractBytesField`, and this is a known divergence rather than an
application of the rule above. Both fields would resolve differently under
it: pdata takes the last `KeyValue.key` (a scalar) and *merges* repeated
`KeyValue.value` occurrences (a message, `orig.Value.UnmarshalProto` into the
same AnyValue). So a KeyValue carrying two `value` occurrences resolves in
pdata to the merged oneof result, while `ValueRaw` returns the first
occurrence's bytes; and `Key` returns the first key where this package's own
`parseKeyValue` — behind `StringValue` and `Resource.StringAttribute` —
returns the last.

They were left on first-match here because they are the per-attribute
hashing views described under "Error and compatibility strategy", the
library's hottest path, and changing them is a behavioral change to existing
accessors that this change did not measure. Repeated occurrences of either
field require a producer no consumer has been observed to run. The
divergence is tracked for the specification-drift work rather than left
implicit.

**Validation-scope change.** Both resolutions need the complete message:
merging must find every occurrence, and last-value-wins must reach the final
one. Neither `extractMergedMessage` nor `extractLastBytesField` can return
early the way `extractBytesField` still does. One consequence: a malformed
field located *after* the last relevant occurrence is now reported as an
error, where a shallow first-match extractor never reached it. This is an
intentional, narrow expansion of validation scope for these accessors — see
the "Resources and attributes" section of
[specification.md](specification.md) — not a general change to the "iterators
validate operation-scoped" contract. Each skipped tag is cheap:
`protowire.ConsumeFieldValue` on a length-delimited field only reads the
length prefix and jumps the body, so cost scales with the enclosing message's
field count, not its payload size, and stays allocation-free. That matters
more for scope containers than resource containers, because a scope container
holds every record where a resource container holds a handful of scopes.
[BENCHMARKS.md](BENCHMARKS.md) has the measured curve and what it costs a
consumer that reads only the scope and stops early.

### `WriteTo`

The three `WriteTo` methods share one writer that prefixes the unchanged
resource container with an export-request field tag and length. Direct
`io.Writer` output avoids an intermediate request buffer and preserves the
writer's byte count and error. In v0.0.4, a short byte count with a nil error
is returned as reported; `WriteTo` does not synthesize `io.ErrShortWrite`.

## Error and compatibility strategy

Selected known fields are strict; unknown fields are forward-compatible.
Malformed tags, lengths, nested values, groups, fixed-width IDs and wrong wire
types in typed accessors and iterators surface as errors. Semantic nested-value
parsing has a finite depth budget so adversarial data cannot recurse without
bound.

There are two intentional semantic levels:

- lightweight field views such as `KeyValue.Key` and `ValueRaw`, which return
  the first matching encoded field;
- semantic accessors such as `StringValue`, `Resource.StringAttribute` and
  `SeverityNumber`, which scan enough of the complete message to reproduce the
  documented protobuf/pdata behavior and expose trailing corruption.

Keeping those levels separate prevents hashing paths from paying for semantics
they do not need while giving routing and detector paths a parity-oriented API.

## Performance design

Counting uses direct tag walks and remains allocation-free. Returned byte
slices alias the input, with one documented exception: `Resource()` and
`Scope()` allocate a concatenated buffer when a container carries 2+
occurrences of that field (see "Singular field resolution" above); the
single-occurrence case every real producer emits stays zero-copy. Ordinary iterator cost is explicit and
amortized at outer levels; zero-allocation yield variants cover repeated
inner levels.

No production path imports pdata or generated OTLP message types. Benchmarks
compare equivalent work, use pdata as the full-decode baseline, report
allocations, and document their fixture and environment in
[BENCHMARKS.md](BENCHMARKS.md). Benchmark numbers are measurements, not portable
latency guarantees.

## Change design rules

When extending or refactoring the implementation:

1. Start from a consumer field or traversal requirement rather than trying to
   generalize the whole OTLP schema.
2. Compose existing wire helpers before adding a bespoke parser.
3. Verify field numbers and protobuf merge/oneof behavior against the pinned
   upstream schema and pdata.
4. Add pdata-built parity fixtures plus omitted, repeated, reordered, unknown,
   malformed and wrong-wire-type cases.
5. Test iterator completion, early stop and error retrieval.
6. Measure allocation-sensitive paths with paired `-benchmem` benchmarks.
7. Treat exported API, aliasing, error timing and `WriteTo` bytes as consumer
   contracts described in [specification.md](specification.md).
