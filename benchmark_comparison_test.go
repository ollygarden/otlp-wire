package otlpwire

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
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

// BenchmarkResource_SingleOccurrence covers the common single-occurrence
// container. The performance gate is that this path stays zero-allocation.
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
	return createScrapeShapedMetrics2(false)
}

// createScrapeShapedMetricsWithMetadata is the same shape built for the
// metadata arms: it attaches the three metadata entries a Prometheus receiver
// sets on every metric, and omits the datapoint attributes, which those arms
// never read and which would otherwise dominate the per-metric skip cost.
func createScrapeShapedMetricsWithMetadata() pmetric.Metrics {
	return createScrapeShapedMetrics2(true)
}

func createScrapeShapedMetrics2(forMetadataWalk bool) pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "scraped-service")
	rm.Resource().Attributes().PutStr("host.name", "host-1")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("prometheus-receiver")

	for i := 0; i < 4800; i++ {
		metric := sm.Metrics().AppendEmpty()
		metric.SetName(fmt.Sprintf("process_metric_%d_total", i))
		if forMetadataWalk {
			metric.SetUnit("1")
			metric.Metadata().PutStr("prometheus.type", "counter")
			metric.Metadata().PutStr("prometheus.help", "total processed items")
			metric.Metadata().PutStr("otel.scope.name", "prometheus-receiver")
		}
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
		if !forMetadataWalk {
			dp.Attributes().PutStr("job", "node-exporter")
			dp.Attributes().PutStr("instance", fmt.Sprintf("10.0.0.%d:9100", i%250))
			dp.Attributes().PutStr("le", "0.5")
			dp.Attributes().PutStr("quantile", "0.99")
		}
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

// BenchmarkResource_ScanScaling pins the complexity of Resource(): merging
// requires scanning every top-level field, so cost grows with the scope count
// instead of staying constant. Backs the scaling table in docs/BENCHMARKS.md.
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

// ========== Scope and schema_url (E-2942) ==========

// scopeContainerWithRecords builds a ScopeLogs carrying one scope (field 1),
// n log records (field 2), and a schema_url (field 3). The scope and
// schema_url accessors must scan past every record to honor merge and
// last-value-wins semantics, so this fixture exposes that cost.
func scopeContainerWithRecords(n int) []byte {
	scope := scopeMessage("checkout-instr", "1.2.3")
	out := protowire.AppendTag(nil, 1, protowire.BytesType)
	out = protowire.AppendVarint(out, uint64(len(scope)))
	out = append(out, scope...)

	// A valid, decodable LogRecord body of a pinned size. The accessors under
	// test never look inside a record — they read its tag and length and step
	// over it — but an undecodable filler would be a trap for anyone who later
	// makes this fixture do more. 64 bytes matches containerWithScopes above,
	// so the two scaling benchmarks stay comparable.
	record := protowire.AppendTag(nil, 5, protowire.BytesType) // LogRecord.body
	body := protowire.AppendTag(nil, 1, protowire.BytesType)   // AnyValue.string_value
	body = protowire.AppendString(body, strings.Repeat("x", 60))
	record = protowire.AppendVarint(record, uint64(len(body)))
	record = append(record, body...)

	for i := 0; i < n; i++ {
		out = protowire.AppendTag(out, 2, protowire.BytesType)
		out = protowire.AppendVarint(out, uint64(len(record)))
		out = append(out, record...)
	}

	out = protowire.AppendTag(out, 3, protowire.BytesType)
	return protowire.AppendString(out, "https://example.test/schema/v1")
}

// BenchmarkScope_SingleOccurrence is the hot path: one scope occurrence
// returns a slice aliasing the input, with no allocation.
func BenchmarkScope_SingleOccurrence(b *testing.B) {
	sm := ScopeMetrics(scopeContainer(scopeMessage("checkout-instr", "1.2.3")))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = sm.Scope()
	}
}

// BenchmarkScope_MultipleOccurrences is the paired benchmark for the
// documented exception: 2+ occurrences concatenate into a new buffer.
func BenchmarkScope_MultipleOccurrences(b *testing.B) {
	sm := ScopeMetrics(scopeContainer(
		scopeMessage("checkout-instr", ""),
		scopeMessage("", "1.2.3"),
		scopeWithStringAttr("library.language", "go"),
	))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = sm.Scope()
	}
}

