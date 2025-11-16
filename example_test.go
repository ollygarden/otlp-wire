package otlpwire_test

import (
	"bytes"
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
	count, _ := data.DataPointCount()

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

	resources, getErr := data.ResourceMetrics()
	i := 0
	for resource := range resources {
		// Hash resource for consistent routing
		resourceBytes, _ := resource.Resource()
		hash := hashBytes(resourceBytes)
		workerID := hash % uint64(numWorkers)

		var buf bytes.Buffer
		resource.WriteTo(&buf)
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

// Example_perServiceRateLimiting demonstrates per-service rate limiting.
func Example_perServiceRateLimiting() {
	metrics := createMultiServiceMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	serviceLimit := 15
	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)

	resources, getErr := data.ResourceMetrics()
	for resource := range resources {
		// Count signals in this resource
		var buf bytes.Buffer
		resource.WriteTo(&buf)
		count, _ := otlpwire.ExportMetricsServiceRequest(buf.Bytes()).DataPointCount()

		if count > serviceLimit {
			fmt.Printf("Resource rejected: %d data points (limit: %d)\n", count, serviceLimit)
		} else {
			fmt.Printf("Resource accepted: %d data points\n", count)
		}
	}
	if err := getErr(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	// Output:
	// Resource accepted: 10 data points
	// Resource accepted: 10 data points
	// Resource accepted: 10 data points
}

// Example_iteratorSharding demonstrates zero-allocation sharding using iterators.
func Example_iteratorSharding() {
	metrics := createMultiServiceMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
	numWorkers := 3

	// Iterator approach - no slice allocation
	resources, getErr := data.ResourceMetrics()
	for resource := range resources {
		// Hash resource for consistent routing
		resourceBytes, _ := resource.Resource()
		hash := hashBytes(resourceBytes)
		workerID := hash % uint64(numWorkers)

		var buf bytes.Buffer
		resource.WriteTo(&buf)
		count, _ := otlpwire.ExportMetricsServiceRequest(buf.Bytes()).DataPointCount()

		fmt.Printf("Worker %d: %d data points\n", workerID, count)
	}
	if err := getErr(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	// Output:
	// Worker 0: 10 data points
	// Worker 1: 10 data points
	// Worker 2: 10 data points
}

// Example_iteratorEarlyExit demonstrates stopping iteration early.
func Example_iteratorEarlyExit() {
	metrics := createMultiServiceMetrics()
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	data := otlpwire.ExportMetricsServiceRequest(otlpBytes)
	limit := 15

	// Process resources until limit is exceeded
	totalProcessed := 0
	resources, getErr := data.ResourceMetrics()
	for resource := range resources {
		var buf bytes.Buffer
		resource.WriteTo(&buf)
		count, _ := otlpwire.ExportMetricsServiceRequest(buf.Bytes()).DataPointCount()

		if totalProcessed+count > limit {
			fmt.Printf("Rate limit reached, skipping remaining resources\n")
			break // Early exit - remaining resources not parsed
		}

		totalProcessed += count
		fmt.Printf("Processed: %d data points (total: %d)\n", count, totalProcessed)
	}
	if err := getErr(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	// Output:
	// Processed: 10 data points (total: 10)
	// Rate limit reached, skipping remaining resources
}

// Example_typeComposition demonstrates how types compose naturally.
func Example_typeComposition() {
	metrics := createSampleMetrics(25)
	marshaler := &pmetric.ProtoMarshaler{}
	otlpBytes, _ := marshaler.MarshalMetrics(metrics)

	// MetricsData wraps complete OTLP message
	batch := otlpwire.ExportMetricsServiceRequest(otlpBytes)
	count, _ := batch.DataPointCount()
	fmt.Printf("Total data points: %d\n", count)

	// Iterate over resources
	resourceCount := 0
	resources, getErr := batch.ResourceMetrics()
	for resource := range resources {
		if resourceCount == 0 {
			// WriteTo writes valid OTLP message
			var buf bytes.Buffer
			resource.WriteTo(&buf)

			// Cast back to MetricsData to count this resource only
			singleResourceBatch := otlpwire.ExportMetricsServiceRequest(buf.Bytes())
			dpCount, _ := singleResourceBatch.DataPointCount()
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
	h.Write(data)
	return h.Sum64()
}
