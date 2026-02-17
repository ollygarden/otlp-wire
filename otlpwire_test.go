package otlpwire

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/protobuf/encoding/protowire"
)

// ========== ExportMetricsServiceRequest Tests ==========

func TestExportMetricsServiceRequest_Count(t *testing.T) {
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

			metricsData := ExportMetricsServiceRequest(data)
			count, err := metricsData.DataPointCount()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestExportMetricsServiceRequest_SplitByResource(t *testing.T) {
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
			metricsData := ExportMetricsServiceRequest(data)
			originalCount, err := metricsData.DataPointCount()
			require.NoError(t, err)
			expectedTotal := 0
			for _, c := range tt.resourceCounts {
				expectedTotal += c
			}
			assert.Equal(t, expectedTotal, originalCount)

			// Iterate over resources
			totalFromSplits := 0
			i := 0
			resources, getErr := metricsData.ResourceMetrics()
			for resource := range resources {
				// Count using WriteTo + cast back to MetricsData
				var buf bytes.Buffer
				_, err := resource.WriteTo(&buf)
				require.NoError(t, err)
				exportBytes := buf.Bytes()

				count, err := ExportMetricsServiceRequest(exportBytes).DataPointCount()
				require.NoError(t, err)
				assert.Equal(t, tt.resourceCounts[i], count, "split %d should have correct count", i)
				totalFromSplits += count

				// Verify split can be unmarshaled
				unmarshaler := &pmetric.ProtoUnmarshaler{}
				splitMetrics, err := unmarshaler.UnmarshalMetrics(exportBytes)
				require.NoError(t, err)
				assert.Equal(t, 1, splitMetrics.ResourceMetrics().Len(), "each split should have exactly 1 resource")

				// Verify Resource() returns bytes
				resourceBytes, err := resource.Resource()
				require.NoError(t, err)
				assert.NotEmpty(t, resourceBytes)

				i++
			}
			require.NoError(t, getErr())

			// Verify we processed the expected number of resources
			assert.Equal(t, len(tt.resourceCounts), i)

			// Verify total count is preserved
			assert.Equal(t, originalCount, totalFromSplits)
		})
	}
}

func TestExportMetricsServiceRequest_SplitByResource_EmptyData(t *testing.T) {
	metrics := pmetric.NewMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	require.NoError(t, err)

	metricsData := ExportMetricsServiceRequest(data)
	count := 0
	resources, getErr := metricsData.ResourceMetrics()
	for range resources {
		count++
	}
	require.NoError(t, getErr())
	assert.Equal(t, 0, count)
}

// ========== ExportLogsServiceRequest Tests ==========

func TestExportLogsServiceRequest_Count(t *testing.T) {
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

			logsData := ExportLogsServiceRequest(data)
			count, err := logsData.LogRecordCount()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestExportLogsServiceRequest_SplitByResource(t *testing.T) {
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
			logsData := ExportLogsServiceRequest(data)
			originalCount, err := logsData.LogRecordCount()
			require.NoError(t, err)
			expectedTotal := 0
			for _, c := range tt.resourceCounts {
				expectedTotal += c
			}
			assert.Equal(t, expectedTotal, originalCount)

			// Iterate over resources
			totalFromSplits := 0
			i := 0
			resources, getErr := logsData.ResourceLogs()
			for resource := range resources {
				var buf bytes.Buffer
				_, err := resource.WriteTo(&buf)
				require.NoError(t, err)
				exportBytes := buf.Bytes()

				count, err := ExportLogsServiceRequest(exportBytes).LogRecordCount()
				require.NoError(t, err)
				assert.Equal(t, tt.resourceCounts[i], count, "split %d should have correct count", i)
				totalFromSplits += count

				// Verify split can be unmarshaled
				unmarshaler := &plog.ProtoUnmarshaler{}
				splitLogs, err := unmarshaler.UnmarshalLogs(exportBytes)
				require.NoError(t, err)
				assert.Equal(t, 1, splitLogs.ResourceLogs().Len())

				// Verify Resource() returns bytes
				resourceBytes, err := resource.Resource()
				require.NoError(t, err)
				assert.NotEmpty(t, resourceBytes)

				i++
			}
			require.NoError(t, getErr())

			// Verify we processed the expected number of resources
			assert.Equal(t, len(tt.resourceCounts), i)

			// Verify total count is preserved
			assert.Equal(t, originalCount, totalFromSplits)
		})
	}
}