// minimalScopeRequest builds an ExportLogsServiceRequest carrying exactly one
// resource, one scope container and one populated scope, so the paired
// benchmarks below compare scope access against decoding the same bytes.
func minimalScopeRequest() []byte {
	sc := scopeContainer(append(
		scopeMessage("checkout-instr", "1.2.3"),
		scopeWithStringAttr("library.language", "go")...))
	return wrapAsRequest(resourceContainerWithScope(sc))
}

// BenchmarkScope_NameVersion_WireFormat reads scope name and version straight
// from the wire. Paired with BenchmarkScope_NameVersion_Unmarshal.
func BenchmarkScope_NameVersion_WireFormat(b *testing.B) {
	payload := minimalScopeRequest()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, resErr := ExportLogsServiceRequest(payload).ResourceLogs()
		for rl := range resources {
			scopes, scopeErr := rl.ScopeLogs()
			for sl := range scopes {
				scope, err := sl.Scope()
				if err != nil {
					b.Fatal(err)
				}
				if _, err := scope.Name(); err != nil {
					b.Fatal(err)
				}
				if _, err := scope.Version(); err != nil {
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
	}
}

// BenchmarkScope_NameVersion_Unmarshal is the full-decode baseline for the
// same bytes. It stands in for the consumer code this API replaces (dibber
// unmarshals a generated InstrumentationScope per scope; sage strict-parses
// one per occurrence); it is not those implementations verbatim, since
// replicating them would add a production proto module as a dependency.
func BenchmarkScope_NameVersion_Unmarshal(b *testing.B) {
	payload := minimalScopeRequest()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := plogotlp.NewExportRequest()
		if err := req.UnmarshalProto(payload); err != nil {
			b.Fatal(err)
		}
		logs := req.Logs()
		for r := 0; r < logs.ResourceLogs().Len(); r++ {
			rl := logs.ResourceLogs().At(r)
			for s := 0; s < rl.ScopeLogs().Len(); s++ {
				scope := rl.ScopeLogs().At(s).Scope()
				_, _ = scope.Name(), scope.Version()
			}
		}
	}
}

// BenchmarkScope_ScanScaling pins the complexity of Scope(): merging requires
// scanning every top-level field, so cost grows with the record count rather
// than staying constant. This matters more than the Resource equivalent
// because a scope container holds records, not scopes. Backs the scaling
// table in docs/BENCHMARKS.md.
func BenchmarkScope_ScanScaling(b *testing.B) {
	for _, records := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("records=%d", records), func(b *testing.B) {
			sl := ScopeLogs(scopeContainerWithRecords(records))
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = sl.Scope()
			}
		})
	}
}

// BenchmarkSchemaUrl_ScanScaling pins the same cost for the scalar accessor,
// which must reach the last occurrence and therefore cannot stop early.
func BenchmarkSchemaUrl_ScanScaling(b *testing.B) {
	for _, records := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("records=%d", records), func(b *testing.B) {
			sl := ScopeLogs(scopeContainerWithRecords(records))
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = sl.SchemaUrl()
			}
		})
	}
}

// BenchmarkMetric_Name reads every metric name from a pdata-marshalled
// payload. pdata's ProtoMarshaler writes fields back-to-front, so name lands
// last in each metric and a first-match scan has to walk the whole metric
// anyway. That makes this arm blind to the resolution change on its own --
// BenchmarkMetric_Name_SDKOrder is the one that sees it. Keep both.
func BenchmarkMetric_Name(b *testing.B) {
	metrics := createScrapeShapedMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	payload, err := marshaler.MarshalMetrics(metrics)
	require.NoError(b, err)

	benchmarkMetricNames(b, payload)
}

// BenchmarkMetric_Name_SDKOrder reads the same names from the same shape in
// ascending field order, where the OTLP SDK exporters put name first. This is
// where first-match and last-value-wins actually differ: last-value-wins has
// to walk past the datapoint body to prove no later name occurrence exists,
// while first-match returns at the first tag.
func BenchmarkMetric_Name_SDKOrder(b *testing.B) {
	benchmarkMetricNames(b, createScrapeShapedMetricsSDKOrder())
}

