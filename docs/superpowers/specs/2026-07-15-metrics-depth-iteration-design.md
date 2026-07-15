# Design: metrics-depth iteration (E-2608)

Extend otlp-wire's metrics side from `ResourceMetrics` down to datapoint
attributes, in the same zero-copy protowire-walking style the traces side
already has (`ScopeSpans → Span → field accessors`). Consumer is marigold
v0.2.0's zero-copy decode path: combination hash = xxh3 over each datapoint's
attribute KeyValues, culprit hashes = xxh3 over each AnyValue's raw bytes.

## API

All new code lives in `otlpwire.go`, composing the existing helpers
(`forEachRepeatedField`, `skipField`, `countOccurrences`).

```go
// []byte aliases, same as ScopeSpans/Span
type ScopeMetrics []byte
type Metric []byte
type KeyValue []byte

type MetricType int // MetricTypeGauge, MetricTypeSum, MetricTypeHistogram,
                    // MetricTypeExponentialHistogram, MetricTypeSummary

// DataPoint is a struct, not a []byte alias: the attributes field number
// differs per datapoint type, so the datapoint must know which oneof body
// it came from.
type DataPoint struct {
    raw []byte
    typ MetricType
}

func (r ResourceMetrics) ScopeMetrics() (iter.Seq[ScopeMetrics], func() error) // field 2
func (s ScopeMetrics) Metrics() (iter.Seq[Metric], func() error)               // field 2
func (m Metric) Name() ([]byte, error)                                         // field 1
func (m Metric) DataPoints() (iter.Seq[DataPoint], func() error)               // oneof 5/7/9/10/11 → field 1
func (d DataPoint) Raw() []byte
func (d DataPoint) Type() MetricType
func (d DataPoint) Timestamp() (uint64, error)                                 // field 3, fixed64 (all types)
func (d DataPoint) Attributes() (iter.Seq[KeyValue], func() error)             // field varies, see below
func (kv KeyValue) Key() ([]byte, error)                                       // field 1
func (kv KeyValue) ValueRaw() ([]byte, error)                                  // field 2, raw AnyValue bytes
```

### Field numbers

Metric oneof bodies: gauge = 5, sum = 7, histogram = 9,
exponential_histogram = 10, summary = 11. Each body holds
`repeated data_points = 1`.

Datapoint `attributes` field number varies by type — this is why `DataPoint`
carries a type tag:

| Datapoint type              | attributes field |
| --------------------------- | ---------------- |
| NumberDataPoint (gauge/sum) | 7                |
| HistogramDataPoint          | 9                |
| ExponentialHistogramDataPoint | 1              |
| SummaryDataPoint            | 7                |

`time_unix_nano` is field 3 (fixed64) for all datapoint types.

KeyValue: key = 1 (string), value = 2 (AnyValue message).

### Decisions

- **Attributes as iterator, not raw region.** Repeated KeyValue fields are
  each a separate tag+length+payload; protobuf does not guarantee adjacency.
  Marigold streams each KeyValue's raw bytes into xxh3 instead of hashing one
  contiguous region.
- **`Metric.Metadata()` (field 12) is out of scope.** Marigold v0.2.0 does not
  consume it; it is a five-line addition later.
- **Zero-copy accessors.** `Name()`, `Key()`, `ValueRaw()` return sub-slices
  of the original buffer via a new `extractBytesField` helper (generalizes
  `extractResourceMessage`). No copies, no new dependencies.
- **Allocation budget.** Each iterator costs the same 2 closure allocations as
  the existing iterators (~2 allocs per metric on the scrape shape for
  DataPoints + Attributes). Field accessors are zero-alloc. Benchmarks report
  this honestly.
- **Error handling.** Same iterator + deferred error-func pattern as every
  existing iterator. Malformed protobuf surfaces through the error func.

## Tests

Canonical pattern: build with pdata, marshal, feed bytes to otlp-wire, assert.

- All five metric types yield their datapoints with correct `Type()`.
- Mixed-type batches (multiple metrics of different types in one scope).
- Attribute key/value round-trip: keys and AnyValue bytes match what pdata
  produces (compare `ValueRaw()` against independently marshaled AnyValue).
- Exponential histogram attributes specifically (field 1, the odd one out).
- Empty attributes, absent timestamp, empty datapoint lists.
- Malformed input error paths through the error funcs.

## Benchmarks

New scrape-shaped fixture in `benchmark_comparison_test.go`: 1 resource,
1 scope, 4,800 metrics × 1 datapoint each (mix of gauge and sum, ~4 string
attributes per datapoint) — the E-2601 traffic shape that hid previous
marigold regressions.

Headline pair, simulating marigold's workload without adding a hash
dependency:

- `BenchmarkMetrics_ScrapeDeepIteration_WireFormat`: iterate
  ResourceMetrics → ScopeMetrics → Metrics → DataPoints, read `Timestamp()`
  and every `Key()`/`ValueRaw()` (consume the bytes).
- `BenchmarkMetrics_ScrapeDeepIteration_Unmarshal`: pdata unmarshal + same
  traversal + per-datapoint attribute re-serialization into a buffer (what
  marigold does today for hashing).

Plus a `DataPoints`-only deep-iteration pair on the existing 5×100 fixture
for continuity with the current benchmark suite. Results table goes into
`docs/BENCHMARKS.md` and the PR description.

Success criterion: wire path shows an order-of-magnitude (target 20–50×)
reduction in ns/op and allocs vs the pdata path on the scrape-shaped fixture.
