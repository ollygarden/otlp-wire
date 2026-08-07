package otlpwire

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/encoding/protowire"
)

// ========== Metrics: Count Comparison ==========

func BenchmarkMetrics_Count_WireFormat(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	metricsData := ExportMetricsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = metricsData.DataPointCount()
	}
}

func BenchmarkMetrics_Count_Unmarshal(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	unmarshaler := &pmetric.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		metrics, err := unmarshaler.UnmarshalMetrics(bytes)
		if err != nil {
			b.Fatal(err)
		}

		_ = metrics.DataPointCount()
	}
}

// ========== Metrics: Split Comparison ==========

func BenchmarkMetrics_Split_WireFormat(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	metricsData := ExportMetricsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, getErr := metricsData.ResourceMetrics()
		for range resources {
		}
		_ = getErr()
	}
}

func BenchmarkMetrics_Split_UnmarshalRemarshal(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	unmarshaler := &pmetric.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		metrics, err := unmarshaler.UnmarshalMetrics(bytes)
		if err != nil {
			b.Fatal(err)
		}

		// Split by creating new metrics for each resource
		splits := make([][]byte, metrics.ResourceMetrics().Len())
		for ri := 0; ri < metrics.ResourceMetrics().Len(); ri++ {
			newMetrics := pmetric.NewMetrics()
			metrics.ResourceMetrics().At(ri).CopyTo(newMetrics.ResourceMetrics().AppendEmpty())

			splitBytes, err := marshaler.MarshalMetrics(newMetrics)
			if err != nil {
				b.Fatal(err)
			}
			splits[ri] = splitBytes
		}
		_ = splits
	}
}

// ========== Traces: Count Comparison ==========

func BenchmarkTraces_Count_WireFormat(b *testing.B) {
	data := createBenchTraces()
	marshaler := &ptrace.ProtoMarshaler{}
	bytes, err := marshaler.MarshalTraces(data)
	require.NoError(b, err)

	tracesData := ExportTracesServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = tracesData.SpanCount()
	}
}

func BenchmarkTraces_Count_Unmarshal(b *testing.B) {
	data := createBenchTraces()
	marshaler := &ptrace.ProtoMarshaler{}
	bytes, err := marshaler.MarshalTraces(data)
	require.NoError(b, err)

	unmarshaler := &ptrace.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		traces, err := unmarshaler.UnmarshalTraces(bytes)
		if err != nil {
			b.Fatal(err)
		}

		_ = traces.SpanCount()
	}
}

// ========== Traces: Split Comparison ==========

func BenchmarkTraces_Split_WireFormat(b *testing.B) {
	data := createBenchTraces()
	marshaler := &ptrace.ProtoMarshaler{}
	bytes, err := marshaler.MarshalTraces(data)
	require.NoError(b, err)

	tracesData := ExportTracesServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, getErr := tracesData.ResourceSpans()
		for range resources {
		}
		_ = getErr()
	}
}

func BenchmarkTraces_Split_UnmarshalRemarshal(b *testing.B) {
	data := createBenchTraces()
	marshaler := &ptrace.ProtoMarshaler{}
	bytes, err := marshaler.MarshalTraces(data)
	require.NoError(b, err)

	unmarshaler := &ptrace.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		traces, err := unmarshaler.UnmarshalTraces(bytes)
		if err != nil {
			b.Fatal(err)
		}

		// Split by creating new traces for each resource
		splits := make([][]byte, traces.ResourceSpans().Len())
		for ri := 0; ri < traces.ResourceSpans().Len(); ri++ {
			newTraces := ptrace.NewTraces()
			traces.ResourceSpans().At(ri).CopyTo(newTraces.ResourceSpans().AppendEmpty())

			splitBytes, err := marshaler.MarshalTraces(newTraces)
			if err != nil {
				b.Fatal(err)
			}
			splits[ri] = splitBytes
		}
		_ = splits
	}
}

// ========== Logs: Count Comparison ==========

func BenchmarkLogs_Count_WireFormat(b *testing.B) {
	data := createBenchLogs()
	marshaler := &plog.ProtoMarshaler{}
	bytes, err := marshaler.MarshalLogs(data)
	require.NoError(b, err)

	logsData := ExportLogsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = logsData.LogRecordCount()
	}
}

func BenchmarkLogs_Count_Unmarshal(b *testing.B) {
	data := createBenchLogs()
	marshaler := &plog.ProtoMarshaler{}
	bytes, err := marshaler.MarshalLogs(data)
	require.NoError(b, err)

	unmarshaler := &plog.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logs, err := unmarshaler.UnmarshalLogs(bytes)
		if err != nil {
			b.Fatal(err)
		}

		_ = logs.LogRecordCount()
	}
}