// BenchmarkMetric_Name_Accessor isolates the accessor from the iteration
// around it, on the metric shape a Prometheus receiver produces: name, unit,
// a datapoint body and three metadata entries. The two arms differ only in
// field order, so the gap between them is the cost of resolving name against
// fields that follow it.
func BenchmarkMetric_Name_Accessor(b *testing.B) {
	for _, ascending := range []bool{true, false} {
		label := "PdataOrder"
		if ascending {
			label = "SDKOrder"
		}
		metric := prometheusShapedMetric(ascending)
		b.Run(label, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := metric.Name(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// prometheusShapedMetric builds one Metric carrying the fields a Prometheus
// receiver sets. ascending emits fields 1, 3, 5 and 12 in wire order the way
// the SDK exporters do; otherwise it emits them back-to-front the way pdata
// does.
func prometheusShapedMetric(ascending bool) Metric {
	name := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), "process_metric_1234_total")
	unit := protowire.AppendString(protowire.AppendTag(nil, 3, protowire.BytesType), "1")
	body := bytesField(5, bytesField(1, numberDataPoint(42)))

	var metadata []byte
	for _, entry := range [][2]string{
		{"prometheus.type", "counter"},
		{"prometheus.help", "total processed items"},
		{"otel.scope.name", "prometheus-receiver"},
	} {
		metadata = append(metadata, metadataEntry(stringKeyValue(entry[0], entry[1]))...)
	}

	if ascending {
		return Metric(slices.Concat(name, unit, body, metadata))
	}
	return Metric(slices.Concat(metadata, body, unit, name))
}

// numberDataPoint builds a NumberDataPoint in ascending field order:
// time_unix_nano (3), as_double (4), then any attributes (7).
func numberDataPoint(value float64, attrs ...[]byte) []byte {
	dp := protowire.AppendTag(nil, 3, protowire.Fixed64Type)
	dp = protowire.AppendFixed64(dp, 1000000000)
	dp = protowire.AppendTag(dp, 4, protowire.Fixed64Type)
	dp = protowire.AppendFixed64(dp, math.Float64bits(value))
	return appendRepeatedMessages(dp, 7, attrs...)
}

func benchmarkMetricNames(b *testing.B, payload []byte) {
	b.Helper()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resources, resErr := ExportMetricsServiceRequest(payload).ResourceMetrics()
		for rm := range resources {
			scopes, scopeErr := rm.ScopeMetrics()
			for sm := range scopes {
				ms, mErr := sm.Metrics()
				for m := range ms {
					if _, err := m.Name(); err != nil {
						b.Fatal(err)
					}
				}
				if err := mErr(); err != nil {
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
	}
}

// createScrapeShapedMetricsSDKOrder builds the same scrape shape as
// createScrapeShapedMetrics -- one resource, one scope, 4800 metrics with one
// datapoint each -- but emits every message with ascending field numbers, the
// way stock protobuf runtimes and therefore the OTLP SDK exporters do. It is
// built with protowire rather than pdata precisely because pdata cannot
// produce this order.
func createScrapeShapedMetricsSDKOrder() []byte {
	resource := appendRepeatedMessages(nil, 1,
		stringKeyValue("service.name", "scraped-service"),
		stringKeyValue("host.name", "host-1"),
	)

	scope := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), "prometheus-receiver")
	scopeMetrics := bytesField(1, scope)

	for i := 0; i < 4800; i++ {
		points := bytesField(1, numberDataPoint(float64(i),
			stringKeyValue("job", "node-exporter"),
			stringKeyValue("instance", fmt.Sprintf("10.0.0.%d:9100", i%250)),
			stringKeyValue("le", "0.5"),
			stringKeyValue("quantile", "0.99"),
		))

		metric := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), fmt.Sprintf("process_metric_%d_total", i))
		if i%2 == 0 {
			metric = append(metric, bytesField(5, points)...)
		} else {
			sum := append(points, varintField(2, uint64(pmetric.AggregationTemporalityCumulative))...)
			sum = append(sum, varintField(3, 1)...)
			metric = append(metric, bytesField(7, sum)...)
		}

		scopeMetrics = appendRepeatedMessages(scopeMetrics, 2, metric)
	}

	resourceMetrics := append(bytesField(1, resource), bytesField(2, scopeMetrics)...)
	return wrapAsRequest(resourceMetrics)
}