// ========== ExportTracesServiceRequest Tests ==========

func TestExportTracesServiceRequest_Count(t *testing.T) {
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

			tracesData := ExportTracesServiceRequest(data)
			count, err := tracesData.SpanCount()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestExportTracesServiceRequest_SplitByResource(t *testing.T) {
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
			tracesData := ExportTracesServiceRequest(data)
			originalCount, err := tracesData.SpanCount()
			require.NoError(t, err)
			expectedTotal := 0
			for _, c := range tt.resourceCounts {
				expectedTotal += c
			}
			assert.Equal(t, expectedTotal, originalCount)

			// Iterate over resources
			totalFromSplits := 0
			i := 0
			resources, getErr := tracesData.ResourceSpans()
			for resource := range resources {
				var buf bytes.Buffer
				_, err := resource.WriteTo(&buf)
				require.NoError(t, err)
				exportBytes := buf.Bytes()

				count, err := ExportTracesServiceRequest(exportBytes).SpanCount()
				require.NoError(t, err)
				assert.Equal(t, tt.resourceCounts[i], count, "split %d should have correct count", i)
				totalFromSplits += count

				// Verify split can be unmarshaled
				unmarshaler := &ptrace.ProtoUnmarshaler{}
				splitTraces, err := unmarshaler.UnmarshalTraces(exportBytes)
				require.NoError(t, err)
				assert.Equal(t, 1, splitTraces.ResourceSpans().Len())

				// Verify Resource() returns bytes
				resourceBytes, err := resource.Resource()
				require.NoError(t, err)
				assert.NotEmpty(t, resourceBytes)

				i++
			}
			require.NoError(t, getErr())

			// Verify we processed the expected number of resources
			assert.Equal(t, len(tt.resourceCounts), i)

			// Verify total count is preserved
			assert.Equal(t, originalCount, totalFromSplits)
		})
	}
}

// ========== ScopeSpans and Span Tests ==========

func TestResourceSpans_ScopeSpans(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()

	// Add 3 ScopeSpans with different span counts
	ss1 := rs.ScopeSpans().AppendEmpty()
	ss1.Spans().AppendEmpty().SetName("span-1a")
	ss1.Spans().AppendEmpty().SetName("span-1b")

	ss2 := rs.ScopeSpans().AppendEmpty()
	ss2.Spans().AppendEmpty().SetName("span-2a")

	ss3 := rs.ScopeSpans().AppendEmpty()
	ss3.Spans().AppendEmpty().SetName("span-3a")
	ss3.Spans().AppendEmpty().SetName("span-3b")
	ss3.Spans().AppendEmpty().SetName("span-3c")

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(t, err)

	wire := ExportTracesServiceRequest(data)
	rsIter, rsErr := wire.ResourceSpans()
	for rs := range rsIter {
		scopeCount := 0
		spanCounts := []int{}
		ssIter, ssErr := rs.ScopeSpans()
		for ss := range ssIter {
			scopeCount++
			count, err := ss.SpanCount()
			require.NoError(t, err)
			spanCounts = append(spanCounts, count)
		}
		require.NoError(t, ssErr())
		assert.Equal(t, 3, scopeCount)
		assert.Equal(t, []int{2, 1, 3}, spanCounts)
	}
	require.NoError(t, rsErr())
}