// ========== Logs: Split Comparison ==========

func BenchmarkLogs_Split_WireFormat(b *testing.B) {
	data := createBenchLogs()
	marshaler := &plog.ProtoMarshaler{}
	bytes, err := marshaler.MarshalLogs(data)
	require.NoError(b, err)

	logsData := ExportLogsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, getErr := logsData.ResourceLogs()
		for range resources {
		}
		_ = getErr()
	}
}

func BenchmarkLogs_Split_UnmarshalRemarshal(b *testing.B) {
	data := createBenchLogs()
	marshaler := &plog.ProtoMarshaler{}
	bytes, err := marshaler.MarshalLogs(data)
	require.NoError(b, err)

	unmarshaler := &plog.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logs, err := unmarshaler.UnmarshalLogs(bytes)
		if err != nil {
			b.Fatal(err)
		}

		// Split by creating new logs for each resource
		splits := make([][]byte, logs.ResourceLogs().Len())
		for ri := 0; ri < logs.ResourceLogs().Len(); ri++ {
			newLogs := plog.NewLogs()
			logs.ResourceLogs().At(ri).CopyTo(newLogs.ResourceLogs().AppendEmpty())

			splitBytes, err := marshaler.MarshalLogs(newLogs)
			if err != nil {
				b.Fatal(err)
			}
			splits[ri] = splitBytes
		}
		_ = splits
	}
}

// benchmarkLogSeveritySink prevents severity classification and service-context
// reads from being optimized away in the paired benchmarks below.
var benchmarkLogSeveritySink int

// BenchmarkLogs_SeverityClassification_WireFormat measures the complete
// resource-context, record-iteration, and severity-classification path used by
// log insight consumers without unmarshaling the OTLP payload.
func BenchmarkLogs_SeverityClassification_WireFormat(b *testing.B) {
	data := createSeverityClassificationBenchLogs()
	bytes, err := (&plog.ProtoMarshaler{}).MarshalLogs(data)
	require.NoError(b, err)
	request := ExportLogsServiceRequest(bytes)
	wireResult, err := classifyWireLogSeverities(request)
	require.NoError(b, err)
	pdataResult, err := classifyPdataLogSeverities(bytes, &plog.ProtoUnmarshaler{})
	require.NoError(b, err)
	require.Equal(b, pdataResult, wireResult, "wire and pdata paths must classify the same fixture identically")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := classifyWireLogSeverities(request)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkLogSeveritySink = result.score()
	}
}

// BenchmarkLogs_SeverityClassification_Unmarshal measures the same work as
// BenchmarkLogs_SeverityClassification_WireFormat after full pdata unmarshal.
func BenchmarkLogs_SeverityClassification_Unmarshal(b *testing.B) {
	data := createSeverityClassificationBenchLogs()
	bytes, err := (&plog.ProtoMarshaler{}).MarshalLogs(data)
	require.NoError(b, err)
	unmarshaler := &plog.ProtoUnmarshaler{}
	wireResult, err := classifyWireLogSeverities(ExportLogsServiceRequest(bytes))
	require.NoError(b, err)
	pdataResult, err := classifyPdataLogSeverities(bytes, unmarshaler)
	require.NoError(b, err)
	require.Equal(b, wireResult, pdataResult, "wire and pdata paths must classify the same fixture identically")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := classifyPdataLogSeverities(bytes, unmarshaler)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkLogSeveritySink = result.score()
	}
}

type logSeverityClassification struct {
	counts          [6]int
	contextStrings  int
	contextByteSize int
}

func (result logSeverityClassification) score() int {
	score := result.contextStrings + result.contextByteSize
	for class, count := range result.counts {
		score += class * count
	}
	return score
}

func classifyWireLogSeverities(request ExportLogsServiceRequest) (logSeverityClassification, error) {
	var result logSeverityClassification
	resources, resourceErr := request.ResourceLogs()
	for resource := range resources {
		res, err := resource.Resource()
		if err != nil {
			return logSeverityClassification{}, err
		}
		for _, key := range []string{"service.name", "deployment.environment"} {
			value, found, err := res.StringAttribute(key)
			if err != nil {
				return logSeverityClassification{}, err
			}
			if found {
				result.contextStrings++
				result.contextByteSize += len(value)
			}
		}

		scopes, scopeErr := resource.ScopeLogs()
		for scope := range scopes {
			for record, err := range scope.LogRecordsSeq {
				if err != nil {
					return logSeverityClassification{}, err
				}
				severity, err := record.SeverityNumber()
				if err != nil {
					return logSeverityClassification{}, err
				}
				result.counts[logSeverityClass(severity)]++
			}
		}
		if err := scopeErr(); err != nil {
			return logSeverityClassification{}, err
		}
	}
	if err := resourceErr(); err != nil {
		return logSeverityClassification{}, err
	}
	return result, nil
}