// ========== Metric.Metadata ==========

type benchMetadataAttribute struct{ key, value []byte }

// forEachBytesFieldHandRolled is the field-12 walk marigold hand-rolls in
// internal/detector/wire.go, copied verbatim as the "before" arm so the
// comparison reproduces from this repository alone. Drop this arm and its
// docs/BENCHMARKS.md section once marigold moves to Metric.Metadata.
func forEachBytesFieldHandRolled(data []byte, want protowire.Number, fn func([]byte) error) error {
	for len(data) > 0 {
		field, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return errors.New("malformed protobuf tag")
		}
		data = data[n:]
		if field == want {
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for bytes field")
			}
			value, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return errors.New("malformed protobuf bytes field")
			}
			if err := fn(value); err != nil {
				return err
			}
			data = data[n:]
			continue
		}
		n = protowire.ConsumeFieldValue(field, typ, data)
		if n < 0 {
			return errors.New("malformed protobuf field")
		}
		data = data[n:]
	}
	return nil
}

func benchMetadataPayload(b *testing.B) []byte {
	b.Helper()
	marshaler := &pmetric.ProtoMarshaler{}
	payload, err := marshaler.MarshalMetrics(createScrapeShapedMetricsWithMetadata())
	require.NoError(b, err)
	return payload
}

