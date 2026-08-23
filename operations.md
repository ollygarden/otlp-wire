# Operations — otlp-wire

Operational characteristics of the `go.olly.garden/otlp-wire` library. Read
this before releasing the module or changing a consumer rollout boundary.

## Runtime shape

- **Business role:** Provides zero-copy OTLP protobuf counting, traversal,
  resource splitting, and selected field access for ingestion hot paths.
- **Deployment:** This repository produces a Go module, not a standalone
  workload. Runtime ownership belongs to each importing service.
- **Hot path:** Protobuf tag walking, nested repeated-field iteration, semantic
  attribute parsing, and per-resource `WriteTo` calls.
- **State:** No durable or process-global state. Returned byte slices alias the
  caller-owned request buffer; ordinary iterators retain only their lazy error.

## Failure scenarios

| Scenario | Status | Symptom | Likely cause | Covering evidence |
| --- | --- | --- | --- | --- |
| Wire-contract regression | observed in tests | Consumer rejects valid OTLP or accepts malformed selected fields | Field number, wire type, merge, or oneof behavior changed | pdata differential and malformed-wire tests |
| Allocation regression | hypothesized | Higher GC/CPU in high-volume consumers | Extra copies, escaping closures, or full decode added to a hot path | Allocation tests and paired `-benchmem` benchmarks |
| Consumer contract regression | observed in tests | Compile failure or changed routing/detector result after upgrade | Exported API, aliasing, iterator timing, or `WriteTo` bytes changed | Consumer-focused tests and canary comparison; the transitions below |
| Benchmark-method error | observed | A performance claim or release decision rests on a delta that does not reproduce | A paired comparison run as sequential blocks (`go test -count=N`) instead of alternating invocations, or a gap claimed from overlapping ranges | Re-run alternating; see the release checklist below |
| Accessor divergence on one message | hypothesized | Two accessors over the same message disagree on whether a payload is valid, so a consumer sees one field read succeed and the other fail on identical bytes | A second accessor added with its own parser instead of reading from the existing walk, or a new accessor scanning a message whose existing accessors scan differently | Shared-walk implementation plus a malformed-parity test asserting the accessors that share a schema-aware walk fail together; see `TestLogRecordSeverity_MalformedParity` and `TestSpanFields_MalformedParity`. Where accessors over one message deliberately do **not** share a walk, the divergence is pinned instead: `TestSpanIdentifiers_FirstMatchDivergence` |

### Known consumer-visible transitions in the API realignment

These change behavior consumers can observe, not only signatures. Each is
intentional and moves the wire path toward pdata parity. The realignment ships
in stages, so release status is not uniform: check each against
`git tag --merged` before assuming it is still pre-release, since reverting a
published one means a consumer version pin.

| Change | Issue | Who is affected |
| --- | --- | --- |
| `ResourceLogs.StringAttribute` removed | E-2941 | mulch (only consumer with source to change) |
| `Resource()` returns `(nil, nil)` for an absent Resource instead of an error | E-2941 | Any consumer treating that error as a signal; not firing in production as of 2026-08-07 |
| `Resource()` merges repeated occurrences instead of returning the first | E-2941 | Consumers reading resource attributes from multi-occurrence payloads |
| `Resource()`, `Scope()` and the last-value-wins scalar accessors report corruption located after the last relevant occurrence | E-2941, E-2942 | Consumers that previously parsed such payloads without error |
| `Metric.Name()` returns the first occurrence of a repeated `name`, where pdata takes the last, and accepts corruption located anywhere after the `name` field | E-2985 | Consumers pairing the wire path with a pdata fallback. The corruption case is **not** limited to the `name` field: a metric whose datapoint body is truncated now traverses without an error from any accessor, because the enclosing iterators check framing only |
| Scanning accessors reject an unclosed group in an unknown trailing field, which pdata accepts | E-2942 | Consumers pairing the wire path with a pdata fallback; only reachable on malformed or adversarial input, never on conformant OTLP |

The canary decision and its measured pre/post comparison are recorded at
release time, per "Release and production analysis" below. Confirm the canary
consumer's telemetry is version-attributed first: see the uncovered-scenario
note under "Telemetry inventory" below.

