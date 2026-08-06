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
kept in `otlpwire.go`; pdata is used only in tests and benchmarks as a fixture
builder and semantic oracle.

The public wire hierarchy is:

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

Request, resource, scope, record, span, metric, Resource and KeyValue types are
named byte slices. They are cheap to construct and make zero-copy ownership
visible. `DataPoint` is a small struct because it must retain the metric body
type that identifies the body and determines its attribute layout.

## Wire walking

Parsing composes a small set of helpers around
`google.golang.org/protobuf/encoding/protowire`:

- repeated-field counting;
- repeated-field iteration;
- length-delimited, fixed-width and scalar extraction;
- resource extraction and re-wrapping;
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

### Logs

Log consumers need both a very cheap per-record path and pdata-compatible
resource context. `LogRecordsSeq` supplies the former. `SeverityNumber` scans
the whole known LogRecord structure so an early severity field cannot hide
trailing corruption, and it implements protobuf last-value-wins scalar
semantics.

`ResourceLogs.StringAttribute` works on the enclosing ResourceLogs rather than
only the first Resource field. This is intentional: protobuf singular message
fields merge when repeated. The implementation scans all occurrences, retains
the first duplicate attribute key as pdata does, and still validates later
messages.

### Traces

Trace access is intentionally narrow. Consumers can iterate scopes and spans,
count spans, and extract the three fixed-width identifiers used by partial
trace detectors. Identifier accessors return arrays to make the required width
part of the Go type and reject incorrectly sized wire values.

## Resource extraction and `WriteTo`

Resource containers share the same outer protobuf shape across metrics, logs
and traces: Resource is field 1, the repeated scope container is field 2, and
the export request repeats resource containers in field 1.

The three `Resource()` methods therefore share one extractor. The three
`WriteTo` methods share one writer that prefixes the unchanged resource
container with an export-request field tag and length. Direct `io.Writer`
output avoids an intermediate request buffer and preserves the writer's byte
count and error.

## Error and compatibility strategy

Selected known fields are strict; unknown fields are forward-compatible.
Malformed tags, lengths, nested values, groups, fixed-width IDs and wrong wire
types in typed accessors and iterators surface as errors. Semantic nested-value
parsing has a finite depth budget so adversarial data cannot recurse without
bound.

The current counting walk has one exception: `DataPointCount` skips a
recognized metric-body tag with a non-length-delimited wire type, whereas the
data-point iterators reject it. This is historical behavior, not a pattern to
copy. A refactor should resolve it deliberately and cover the decision with a
test.

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
slices alias the input. Ordinary iterator cost is explicit and amortized at
outer levels; zero-allocation yield variants cover repeated inner levels.

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