// Keep the arms as separate functions: dispatching through a func value
// defeats inlining and changes what is measured.
func forEachBenchMetric(b *testing.B, payload []byte, fn func(Metric)) {
	resources, resErr := ExportMetricsServiceRequest(payload).ResourceMetrics()
	for rm := range resources {
		scopes, scopeErr := rm.ScopeMetrics()
		for sm := range scopes {
			ms, mErr := sm.Metrics()
			for m := range ms {
				fn(m)
			}
			if err := mErr(); err != nil {
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
}

func BenchmarkMetric_Metadata_HandRolled(b *testing.B) {
	payload := benchMetadataPayload(b)
	attrs := make([]benchMetadataAttribute, 0, 8)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		forEachBenchMetric(b, payload, func(m Metric) {
			attrs = attrs[:0]
			err := forEachBytesFieldHandRolled([]byte(m), 12, func(raw []byte) error {
				kv := KeyValue(raw)
				key, err := kv.Key()
				if err != nil {
					return err
				}
				value, err := kv.ValueRaw()
				if err != nil {
					return err
				}
				attrs = append(attrs, benchMetadataAttribute{key, value})
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
		})
	}
}

func BenchmarkMetric_Metadata_Seq(b *testing.B) {
	payload := benchMetadataPayload(b)
	attrs := make([]benchMetadataAttribute, 0, 8)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		forEachBenchMetric(b, payload, func(m Metric) {
			attrs = attrs[:0]
			for kv, err := range m.MetadataSeq {
				if err != nil {
					b.Fatal(err)
				}
				key, keyErr := kv.Key()
				if keyErr != nil {
					b.Fatal(keyErr)
				}
				value, valueErr := kv.ValueRaw()
				if valueErr != nil {
					b.Fatal(valueErr)
				}
				attrs = append(attrs, benchMetadataAttribute{key, value})
			}
		})
	}
}

// ========== LogRecord severity accessors ==========

// benchSeveritySink prevents the severity arms from being optimized away.
var benchSeveritySink int

func benchSeverityTextPayload(b *testing.B) []byte {
	b.Helper()
	payload, err := (&plog.ProtoMarshaler{}).MarshalLogs(createBenchLogs())
	require.NoError(b, err)
	return payload
}

// forEachBenchLogRecord walks down to every LogRecord so the arms differ only
// in how they read severity_text. Keep the arms as separate functions:
// dispatching through a func value defeats inlining and changes what is
// measured.
func forEachBenchLogRecord(b *testing.B, payload []byte, fn func(LogRecord)) {
	resources, resErr := ExportLogsServiceRequest(payload).ResourceLogs()
	for rl := range resources {
		scopes, scopeErr := rl.ScopeLogs()
		for sl := range scopes {
			for record, err := range sl.LogRecordsSeq {
				if err != nil {
					b.Fatal(err)
				}
				fn(record)
			}
		}
		if err := scopeErr(); err != nil {
			b.Fatal(err)
		}
	}
	if err := resErr(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkLogRecord_SeverityText is the single-field baseline: one walk per
// record, which is what a consumer reading only severity_text pays.
func BenchmarkLogRecord_SeverityText(b *testing.B) {
	payload := benchSeverityTextPayload(b)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		total := 0
		forEachBenchLogRecord(b, payload, func(record LogRecord) {
			text, err := record.SeverityText()
			if err != nil {
				b.Fatal(err)
			}
			total += len(text)
		})
		benchSeveritySink = total
	}
}

// BenchmarkLogRecord_SeverityNumberAndText reads both fields through the
// single-field accessors, which runs the shared walk twice per record. It is
// the arm BenchmarkLogRecord_Severity replaces; keep the two doing identical
// work at the call site so the delta is the second walk and nothing else.
func BenchmarkLogRecord_SeverityNumberAndText(b *testing.B) {
	payload := benchSeverityTextPayload(b)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		total := 0
		forEachBenchLogRecord(b, payload, func(record LogRecord) {
			number, err := record.SeverityNumber()
			if err != nil {
				b.Fatal(err)
			}
			text, err := record.SeverityText()
			if err != nil {
				b.Fatal(err)
			}
			total += int(number) + len(text)
		})
		benchSeveritySink = total
	}
}

func BenchmarkLogRecord_Severity(b *testing.B) {
	payload := benchSeverityTextPayload(b)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		total := 0
		forEachBenchLogRecord(b, payload, func(record LogRecord) {
			number, text, err := record.Severity()
			if err != nil {
				b.Fatal(err)
			}
			total += int(number) + len(text)
		})
		benchSeveritySink = total
	}
}

func BenchmarkLogRecord_SeverityFields(b *testing.B) {
	payload := benchSeverityTextPayload(b)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		total := 0
		forEachBenchLogRecord(b, payload, func(record LogRecord) {
			number, text, err := record.SeverityFields()
			if err != nil {
				b.Fatal(err)
			}
			total += int(number) + len(text)
		})
		benchSeveritySink = total
	}
}

// ========== Span field accessors: overstory's internal-spans detector ==========

// benchSpanFieldsSink prevents the internal-span analysis from being optimized
// away.
var benchSpanFieldsSink int64

// readSpanFieldsHandRolled is the walk overstory hand-rolls in
// internal/detector/wire.go, copied verbatim as the "before" arm so the
// comparison reproduces from this repository alone. It is deliberately not
// equivalent work: it reads all five fields in one pass and checks the name
// for UTF-8 validity, where the accessors each walk the span once and return
// the name as raw bytes. Drop this arm and its docs/BENCHMARKS.md section once
// overstory moves to the Span accessors.
func readSpanFieldsHandRolled(span []byte) (handRolledSpanFields, error) {
	var result handRolledSpanFields
	data := span
	for len(data) > 0 {
		field, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return handRolledSpanFields{}, errors.New("malformed protobuf tag")
		}
		data = data[n:]
		switch field {
		case 1: // trace_id
			if typ != protowire.BytesType {
				return handRolledSpanFields{}, errors.New("wrong wire type for trace ID")
			}
			var raw []byte
			raw, n = protowire.ConsumeBytes(data)
			if n >= 0 {
				if len(raw) != 0 && len(raw) != len(result.traceID) {
					return handRolledSpanFields{}, errors.New("trace ID has unexpected size")
				}
				result.traceID = [16]byte{}
				copy(result.traceID[:], raw)
			}
		case 5: // name
			if typ != protowire.BytesType {
				return handRolledSpanFields{}, errors.New("wrong wire type for span name")
			}
			result.name, n = protowire.ConsumeBytes(data)
		case 6: // kind
			if typ != protowire.VarintType {
				return handRolledSpanFields{}, errors.New("wrong wire type for span kind")
			}
			result.kind, n = protowire.ConsumeVarint(data)
		case 7: // start_time_unix_nano
			if typ != protowire.Fixed64Type {
				return handRolledSpanFields{}, errors.New("wrong wire type for span start time")
			}
			result.startUnixNano, n = protowire.ConsumeFixed64(data)
		case 8: // end_time_unix_nano
			if typ != protowire.Fixed64Type {
				return handRolledSpanFields{}, errors.New("wrong wire type for span end time")
			}
			result.endUnixNano, n = protowire.ConsumeFixed64(data)
		default:
			n = protowire.ConsumeFieldValue(field, typ, data)
		}
		if n < 0 {
			return handRolledSpanFields{}, errors.New("malformed protobuf field")
		}
		data = data[n:]
	}
	if !utf8.Valid(result.name) {
		return handRolledSpanFields{}, errors.New("span name is not valid UTF-8")
	}
	return result, nil
}

