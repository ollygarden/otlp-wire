package otlpwire_test

import (
	"bytes"
	"fmt"
	"hash/fnv"

	"go.opentelemetry.io/collector/pdata/pmetric"

	"go.olly.garden/otlp-wire"
)

// Example_observabilityStats demonstrates using Count() for observability metrics.
func Example_observabilityStats() {
	// Simulate receiving OTLP metrics data
	metrics := createSampleMetrics(100)
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	// Count signals for observability
	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
	count, _ := data.DataPointCount()

	// Emit metrics about incoming data (cardinality monitoring, billing, etc.)
	fmt.Printf("Received %d data points for processing\n", count)

	// Output: Received 100 data points for processing
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

	resources, getErr := data.ResourceMetrics()
	i := 0
	for resource := range resources {
		// Hash resource for consistent routing
		resourceBytes, _ := resource.Resource()
		hash := hashBytes(resourceBytes)
		workerID := int(hash % uint64(numWorkers))

		var buf bytes.Buffer
		_, _ = resource.WriteTo(&buf)
		count, _ := otlpwire.ExportMetricsServiceRequest(buf.Bytes()).DataPointCount()

		fmt.Printf("Resource %d → Worker %d (%d data points)\n", i, workerID, count)
		i++
	}
	if err := getErr(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	// Output:
	// Resource 0 → Worker 0 (10 data points)
	// Resource 1 → Worker 1 (10 data points)
	// Resource 2 → Worker 2 (10 data points)
}

// Example_typeComposition demonstrates how types compose naturally.
func Example_typeComposition() {
	metrics := createSampleMetrics(25)
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	// Count at batch level
	batch := otlpwire.ExportMetricsServiceRequest(otlpBytes)
	count, _ := batch.DataPointCount()
	fmt.Printf("Total data points: %d\n", count)

	// Iterate and count at resource level (zero allocation)
	resourceCount := 0
	resources, getErr := batch.ResourceMetrics()
	for resource := range resources {
		if resourceCount == 0 {
			// Count signals in this resource (zero allocation)
			dpCount, _ := resource.DataPointCount()
			fmt.Printf("Resource 0 data points: %d\n", dpCount)
		}

		resourceCount++
	}
	if err := getErr(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	fmt.Printf("Number of resources: %d\n", resourceCount)

	// Output:
	// Total data points: 25
	// Resource 0 data points: 25
	// Number of resources: 1
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
	_, _ = h.Write(data)
	return h.Sum64()
}
