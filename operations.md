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
| Consumer contract regression | observed in tests | Compile failure or changed routing/detector result after upgrade | Exported API, aliasing, iterator timing, or `WriteTo` bytes changed | Consumer-focused tests and canary comparison; the v0.1.0 transitions below |

### Known consumer-visible transitions in v0.1.0 (unreleased)

The v0.1.0 API realignment (E-2940) changes behavior consumers can observe,
not only signatures. Each is intentional and moves the wire path toward pdata
parity; all are pre-release, so no rollback of a shipped tag is involved.

| Change | Issue | Who is affected |
| --- | --- | --- |
| `ResourceLogs.StringAttribute` removed | E-2941 | mulch (only consumer with source to change) |
| `Resource()` returns `(nil, nil)` for an absent Resource instead of an error | E-2941 | Any consumer treating that error as a signal; not firing in production as of 2026-08-07 |
| `Resource()` merges repeated occurrences instead of returning the first | E-2941 | Consumers reading resource attributes from multi-occurrence payloads |
| `Resource()`, `Scope()` and the singular-scalar accessors report corruption located after the last relevant occurrence | E-2941, E-2942 | Consumers that previously parsed such payloads without error |
| `Metric.Name()` returns the last occurrence of a repeated `name` instead of the first | E-2942 | Consumers reading names from payloads with a repeated `Metric.name` |

The canary decision and its measured pre/post comparison are recorded at
release time, per "Release and production analysis" below. Confirm the canary
consumer's telemetry is version-attributed first: see the uncovered-scenario
note above.

## Telemetry inventory

The library emits no telemetry. Importing services own throughput, error,
queue, allocation, GC, and business-output signals. A library benchmark is not
production validation.

**Uncovered scenarios:** A consumer without version-attributed processing and
queue telemetry cannot distinguish an idle path from a parser regression.

## Release and production analysis

1. Verify race tests, vet, malformed-wire coverage, allocation gates, and
   paired benchmarks on the exact release candidate.
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
