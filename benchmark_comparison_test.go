package otlpwire

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
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