func TestScopeSpans_Spans(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()

	expectedTraceID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	expectedSpanIDs := [][8]byte{
		{1, 0, 0, 0, 0, 0, 0, 0},
		{2, 0, 0, 0, 0, 0, 0, 0},
		{3, 0, 0, 0, 0, 0, 0, 0},
	}

	for _, id := range expectedSpanIDs {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID(expectedTraceID))
		span.SetSpanID(pcommon.SpanID(id))
	}

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(t, err)

	wire := ExportTracesServiceRequest(data)
	rsIter, rsErr := wire.ResourceSpans()
	for rs := range rsIter {
		ssIter, ssErr := rs.ScopeSpans()
		for ss := range ssIter {
			i := 0
			spanIter, spanErr := ss.Spans()
			for s := range spanIter {
				traceID, err := s.TraceID()
				require.NoError(t, err)
				assert.Equal(t, expectedTraceID, traceID)

				spanID, err := s.SpanID()
				require.NoError(t, err)
				assert.Equal(t, expectedSpanIDs[i], spanID)

				i++
			}
			require.NoError(t, spanErr())
			assert.Equal(t, 3, i)
		}
		require.NoError(t, ssErr())
	}
	require.NoError(t, rsErr())
}

func TestSpanFieldAccessors(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	expectedTraceID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	expectedSpanID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	expectedParentID := [8]byte{8, 7, 6, 5, 4, 3, 2, 1}

	span.SetTraceID(pcommon.TraceID(expectedTraceID))
	span.SetSpanID(pcommon.SpanID(expectedSpanID))
	span.SetParentSpanID(pcommon.SpanID(expectedParentID))
	span.SetName("test-span")

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(t, err)

	wire := ExportTracesServiceRequest(data)
	rsIter, rsErr := wire.ResourceSpans()
	for rs := range rsIter {
		ssIter, ssErr := rs.ScopeSpans()
		for ss := range ssIter {
			spanIter, spanErr := ss.Spans()
			for s := range spanIter {
				traceID, err := s.TraceID()
				require.NoError(t, err)
				assert.Equal(t, expectedTraceID, traceID)

				spanID, err := s.SpanID()
				require.NoError(t, err)
				assert.Equal(t, expectedSpanID, spanID)

				parentID, err := s.ParentSpanID()
				require.NoError(t, err)
				assert.Equal(t, expectedParentID, parentID)
			}
			require.NoError(t, spanErr())
		}
		require.NoError(t, ssErr())
	}
	require.NoError(t, rsErr())
}

func TestSpanFieldAccessors_RootSpan(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	expectedTraceID := [16]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160}
	expectedSpanID := [8]byte{10, 20, 30, 40, 50, 60, 70, 80}

	span.SetTraceID(pcommon.TraceID(expectedTraceID))
	span.SetSpanID(pcommon.SpanID(expectedSpanID))
	// No parent set — root span

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(t, err)

	wire := ExportTracesServiceRequest(data)
	rsIter, rsErr := wire.ResourceSpans()
	for rs := range rsIter {
		ssIter, ssErr := rs.ScopeSpans()
		for ss := range ssIter {
			spanIter, spanErr := ss.Spans()
			for s := range spanIter {
				traceID, err := s.TraceID()
				require.NoError(t, err)
				assert.Equal(t, expectedTraceID, traceID)

				spanID, err := s.SpanID()
				require.NoError(t, err)
				assert.Equal(t, expectedSpanID, spanID)

				parentID, err := s.ParentSpanID()
				require.NoError(t, err)
				assert.Equal(t, [8]byte{}, parentID, "root span should have zero parent ID")
			}
			require.NoError(t, spanErr())
		}
		require.NoError(t, ssErr())
	}
	require.NoError(t, rsErr())
}