### Diagnosing the LogRecord severity accessors

`SeverityNumber`, `SeverityText` and `Severity` share one schema-aware walk of
the whole LogRecord, so they can never disagree about whether a record is
valid. Two consequences when reading a consumer's CPU profile:

- **Each call walks the record once.** A severity path showing roughly double
  the expected parse cost is usually a consumer calling `SeverityNumber` and
  `SeverityText` on the same record, not a regression — that pair costs a
  measured ~1.9x what `Severity` costs, which returns both from one walk.
  Confirm it
  from the call site rather than the profile shape: both arrangements attribute
  their time to the same function, so only the call count separates them. A
  consumer that guards its `severity_text` read behind the number pays the same
  either way once it uses `Severity`, so a sparse-missing-severity shape and an
  every-record shape converge on the same cost.
- **The walk descends into the body and every attribute.** Cost tracks record
  contents, not just field count, unlike the shallow field-scan accessors. A
  consumer whose records carry large bodies or many attributes pays more per
  severity read than one whose records are bare, even at identical record
  counts. The published benchmarks all use one record shape, so they size the
  per-record cost but do not show how it scales with body and attribute
  volume; a consumer suspecting that scaling should measure its own payloads.

`SeverityFields` is the field-scoped alternative for consumers that read only
the two severity fields. It shares the same complete top-level walk and
last-value-wins resolution, while treating body and attribute messages as
opaque length-delimited values. It still rejects malformed record framing,
wrong top-level wire types, and truncated nested values, but accepts malformed
contents inside a correctly framed body or attribute. Use the strict accessors
when those nested values are consumed next or full-record validation is part of
the caller's contract. `TestLogRecordSeverityFields_ValidationScope` pins this
boundary.

**Known divergences from pdata.** These matter only to consumers that pair the
strict accessors with a pdata fallback and expect the two to agree on validity.
All are reachable only on malformed or adversarial input, never on conformant
OTLP from a normal producer, and none is specific to one strict accessor — they
are properties of the strict LogRecord walk. `TestLogRecordSeverity_PdataDivergence`
pins each one, so a change in either direction shows up as a test failure
rather than as silently different consumer behavior.

| Input | Wire path | pdata |
| --- | --- | --- |
| Well-formed group in an unknown LogRecord field, or inside the body's `AnyValue` | accepts | rejects (`unexpected EOF`) |
| Varint overflowing uint64 (10 bytes, final byte ≥ 2), in any varint field including a `severity_text` length prefix | rejects | accepts (truncated) |
| Field number above `MaxInt32` whose `int32` truncation is positive | rejects (`malformed protobuf tag`) | accepts (truncates, then skips) |

The first row is the mirror of the unclosed-group row above. The last two come
from `protowire` being stricter than pdata's generated unmarshal: `protowire`
rejects a varint whose value exceeds `uint64` and a field number above
`MaxInt32`, where the generated loop silently truncates both. A consumer
cannot conclude "pdata would also reject this" from a wire-path error, nor the
reverse.

### Diagnosing the Span accessors

`Name`, `Kind`, `StartTimeUnixNano` and `EndTimeUnixNano` share one
schema-aware walk of the whole span, so those four can never disagree about
whether a span is valid. `TraceID`, `SpanID` and `ParentSpanID` scan
first-match and stop at their field. Three consequences when reading a
consumer's CPU profile:

- **Each scalar call walks the span once.** A consumer reading all four pays
  four walks, several times the cost of a single-pass hand-rolled walk. A span
  path showing that multiple is usually this, not a regression; it is still
  well cheaper than the pdata unmarshal it replaces.
- **The identifier accessors are cheap and unchanged**, and are not a
  suspect when a span path slows down.
- **Cost tracks field count, not payload size.** Unlike the LogRecord walk,
  this one does not descend into `attributes`, `events`, `links` or `status`.
  A span carrying large attribute values costs the same as one carrying small
  ones at equal field counts.

[docs/BENCHMARKS.md](docs/BENCHMARKS.md) carries the figures, with the fixture,
environment and commands. Do not copy them here: a stale number in a runbook is
worse than a pointer to a measured one. It also records why the identifier
accessors were left on first-match, including how much a full walk would have
cost them — the answer depends on the producer, because pdata marshals fields
back-to-front and puts `trace_id` last where the SDK exporters put it first.

