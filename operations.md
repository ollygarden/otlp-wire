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
| Accessor divergence on one message | hypothesized | Two accessors over the same message disagree on whether a payload is valid, so a consumer sees one field read succeed and the other fail on identical bytes | A second accessor added with its own parser instead of reading from the existing walk | Shared-walk implementation plus a malformed-parity test asserting both accessors fail together; see `TestLogRecordSeverity_MalformedParity` |

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
| `Resource()`, `Scope()` and the singular-scalar accessors report corruption located after the last relevant occurrence | E-2941, E-2942 | Consumers that previously parsed such payloads without error |
| `Metric.Name()` returns the last occurrence of a repeated `name` instead of the first | E-2942 | Consumers reading names from payloads with a repeated `Metric.name` |
| Scanning accessors reject an unclosed group in an unknown trailing field, which pdata accepts | E-2942 | Consumers pairing the wire path with a pdata fallback; only reachable on malformed or adversarial input, never on conformant OTLP |

The canary decision and its measured pre/post comparison are recorded at
release time, per "Release and production analysis" below. Confirm the canary
consumer's telemetry is version-attributed first: see the uncovered-scenario
note under "Telemetry inventory" below.

### Diagnosing the LogRecord severity accessors

`SeverityNumber` and `SeverityText` share one schema-aware walk of the whole
LogRecord, so they can never disagree about whether a record is valid. Two
consequences when reading a consumer's CPU profile:

- **Each call walks the record once.** A consumer reading both fields pays two
  walks. A severity path showing roughly double the expected parse cost is
  usually this, not a regression.
- **The walk descends into the body and every attribute.** Cost tracks record
  contents, not just field count, unlike the shallow field-scan accessors. A
  consumer whose records carry large bodies or many attributes pays more per
  severity read than one whose records are bare, even at identical record
  counts. [docs/BENCHMARKS.md](docs/BENCHMARKS.md) has the measured comparison
  against a shallow hand-rolled walk.

**Known divergences from pdata.** These matter only to consumers that pair the
wire path with a pdata fallback and expect the two to agree on validity. All
are reachable only on malformed or adversarial input, never on conformant OTLP
from a normal producer, and none is specific to one accessor — they are
properties of the shared LogRecord walk. No test covers them.

| Input | Wire path | pdata |
| --- | --- | --- |
| Well-formed group in an unknown LogRecord field, or inside the body's `AnyValue` | accepts | rejects (`unexpected EOF`) |
| Non-canonical 10-byte varint (10th byte ≥ 2), in any varint field including a `severity_text` length prefix | rejects | accepts |
| Field number above `MaxInt32` whose `int32` truncation is positive | rejects (`malformed protobuf tag`) | accepts (truncates, then skips) |

The first row is the mirror of the unclosed-group row above. The last two come
from `protowire` being stricter than pdata's generated unmarshal: `protowire`
enforces canonical varint encoding and rejects out-of-range field numbers,
where the gogo-generated loop does neither. A consumer cannot conclude "pdata
would also reject this" from a wire-path error, nor the reverse.

## Telemetry inventory

The library emits no telemetry. Importing services own throughput, error,
queue, allocation, GC, and business-output signals. A library benchmark is not
production validation.

**Uncovered scenarios:** A consumer without version-attributed processing and
queue telemetry cannot distinguish an idle path from a parser regression.

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