func classifyPdataLogSeverities(data []byte, unmarshaler *plog.ProtoUnmarshaler) (logSeverityClassification, error) {
	logs, err := unmarshaler.UnmarshalLogs(data)
	if err != nil {
		return logSeverityClassification{}, err
	}
	var result logSeverityClassification
	for resourceIndex := 0; resourceIndex < logs.ResourceLogs().Len(); resourceIndex++ {
		resource := logs.ResourceLogs().At(resourceIndex)
		for _, key := range []string{"service.name", "deployment.environment"} {
			if value, ok := resource.Resource().Attributes().Get(key); ok && value.Type() == pcommon.ValueTypeStr {
				result.contextStrings++
				result.contextByteSize += len(value.Str())
			}
		}
		for scopeIndex := 0; scopeIndex < resource.ScopeLogs().Len(); scopeIndex++ {
			scope := resource.ScopeLogs().At(scopeIndex)
			for recordIndex := 0; recordIndex < scope.LogRecords().Len(); recordIndex++ {
				record := scope.LogRecords().At(recordIndex)
				result.counts[logSeverityClass(int32(record.SeverityNumber()))]++
			}
		}
	}
	return result, nil
}

func logSeverityClass(severity int32) int {
	switch {
	case severity < 1:
		return 0
	case severity <= 4:
		return 1
	case severity <= 8:
		return 2
	case severity <= 12:
		return 3
	case severity <= 16:
		return 4
	default:
		return 5
	}
}

// ========== Helper Functions ==========

func createBenchMetrics() pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	for i := 0; i < 5; i++ {
		rm := metrics.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))
		rm.Resource().Attributes().PutStr("host.name", "host-"+string(rune('1'+i)))
		rm.Resource().Attributes().PutStr("deployment.environment", "production")

		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("test-instrumentation")
		sm.Scope().SetVersion("1.0.0")

		metric := sm.Metrics().AppendEmpty()
		metric.SetName("request.count")
		metric.SetDescription("Number of requests")
		metric.SetUnit("1")
		gauge := metric.SetEmptyGauge()

		for j := 0; j < 100; j++ {
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetIntValue(int64(j))
			dp.SetTimestamp(1000000000)
			dp.Attributes().PutStr("method", "GET")
			dp.Attributes().PutStr("status", "200")
		}
	}
	return metrics
}

func createBenchTraces() ptrace.Traces {
	traces := ptrace.NewTraces()
	for i := 0; i < 5; i++ {
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))
		rs.Resource().Attributes().PutStr("host.name", "host-"+string(rune('1'+i)))
		rs.Resource().Attributes().PutStr("deployment.environment", "production")

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("test-instrumentation")
		ss.Scope().SetVersion("1.0.0")

		for j := 0; j < 100; j++ {
			span := ss.Spans().AppendEmpty()
			span.SetName("test.operation")
			span.SetKind(ptrace.SpanKindServer)
			span.SetStartTimestamp(1000000000)
			span.SetEndTimestamp(1000001000)
			span.Attributes().PutStr("http.method", "GET")
			span.Attributes().PutStr("http.status_code", "200")
		}
	}
	return traces
}

func createBenchLogs() plog.Logs {
	logs := plog.NewLogs()
	for i := 0; i < 5; i++ {
		rl := logs.ResourceLogs().AppendEmpty()
		rl.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))
		rl.Resource().Attributes().PutStr("host.name", "host-"+string(rune('1'+i)))
		rl.Resource().Attributes().PutStr("deployment.environment", "production")

		sl := rl.ScopeLogs().AppendEmpty()
		sl.Scope().SetName("test-instrumentation")
		sl.Scope().SetVersion("1.0.0")

		for j := 0; j < 100; j++ {
			lr := sl.LogRecords().AppendEmpty()
			lr.Body().SetStr("Test log message with some content")
			lr.SetTimestamp(1000000000)
			lr.SetSeverityNumber(plog.SeverityNumberInfo)
			lr.SetSeverityText("INFO")
			lr.Attributes().PutStr("log.level", "info")
			lr.Attributes().PutStr("logger.name", "test.logger")
		}
	}
	return logs
}

