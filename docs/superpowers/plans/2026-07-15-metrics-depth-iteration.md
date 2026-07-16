# Metrics-Depth Iteration (E-2608) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend otlp-wire's metrics side with zero-copy iteration from `ResourceMetrics` down to datapoint attributes (`ScopeMetrics → Metric → DataPoint → KeyValue`), and benchmark it against pdata unmarshal on a scrape-shaped fixture.

**Architecture:** All public types are views over protobuf wire bytes, navigated with `protowire` by field number. New iterators compose the existing `forEachRepeatedField` helper and follow the `(iter.Seq[T], func() error)` pattern. `DataPoint` is the one struct (not `[]byte` alias) because its `attributes` field number depends on which metric oneof body it came from.

**Tech Stack:** Go, `google.golang.org/protobuf/encoding/protowire`, tests via `pdata` (`go.opentelemetry.io/collector/pdata/pmetric`) + testify.

## Global Constraints

- Single package; all library code in `otlpwire.go`, tests in `otlpwire_test.go`, benchmarks in `benchmark_comparison_test.go`.
- No new dependencies (library or test).
- Iterator pattern is exactly `(iter.Seq[T], func() error)` with the error func checked after iteration — never deviate.
- Zero-copy: accessors return sub-slices of the input buffer, never copies.
- Test pattern: build with pdata, marshal, feed bytes to otlp-wire, assert.
- CI gates: `go test -v -race ./...` and `go vet ./...` must pass.
- Field numbers (from OTLP metrics.proto):
  - ResourceMetrics: scope_metrics = 2. ScopeMetrics: metrics = 2.
  - Metric: name = 1, gauge = 5, sum = 7, histogram = 9, exponential_histogram = 10, summary = 11. Each body: repeated data_points = 1.
  - Datapoint attributes: NumberDataPoint = 7, HistogramDataPoint = 9, ExponentialHistogramDataPoint = 1, SummaryDataPoint = 7. time_unix_nano = 3 (fixed64) for all.
  - KeyValue: key = 1, value (AnyValue) = 2.

---

### Task 1: ScopeMetrics and Metric iteration

**Files:**
- Modify: `otlpwire.go` (types near line 30, methods after `ResourceMetrics.WriteTo` around line 77)
- Test: `otlpwire_test.go`

**Interfaces:**
- Consumes: existing `forEachRepeatedField`, existing `ResourceMetrics` type.
- Produces: `type ScopeMetrics []byte`, `type Metric []byte`, `func (r ResourceMetrics) ScopeMetrics() (iter.Seq[ScopeMetrics], func() error)`, `func (s ScopeMetrics) Metrics() (iter.Seq[Metric], func() error)`.

- [ ] **Step 1: Write the failing test**

Append to `otlpwire_test.go` (a helper used by later tasks too):

```go
// buildScopedMetrics builds a request with the given number of scopes per
// resource and metrics per scope, all gauges with one datapoint.
func buildScopedMetrics(t *testing.T, resources, scopes, metricsPerScope int) []byte {
	t.Helper()
	metrics := pmetric.NewMetrics()
	for r := 0; r < resources; r++ {
		rm := metrics.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", fmt.Sprintf("service-%d", r))
		for s := 0; s < scopes; s++ {
			sm := rm.ScopeMetrics().AppendEmpty()
			sm.Scope().SetName(fmt.Sprintf("scope-%d", s))
			for m := 0; m < metricsPerScope; m++ {
				metric := sm.Metrics().AppendEmpty()
				metric.SetName(fmt.Sprintf("metric.%d.%d", s, m))
				dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
				dp.SetIntValue(int64(m))
				dp.SetTimestamp(1000000000)
			}
		}
	}
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(metrics)
	require.NoError(t, err)
	return bytes
}

func TestScopeMetricsIteration(t *testing.T) {
	bytes := buildScopedMetrics(t, 2, 3, 4)
	req := ExportMetricsServiceRequest(bytes)

	totalScopes := 0
	totalMetrics := 0
	resources, resErr := req.ResourceMetrics()
	for rm := range resources {
		scopeSeq, scopeErr := rm.ScopeMetrics()
		for sm := range scopeSeq {
			totalScopes++
			metricSeq, metricErr := sm.Metrics()
			for range metricSeq {
				totalMetrics++
			}
			require.NoError(t, metricErr())
		}
		require.NoError(t, scopeErr())
	}
	require.NoError(t, resErr())

	require.Equal(t, 6, totalScopes)   // 2 resources × 3 scopes
	require.Equal(t, 24, totalMetrics) // 6 scopes × 4 metrics
}

func TestScopeMetricsIteration_Malformed(t *testing.T) {
	// Field 2 (scope_metrics) with wrong wire type: varint instead of bytes.
	bad := ResourceMetrics{0x10, 0x01}
	seq, errFn := bad.ScopeMetrics()
	for range seq {
	}
	require.Error(t, errFn())
}
```