**Known divergences from pdata.** These matter only to consumers that pair the
wire path with a pdata fallback and expect the two to agree on validity. All
are reachable only on malformed or adversarial input, never on conformant OTLP
from a normal producer. `TestSpanFields_PdataDivergence` and
`TestSpanIdentifiers_FirstMatchDivergence` pin each one, so a change in either
direction shows up as a test failure rather than as silently different consumer
behavior.

| Input | Wire path | pdata |
| --- | --- | --- |
| Malformed contents inside a correctly framed `attributes`, `events`, `links` or `status` | accepts | rejects |
| Repeated `trace_id`/`span_id`/`parent_span_id` | reports the **first** occurrence | reports the last |
| Empty trailing `trace_id` after a populated one | reports the populated one | resets to zero |
| Corruption located after an identifier | the identifier accessor accepts; the four scalar accessors reject | rejects |
| Unknown group closing at the very end of the span | accepts | rejects (`unexpected EOF`) |
| Unknown group closed with a field number that does not match its StartGroup | rejects | accepts |
| Varint overflowing uint64 (10 bytes, final byte ≥ 2), in any varint field including a `name` length prefix | rejects | accepts (truncated) |
| Field number above `MaxInt32` whose `int32` truncation is positive | rejects (`malformed protobuf tag`) | accepts (truncates, then skips) |

Rows two to four are the price of leaving the identifier accessors on a
first-match scan, and they are unreachable from a conformant producer, which
emits each singular field exactly once. Row one follows from the walk being
framing-only below the Span level; it is the one divergence class with no
LogRecord equivalent, since `parseLogRecordSeverity` does descend into the body
and attributes. [docs/DESIGN.md](docs/DESIGN.md) records both decisions. A
consumer cannot conclude "pdata would also accept this" from a successful Span
accessor read.

### Diagnosing the Metric accessors

`Metric.Name` scans first-match and stops at field 1. `Metric.Metadata`,
`MetadataSeq` and `DataPoints` walk what they read. Two consequences when
reading a consumer's CPU profile:

- **`Name` is cheap and its cost does not track the metric's field count.**
  A metric path that slows down is not `Name`; look at the datapoint and
  attribute iteration instead. This is the opposite of the last-value-wins
  scalars on the containers above, whose cost does grow with the enclosing
  message.
- **What `Name` costs depends on where the producer puts the field.** It
  returns at the first `name` tag, so an SDK exporter emitting ascending field
  numbers pays almost nothing, while pdata-marshalled bytes put `name` last
  and make it walk the metric anyway. A benchmark built only from pdata
  fixtures cannot see the difference between the two resolutions — that is how
  the regression E-2985 fixed stayed invisible to the benchmark meant to guard
  it. `BenchmarkMetric_Name_SDKOrder` exists for this reason; keep both arms.

[docs/BENCHMARKS.md](docs/BENCHMARKS.md) carries the figures, with the fixture,
environment and commands. Do not copy them here.

**Known divergences from pdata.** These matter only to consumers that pair the
wire path with a pdata fallback and expect the two to agree. Both are reachable
only on malformed or adversarial input, never on conformant OTLP.
`TestMetricName_FirstMatchDivergesFromPdata` and
`TestMetricName_MalformedTrailingFieldNotReported` pin them.

| Input | Wire path | pdata |
| --- | --- | --- |
| Repeated `Metric.name` | reports the **first** occurrence | reports the last |
| Corruption anywhere after the `name` field, including inside the datapoint body | `Name` accepts | rejects |
| Later `name` occurrence carrying a wrong wire type | `Name` accepts, returning the first | rejects |
| Unclosed group in an unknown field *before* `name` | `Name` rejects | accepts |

### The deprecated scope containers (field 1000)

`ResourceLogs`, `ResourceSpans` and `ResourceMetrics` each carry a field 1000
holding the pre-rename scope container — `deprecated_scope_logs` and its
siblings, from the OTLP `InstrumentationLibrary` → `Scope` transition. The
traversal treats it as an unknown field: framing is checked, contents are not,
and the records inside are **not** traversed.