// createSeverityClassificationBenchLogs covers every severity class plus
// absent, empty, and non-string resource context values. The paired benchmark
// asserts wire/pdata parity before timing this representative consumer path.
func createSeverityClassificationBenchLogs() plog.Logs {
	logs := plog.NewLogs()
	severities := []plog.SeverityNumber{
		plog.SeverityNumberUnspecified,
		plog.SeverityNumberTrace,
		plog.SeverityNumberDebug,
		plog.SeverityNumberInfo,
		plog.SeverityNumberWarn,
		plog.SeverityNumberError,
	}

	for resourceIndex := 0; resourceIndex < 5; resourceIndex++ {
		resource := logs.ResourceLogs().AppendEmpty()
		switch resourceIndex {
		case 0:
			resource.Resource().Attributes().PutStr("service.name", "checkout")
			resource.Resource().Attributes().PutStr("deployment.environment", "production")
		case 1:
			resource.Resource().Attributes().PutStr("service.name", "")
			resource.Resource().Attributes().PutStr("deployment.environment", "staging")
		case 2:
			resource.Resource().Attributes().PutInt("service.name", 42)
			resource.Resource().Attributes().PutStr("deployment.environment", "development")
		case 3:
			resource.Resource().Attributes().PutStr("service.name", "payments")
			resource.Resource().Attributes().PutInt("deployment.environment", 7)
		case 4:
			// Deliberately omit both attributes.
		}

		scope := resource.ScopeLogs().AppendEmpty()
		for recordIndex := 0; recordIndex < 120; recordIndex++ {
			record := scope.LogRecords().AppendEmpty()
			record.Body().SetStr("representative application log message")
			record.Attributes().PutStr("logger.name", "checkout.handler")
			record.Attributes().PutStr("http.request.method", "GET")
			severity := severities[recordIndex%len(severities)]
			if severity != plog.SeverityNumberUnspecified {
				record.SetSeverityNumber(severity)
			}
		}
	}
	return logs
}

// ========== Metrics: Pure Iterator Comparison ==========

func BenchmarkMetrics_Iterator_WireFormat(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	metricsData := ExportMetricsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, getErr := metricsData.ResourceMetrics()
		for range resources {
		}
		_ = getErr()
	}
}

func BenchmarkMetrics_Iterator_Unmarshal(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	unmarshaler := &pmetric.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		metrics, err := unmarshaler.UnmarshalMetrics(bytes)
		if err != nil {
			b.Fatal(err)
		}

		for ri := 0; ri < metrics.ResourceMetrics().Len(); ri++ {
			_ = metrics.ResourceMetrics().At(ri)
		}
	}
}

// ========== Traces: Pure Iterator Comparison ==========

func BenchmarkTraces_Iterator_WireFormat(b *testing.B) {
	data := createBenchTraces()
	marshaler := &ptrace.ProtoMarshaler{}
	bytes, err := marshaler.MarshalTraces(data)
	require.NoError(b, err)

	tracesData := ExportTracesServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, getErr := tracesData.ResourceSpans()
		for range resources {
		}
		_ = getErr()
	}
}

func BenchmarkTraces_Iterator_Unmarshal(b *testing.B) {
	data := createBenchTraces()
	marshaler := &ptrace.ProtoMarshaler{}
	bytes, err := marshaler.MarshalTraces(data)
	require.NoError(b, err)

	unmarshaler := &ptrace.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		traces, err := unmarshaler.UnmarshalTraces(bytes)
		if err != nil {
			b.Fatal(err)
		}

		for ri := 0; ri < traces.ResourceSpans().Len(); ri++ {
			_ = traces.ResourceSpans().At(ri)
		}
	}
}

// ========== Logs: Pure Iterator Comparison ==========

func BenchmarkLogs_Iterator_WireFormat(b *testing.B) {
	data := createBenchLogs()
	marshaler := &plog.ProtoMarshaler{}
	bytes, err := marshaler.MarshalLogs(data)
	require.NoError(b, err)

	logsData := ExportLogsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, getErr := logsData.ResourceLogs()
		for range resources {
		}
		_ = getErr()
	}
}

func BenchmarkLogs_Iterator_Unmarshal(b *testing.B) {
	data := createBenchLogs()
	marshaler := &plog.ProtoMarshaler{}
	bytes, err := marshaler.MarshalLogs(data)
	require.NoError(b, err)

	unmarshaler := &plog.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logs, err := unmarshaler.UnmarshalLogs(bytes)
		if err != nil {
			b.Fatal(err)
		}

		for ri := 0; ri < logs.ResourceLogs().Len(); ri++ {
			_ = logs.ResourceLogs().At(ri)
		}
	}
}

