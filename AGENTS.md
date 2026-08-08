# otlp-wire repository guide

`otlp-wire` is a public, performance-sensitive Go library for counting,
iterating, splitting, and inspecting OTLP protobuf wire bytes without fully
unmarshaling them. Read [CONTRIBUTING.md](CONTRIBUTING.md) for contributor,
commit, and pull-request expectations.

## Development commands

Use Go 1.25 or newer to match the module directive and iterator APIs. CI uses
the latest stable Go release.

```bash
go mod tidy
git diff --exit-code -- go.mod go.sum
go test -v -race ./...
go vet ./...
git diff --check
test -z "${BASE_SHA:-}" || git diff --check "${BASE_SHA}...HEAD"
```

Useful focused and performance commands:

```bash
go test -run TestName ./...
go test -bench=. ./...
go test -bench=BenchmarkName -benchmem ./...
```

`go fmt ./...` modifies files. Run it intentionally and review the resulting
diff. Benchmarks are informative locally and can vary by machine; compare
before/after results under the same conditions.

## Architecture

This is one package and one module, `go.olly.garden/otlp-wire`. Production
code is split by domain: public types in `types.go`, signal traversal in
`metrics.go`, `logs.go`, and `traces.go`, shared attribute semantics in
`attributes.go`, signal-generic instrumentation-scope accessors in `scope.go`,
and low-level protobuf walking in `wire.go`. Functional tests are in
`otlpwire_test.go`, `log_iteration_test.go`, `resource_test.go`, and
`scope_test.go`, usage examples in `example_test.go`, and comparative
benchmarks in `benchmark_comparison_test.go`.

Public wire types are byte slices or small wrappers over byte slices. They
navigate protobuf fields directly with `protowire.ConsumeTag`,
`protowire.ConsumeBytes`, and related helpers:

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

Field numbers and wire types must stay aligned with the upstream OTLP protobuf
definitions. Shared helpers such as repeated-field counting/iteration, field
skipping, merged singular-message extraction, resource writing, and
first-match, last-value-wins and fixed-byte extraction are the building blocks
for new accessors.

## Iterator and error contracts

- Closure-based iterators return `(iter.Seq[T], func() error)`. Call the error
  closure after iteration, including after early exit. Do not replace this
  repository-wide contract casually.
- Deep metrics hot paths also expose zero-allocation yield-based variants:
  `Metric.DataPointsSeq` and `DataPoint.AttributesSeq`. Prefer the
  closure-based APIs for ordinary code and the sequence variants only when
  per-element allocation cost matters.
- `DataPoint` carries `MetricType` because OTLP metric bodies use different
  field numbers for timestamps and attributes. Preserve that association.
- A new accessor for a *singular* field must pick its resolution deliberately:
  singular messages (`Resource`, `InstrumentationScope`) merge repeated
  occurrences via `extractMergedMessage`; singular scalars (`SchemaUrl`,
  scope `Name`/`Version`, `Metric.Name`) take the last occurrence via
  `extractLastBytesField`. Both scan the whole message, so neither can return
  early. `extractBytesField`'s first-match behavior is reserved for the
  documented `KeyValue.Key`/`ValueRaw` hashing views. Verify the choice
  against pdata's generated unmarshal — merge appends, replacement assigns.
- Malformed tags, lengths, wire types, identifiers, metric bodies, or nested
  messages must return parse errors. Never silently accept corruption to keep
  an iterator moving.
- `WriteTo` reconstructs the enclosing repeated-field message without a full
  unmarshal. Preserve byte-level output semantics and short-write/error
  propagation.

## Performance guardrails

- Counting should remain zero-allocation. Resource iteration and splitting
  target the documented minimal allocation profile.
- Do not introduce `pdata` or generated-protobuf unmarshaling into production
  paths. `pdata` is a test oracle used to build and marshal fixtures.
- Compose existing wire helpers before adding bespoke parsing loops. Keep
  bounds checks and wrong-wire-type handling explicit.
- Add benchmarks for changes to iteration depth, hot-path accessors, parsing,
  or writing. Run paired benchmarks under the same environment and include
  `-benchmem` results.
- Update [docs/BENCHMARKS.md](docs/BENCHMARKS.md) and README performance claims
  only from reproducible measurements. State hardware, fixtures, command, and
  comparison method.

## Testing conventions

Build canonical OTLP objects with
`go.opentelemetry.io/collector/pdata`, marshal them, run the wire-format
operation, and compare the result with the expected pdata structure or bytes.

Cover:

- metrics, logs, and traces where the helper is signal-generic;
- empty, omitted, repeated, unknown, and out-of-order fields;
- truncated values, invalid lengths/tags, and wrong wire types;
- iterator completion, early stop, and deferred error-closure checks;
- fixed-width identifier validation and writer error propagation;
- allocation-sensitive paths with benchmarks or `testing.AllocsPerRun` where
  appropriate.

Examples are executable tests. Keep [example_test.go](example_test.go), the
README, and the public API synchronized.

## Documentation and workflow

- [docs/specification.md](docs/specification.md) is the canonical product,
  compatibility, consumer, and rollout contract.
- [docs/DESIGN.md](docs/DESIGN.md) records wire-format design decisions.
- [docs/BENCHMARKS.md](docs/BENCHMARKS.md) records benchmark fixtures and
  methodology.
- Update the specification when an API, behavioral contract, consumer
  dependency, or rollout gate changes. Keep temporary implementation plans out
  of the repository unless maintainers explicitly request one.
- Read [operations.md](operations.md) before releasing the library or changing
  a consumer rollout boundary.

Avoid breaking exported APIs or iterator/error behavior. If a change must be
incompatible, document the migration, update examples and design material, and
mark the Conventional Commit and pull request as breaking.

## Validation matrix

| Change | Required validation |
| --- | --- |
| Documentation only | `git diff --check`, link review, and example/API consistency review |
| Go implementation | tidy diff, race tests, vet, and focused malformed-wire tests |
| Public API or examples | Go checks plus executable examples and README/design updates |
| Iterator or parser | Go checks plus early-stop, corruption, wrong-wire-type, and allocation coverage |
| Performance claim or hot path | Go checks plus paired benchmarks with `-benchmem` and benchmark-doc updates |
| Dependencies | Full CI-equivalent gate and clean tidy diff |
