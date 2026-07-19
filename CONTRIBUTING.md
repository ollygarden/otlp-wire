# Contributing to otlp-wire

Thanks for contributing. Check current issues and pull requests before
starting. For substantial public API, wire-contract, iterator, or performance
changes, open an issue and align the approach with maintainers before investing
in implementation.

Read [AGENTS.md](AGENTS.md) for the codebase map, parser and performance
guardrails, testing patterns, and change-specific validation matrix.

## Set up and validate

Use Go 1.25 or newer. CI uses the latest stable Go release.

```bash
go test -v -race ./...
go vet ./...
git diff --check
test -z "${BASE_SHA:-}" || git diff --check "${BASE_SHA}...HEAD"
```

The current `go.sum` retains checksum history from earlier dependency versions,
so `go mod tidy -diff` reports baseline drift. For dependency changes, run
`go mod tidy` and review both `go.mod` and `go.sum` intentionally, but keep
unrelated checksum cleanup out of a focused change unless reconciling that
baseline is part of the task.

Run focused tests and benchmarks as needed:

```bash
go test -run TestName ./...
go test -bench=BenchmarkName -benchmem ./...
```

`go fmt ./...` modifies files, so run it intentionally and review the
resulting diff. Performance comparisons must use the same hardware, fixture,
toolchain, and benchmark command for both revisions.

## Commits

Use Conventional Commits:

```text
feat(metrics): expose scope iteration
fix(parser): reject truncated fixed-width identifiers
test(traces): cover early iterator termination
docs: record benchmark methodology
```

Keep descriptions concise and imperative. Mark incompatible changes with `!`
and explain migration requirements in a `BREAKING CHANGE:` footer.

## Pull requests

Work from a focused branch in your fork or a maintainer-approved branch and
open a pull request against current `main`. Pull-request titles follow
Conventional Commits because changes are squash-merged.

Each pull request should:

- solve one logical problem without unrelated refactors, dependency updates,
  or broad formatting;
- explain the motivation and wire/API approach;
- link the relevant issue when applicable;
- list exact validation and benchmark commands with results;
- call out API compatibility, allocation, correctness, and rollout risks, or
  state `None`;
- update executable examples, README API summaries, design records, and
  benchmark documentation when their contracts or claims change.

CodeRabbit may review pull requests. Verify findings against the protobuf and
iterator contracts before applying them and reply on each actionable thread.
Maintainers squash-merge after required checks and review feedback are
satisfied.

By contributing, you agree that your contribution is provided under this
repository's [Apache License 2.0](LICENSE). Do not include credentials, real
telemetry, production payloads, customer data, or other sensitive information
in fixtures, benchmarks, logs, or review material.

All contributors are responsible for understanding and validating their
submissions, including agent-generated work. A human contributor must review
and take ownership of agent-generated output, be able to respond to feedback,
and disclose material agent involvement in the pull request. Review the
complete diff and make sure malformed-input and performance-sensitive behavior
are covered before requesting review.