// ========== Resource Extraction Comparison ==========

func BenchmarkMetrics_ResourceExtraction_WireFormat(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	metricsData := ExportMetricsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, getErr := metricsData.ResourceMetrics()
		for rm := range resources {
			_, _ = rm.Resource()
		}
		_ = getErr()
	}
}

func BenchmarkMetrics_ResourceExtraction_Unmarshal(b *testing.B) {
	data := createBenchMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	unmarshaler := &pmetric.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		metrics, err := unmarshaler.UnmarshalMetrics(bytes)
		if err != nil {
			b.Fatal(err)
		}

		for ri := 0; ri < metrics.ResourceMetrics().Len(); ri++ {
			_ = metrics.ResourceMetrics().At(ri).Resource()
		}
	}
}

// BenchmarkResource_SingleOccurrence isolates ResourceMetrics.Resource()
// itself (no outer iterator cost) on the common, single-Resource-occurrence
// container that every real producer emits. E-2941's hard performance gate is
// that this path stays zero-allocation: extracting the Resource returns a
// slice aliasing the input instead of copying it, even though the underlying
// extractor now scans the complete container (to find every Resource
// occurrence for merging) rather than returning as soon as it finds the
// field.
func BenchmarkResource_SingleOccurrence(b *testing.B) {
	container := containerWithResource(resourceWithStringAttr("service.name", "checkout"))
	rm := ResourceMetrics(container)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rm.Resource()
	}
}

// BenchmarkResource_MultipleOccurrences is the paired benchmark for the
// documented exception: 2+ Resource occurrences require concatenating the
// encoded bodies into a new buffer, so this path allocates.
func BenchmarkResource_MultipleOccurrences(b *testing.B) {
	container := containerWithResources(
		resourceWithStringAttr("service.name", "checkout"),
		resourceWithStringAttr("deployment.environment", "prod"),
		resourceWithStringAttr("host.name", "host-1"),
	)
	rm := ResourceMetrics(container)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rm.Resource()
	}
}

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

// deepIterateWireSeq is deepIterateWire using the zero-allocation Seq
// variants for the two per-element levels.
func deepIterateWireSeq(b *testing.B, req ExportMetricsServiceRequest) (datapoints int, consumed int) {
	resources, resErr := req.ResourceMetrics()
	for rm := range resources {
		scopeSeq, scopeErr := rm.ScopeMetrics()
		for sm := range scopeSeq {
			metricSeq, metricErr := sm.Metrics()
			for m := range metricSeq {
				for dp, err := range m.DataPointsSeq {
					if err != nil {
						b.Fatal(err)
					}
					datapoints++
					ts, err := dp.Timestamp()
					if err != nil {
						b.Fatal(err)
					}
					consumed += int(ts % 2)
					for kv, err := range dp.AttributesSeq {
						if err != nil {
							b.Fatal(err)
						}
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

func BenchmarkMetrics_ScrapeDeepIterationSeq_WireFormat(b *testing.B) {
	data := createScrapeShapedMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	bytes, err := marshaler.MarshalMetrics(data)
	require.NoError(b, err)

	req := ExportMetricsServiceRequest(bytes)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		datapoints, _ := deepIterateWireSeq(b, req)
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

// containerWithScopes builds a resource container holding one Resource
// followed by n scope entries, so the cost of locating the Resource can be
// measured against the container's top-level field count.
func containerWithScopes(n int) []byte {
	res := resourceWithStringAttr("service.name", "checkout")
	out := protowire.AppendTag(nil, 1, protowire.BytesType)
	out = protowire.AppendVarint(out, uint64(len(res)))
	out = append(out, res...)

	scope := make([]byte, 64)
	for i := 0; i < n; i++ {
		out = protowire.AppendTag(out, 2, protowire.BytesType)
		out = protowire.AppendVarint(out, uint64(len(scope)))
		out = append(out, scope...)
	}
	return out
}

// BenchmarkResource_ScanScaling pins the complexity of Resource(). Merging
// repeated occurrences requires scanning every top-level field of the
// container, so the cost grows with the number of scope entries rather than
// staying constant the way the pre-v0.1.0 first-match-and-return
// implementation did. These numbers are the evidence behind the scaling note
// in docs/BENCHMARKS.md; keep them honest if the traversal changes.
func BenchmarkResource_ScanScaling(b *testing.B) {
	for _, scopes := range []int{1, 10, 50} {
		b.Run(fmt.Sprintf("scopes=%d", scopes), func(b *testing.B) {
			rm := ResourceMetrics(containerWithScopes(scopes))
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = rm.Resource()
			}
		})
	}
}