type handRolledSpanFields struct {
	traceID       [16]byte
	name          []byte
	kind          uint64
	startUnixNano uint64
	endUnixNano   uint64
}

// internalSpanAnalysis is the reduction overstory's detector performs: count
// internal spans, tally their names, and total their durations.
type internalSpanAnalysis struct {
	count              int64
	nameBytes          int64
	totalDurationNanos int64
	traceIDSum         int64
}

func (a internalSpanAnalysis) score() int64 {
	return a.count + a.nameBytes + a.totalDurationNanos + a.traceIDSum
}

const benchInternalSpanKind = int64(ptrace.SpanKindInternal)

func benchSpanFieldsPayload(b *testing.B) []byte {
	b.Helper()
	payload, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(createInternalSpansBenchTraces())
	require.NoError(b, err)
	return payload
}

// createInternalSpansBenchTraces builds spans shaped like the ones overstory
// sees: a mix of kinds, realistic attribute counts, and an event on the spans
// that carry one. The attributes matter to this comparison — they are the
// fields both walks must skip past to reach start_time and end_time.
func createInternalSpansBenchTraces() ptrace.Traces {
	traces := ptrace.NewTraces()
	kinds := []ptrace.SpanKind{
		ptrace.SpanKindServer,
		ptrace.SpanKindInternal,
		ptrace.SpanKindClient,
		ptrace.SpanKindInternal,
		ptrace.SpanKindProducer,
	}

	for resourceIndex := 0; resourceIndex < 5; resourceIndex++ {
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+resourceIndex)))
		rs.Resource().Attributes().PutStr("deployment.environment", "production")

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("test-instrumentation")

		for spanIndex := 0; spanIndex < 100; spanIndex++ {
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, byte(spanIndex)}))
			span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, byte(spanIndex)}))
			span.SetParentSpanID(pcommon.SpanID([8]byte{8, 7, 6, 5, 4, 3, 2, 1}))
			span.SetName("orders.handler.process")
			span.SetKind(kinds[spanIndex%len(kinds)])
			span.SetStartTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000 + uint64(spanIndex)*1_000))
			span.SetEndTimestamp(pcommon.Timestamp(1_700_000_000_000_500_000 + uint64(spanIndex)*1_000))
			span.Attributes().PutStr("http.request.method", "GET")
			span.Attributes().PutStr("url.path", "/api/v1/orders")
			span.Attributes().PutInt("http.response.status_code", 200)
			span.Attributes().PutStr("network.protocol.version", "1.1")
			span.Attributes().PutStr("user_agent.original", "checkout-client/2.1")
			if spanIndex%10 == 0 {
				event := span.Events().AppendEmpty()
				event.SetName("exception")
				event.Attributes().PutStr("exception.type", "TimeoutError")
			}
		}
	}
	return traces
}