func TestSpanFieldAccessors_MultipleResourcesAndScopes(t *testing.T) {
	traces := ptrace.NewTraces()

	// Resource 1 with 2 ScopeSpans
	rs1 := traces.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("service.name", "svc-A")

	ss1a := rs1.ScopeSpans().AppendEmpty()
	s1 := ss1a.Spans().AppendEmpty()
	s1.SetTraceID(pcommon.TraceID([16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}))
	s1.SetSpanID(pcommon.SpanID([8]byte{1, 1, 1, 1, 1, 1, 1, 1}))

	ss1b := rs1.ScopeSpans().AppendEmpty()
	s2 := ss1b.Spans().AppendEmpty()
	s2.SetTraceID(pcommon.TraceID([16]byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}))
	s2.SetSpanID(pcommon.SpanID([8]byte{2, 2, 2, 2, 2, 2, 2, 2}))
	s2.SetParentSpanID(pcommon.SpanID([8]byte{1, 1, 1, 1, 1, 1, 1, 1}))

	// Resource 2 with 1 ScopeSpans
	rs2 := traces.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("service.name", "svc-B")

	ss2 := rs2.ScopeSpans().AppendEmpty()
	s3 := ss2.Spans().AppendEmpty()
	s3.SetTraceID(pcommon.TraceID([16]byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}))
	s3.SetSpanID(pcommon.SpanID([8]byte{3, 3, 3, 3, 3, 3, 3, 3}))

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(t, err)

	wire := ExportTracesServiceRequest(data)

	type spanInfo struct {
		traceID  [16]byte
		spanID   [8]byte
		parentID [8]byte
	}
	var spans []spanInfo

	rsIter, rsErr := wire.ResourceSpans()
	for rs := range rsIter {
		ssIter, ssErr := rs.ScopeSpans()
		for ss := range ssIter {
			spanIter, spanErr := ss.Spans()
			for s := range spanIter {
				traceID, err := s.TraceID()
				require.NoError(t, err)
				spanID, err := s.SpanID()
				require.NoError(t, err)
				parentID, err := s.ParentSpanID()
				require.NoError(t, err)
				spans = append(spans, spanInfo{traceID, spanID, parentID})
			}
			require.NoError(t, spanErr())
		}
		require.NoError(t, ssErr())
	}
	require.NoError(t, rsErr())

	require.Len(t, spans, 3)

	assert.Equal(t, [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, spans[0].traceID)
	assert.Equal(t, [8]byte{1, 1, 1, 1, 1, 1, 1, 1}, spans[0].spanID)
	assert.Equal(t, [8]byte{}, spans[0].parentID)

	assert.Equal(t, [16]byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}, spans[1].traceID)
	assert.Equal(t, [8]byte{2, 2, 2, 2, 2, 2, 2, 2}, spans[1].spanID)
	assert.Equal(t, [8]byte{1, 1, 1, 1, 1, 1, 1, 1}, spans[1].parentID)

	assert.Equal(t, [16]byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}, spans[2].traceID)
	assert.Equal(t, [8]byte{3, 3, 3, 3, 3, 3, 3, 3}, spans[2].spanID)
	assert.Equal(t, [8]byte{}, spans[2].parentID)
}

func TestSpanFieldAccessors_RoundTrip(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()

	expectedTraceID := pcommon.TraceID([16]byte{0xde, 0xad, 0xbe, 0xef, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})
	expectedSpanID := pcommon.SpanID([8]byte{0xca, 0xfe, 0xba, 0xbe, 1, 2, 3, 4})
	expectedParentID := pcommon.SpanID([8]byte{0xfe, 0xed, 0xfa, 0xce, 5, 6, 7, 8})

	span := ss.Spans().AppendEmpty()
	span.SetTraceID(expectedTraceID)
	span.SetSpanID(expectedSpanID)
	span.SetParentSpanID(expectedParentID)
	span.SetName("round-trip-span")

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(t, err)

	// Extract via otlp-wire
	wire := ExportTracesServiceRequest(data)
	rsIter, rsErr := wire.ResourceSpans()
	for rs := range rsIter {
		ssIter, ssErr := rs.ScopeSpans()
		for ss := range ssIter {
			spanIter, spanErr := ss.Spans()
			for s := range spanIter {
				gotTraceID, err := s.TraceID()
				require.NoError(t, err)
				gotSpanID, err := s.SpanID()
				require.NoError(t, err)
				gotParentID, err := s.ParentSpanID()
				require.NoError(t, err)

				assert.Equal(t, [16]byte(expectedTraceID), gotTraceID)
				assert.Equal(t, [8]byte(expectedSpanID), gotSpanID)
				assert.Equal(t, [8]byte(expectedParentID), gotParentID)
			}
			require.NoError(t, spanErr())
		}
		require.NoError(t, ssErr())
	}
	require.NoError(t, rsErr())

	// Also unmarshal via pdata and confirm values match
	unmarshaler := &ptrace.ProtoUnmarshaler{}
	pdataTraces, err := unmarshaler.UnmarshalTraces(data)
	require.NoError(t, err)
	pdataSpan := pdataTraces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	assert.Equal(t, expectedTraceID, pdataSpan.TraceID())
	assert.Equal(t, expectedSpanID, pdataSpan.SpanID())
	assert.Equal(t, expectedParentID, pdataSpan.ParentSpanID())
}

