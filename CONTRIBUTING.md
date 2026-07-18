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
go mod tidy
git diff --exit-code -- go.mod go.sum
go test -v -race ./...
go vet ./...
git diff --check
```

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
iterator contracts before applying them and reply on each actionable thread. A
human maintainer squash-merges; contributors and coding agents do not merge
their own changes.

By contributing, you agree that your contribution is provided under this
repository's [Apache License 2.0](LICENSE). Do not include credentials, real
telemetry, production payloads, customer data, or other sensitive information
in fixtures, benchmarks, logs, or review material.

All contributors are responsible for understanding and validating their
submissions, including agent-generated work. Review the complete diff and make
sure malformed-input and performance-sensitive behavior are covered before
requesting review.
