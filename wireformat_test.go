package wireformat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

// ========== MetricsData Tests ==========

func TestMetricsData_Count(t *testing.T) {
	tests := []struct {
		name          string
		setupRequest  func() pmetricotlp.ExportRequest
		expectedCount int
	}{
		{
			name: "empty request",
			setupRequest: func() pmetricotlp.ExportRequest {
				return pmetricotlp.NewExportRequest()
			},
			expectedCount: 0,
		},
		{
			name: "single gauge with one data point",
			setupRequest: func() pmetricotlp.ExportRequest {
				req := pmetricotlp.NewExportRequest()
				rm := req.Metrics().ResourceMetrics().AppendEmpty()
				sm := rm.ScopeMetrics().AppendEmpty()
				m := sm.Metrics().AppendEmpty()
				m.SetName("test.metric")
				m.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(42)
				return req
			},
			expectedCount: 1,
		},
		{
			name: "multiple metrics with multiple data points",
			setupRequest: func() pmetricotlp.ExportRequest {
				req := pmetricotlp.NewExportRequest()
				rm := req.Metrics().ResourceMetrics().AppendEmpty()
				sm := rm.ScopeMetrics().AppendEmpty()

				// Gauge with 3 data points
				gauge := sm.Metrics().AppendEmpty()
				gauge.SetName("gauge.metric")
				gauge.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)
				gauge.Gauge().DataPoints().AppendEmpty().SetIntValue(2)
				gauge.Gauge().DataPoints().AppendEmpty().SetIntValue(3)

				// Sum with 2 data points
				sum := sm.Metrics().AppendEmpty()
				sum.SetName("sum.metric")
				sum.SetEmptySum().DataPoints().AppendEmpty().SetIntValue(10)
				sum.Sum().DataPoints().AppendEmpty().SetIntValue(20)

				return req
			},
			expectedCount: 5,
		},
		{
			name: "multiple resource metrics",
			setupRequest: func() pmetricotlp.ExportRequest {
				req := pmetricotlp.NewExportRequest()

				// First resource
				rm1 := req.Metrics().ResourceMetrics().AppendEmpty()
				sm1 := rm1.ScopeMetrics().AppendEmpty()
				m1 := sm1.Metrics().AppendEmpty()
				m1.SetName("metric1")
				m1.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)
				m1.Gauge().DataPoints().AppendEmpty().SetIntValue(2)

				// Second resource
				rm2 := req.Metrics().ResourceMetrics().AppendEmpty()
				sm2 := rm2.ScopeMetrics().AppendEmpty()
				m2 := sm2.Metrics().AppendEmpty()
				m2.SetName("metric2")
				m2.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(3)

				return req
			},
			expectedCount: 3,
		},
		{
			name: "histogram metrics",
			setupRequest: func() pmetricotlp.ExportRequest {
				req := pmetricotlp.NewExportRequest()
				rm := req.Metrics().ResourceMetrics().AppendEmpty()
				sm := rm.ScopeMetrics().AppendEmpty()

				hist := sm.Metrics().AppendEmpty()
				hist.SetName("histogram.metric")
				hist.SetEmptyHistogram().DataPoints().AppendEmpty().SetCount(10)
				hist.Histogram().DataPoints().AppendEmpty().SetCount(20)

				return req
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			marshaler := &pmetric.ProtoMarshaler{}
			data, err := marshaler.MarshalMetrics(req.Metrics())
			require.NoError(t, err)

			metricsData := MetricsData(data)
			count := metricsData.Count()
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestMetricsData_SplitByResource(t *testing.T) {
	tests := []struct {
		name           string
		resourceCounts []int // data points per resource
	}{
		{
			name:           "single resource",
			resourceCounts: []int{10},
		},
		{
			name:           "two resources",
			resourceCounts: []int{5, 15},
		},
		{
			name:           "three resources with different counts",
			resourceCounts: []int{1, 100, 50},
		},
		{
			name:           "five resources",
			resourceCounts: []int{10, 20, 30, 40, 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data with multiple resources
			metrics := pmetric.NewMetrics()
			for i, dpCount := range tt.resourceCounts {
				rm := metrics.ResourceMetrics().AppendEmpty()
				rm.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.metric")
				gauge := metric.SetEmptyGauge()

				// Add data points
				for j := 0; j < dpCount; j++ {
					dp := gauge.DataPoints().AppendEmpty()
					dp.SetIntValue(int64(j))
				}
			}

			// Marshal to protobuf
			marshaler := &pmetric.ProtoMarshaler{}
			data, err := marshaler.MarshalMetrics(metrics)
			require.NoError(t, err)

			// Verify original count
			metricsData := MetricsData(data)
			originalCount := metricsData.Count()
			expectedTotal := 0
			for _, c := range tt.resourceCounts {
				expectedTotal += c
			}
			assert.Equal(t, expectedTotal, originalCount)

			// Split by resource
			splits := metricsData.SplitByResource()
			assert.Len(t, splits, len(tt.resourceCounts))

			// Verify each split
			totalFromSplits := 0
			for i, resource := range splits {
				// Count using AsExportRequest + cast back to MetricsData
				exportBytes := resource.AsExportRequest()
				count := MetricsData(exportBytes).Count()
				assert.Equal(t, tt.resourceCounts[i], count, "split %d should have correct count", i)
				totalFromSplits += count

				// Verify split can be unmarshaled
				unmarshaler := &pmetric.ProtoUnmarshaler{}
				splitMetrics, err := unmarshaler.UnmarshalMetrics(exportBytes)
				require.NoError(t, err)
				assert.Equal(t, 1, splitMetrics.ResourceMetrics().Len(), "each split should have exactly 1 resource")

				// Verify Resource() returns bytes
				assert.NotEmpty(t, resource.Resource())
			}

			// Verify total count is preserved
			assert.Equal(t, originalCount, totalFromSplits)
		})
	}
}

func TestMetricsData_SplitByResource_EmptyData(t *testing.T) {
	metrics := pmetric.NewMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	require.NoError(t, err)

	metricsData := MetricsData(data)
	splits := metricsData.SplitByResource()
	assert.Empty(t, splits)
}

// ========== LogsData Tests ==========

func TestLogsData_Count(t *testing.T) {
	tests := []struct {
		name          string
		setupRequest  func() plogotlp.ExportRequest
		expectedCount int
	}{
		{
			name: "empty request",
			setupRequest: func() plogotlp.ExportRequest {
				return plogotlp.NewExportRequest()
			},
			expectedCount: 0,
		},
		{
			name: "single log record",
			setupRequest: func() plogotlp.ExportRequest {
				req := plogotlp.NewExportRequest()
				rl := req.Logs().ResourceLogs().AppendEmpty()
				sl := rl.ScopeLogs().AppendEmpty()
				sl.LogRecords().AppendEmpty().Body().SetStr("test log")
				return req
			},
			expectedCount: 1,
		},
		{
			name: "multiple log records",
			setupRequest: func() plogotlp.ExportRequest {
				req := plogotlp.NewExportRequest()
				rl := req.Logs().ResourceLogs().AppendEmpty()
				sl := rl.ScopeLogs().AppendEmpty()

				for i := 0; i < 5; i++ {
					sl.LogRecords().AppendEmpty().Body().SetStr("log")
				}

				return req
			},
			expectedCount: 5,
		},
		{
			name: "multiple resources",
			setupRequest: func() plogotlp.ExportRequest {
				req := plogotlp.NewExportRequest()

				// Resource 1 with 3 logs
				rl1 := req.Logs().ResourceLogs().AppendEmpty()
				sl1 := rl1.ScopeLogs().AppendEmpty()
				for i := 0; i < 3; i++ {
					sl1.LogRecords().AppendEmpty().Body().SetStr("log")
				}

				// Resource 2 with 2 logs
				rl2 := req.Logs().ResourceLogs().AppendEmpty()
				sl2 := rl2.ScopeLogs().AppendEmpty()
				for i := 0; i < 2; i++ {
					sl2.LogRecords().AppendEmpty().Body().SetStr("log")
				}

				return req
			},
			expectedCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			marshaler := &plog.ProtoMarshaler{}
			data, err := marshaler.MarshalLogs(req.Logs())
			require.NoError(t, err)

			logsData := LogsData(data)
			count := logsData.Count()
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestLogsData_SplitByResource(t *testing.T) {
	tests := []struct {
		name           string
		resourceCounts []int // log records per resource
	}{
		{
			name:           "single resource",
			resourceCounts: []int{25},
		},
		{
			name:           "two resources",
			resourceCounts: []int{10, 30},
		},
		{
			name:           "multiple resources",
			resourceCounts: []int{5, 15, 25, 35},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data with multiple resources
			logs := plog.NewLogs()
			for i, logCount := range tt.resourceCounts {
				rl := logs.ResourceLogs().AppendEmpty()
				rl.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))

				sl := rl.ScopeLogs().AppendEmpty()

				// Add log records
				for j := 0; j < logCount; j++ {
					lr := sl.LogRecords().AppendEmpty()
					lr.Body().SetStr("log message")
				}
			}

			// Marshal to protobuf
			marshaler := &plog.ProtoMarshaler{}
			data, err := marshaler.MarshalLogs(logs)
			require.NoError(t, err)

			// Verify original count
			logsData := LogsData(data)
			originalCount := logsData.Count()
			expectedTotal := 0
			for _, c := range tt.resourceCounts {
				expectedTotal += c
			}
			assert.Equal(t, expectedTotal, originalCount)

			// Split by resource
			splits := logsData.SplitByResource()
			assert.Len(t, splits, len(tt.resourceCounts))

			// Verify each split
			totalFromSplits := 0
			for i, resource := range splits {
				exportBytes := resource.AsExportRequest()
				count := LogsData(exportBytes).Count()
				assert.Equal(t, tt.resourceCounts[i], count, "split %d should have correct count", i)
				totalFromSplits += count

				// Verify split can be unmarshaled
				unmarshaler := &plog.ProtoUnmarshaler{}
				splitLogs, err := unmarshaler.UnmarshalLogs(exportBytes)
				require.NoError(t, err)
				assert.Equal(t, 1, splitLogs.ResourceLogs().Len())

				// Verify Resource() returns bytes
				assert.NotEmpty(t, resource.Resource())
			}

			// Verify total count is preserved
			assert.Equal(t, originalCount, totalFromSplits)
		})
	}
}

// ========== TracesData Tests ==========

func TestTracesData_Count(t *testing.T) {
	tests := []struct {
		name          string
		setupRequest  func() ptraceotlp.ExportRequest
		expectedCount int
	}{
		{
			name: "empty request",
			setupRequest: func() ptraceotlp.ExportRequest {
				return ptraceotlp.NewExportRequest()
			},
			expectedCount: 0,
		},
		{
			name: "single span",
			setupRequest: func() ptraceotlp.ExportRequest {
				req := ptraceotlp.NewExportRequest()
				rs := req.Traces().ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()
				ss.Spans().AppendEmpty().SetName("test.span")
				return req
			},
			expectedCount: 1,
		},
		{
			name: "multiple spans",
			setupRequest: func() ptraceotlp.ExportRequest {
				req := ptraceotlp.NewExportRequest()
				rs := req.Traces().ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()

				for i := 0; i < 7; i++ {
					ss.Spans().AppendEmpty().SetName("span")
				}

				return req
			},
			expectedCount: 7,
		},
		{
			name: "multiple resources",
			setupRequest: func() ptraceotlp.ExportRequest {
				req := ptraceotlp.NewExportRequest()

				// Resource 1 with 4 spans
				rs1 := req.Traces().ResourceSpans().AppendEmpty()
				ss1 := rs1.ScopeSpans().AppendEmpty()
				for i := 0; i < 4; i++ {
					ss1.Spans().AppendEmpty().SetName("span")
				}

				// Resource 2 with 3 spans
				rs2 := req.Traces().ResourceSpans().AppendEmpty()
				ss2 := rs2.ScopeSpans().AppendEmpty()
				for i := 0; i < 3; i++ {
					ss2.Spans().AppendEmpty().SetName("span")
				}

				return req
			},
			expectedCount: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			marshaler := &ptrace.ProtoMarshaler{}
			data, err := marshaler.MarshalTraces(req.Traces())
			require.NoError(t, err)

			tracesData := TracesData(data)
			count := tracesData.Count()
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestTracesData_SplitByResource(t *testing.T) {
	tests := []struct {
		name           string
		resourceCounts []int // spans per resource
	}{
		{
			name:           "single resource",
			resourceCounts: []int{15},
		},
		{
			name:           "two resources",
			resourceCounts: []int{8, 12},
		},
		{
			name:           "multiple resources",
			resourceCounts: []int{3, 7, 11, 13},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data with multiple resources
			traces := ptrace.NewTraces()
			for i, spanCount := range tt.resourceCounts {
				rs := traces.ResourceSpans().AppendEmpty()
				rs.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))

				ss := rs.ScopeSpans().AppendEmpty()

				// Add spans
				for j := 0; j < spanCount; j++ {
					span := ss.Spans().AppendEmpty()
					span.SetName("test.span")
				}
			}

			// Marshal to protobuf
			marshaler := &ptrace.ProtoMarshaler{}
			data, err := marshaler.MarshalTraces(traces)
			require.NoError(t, err)

			// Verify original count
			tracesData := TracesData(data)
			originalCount := tracesData.Count()
			expectedTotal := 0
			for _, c := range tt.resourceCounts {
				expectedTotal += c
			}
			assert.Equal(t, expectedTotal, originalCount)

			// Split by resource
			splits := tracesData.SplitByResource()
			assert.Len(t, splits, len(tt.resourceCounts))

			// Verify each split
			totalFromSplits := 0
			for i, resource := range splits {
				exportBytes := resource.AsExportRequest()
				count := TracesData(exportBytes).Count()
				assert.Equal(t, tt.resourceCounts[i], count, "split %d should have correct count", i)
				totalFromSplits += count

				// Verify split can be unmarshaled
				unmarshaler := &ptrace.ProtoUnmarshaler{}
				splitTraces, err := unmarshaler.UnmarshalTraces(exportBytes)
				require.NoError(t, err)
				assert.Equal(t, 1, splitTraces.ResourceSpans().Len())

				// Verify Resource() returns bytes
				assert.NotEmpty(t, resource.Resource())
			}

			// Verify total count is preserved
			assert.Equal(t, originalCount, totalFromSplits)
		})
	}
}

// ========== Resource Tests ==========

func TestResourceMetrics_Resource(t *testing.T) {
	// Create test data with specific resource attributes
	metrics := pmetric.NewMetrics()

	rm1 := metrics.ResourceMetrics().AppendEmpty()
	rm1.Resource().Attributes().PutStr("service.name", "service-A")
	sm1 := rm1.ScopeMetrics().AppendEmpty()
	m1 := sm1.Metrics().AppendEmpty()
	m1.SetName("test.metric")
	m1.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(10)

	rm2 := metrics.ResourceMetrics().AppendEmpty()
	rm2.Resource().Attributes().PutStr("service.name", "service-B")
	sm2 := rm2.ScopeMetrics().AppendEmpty()
	m2 := sm2.Metrics().AppendEmpty()
	m2.SetName("test.metric")
	m2.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(20)

	marshaler := &pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	require.NoError(t, err)

	// Split and verify resources
	metricsData := MetricsData(data)
	splits := metricsData.SplitByResource()
	require.Len(t, splits, 2)

	// Different resources should have different bytes
	assert.NotEqual(t, splits[0].Resource(), splits[1].Resource())
	assert.NotEmpty(t, splits[0].Resource())
	assert.NotEmpty(t, splits[1].Resource())
}

func TestResourceMetrics_Resource_SameAttributes(t *testing.T) {
	// Create test data where two ResourceMetrics have same attributes
	metrics := pmetric.NewMetrics()

	rm1 := metrics.ResourceMetrics().AppendEmpty()
	rm1.Resource().Attributes().PutStr("service.name", "service-A")
	sm1 := rm1.ScopeMetrics().AppendEmpty()
	m1 := sm1.Metrics().AppendEmpty()
	m1.SetName("test.metric")
	m1.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(10)

	rm2 := metrics.ResourceMetrics().AppendEmpty()
	rm2.Resource().Attributes().PutStr("service.name", "service-A")
	sm2 := rm2.ScopeMetrics().AppendEmpty()
	m2 := sm2.Metrics().AppendEmpty()
	m2.SetName("test.metric")
	m2.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(30)

	marshaler := &pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	require.NoError(t, err)

	metricsData := MetricsData(data)
	splits := metricsData.SplitByResource()
	require.Len(t, splits, 2)

	// Same attributes should produce identical resource bytes
	assert.Equal(t, splits[0].Resource(), splits[1].Resource())
}

// ========== Benchmarks ==========

func BenchmarkMetricsData_Count(b *testing.B) {
	// Create test data with 5 resources, 100 data points each
	metrics := pmetric.NewMetrics()
	for i := 0; i < 5; i++ {
		rm := metrics.ResourceMetrics().AppendEmpty()
		sm := rm.ScopeMetrics().AppendEmpty()
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("test.metric")
		gauge := metric.SetEmptyGauge()

		for j := 0; j < 100; j++ {
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetIntValue(int64(j))
		}
	}

	marshaler := &pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	require.NoError(b, err)

	metricsData := MetricsData(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = metricsData.Count()
	}
}

func BenchmarkMetricsData_SplitByResource(b *testing.B) {
	// Create test data with 5 resources, 100 data points each
	metrics := pmetric.NewMetrics()
	for i := 0; i < 5; i++ {
		rm := metrics.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))

		sm := rm.ScopeMetrics().AppendEmpty()
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("test.metric")
		gauge := metric.SetEmptyGauge()

		for j := 0; j < 100; j++ {
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetIntValue(int64(j))
		}
	}

	marshaler := &pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	require.NoError(b, err)

	metricsData := MetricsData(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = metricsData.SplitByResource()
	}
}