func TestScopeSpans_SpanCount(t *testing.T) {
	tests := []struct {
		name      string
		spanCount int
	}{
		{"zero spans", 0},
		{"one span", 1},
		{"many spans", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			rs := traces.ResourceSpans().AppendEmpty()
			ss := rs.ScopeSpans().AppendEmpty()

			for i := 0; i < tt.spanCount; i++ {
				ss.Spans().AppendEmpty().SetName("span")
			}

			marshaler := &ptrace.ProtoMarshaler{}
			data, err := marshaler.MarshalTraces(traces)
			require.NoError(t, err)

			wire := ExportTracesServiceRequest(data)
			rsIter, rsErr := wire.ResourceSpans()
			for rs := range rsIter {
				ssIter, ssErr := rs.ScopeSpans()
				for ss := range ssIter {
					count, err := ss.SpanCount()
					require.NoError(t, err)
					assert.Equal(t, tt.spanCount, count)
				}
				require.NoError(t, ssErr())
			}
			require.NoError(t, rsErr())
		})
	}
}

func TestSpan_MalformedData(t *testing.T) {
	// Truncated protobuf
	s := Span([]byte{0x0a, 0x10, 0x01, 0x02}) // field 1, bytes, length 16, but only 2 bytes
	_, err := s.TraceID()
	require.Error(t, err)
}

func TestSpan_WrongFieldSize(t *testing.T) {
	// Craft a span with trace_id field having wrong size (8 instead of 16)
	buf := []byte{}
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	buf = protowire.AppendBytes(buf, []byte{1, 2, 3, 4, 5, 6, 7, 8}) // 8 bytes instead of 16

	s := Span(buf)
	_, err := s.TraceID()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected size")
}

// ========== Span Benchmarks ==========

func BenchmarkSpanFieldAccessors(b *testing.B) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.SetParentSpanID(pcommon.SpanID([8]byte{8, 7, 6, 5, 4, 3, 2, 1}))
	span.SetName("bench-span")

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(b, err)

	// Extract a single Span from the wire data
	wire := ExportTracesServiceRequest(data)
	var wireSpan Span
	rsIter, _ := wire.ResourceSpans()
	for rs := range rsIter {
		ssIter, _ := rs.ScopeSpans()
		for ss := range ssIter {
			spanIter, _ := ss.Spans()
			for s := range spanIter {
				wireSpan = s
			}
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = wireSpan.TraceID()
		_, _ = wireSpan.SpanID()
		_, _ = wireSpan.ParentSpanID()
	}
}

func BenchmarkSpanIteration(b *testing.B) {
	tracesData := createBenchTracesData(b, true)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rsIter, _ := tracesData.ResourceSpans()
		for rs := range rsIter {
			ssIter, _ := rs.ScopeSpans()
			for ss := range ssIter {
				spanIter, _ := ss.Spans()
				for s := range spanIter {
					_, _ = s.TraceID()
					_, _ = s.SpanID()
					_, _ = s.ParentSpanID()
				}
			}
		}
	}
}