If `otlpwire_test.go` does not already import `fmt`, add it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestScopeMetricsIteration' ./...`
Expected: compile error — `rm.ScopeMetrics undefined`.

- [ ] **Step 3: Write minimal implementation**

In `otlpwire.go`, add type declarations after `type Span []byte` (line 34):

```go
// ScopeMetrics represents a single ScopeMetrics message (raw wire bytes).
type ScopeMetrics []byte

// Metric represents a single Metric message (raw wire bytes).
type Metric []byte
```

Add methods after `ResourceMetrics.WriteTo` (around line 77), mirroring `ResourceSpans.ScopeSpans`:

```go
// ScopeMetrics returns an iterator over ScopeMetrics in this ResourceMetrics.
// Field 2 in the ResourceMetrics protobuf message.
// The returned function should be called after iteration to check for errors.
func (r ResourceMetrics) ScopeMetrics() (iter.Seq[ScopeMetrics], func() error) {
	var iterErr error

	seq := func(yield func(ScopeMetrics) bool) {
		forEachRepeatedField([]byte(r), 2, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(ScopeMetrics(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// Metrics returns an iterator over Metrics in this ScopeMetrics.
// Field 2 in the ScopeMetrics protobuf message.
// The returned function should be called after iteration to check for errors.
func (s ScopeMetrics) Metrics() (iter.Seq[Metric], func() error) {
	var iterErr error

	seq := func(yield func(Metric) bool) {
		forEachRepeatedField([]byte(s), 2, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(Metric(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -run 'TestScopeMetricsIteration' -v ./...`
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add otlpwire.go otlpwire_test.go
git commit -m "feat: add ScopeMetrics and Metric iteration"
```

---

### Task 2: Metric.Name and extractBytesField helper

**Files:**
- Modify: `otlpwire.go` (helper next to `extractResourceMessage` around line 514; method after `ScopeMetrics.Metrics`)
- Test: `otlpwire_test.go`

**Interfaces:**
- Consumes: `Metric` from Task 1, existing `skipField`.
- Produces: `func (m Metric) Name() ([]byte, error)`; internal `func extractBytesField(data []byte, fieldNum protowire.Number) ([]byte, error)` (returns `nil, nil` when absent) — reused by Task 4's KeyValue accessors.

- [ ] **Step 1: Write the failing test**

```go
func TestMetricName(t *testing.T) {
	bytes := buildScopedMetrics(t, 1, 1, 2)
	req := ExportMetricsServiceRequest(bytes)

	var names []string
	resources, resErr := req.ResourceMetrics()
	for rm := range resources {
		scopeSeq, scopeErr := rm.ScopeMetrics()
		for sm := range scopeSeq {
			metricSeq, metricErr := sm.Metrics()
			for m := range metricSeq {
				name, err := m.Name()
				require.NoError(t, err)
				names = append(names, string(name))
			}
			require.NoError(t, metricErr())
		}
		require.NoError(t, scopeErr())
	}
	require.NoError(t, resErr())

	require.Equal(t, []string{"metric.0.0", "metric.0.1"}, names)
}

func TestMetricName_Absent(t *testing.T) {
	// A metric message with only a unit (field 3), no name.
	var m Metric
	m = protowire.AppendTag(m, 3, protowire.BytesType)
	m = protowire.AppendBytes(m, []byte("1"))
	name, err := m.Name()
	require.NoError(t, err)
	require.Nil(t, name)
}
```

Add `"google.golang.org/protobuf/encoding/protowire"` to the test imports if absent.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestMetricName' ./...`
Expected: compile error — `m.Name undefined`.

- [ ] **Step 3: Write minimal implementation**