func BenchmarkResourceMetrics_AsExportRequest(b *testing.B) {
	// Create single resource
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("test.metric")
	gauge := metric.SetEmptyGauge()

	for j := 0; j < 100; j++ {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetIntValue(int64(j))
	}

	marshaler := &pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	require.NoError(b, err)

	metricsData := MetricsData(data)
	splits := metricsData.SplitByResource()
	require.Len(b, splits, 1)

	resource := splits[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resource.AsExportRequest()
	}
}

func BenchmarkTracesData_Count(b *testing.B) {
	// Create test data with 5 resources, 100 spans each
	traces := ptrace.NewTraces()
	for i := 0; i < 5; i++ {
		rs := traces.ResourceSpans().AppendEmpty()
		ss := rs.ScopeSpans().AppendEmpty()

		for j := 0; j < 100; j++ {
			span := ss.Spans().AppendEmpty()
			span.SetName("test.span")
		}
	}

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(b, err)

	tracesData := TracesData(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracesData.Count()
	}
}

func BenchmarkTracesData_SplitByResource(b *testing.B) {
	// Create test data with 5 resources, 100 spans each
	traces := ptrace.NewTraces()
	for i := 0; i < 5; i++ {
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))

		ss := rs.ScopeSpans().AppendEmpty()

		for j := 0; j < 100; j++ {
			span := ss.Spans().AppendEmpty()
			span.SetName("test.span")
		}
	}

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(b, err)

	tracesData := TracesData(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracesData.SplitByResource()
	}
}

