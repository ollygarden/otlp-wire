package otlpwire_test

import (
	"bytes"
	"fmt"
	"hash/fnv"

	"go.opentelemetry.io/collector/pdata/plog"
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

// ExampleMetric_DataPoints demonstrates walking the metrics-depth API down to
// individual data point attributes.
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

// ExampleMetric_Metadata reads the metadata a receiver attaches to a metric.
// MetadataSeq is the zero-allocation variant for hot paths.
func ExampleMetric_Metadata() {
	metrics := pmetric.NewMetrics()
	sm := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("http.server.duration")
	metric.Metadata().PutStr("prometheus.type", "histogram")
	metric.Metadata().PutStr("prometheus.unit", "seconds")
	metric.SetEmptyGauge().DataPoints().AppendEmpty().SetDoubleValue(0.35)

	marshaler := &pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	if err != nil {
		fmt.Println(err)
		return
	}

	if err := printMetricMetadata(data); err != nil {
		fmt.Println(err)
	}
	// Output:
	// http.server.duration prometheus.type=histogram
	// http.server.duration prometheus.unit=seconds
}

func printMetricMetadata(data []byte) error {
	resources, resourcesErr := otlpwire.ExportMetricsServiceRequest(data).ResourceMetrics()
	for rm := range resources {
		scopes, scopesErr := rm.ScopeMetrics()
		for sm := range scopes {
			metrics, metricsErr := sm.Metrics()
			for m := range metrics {
				name, err := m.Name()
				if err != nil {
					return err
				}
				metadata, metadataErr := m.Metadata()
				for kv := range metadata {
					key, err := kv.Key()
					if err != nil {
						return err
					}
					value, _, err := kv.StringValue()
					if err != nil {
						return err
					}
					fmt.Printf("%s %s=%s\n", name, key, value)
				}
				if err := metadataErr(); err != nil {
					return err
				}
			}
			if err := metricsErr(); err != nil {
				return err
			}
		}
		if err := scopesErr(); err != nil {
			return err
		}
	}
	return resourcesErr()
}

// ExampleResourceLogs_Resource walks resource context and log records
// without unmarshaling the OTLP request. It replaces the removed
// ResourceLogs.StringAttribute: call Resource() to get the one, merged
// Resource for this container, then StringAttribute on that Resource.
func ExampleResourceLogs_Resource() {
	logs := plog.NewLogs()
	pdataResource := logs.ResourceLogs().AppendEmpty()
	pdataResource.Resource().Attributes().PutStr("service.name", "checkout")
	scope := pdataResource.ScopeLogs().AppendEmpty()
	scope.LogRecords().AppendEmpty().SetSeverityNumber(plog.SeverityNumberWarn)

	data, err := (&plog.ProtoMarshaler{}).MarshalLogs(logs)
	if err != nil {
		fmt.Println(err)
		return
	}

	request := otlpwire.ExportLogsServiceRequest(data)
	resources, resourceErr := request.ResourceLogs()
	for resourceLogs := range resources {
		resource, err := resourceLogs.Resource()
		if err != nil {
			fmt.Println(err)
			break
		}
		service, found, err := resource.StringAttribute("service.name")
		if err != nil {
			fmt.Println(err)
			break
		}
		if !found {
			fmt.Println("service.name missing")
			break
		}

		scopes, scopeErr := resourceLogs.ScopeLogs()
		for scope := range scopes {
			for record, err := range scope.LogRecordsSeq {
				if err != nil {
					fmt.Println(err)
					break
				}
				severity, err := record.SeverityNumber()
				if err != nil {
					fmt.Println(err)
					break
				}
				fmt.Printf("%s severity=%d\n", service, severity)
			}
		}
		if err := scopeErr(); err != nil {
			fmt.Println(err)
			break
		}
	}
	if err := resourceErr(); err != nil {
		fmt.Println(err)
	}

	// Output: checkout severity=13
}

// ExampleLogRecord_SeverityText reads both severity fields together, which is
// what a severity gate needs: the number orders records, the text carries the
// producer's own label. SeverityText returns a view into the request buffer,
// and nil distinguishes an absent severity_text from a present empty one.
func ExampleLogRecord_SeverityText() {
	logs := plog.NewLogs()
	records := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	labelled := records.AppendEmpty()
	labelled.SetSeverityNumber(plog.SeverityNumberError)
	labelled.SetSeverityText("ERR")
	records.AppendEmpty().SetSeverityNumber(plog.SeverityNumberInfo)

	data, err := (&plog.ProtoMarshaler{}).MarshalLogs(logs)
	if err != nil {
		fmt.Println(err)
		return
	}

	if err := printLogSeverities(data); err != nil {
		fmt.Println(err)
	}

	// Output:
	// severity=17 text=ERR
	// severity=9 text=<unset>
}

func printLogSeverities(data []byte) error {
	resources, resourcesErr := otlpwire.ExportLogsServiceRequest(data).ResourceLogs()
	var failure error
	for resourceLogs := range resources {
		if failure = printResourceLogSeverities(resourceLogs); failure != nil {
			break
		}
	}
	// The error closure must be checked even when the loop exited early,
	// so the split into one function per level is the point of this shape,
	// not incidental.
	if err := resourcesErr(); err != nil {
		return err
	}
	return failure
}

func printResourceLogSeverities(resourceLogs otlpwire.ResourceLogs) error {
	scopes, scopesErr := resourceLogs.ScopeLogs()
	var failure error
	for scope := range scopes {
		if failure = printScopeLogSeverities(scope); failure != nil {
			break
		}
	}
	if err := scopesErr(); err != nil {
		return err
	}
	return failure
}

// printScopeLogSeverities needs no closure check: LogRecordsSeq yields its
// errors inline rather than deferring them.
func printScopeLogSeverities(scope otlpwire.ScopeLogs) error {
	for record, err := range scope.LogRecordsSeq {
		if err != nil {
			return err
		}
		number, err := record.SeverityNumber()
		if err != nil {
			return err
		}
		text, err := record.SeverityText()
		if err != nil {
			return err
		}
		if text == nil {
			fmt.Printf("severity=%d text=<unset>\n", number)
			continue
		}
		fmt.Printf("severity=%d text=%s\n", number, text)
	}
	return nil
}

// ExampleScopeLogs_Scope reads instrumentation scope identity and schema URL
// without unmarshaling the request. Each scope container has exactly one
// InstrumentationScope, mirroring pdata's object model.
func ExampleScopeLogs_Scope() {
	logs := plog.NewLogs()
	resourceLogs := logs.ResourceLogs().AppendEmpty()
	resourceLogs.SetSchemaUrl("https://opentelemetry.io/schemas/1.29.0")
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()
	scopeLogs.Scope().SetName("checkout-instrumentation")
	scopeLogs.Scope().SetVersion("1.2.3")
	scopeLogs.LogRecords().AppendEmpty()

	data, err := (&plog.ProtoMarshaler{}).MarshalLogs(logs)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Each iterator's error closure must run even when the loop exits early,
	// so accessor errors break out rather than returning past the check.
	request := otlpwire.ExportLogsServiceRequest(data)
	resources, resourceErr := request.ResourceLogs()
	for resource := range resources {
		schemaURL, err := resource.SchemaUrl()
		if err != nil {
			fmt.Println(err)
			break
		}

		scopes, scopeErr := resource.ScopeLogs()
		for scopeLogs := range scopes {
			name, version, err := scopeIdentity(scopeLogs)
			if err != nil {
				fmt.Println(err)
				break
			}
			fmt.Printf("%s@%s schema=%s\n", name, version, schemaURL)
		}
		if err := scopeErr(); err != nil {
			fmt.Println(err)
			break
		}
	}
	if err := resourceErr(); err != nil {
		fmt.Println(err)
	}

	// Output: checkout-instrumentation@1.2.3 schema=https://opentelemetry.io/schemas/1.29.0
}

// scopeIdentity reads a scope's name and version, keeping the example's loop
// body free of the three error checks the lazy accessors require.
func scopeIdentity(scopeLogs otlpwire.ScopeLogs) (name, version []byte, err error) {
	scope, err := scopeLogs.Scope()
	if err != nil {
		return nil, nil, err
	}
	if name, err = scope.Name(); err != nil {
		return nil, nil, err
	}
	version, err = scope.Version()
	return name, version, err
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