Add the helper next to `extractResourceMessage`:

```go
// extractBytesField extracts the first occurrence of a length-delimited
// field from protobuf data. Returns nil (not an error) if absent.
// The returned slice aliases data; no copy is made.
func extractBytesField(data []byte, fieldNum protowire.Number) ([]byte, error) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return nil, errors.New("wrong wire type for field")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in field")
			}
			return msgBytes, nil
		}

		n := skipField(data[pos:], wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	return nil, nil
}
```

Add the method after `ScopeMetrics.Metrics`:

```go
// Name returns the metric name (field 1) as a view into the underlying
// buffer. Returns nil if the field is not present.
func (m Metric) Name() ([]byte, error) {
	return extractBytesField([]byte(m), 1)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -run 'TestMetricName' -v ./...`
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add otlpwire.go otlpwire_test.go
git commit -m "feat: add Metric.Name and extractBytesField helper"
```

---

### Task 3: MetricType, DataPoint, and Metric.DataPoints iteration

**Files:**
- Modify: `otlpwire.go`
- Test: `otlpwire_test.go`

**Interfaces:**
- Consumes: `Metric` from Task 1, existing `forEachRepeatedField`, `skipField`.
- Produces:
  - `type MetricType int` with constants `MetricTypeGauge`, `MetricTypeSum`, `MetricTypeHistogram`, `MetricTypeExponentialHistogram`, `MetricTypeSummary`.
  - `type DataPoint struct { raw []byte; typ MetricType }` with `func (d DataPoint) Raw() []byte` and `func (d DataPoint) Type() MetricType`.
  - `func (m Metric) DataPoints() (iter.Seq[DataPoint], func() error)`.

- [ ] **Step 1: Write the failing test**

```go
// buildAllTypesMetrics builds one metric of each of the five types, each
// with two datapoints carrying attributes {"method":"GET","status":"200"}
// and timestamp 1000000000.
func buildAllTypesMetrics(t *testing.T) []byte {
	t.Helper()
	metrics := pmetric.NewMetrics()
	sm := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()

	setNumberDP := func(dp pmetric.NumberDataPoint) {
		dp.SetIntValue(42)
		dp.SetTimestamp(1000000000)
		dp.Attributes().PutStr("method", "GET")
		dp.Attributes().PutStr("status", "200")
	}

	gauge := sm.Metrics().AppendEmpty()
	gauge.SetName("test.gauge")
	setNumberDP(gauge.SetEmptyGauge().DataPoints().AppendEmpty())
	setNumberDP(gauge.Gauge().DataPoints().AppendEmpty())

	sum := sm.Metrics().AppendEmpty()
	sum.SetName("test.sum")
	setNumberDP(sum.SetEmptySum().DataPoints().AppendEmpty())
	setNumberDP(sum.Sum().DataPoints().AppendEmpty())

	hist := sm.Metrics().AppendEmpty()
	hist.SetName("test.histogram")
	histBody := hist.SetEmptyHistogram()
	for i := 0; i < 2; i++ {
		dp := histBody.DataPoints().AppendEmpty()
		dp.SetCount(10)
		dp.SetTimestamp(1000000000)
		dp.Attributes().PutStr("method", "GET")
		dp.Attributes().PutStr("status", "200")
	}

	expHist := sm.Metrics().AppendEmpty()
	expHist.SetName("test.exphistogram")
	expHistBody := expHist.SetEmptyExponentialHistogram()
	for i := 0; i < 2; i++ {
		dp := expHistBody.DataPoints().AppendEmpty()
		dp.SetCount(10)
		dp.SetTimestamp(1000000000)
		dp.Attributes().PutStr("method", "GET")
		dp.Attributes().PutStr("status", "200")
	}

	summary := sm.Metrics().AppendEmpty()
	summary.SetName("test.summary")
	summaryBody := summary.SetEmptySummary()
	for i := 0; i < 2; i++ {
		dp := summaryBody.DataPoints().AppendEmpty()
		dp.SetCount(10)
		dp.SetTimestamp(1000000000)
		dp.Attributes().PutStr("method", "GET")
		dp.Attributes().PutStr("status", "200")
	}

	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(metrics)
	require.NoError(t, err)
	return bytes
}

