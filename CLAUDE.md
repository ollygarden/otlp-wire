# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go test -v -race ./...          # Run all tests with race detector
go test -run TestName ./...     # Run a single test
go test -bench=. ./...          # Run benchmarks
go test -bench=BenchmarkName -benchmem ./...  # Run specific benchmark with alloc stats
go vet ./...                    # Lint (this is what CI runs)
go fmt ./...                    # Format code
```

CI runs `go test -v -race ./...` and `go vet ./...`.

## Architecture

Single-package Go library (`go.olly.garden/otlp-wire`) that operates on OTLP protobuf wire format bytes without unmarshaling. All code is in `otlpwire.go`.

### Core design

Every public type is a `[]byte` alias representing a slice of protobuf wire data:

```
ExportMetricsServiceRequest / ExportLogsServiceRequest / ExportTracesServiceRequest
  └─ ResourceMetrics / ResourceLogs / ResourceSpans
       └─ ScopeSpans (traces only)
            └─ Span (traces only)
```

The library navigates protobuf fields by their field numbers using `protowire.ConsumeTag` / `protowire.ConsumeBytes` directly. Field numbers map to the OTLP proto definitions (e.g., field 1 = resource entries, field 2 = scope entries).

### Key patterns

- **Iterator + error closure**: All iterators return `(iter.Seq[T], func() error)`. The error function must be called after iteration to check for parse errors. This is the pattern throughout — never change it.
- **Shared helpers**: `countRepeatedField`, `countOccurrences`, `forEachRepeatedField`, `skipField`, `extractResourceMessage`, `writeResourceMessage`, `extractFixedBytesField` are the building blocks. New signal types or field accessors should compose these.
- **Zero allocation goal**: The library targets zero allocations for counting and minimal (2) allocations for iteration. The only allocations come from the iterator closure capture. Benchmarks verify this.

### Test structure

Tests use `pdata` (from `go.opentelemetry.io/collector/pdata`) to construct OTLP data, marshal it, then verify the wire-format library produces correct results. This is the canonical test pattern: build with pdata, marshal, pass to otlp-wire, assert.

`benchmark_comparison_test.go` contains side-by-side benchmarks comparing wire-format operations against full pdata unmarshal.