// forEachBenchSpan walks down to every Span so the arms differ only in how
// they read the span's fields. Keep the arms as separate functions:
// dispatching through a func value defeats inlining and changes what is
// measured.
func forEachBenchSpan(b *testing.B, payload []byte, fn func(Span)) {
	resources, resErr := ExportTracesServiceRequest(payload).ResourceSpans()
	for rs := range resources {
		scopes, scopeErr := rs.ScopeSpans()
		for ss := range scopes {
			spans, spanErr := ss.Spans()
			for span := range spans {
				fn(span)
			}
			if err := spanErr(); err != nil {
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
}

// BenchmarkSpan_InternalSpans_HandRolled is overstory's detector as written
// today: one walk per span reading all five fields.
func BenchmarkSpan_InternalSpans_HandRolled(b *testing.B) {
	payload := benchSpanFieldsPayload(b)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var analysis internalSpanAnalysis
		forEachBenchSpan(b, payload, func(span Span) {
			fields, err := readSpanFieldsHandRolled([]byte(span))
			if err != nil {
				b.Fatal(err)
			}
			analysis.traceIDSum += int64(fields.traceID[15])
			if int64(fields.kind) != benchInternalSpanKind {
				return
			}
			analysis.count++
			analysis.nameBytes += int64(len(fields.name))
			if fields.endUnixNano > fields.startUnixNano {
				analysis.totalDurationNanos += int64(fields.endUnixNano - fields.startUnixNano)
			}
		})
		benchSpanFieldsSink = analysis.score()
	}
}

// BenchmarkSpan_InternalSpans_Accessors is the same reduction through the
// public accessors. Each of the four scalar accessors walks the span, so this
// arm pays one walk per field read rather than one per span — the cost of
// resolving a scalar last-value-wins the way pdata does. TraceID scans
// first-match and stops.
func BenchmarkSpan_InternalSpans_Accessors(b *testing.B) {
	payload := benchSpanFieldsPayload(b)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var analysis internalSpanAnalysis
		forEachBenchSpan(b, payload, func(span Span) {
			traceID, err := span.TraceID()
			if err != nil {
				b.Fatal(err)
			}
			analysis.traceIDSum += int64(traceID[15])
			kind, err := span.Kind()
			if err != nil {
				b.Fatal(err)
			}
			if int64(kind) != benchInternalSpanKind {
				return
			}
			name, err := span.Name()
			if err != nil {
				b.Fatal(err)
			}
			start, err := span.StartTimeUnixNano()
			if err != nil {
				b.Fatal(err)
			}
			end, err := span.EndTimeUnixNano()
			if err != nil {
				b.Fatal(err)
			}
			analysis.count++
			analysis.nameBytes += int64(len(name))
			if end > start {
				analysis.totalDurationNanos += int64(end - start)
			}
		})
		benchSpanFieldsSink = analysis.score()
	}
}

// BenchmarkSpan_InternalSpans_Unmarshal is the same reduction after a full
// pdata unmarshal, the path overstory runs on main today.
func BenchmarkSpan_InternalSpans_Unmarshal(b *testing.B) {
	payload := benchSpanFieldsPayload(b)
	unmarshaler := &ptrace.ProtoUnmarshaler{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		traces, err := unmarshaler.UnmarshalTraces(payload)
		if err != nil {
			b.Fatal(err)
		}
		var analysis internalSpanAnalysis
		for j := 0; j < traces.ResourceSpans().Len(); j++ {
			rs := traces.ResourceSpans().At(j)
			for k := 0; k < rs.ScopeSpans().Len(); k++ {
				ss := rs.ScopeSpans().At(k)
				for l := 0; l < ss.Spans().Len(); l++ {
					span := ss.Spans().At(l)
					traceID := span.TraceID()
					analysis.traceIDSum += int64(traceID[15])
					if int64(span.Kind()) != benchInternalSpanKind {
						continue
					}
					analysis.count++
					analysis.nameBytes += int64(len(span.Name()))
					if span.EndTimestamp() > span.StartTimestamp() {
						analysis.totalDurationNanos += int64(span.EndTimestamp() - span.StartTimestamp())
					}
				}
			}
		}
		benchSpanFieldsSink = analysis.score()
	}
}

// BenchmarkSpan_TraceIDOnly isolates the identifier read, which scans
// first-match and is unchanged by this package's shared walk. It is the
// control arm: whatever the four scalar accessors cost, this is what a
// consumer reading only an identifier still pays.
func BenchmarkSpan_TraceIDOnly(b *testing.B) {
	payload := benchSpanFieldsPayload(b)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var total int64
		forEachBenchSpan(b, payload, func(span Span) {
			traceID, err := span.TraceID()
			if err != nil {
				b.Fatal(err)
			}
			total += int64(traceID[15])
		})
		benchSpanFieldsSink = total
	}
}