func BenchmarkSpanIteration_PdataUnmarshal(b *testing.B) {
	tracesData := createBenchTracesData(b, true)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		unmarshaler := &ptrace.ProtoUnmarshaler{}
		traces, _ := unmarshaler.UnmarshalTraces([]byte(tracesData))
		for j := 0; j < traces.ResourceSpans().Len(); j++ {
			rs := traces.ResourceSpans().At(j)
			for k := 0; k < rs.ScopeSpans().Len(); k++ {
				ss := rs.ScopeSpans().At(k)
				for l := 0; l < ss.Spans().Len(); l++ {
					s := ss.Spans().At(l)
					_ = s.TraceID()
					_ = s.SpanID()
					_ = s.ParentSpanID()
				}
			}
		}
	}
}

func TestSpan_WrongWireType(t *testing.T) {
	// Craft a span with trace_id (field 1) encoded as varint instead of bytes
	buf := []byte{}
	buf = protowire.AppendTag(buf, 1, protowire.VarintType)
	buf = protowire.AppendVarint(buf, 12345)

	s := Span(buf)
	_, err := s.TraceID()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong wire type")
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

	// Iterate over resources and verify
	metricsData := ExportMetricsServiceRequest(data)
	var resources [][]byte
	resourcesIter, getErr := metricsData.ResourceMetrics()
	for resource := range resourcesIter {
		resourceBytes, err := resource.Resource()
		require.NoError(t, err)
		resources = append(resources, resourceBytes)
	}
	require.NoError(t, getErr())

	require.Len(t, resources, 2)

	// Different resources should have different bytes
	assert.NotEqual(t, resources[0], resources[1])
	assert.NotEmpty(t, resources[0])
	assert.NotEmpty(t, resources[1])
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

	metricsData := ExportMetricsServiceRequest(data)
	var resources [][]byte
	resourcesIter, getErr := metricsData.ResourceMetrics()
	for resource := range resourcesIter {
		resourceBytes, err := resource.Resource()
		require.NoError(t, err)
		resources = append(resources, resourceBytes)
	}
	require.NoError(t, getErr())

	require.Len(t, resources, 2)

	// Same attributes should produce identical resource bytes
	assert.Equal(t, resources[0], resources[1])
}

// ========== Error Handling Tests ==========

func TestResourceMetrics_Resource_WrongWireType(t *testing.T) {
	// Craft ResourceMetrics with Resource field having wrong wire type
	buf := []byte{}

	// Field 1 (Resource) with wire type 0 (varint) instead of 2 (bytes)
	buf = protowire.AppendTag(buf, 1, protowire.VarintType)
	buf = protowire.AppendVarint(buf, 12345)

	rm := ResourceMetrics(buf)
	_, err := rm.Resource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong wire type")
}