// forEachTestDataPoint iterates all datapoints in a marshaled request,
// failing the test on any iterator error.
func forEachTestDataPoint(t *testing.T, bytes []byte, fn func(metricName string, dp DataPoint)) {
	t.Helper()
	req := ExportMetricsServiceRequest(bytes)
	resources, resErr := req.ResourceMetrics()
	for rm := range resources {
		scopeSeq, scopeErr := rm.ScopeMetrics()
		for sm := range scopeSeq {
			metricSeq, metricErr := sm.Metrics()
			for m := range metricSeq {
				name, err := m.Name()
				require.NoError(t, err)
				dpSeq, dpErr := m.DataPoints()
				for dp := range dpSeq {
					fn(string(name), dp)
				}
				require.NoError(t, dpErr())
			}
			require.NoError(t, metricErr())
		}
		require.NoError(t, scopeErr())
	}
	require.NoError(t, resErr())
}

func TestDataPointsIteration_AllTypes(t *testing.T) {
	bytes := buildAllTypesMetrics(t)

	typeByMetric := map[string]MetricType{}
	countByMetric := map[string]int{}
	forEachTestDataPoint(t, bytes, func(name string, dp DataPoint) {
		typeByMetric[name] = dp.Type()
		countByMetric[name]++
		require.NotEmpty(t, dp.Raw())
	})

	require.Equal(t, map[string]MetricType{
		"test.gauge":        MetricTypeGauge,
		"test.sum":          MetricTypeSum,
		"test.histogram":    MetricTypeHistogram,
		"test.exphistogram": MetricTypeExponentialHistogram,
		"test.summary":      MetricTypeSummary,
	}, typeByMetric)
	for name, count := range countByMetric {
		require.Equal(t, 2, count, "metric %s", name)
	}
}

