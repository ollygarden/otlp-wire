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
| Consumer contract regression | hypothesized | Compile failure or changed routing/detector result after upgrade | Exported API, aliasing, iterator timing, or `WriteTo` bytes changed | Consumer-focused tests and canary comparison |

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