func TestResourceMetrics_Resource_Missing(t *testing.T) {
	// Craft ResourceMetrics without Resource field (only ScopeMetrics)
	buf := []byte{}

	// Field 2 = ScopeMetrics (empty)
	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendBytes(buf, []byte{})

	rm := ResourceMetrics(buf)
	_, err := rm.Resource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResourceLogs_Resource_WrongWireType(t *testing.T) {
	buf := []byte{}
	buf = protowire.AppendTag(buf, 1, protowire.VarintType)
	buf = protowire.AppendVarint(buf, 12345)

	rl := ResourceLogs(buf)
	_, err := rl.Resource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong wire type")
}

func TestResourceLogs_Resource_Missing(t *testing.T) {
	buf := []byte{}
	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendBytes(buf, []byte{})

	rl := ResourceLogs(buf)
	_, err := rl.Resource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResourceSpans_Resource_WrongWireType(t *testing.T) {
	buf := []byte{}
	buf = protowire.AppendTag(buf, 1, protowire.VarintType)
	buf = protowire.AppendVarint(buf, 12345)

	rs := ResourceSpans(buf)
	_, err := rs.Resource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong wire type")
}

func TestResourceSpans_Resource_Missing(t *testing.T) {
	buf := []byte{}
	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendBytes(buf, []byte{})

	rs := ResourceSpans(buf)
	_, err := rs.Resource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ========== Benchmarks ==========

func createBenchMetricsData(b *testing.B, withAttributes bool) ExportMetricsServiceRequest {
	metrics := pmetric.NewMetrics()
	for i := 0; i < 5; i++ {
		rm := metrics.ResourceMetrics().AppendEmpty()
		if withAttributes {
			rm.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))
		}

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

	return ExportMetricsServiceRequest(data)
}

func BenchmarkMetricsData_Count(b *testing.B) {
	metricsData := createBenchMetricsData(b, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = metricsData.DataPointCount()
	}
}

func BenchmarkMetricsData_SplitByResource(b *testing.B) {
	metricsData := createBenchMetricsData(b, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resources, getErr := metricsData.ResourceMetrics()
		for range resources {
		}
		_ = getErr()
	}
}

func createSingleResourceMetric(b *testing.B) ResourceMetrics {
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

	metricsData := ExportMetricsServiceRequest(data)
	resources, getErr := metricsData.ResourceMetrics()
	for r := range resources {
		require.NoError(b, getErr())
		return r
	}
	b.Fatal("no resource found")
	return nil
}

func BenchmarkResourceMetrics_WriteTo_Discard(b *testing.B) {
	resource := createSingleResourceMetric(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resource.WriteTo(discard{})
	}
}

func BenchmarkResourceMetrics_WriteTo_Buffer(b *testing.B) {
	resource := createSingleResourceMetric(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_, _ = resource.WriteTo(&buf)
	}
}

// discard is a zero-allocation io.Writer for benchmarking
type discard struct{}

func (discard) Write(p []byte) (int, error) {
	return len(p), nil
}

func createBenchTracesData(b *testing.B, withAttributes bool) ExportTracesServiceRequest {
	traces := ptrace.NewTraces()
	for i := 0; i < 5; i++ {
		rs := traces.ResourceSpans().AppendEmpty()
		if withAttributes {
			rs.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))
		}

		ss := rs.ScopeSpans().AppendEmpty()

		for j := 0; j < 100; j++ {
			span := ss.Spans().AppendEmpty()
			span.SetName("test.span")
		}
	}

	marshaler := &ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(b, err)

	return ExportTracesServiceRequest(data)
}

func BenchmarkTracesData_Count(b *testing.B) {
	tracesData := createBenchTracesData(b, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tracesData.SpanCount()
	}
}

func BenchmarkTracesData_SplitByResource(b *testing.B) {
	tracesData := createBenchTracesData(b, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resources, getErr := tracesData.ResourceSpans()
		for range resources {
		}
		_ = getErr()
	}
}

func createBenchLogsData(b *testing.B, withAttributes bool) ExportLogsServiceRequest {
	logs := plog.NewLogs()
	for i := 0; i < 5; i++ {
		rl := logs.ResourceLogs().AppendEmpty()
		if withAttributes {
			rl.Resource().Attributes().PutStr("service.name", "service-"+string(rune('A'+i)))
		}

		sl := rl.ScopeLogs().AppendEmpty()

		for j := 0; j < 100; j++ {
			lr := sl.LogRecords().AppendEmpty()
			lr.Body().SetStr("log message")
		}
	}

	marshaler := &plog.ProtoMarshaler{}
	data, err := marshaler.MarshalLogs(logs)
	require.NoError(b, err)

	return ExportLogsServiceRequest(data)
}

func BenchmarkLogsData_Count(b *testing.B) {
	logsData := createBenchLogsData(b, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = logsData.LogRecordCount()
	}
}

func BenchmarkLogsData_SplitByResource(b *testing.B) {
	logsData := createBenchLogsData(b, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resources, getErr := logsData.ResourceLogs()
		for range resources {
		}
		_ = getErr()
	}
}