func TestDataPointsIteration_EmptyMetric(t *testing.T) {
	// Metric with a name but no oneof body.
	var m Metric
	m = protowire.AppendTag(m, 1, protowire.BytesType)
	m = protowire.AppendBytes(m, []byte("empty"))
	seq, errFn := m.DataPoints()
	count := 0
	for range seq {
		count++
	}
	require.NoError(t, errFn())
	require.Equal(t, 0, count)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestDataPointsIteration' ./...`
Expected: compile error — `MetricType` and `m.DataPoints` undefined.

- [ ] **Step 3: Write minimal implementation**

Add after the `Metric` type declaration:

```go
// MetricType identifies which oneof body a DataPoint came from.
type MetricType int

// Metric oneof body field numbers in the Metric protobuf message.
const (
	MetricTypeGauge                MetricType = 5
	MetricTypeSum                  MetricType = 7
	MetricTypeHistogram            MetricType = 9
	MetricTypeExponentialHistogram MetricType = 10
	MetricTypeSummary              MetricType = 11
)

// DataPoint represents a single datapoint message (raw wire bytes) together
// with the metric type it came from. The type is needed because the
// attributes field number differs between datapoint message types.
type DataPoint struct {
	raw []byte
	typ MetricType
}

// Raw returns the raw datapoint message bytes.
func (d DataPoint) Raw() []byte { return d.raw }

// Type returns the metric type this datapoint came from.
func (d DataPoint) Type() MetricType { return d.typ }
```

Add after `Metric.Name`:

```go
// DataPoints returns an iterator over datapoints in this Metric, descending
// whichever oneof body is present (gauge 5, sum 7, histogram 9,
// exponential_histogram 10, summary 11). Each body holds its datapoints in
// field 1.
// The returned function should be called after iteration to check for errors.
func (m Metric) DataPoints() (iter.Seq[DataPoint], func() error) {
	var iterErr error

	seq := func(yield func(DataPoint) bool) {
		data := []byte(m)
		pos := 0

		for pos < len(data) {
			fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
			if tagLen < 0 {
				iterErr = errors.New("malformed protobuf tag in metric")
				return
			}
			pos += tagLen

			typ := MetricType(fieldNum)
			isBody := typ == MetricTypeGauge || typ == MetricTypeSum ||
				typ == MetricTypeHistogram || typ == MetricTypeExponentialHistogram ||
				typ == MetricTypeSummary
			if isBody && wireType == protowire.BytesType {
				body, n := protowire.ConsumeBytes(data[pos:])
				if n < 0 {
					iterErr = errors.New("invalid bytes in metric data")
					return
				}
				pos += n

				stopped := false
				forEachRepeatedField(body, 1, func(dpBytes []byte, err error) bool {
					if err != nil {
						iterErr = err
						return false
					}
					if !yield(DataPoint{raw: dpBytes, typ: typ}) {
						stopped = true
						return false
					}
					return true
				})
				if iterErr != nil || stopped {
					return
				}
			} else {
				n := skipField(data[pos:], wireType)
				if n < 0 {
					iterErr = errors.New("failed to skip field")
					return
				}
				pos += n
			}
		}
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -run 'TestDataPointsIteration' -v ./...`
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add otlpwire.go otlpwire_test.go
git commit -m "feat: add Metric.DataPoints iteration with MetricType tagging"
```

---

### Task 4: DataPoint.Timestamp, DataPoint.Attributes, KeyValue accessors

**Files:**
- Modify: `otlpwire.go`
- Test: `otlpwire_test.go`

**Interfaces:**
- Consumes: `DataPoint` from Task 3, `extractBytesField` from Task 2, existing `forEachRepeatedField`, `skipField`.
- Produces:
  - `type KeyValue []byte` with `func (kv KeyValue) Key() ([]byte, error)` and `func (kv KeyValue) ValueRaw() ([]byte, error)`.
  - `func (d DataPoint) Timestamp() (uint64, error)` (0 when absent).
  - `func (d DataPoint) Attributes() (iter.Seq[KeyValue], func() error)`.
  - Internal `func extractFixed64Field(data []byte, fieldNum protowire.Number) (uint64, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestDataPointTimestampAndAttributes_AllTypes(t *testing.T) {
	bytes := buildAllTypesMetrics(t)

	// Expected AnyValue wire bytes for string values, built independently.
	anyValueStr := func(s string) []byte {
		var b []byte
		b = protowire.AppendTag(b, 1, protowire.BytesType) // AnyValue.string_value = 1
		b = protowire.AppendBytes(b, []byte(s))
		return b
	}
	expected := map[string][]byte{
		"method": anyValueStr("GET"),
		"status": anyValueStr("200"),
	}

	checked := 0
	forEachTestDataPoint(t, bytes, func(name string, dp DataPoint) {
		ts, err := dp.Timestamp()
		require.NoError(t, err)
		require.Equal(t, uint64(1000000000), ts)

		attrs := map[string][]byte{}
		attrSeq, attrErr := dp.Attributes()
		for kv := range attrSeq {
			key, err := kv.Key()
			require.NoError(t, err)
			val, err := kv.ValueRaw()
			require.NoError(t, err)
			attrs[string(key)] = val
		}
		require.NoError(t, attrErr())
		require.Equal(t, expected, attrs, "metric %s", name)
		checked++
	})
	require.Equal(t, 10, checked) // 5 types × 2 datapoints
}

func TestDataPointAttributes_Empty(t *testing.T) {
	metrics := pmetric.NewMetrics()
	sm := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("no.attrs")
	dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetIntValue(1)

	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(metrics)
	require.NoError(t, err)

	forEachTestDataPoint(t, bytes, func(name string, dp DataPoint) {
		ts, err := dp.Timestamp()
		require.NoError(t, err)
		require.Zero(t, ts)

		count := 0
		attrSeq, attrErr := dp.Attributes()
		for range attrSeq {
			count++
		}
		require.NoError(t, attrErr())
		require.Zero(t, count)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestDataPoint' ./...`
Expected: compile error — `dp.Timestamp`, `dp.Attributes`, `KeyValue` undefined.

- [ ] **Step 3: Write minimal implementation**

Add after the `DataPoint` type declaration:

```go
// KeyValue represents a single KeyValue message (raw wire bytes).
type KeyValue []byte

// Key returns the attribute key (field 1) as a view into the underlying
// buffer. Returns nil if the field is not present.
func (kv KeyValue) Key() ([]byte, error) {
	return extractBytesField([]byte(kv), 1)
}

// ValueRaw returns the raw AnyValue message bytes (field 2) as a view into
// the underlying buffer, suitable for type-tagged hashing.
// Returns nil if the field is not present.
func (kv KeyValue) ValueRaw() ([]byte, error) {
	return extractBytesField([]byte(kv), 2)
}
```

Add after `DataPoint.Type`:

```go
// attributesFieldNum returns the field number of the repeated KeyValue
// attributes for each datapoint message type.
func (d DataPoint) attributesFieldNum() protowire.Number {
	switch d.typ {
	case MetricTypeHistogram:
		return 9
	case MetricTypeExponentialHistogram:
		return 1
	default: // NumberDataPoint (gauge, sum) and SummaryDataPoint
		return 7
	}
}

// Timestamp returns the datapoint's time_unix_nano (field 3, fixed64).
// Returns 0 if the field is not present.
func (d DataPoint) Timestamp() (uint64, error) {
	return extractFixed64Field(d.raw, 3)
}

// Attributes returns an iterator over the datapoint's attribute KeyValues.
// The returned function should be called after iteration to check for errors.
func (d DataPoint) Attributes() (iter.Seq[KeyValue], func() error) {
	var iterErr error
	fieldNum := d.attributesFieldNum()

	seq := func(yield func(KeyValue) bool) {
		forEachRepeatedField(d.raw, fieldNum, func(rb []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(KeyValue(rb))
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}
```

Add the helper next to `extractBytesField`:

```go
// extractFixed64Field extracts the first occurrence of a fixed64 field from
// protobuf data. Returns 0 (not an error) if absent.
func extractFixed64Field(data []byte, fieldNum protowire.Number) (uint64, error) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.Fixed64Type {
				return 0, errors.New("wrong wire type for field")
			}
			v, n := protowire.ConsumeFixed64(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid fixed64 in field")
			}
			return v, nil
		}

		n := skipField(data[pos:], wireType)
		if n < 0 {
			return 0, errors.New("failed to skip field")
		}
		pos += n
	}

	return 0, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -run 'TestDataPoint' -v ./...`
Expected: both tests PASS. The all-types test proves the exponential-histogram field-1 attributes and histogram field-9 attributes are handled, since `expected` matches for every metric name.

- [ ] **Step 5: Run the full suite and vet**

Run: `go test -race ./... && go vet ./...`
Expected: all PASS, no vet findings.

- [ ] **Step 6: Commit**

```bash
git add otlpwire.go otlpwire_test.go
git commit -m "feat: add DataPoint timestamp/attribute access and KeyValue accessors"
```

---

### Task 5: Scrape-shaped benchmarks vs pdata

**Files:**
- Modify: `benchmark_comparison_test.go`
- Modify: `docs/BENCHMARKS.md` (append results section)

**Interfaces:**
- Consumes: everything from Tasks 1–4.
- Produces: `BenchmarkMetrics_ScrapeDeepIteration_WireFormat`, `BenchmarkMetrics_ScrapeDeepIteration_Unmarshal`, `BenchmarkMetrics_DeepIteration_WireFormat`, `BenchmarkMetrics_DeepIteration_Unmarshal`, helper `createScrapeShapedMetrics()`.

- [ ] **Step 1: Add the scrape-shaped fixture and benchmarks**

Append to `benchmark_comparison_test.go`:

```go
// ========== Metrics: Deep Iteration (E-2608, marigold workload) ==========

// createScrapeShapedMetrics mirrors the traffic shape from E-2601: one
// resource, one scope, thousands of metrics with a single datapoint each.
func createScrapeShapedMetrics() pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "scraped-service")
	rm.Resource().Attributes().PutStr("host.name", "host-1")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("prometheus-receiver")

	for i := 0; i < 4800; i++ {
		metric := sm.Metrics().AppendEmpty()
		metric.SetName(fmt.Sprintf("process_metric_%d_total", i))
		var dp pmetric.NumberDataPoint
		if i%2 == 0 {
			dp = metric.SetEmptyGauge().DataPoints().AppendEmpty()
		} else {
			sum := metric.SetEmptySum()
			sum.SetIsMonotonic(true)
			sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			dp = sum.DataPoints().AppendEmpty()
		}
		dp.SetDoubleValue(float64(i))
		dp.SetTimestamp(1000000000)
		dp.Attributes().PutStr("job", "node-exporter")
		dp.Attributes().PutStr("instance", fmt.Sprintf("10.0.0.%d:9100", i%250))
		dp.Attributes().PutStr("le", "0.5")
		dp.Attributes().PutStr("quantile", "0.99")
	}
	return metrics
}

// deepIterateWire simulates marigold's zero-copy hashing workload: visit
// every datapoint, read the timestamp, and consume every attribute's key
// and raw AnyValue bytes (stand-in for feeding them to xxh3).
func deepIterateWire(b *testing.B, req ExportMetricsServiceRequest) (datapoints int, consumed int) {
	resources, resErr := req.ResourceMetrics()
	for rm := range resources {
		scopeSeq, scopeErr := rm.ScopeMetrics()
		for sm := range scopeSeq {
			metricSeq, metricErr := sm.Metrics()
			for m := range metricSeq {
				dpSeq, dpErr := m.DataPoints()
				for dp := range dpSeq {
					datapoints++
					ts, err := dp.Timestamp()
					if err != nil {
						b.Fatal(err)
					}
					consumed += int(ts % 2)
					attrSeq, attrErr := dp.Attributes()
					for kv := range attrSeq {
						key, err := kv.Key()
						if err != nil {
							b.Fatal(err)
						}
						val, err := kv.ValueRaw()
						if err != nil {
							b.Fatal(err)
						}
						consumed += len(key) + len(val)
					}
					if err := attrErr(); err != nil {
						b.Fatal(err)
					}
				}
				if err := dpErr(); err != nil {
					b.Fatal(err)
				}
			}
			if err := metricErr(); err != nil {
				b.Fatal(err)
			}
		}
		if err := scopeErr(); err != nil {
			b.Fatal(err)
		}
	}
	if err := resErr(); err != nil {
		b.Fatal(err)
	}
	return datapoints, consumed
}

// deepIteratePdata is the equivalent workload through pdata: full unmarshal,
// visit every datapoint, and re-serialize each datapoint's attributes into a
// buffer for hashing (what marigold does today).
func deepIteratePdata(b *testing.B, unmarshaler *pmetric.ProtoUnmarshaler, bytes []byte) (datapoints int, consumed int) {
	metrics, err := unmarshaler.UnmarshalMetrics(bytes)
	if err != nil {
		b.Fatal(err)
	}

	buf := make([]byte, 0, 256)
	rms := metrics.ResourceMetrics()
	for ri := 0; ri < rms.Len(); ri++ {
		sms := rms.At(ri).ScopeMetrics()
		for si := 0; si < sms.Len(); si++ {
			ms := sms.At(si).Metrics()
			for mi := 0; mi < ms.Len(); mi++ {
				m := ms.At(mi)
				var dps pmetric.NumberDataPointSlice
				switch m.Type() {
				case pmetric.MetricTypeGauge:
					dps = m.Gauge().DataPoints()
				case pmetric.MetricTypeSum:
					dps = m.Sum().DataPoints()
				default:
					continue
				}
				for di := 0; di < dps.Len(); di++ {
					dp := dps.At(di)
					datapoints++
					consumed += int(uint64(dp.Timestamp()) % 2)
					buf = buf[:0]
					for k, v := range dp.Attributes().All() {
						buf = append(buf, k...)
						buf = append(buf, v.AsString()...)
					}
					consumed += len(buf)
				}
			}
		}
	}
	return datapoints, consumed
}

func BenchmarkMetrics_ScrapeDeepIteration_WireFormat(b *testing.B) {
	data := createScrapeShapedMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	req := ExportMetricsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		datapoints, _ := deepIterateWire(b, req)
		if datapoints != 4800 {
			b.Fatalf("expected 4800 datapoints, got %d", datapoints)
		}
	}
}

func BenchmarkMetrics_ScrapeDeepIteration_Unmarshal(b *testing.B) {
	data := createScrapeShapedMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	unmarshaler := &pmetric.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		datapoints, _ := deepIteratePdata(b, unmarshaler, bytes)
		if datapoints != 4800 {
			b.Fatalf("expected 4800 datapoints, got %d", datapoints)
		}
	}
}

// Continuity pair on the existing 5×100 fixture.

func BenchmarkMetrics_DeepIteration_WireFormat(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	req := ExportMetricsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		deepIterateWire(b, req)
	}
}

func BenchmarkMetrics_DeepIteration_Unmarshal(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	unmarshaler := &pmetric.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		deepIteratePdata(b, unmarshaler, bytes)
	}
}
```

Add `"fmt"` to the benchmark file imports.

Note: if `dp.Attributes().All()` does not exist in the pinned pdata version, use `dp.Attributes().Range(func(k string, v pcommon.Value) bool { buf = append(buf, k...); buf = append(buf, v.AsString()...); return true })` with import `go.opentelemetry.io/collector/pdata/pcommon` instead.

- [ ] **Step 2: Verify benchmarks compile and run briefly**

Run: `go test -run '^$' -bench 'BenchmarkMetrics_(Scrape)?DeepIteration' -benchtime=1x ./...`
Expected: all four benchmarks execute once without failure.

- [ ] **Step 3: Run the real benchmark comparison**

Run: `go test -run '^$' -bench 'BenchmarkMetrics_(Scrape)?DeepIteration' -benchmem -count=5 ./... | tee /tmp/e2608-bench.txt`
Expected: WireFormat variants show order-of-magnitude lower ns/op and B/op than Unmarshal variants on the scrape shape. If the ratio is under ~10×, investigate before proceeding (check for accidental allocations with `-gcflags='-m'` or a CPU profile).

- [ ] **Step 4: Record results in docs/BENCHMARKS.md**

Append a section to `docs/BENCHMARKS.md` titled `## Deep iteration (metrics depth, E-2608)` containing: the machine/Go version line from the benchmark output, a results table (benchmark, ns/op, B/op, allocs/op, speedup ratio), and one paragraph explaining the scrape-shaped fixture (4,800 metrics × 1 datapoint × 4 attributes) and what each side of the comparison does. Use the median of the 5 counts.

- [ ] **Step 5: Run full suite and vet**

Run: `go test -race ./... && go vet ./...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add benchmark_comparison_test.go docs/BENCHMARKS.md
git commit -m "bench: add scrape-shaped deep-iteration benchmarks vs pdata"
```

---

### Task 6: Documentation and example

**Files:**
- Modify: `README.md` (API listing/usage section)
- Modify: `example_test.go`

**Interfaces:**
- Consumes: full public API from Tasks 1–4.
- Produces: `ExampleMetric_DataPoints` runnable example.

- [ ] **Step 1: Add a runnable example**

Append to `example_test.go` (match the existing example style in that file; the example builds a small request with pdata, then walks it):

```go
func ExampleMetric_DataPoints() {
	metrics := pmetric.NewMetrics()
	sm := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("request.duration")
	dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetDoubleValue(0.35)
	dp.SetTimestamp(1000000000)
	dp.Attributes().PutStr("method", "GET")

	marshaler := &pmetric.ProtoMarshaler{}
	data, _ := marshaler.MarshalMetrics(metrics)

	req := otlpwire.ExportMetricsServiceRequest(data)
	resources, _ := req.ResourceMetrics()
	for rm := range resources {
		scopes, _ := rm.ScopeMetrics()
		for sm := range scopes {
			metricsSeq, _ := sm.Metrics()
			for m := range metricsSeq {
				name, _ := m.Name()
				dps, _ := m.DataPoints()
				for dp := range dps {
					ts, _ := dp.Timestamp()
					attrs, _ := dp.Attributes()
					for kv := range attrs {
						key, _ := kv.Key()
						fmt.Printf("%s ts=%d attr=%s\n", name, ts, key)
					}
				}
			}
		}
	}
	// Output: request.duration ts=1000000000 attr=method
}
```

Adjust imports/package prefix to match how `example_test.go` references the package (it may be an external test package `otlpwire_test` importing `go.olly.garden/otlp-wire`, or the internal package — follow the existing file).

- [ ] **Step 2: Run the example**

Run: `go test -run 'ExampleMetric_DataPoints' -v ./...`
Expected: PASS (output matched).

- [ ] **Step 3: Update README**

Add the new metrics-depth API to README.md wherever the existing API surface is listed (types `ScopeMetrics`, `Metric`, `DataPoint`, `KeyValue`, `MetricType`; the iteration chain; a note that `DataPoint` carries its metric type because attribute field numbers differ per datapoint type). Include the headline benchmark numbers from Task 5.

- [ ] **Step 4: Full suite, vet, commit**

Run: `go test -race ./... && go vet ./...`
Expected: all PASS.

```bash
git add example_test.go README.md
git commit -m "docs: document metrics-depth iteration API with example"
```