That is a deliberate deviation from pdata, made for the reasons below. It is
recorded here rather than fixed, so a consumer investigating a count that looks
low can rule it in or out quickly.

| Input | Wire path | pdata |
| --- | --- | --- |
| Wrong wire type on field 1000 | accepts | rejects (`wrong wireType ... for field DeprecatedScopeLogs`) |
| Well-formed records in field 1000, none in field 2 | counts zero, no error | `plog.ProtoUnmarshaler` also counts zero; `plogotlp.ExportRequest` and the gRPC server path migrate them and count them |

The second row is the one that could bite, and it depends on which pdata door a
consumer uses. `otlp.MigrateLogs` promotes field 1000 into `ScopeLogs` when
`ScopeLogs` is empty, but it is called only from `plogotlp/grpc.go`,
`plogotlp/request.go` and `plog/json.go` — **not** from `plog.ProtoUnmarshaler`,
despite that function's own comment requiring every unmarshaler to call it. The
downstream consumers use `ProtoUnmarshaler`, so they and the wire path agree.
Raincatcher, at the ingest edge, uses the migrating API; whether a field-1000
payload can reach downstream at all therefore depends on whether raincatcher
re-marshals after pdata migrates it, which has not been established.

No producer has emitted field 1000 in years, which is why this is accepted
rather than closed. If a consumer ever reports logs, spans or metrics counted as
zero on a payload another tool reads as non-empty, check for field 1000 before
anything else. E-2976 holds the full measurements.

## Telemetry inventory

The library emits no telemetry. Importing services own throughput, error,
queue, allocation, GC, and business-output signals. A library benchmark is not
production validation.

**Uncovered scenarios:** A consumer without version-attributed processing and
queue telemetry cannot distinguish an idle path from a parser regression.

### Diagnosing container-iterator allocations

The ordinary request/resource iterators return closures and an error function.
When a consumer ranges over one of those function-typed values, its loop body
and captured state may escape to the heap once per opened iterator. The effect
scales with resource and scope count, so a record-heavy benchmark can hide a
cost that becomes material on resource-heavy traffic.

Hot paths can range directly over `ResourceMetricsSeq`/`ScopeMetricsSeq`,
`ResourceLogsSeq`/`ScopeLogsSeq`, or `ResourceSpansSeq`/`ScopeSpansSeq` to
receive errors inline and keep container traversal allocation-free. The
ordinary methods remain the ergonomic form and preserve their deferred-error
contract. `TestContainerSeq_ZeroAllocations` is the library gate; production
impact must still be established in the importing service because this module
emits no allocation or GC telemetry of its own.

The ordinary adapter bodies are intentionally explicit rather than delegated
to `repeatedFieldSeq`. E-3051 measured that seemingly mechanical cleanup at an
additional 24 B per `BenchmarkContainerSeq` operation with no allocation-count
or credible runtime benefit under Go 1.25.12. If a future compiler changes
escape behavior, rerun that benchmark with alternating prebuilt binaries
before trying the consolidation again.

## Release and production analysis

1. Verify race tests, vet, malformed-wire coverage, allocation gates, and
   paired benchmarks on the exact release candidate. Run paired comparisons as
   alternating invocations of a prebuilt test binary, never `go test -count=N`,
   which runs a benchmark's iterations consecutively rather than interleaving
   the arms. Report the median of the paired per-round deltas and how many
   rounds carried the sign; single rounds invert on a loaded machine.
2. Tag the exact reviewed merge commit; never move or reuse a module tag.
3. Upgrade one prominent consumer first and preserve its full transport,
   acknowledgement, retry, telemetry, and publication behavior.
4. Compare equivalent pre/post traffic windows for output parity, errors,
   throughput, backlog slope, CPU, allocations, and GC behavior.
5. Fall back to the previous module version if correctness or consumer health
   regresses. Broader consumer upgrades require separate evidence.

See [docs/specification.md](docs/specification.md) for the compatibility and
rollout contract and [docs/BENCHMARKS.md](docs/BENCHMARKS.md) for reproducible
benchmark methodology.
