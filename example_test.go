package otlpwire_test

import (
	"fmt"
	"hash/fnv"

	"go.olly.garden/otlp-wire"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// Example_globalRateLimiting demonstrates using Count() for rate limiting.
func Example_globalRateLimiting() {
	// Simulate receiving OTLP metrics data
	metrics := createSampleMetrics(100)
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	// Count signals for rate limiting
	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
	count := data.DataPointCount()

	globalLimit := 50
	if count > globalLimit {
		fmt.Println("Rate limit exceeded")
	} else {
		fmt.Printf("Accepted %d data points\n", count)
	}

	// Output: Rate limit exceeded
}

// Example_shardingByService demonstrates splitting batches for distributed processing.
func Example_shardingByService() {
	// Create metrics from multiple services
	metrics := createMultiServiceMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	// Split batch by resource for sharding
	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
	numWorkers := 3

	for i, resource := range data.SplitByResource() {
		// Hash resource for consistent routing
		hash := hashBytes(resource.Resource())
		workerID := hash % uint64(numWorkers)

		exportBytes := resource.AsExportRequest()
		count := otlpwire.ExportMetricsServiceRequest(exportBytes).DataPointCount()

		fmt.Printf("Resource %d → Worker %d (%d data points)\n", i, workerID, count)
	}

	// Output:
	// Resource 0 → Worker 0 (10 data points)
	// Resource 1 → Worker 1 (10 data points)
	// Resource 2 → Worker 2 (10 data points)
}

// Example_perServiceRateLimiting demonstrates per-service rate limiting.
func Example_perServiceRateLimiting() {
	metrics := createMultiServiceMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	serviceLimit := 15
	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

	for _, resource := range data.SplitByResource() {
		// Count signals in this resource
		exportBytes := resource.AsExportRequest()
		count := otlpwire.ExportMetricsServiceRequest(exportBytes).DataPointCount()

		if count > serviceLimit {
			fmt.Printf("Resource rejected: %d data points (limit: %d)\n", count, serviceLimit)
		} else {
			fmt.Printf("Resource accepted: %d data points\n", count)
		}
	}

	// Output:
	// Resource accepted: 10 data points
	// Resource accepted: 10 data points
	// Resource accepted: 10 data points
}

// Example_typeComposition demonstrates how types compose naturally.
func Example_typeComposition() {
	metrics := createSampleMetrics(25)
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	// MetricsData wraps complete OTLP message
	batch := otlpwire.ExportMetricsServiceRequest(otlpBytes)
	fmt.Printf("Total data points: %d\n", batch.DataPointCount())

	// Split returns []ResourceMetrics
	resources := batch.SplitByResource()
	fmt.Printf("Number of resources: %d\n", len(resources))

	// AsExportRequest returns []byte (valid OTLP message)
	exportBytes := resources[0].AsExportRequest()

	// Cast back to MetricsData to count this resource only
	singleResourceBatch := otlpwire.ExportMetricsServiceRequest(exportBytes)
	fmt.Printf("Resource 0 data points: %d\n", singleResourceBatch.DataPointCount())

	// Output:
	// Total data points: 25
	// Number of resources: 1
	// Resource 0 data points: 25
}

// Helper functions

func createSampleMetrics(dataPoints int) pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-service")

	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("test.metric")
	gauge := metric.SetEmptyGauge()

	for i := 0; i < dataPoints; i++ {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetIntValue(int64(i))
	}

	return metrics
}

func createMultiServiceMetrics() pmetric.Metrics {
	metrics := pmetric.NewMetrics()

	services := []string{"frontend", "backend", "database"}
	for _, svc := range services {
		rm := metrics.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", svc)

		sm := rm.ScopeMetrics().AppendEmpty()
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("request.count")
		gauge := metric.SetEmptyGauge()

		for i := 0; i < 10; i++ {
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetIntValue(int64(i))
		}
	}

	return metrics
}

func hashBytes(data []byte) uint64 {
	h := fnv.New64a()
	h.Write(data)
	return h.Sum64()
}