func BenchmarkLogsData_Count(b *testing.B) {
	// Create test data with 5 resources, 100 logs each
	logs := plog.NewLogs()
	for i := 0; i < 5; i++ {
		rl := logs.ResourceLogs().AppendEmpty()
		sl := rl.ScopeLogs().AppendEmpty()

		for j := 0; j < 100; j++ {
			lr := sl.LogRecords().AppendEmpty()
			lr.Body().SetStr("log message")
		}
	}

	marshaler := &plog.ProtoMarshaler{}
	data, err := marshaler.MarshalLogs(logs)
	require.NoError(b, err)

	logsData := LogsData(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = logsData.Count()
	}
}

func BenchmarkLogsData_SplitByResource(b *testing.B) {
	// Create test data with 5 resources, 100 logs each
	logs := plog.NewLogs()
	for i := 0; i < 5; i++ {
		rl := logs.ResourceLogs().AppendEmpty()
		rl.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))

		sl := rl.ScopeLogs().AppendEmpty()

		for j := 0; j < 100; j++ {
			lr := sl.LogRecords().AppendEmpty()
			lr.Body().SetStr("log message")
		}
	}

	marshaler := &plog.ProtoMarshaler{}
	data, err := marshaler.MarshalLogs(logs)
	require.NoError(b, err)

	logsData := LogsData(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = logsData.SplitByResource()
	}
}
