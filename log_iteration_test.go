package otlpwire

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestLogRecordSeverityNumber(t *testing.T) {
	tests := []struct {
		name     string
		set      bool
		severity plog.SeverityNumber
		expected int32
	}{
		{name: "unset", expected: 0},
		{name: "trace minimum", set: true, severity: plog.SeverityNumberTrace, expected: 1},
		{name: "trace maximum", set: true, severity: plog.SeverityNumberTrace4, expected: 4},
		{name: "debug minimum", set: true, severity: plog.SeverityNumberDebug, expected: 5},
		{name: "debug maximum", set: true, severity: plog.SeverityNumberDebug4, expected: 8},
		{name: "info minimum", set: true, severity: plog.SeverityNumberInfo, expected: 9},
		{name: "info maximum", set: true, severity: plog.SeverityNumberInfo4, expected: 12},
		{name: "warn minimum", set: true, severity: plog.SeverityNumberWarn, expected: 13},
		{name: "warn maximum", set: true, severity: plog.SeverityNumberWarn4, expected: 16},
		{name: "error minimum", set: true, severity: plog.SeverityNumberError, expected: 17},
		{name: "error maximum", set: true, severity: plog.SeverityNumberFatal4, expected: 24},
		{name: "unexpected negative", set: true, severity: plog.SeverityNumber(-1), expected: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := plog.NewLogs()
			record := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
			if tt.set {
				record.SetSeverityNumber(tt.severity)
			}

			wireRecord := onlyLogRecord(t, marshalLogs(t, logs))
			got, err := wireRecord.SeverityNumber()
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestLogRecordSeverityNumber_CompleteMessageSemantics(t *testing.T) {
	var record LogRecord
	record = appendUnknownGroup(record, 90)
	record = protowire.AppendTag(record, 2, protowire.VarintType)
	record = protowire.AppendVarint(record, uint64(plog.SeverityNumberInfo))
	record = protowire.AppendTag(record, 91, protowire.VarintType)
	record = protowire.AppendVarint(record, 1)
	record = protowire.AppendTag(record, 2, protowire.VarintType)
	record = protowire.AppendVarint(record, uint64(plog.SeverityNumberError))
	record = appendUnknownGroup(record, 92)

	severity, err := record.SeverityNumber()
	require.NoError(t, err)
	require.Equal(t, int32(plog.SeverityNumberError), severity, "protobuf singular scalars are last-value-wins")

	trailingMalformed := append(append(LogRecord(nil), record...), 0x80)
	_, err = trailingMalformed.SeverityNumber()
	require.Error(t, err, "a valid early severity must not hide malformed trailing data")

	trailingWrongWire := append(append(LogRecord(nil), record...), protowire.AppendTag(nil, 2, protowire.BytesType)...)
	trailingWrongWire = protowire.AppendBytes(trailingWrongWire, []byte("bad"))
	_, err = trailingWrongWire.SeverityNumber()
	require.Error(t, err, "every severity occurrence must use the enum varint wire type")
}

func TestLogRecordSeverityNumber_PdataMalformedParity(t *testing.T) {
	tests := []struct {
		name   string
		record LogRecord
	}{
		{name: "malformed body any value", record: LogRecord{0x2a, 0x02, 0x0a, 0x80}},
		{name: "malformed attribute key value", record: LogRecord{0x32, 0x02, 0x08, 0x01}},
		{name: "timestamp wrong wire type", record: LogRecord{0x0a, 0x01, 0x00}},
		{name: "flags wrong wire type", record: LogRecord{0x40, 0x01}},
		{name: "trace id wrong wire type", record: LogRecord{0x48, 0x01}},
		{name: "event name wrong wire type", record: LogRecord{0x60, 0x01}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, wireErr := tt.record.SeverityNumber()
			_, pdataErr := (&plog.ProtoUnmarshaler{}).UnmarshalLogs(exportLogsWithRecord(tt.record))
			require.Error(t, wireErr)
			require.Error(t, pdataErr)
		})
	}
}

func TestLogTraversalAndResourceAttributes(t *testing.T) {
	logs := plog.NewLogs()
	withContext := logs.ResourceLogs().AppendEmpty()
	withContext.Resource().Attributes().PutStr("service.name", "checkout")
	withContext.Resource().Attributes().PutStr("service.namespace", "storefront")
	withContext.Resource().Attributes().PutStr("service.version", "1.2.3")
	withContext.Resource().Attributes().PutStr("deployment.environment", "production")
	withContext.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetSeverityNumber(plog.SeverityNumberWarn)

	withoutContext := logs.ResourceLogs().AppendEmpty()
	withoutContext.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetSeverityNumber(plog.SeverityNumberError)

	request := ExportLogsServiceRequest(marshalLogs(t, logs))
	resources, resourceErr := request.ResourceLogs()
	seen := 0
	resourceCount := 0
	for resource := range resources {
		resourceCount++
		scopes, scopeErr := resource.ScopeLogs()
		for scope := range scopes {
			records, recordErr := scope.LogRecords()
			for record := range records {
				severity, err := record.SeverityNumber()
				require.NoError(t, err)
				if seen == 0 {
					require.Equal(t, int32(plog.SeverityNumberWarn), severity)
				} else {
					require.Equal(t, int32(plog.SeverityNumberError), severity)
				}
				seen++
			}
			require.NoError(t, recordErr())
		}
		require.NoError(t, scopeErr())

		raw, err := resource.Resource()
		require.NoError(t, err)
		attrs := Resource(raw)
		for key, expected := range map[string]string{
			"service.name":           "checkout",
			"service.namespace":      "storefront",
			"service.version":        "1.2.3",
			"deployment.environment": "production",
		} {
			value, found, err := attrs.StringAttribute(key)
			require.NoError(t, err)
			if seen == 1 {
				require.True(t, found, key)
				require.Equal(t, expected, string(value))
			} else {
				require.False(t, found, key)
				require.Nil(t, value)
			}
		}
	}
	require.NoError(t, resourceErr())
	require.Equal(t, 2, resourceCount)
	require.Equal(t, 2, seen)
}

func TestResourceStringAttribute_EmptyAndNonString(t *testing.T) {
	logs := plog.NewLogs()
	resource := logs.ResourceLogs().AppendEmpty().Resource()
	resource.Attributes().PutStr("empty", "")
	resource.Attributes().PutInt("not.string", 42)

	request := ExportLogsServiceRequest(marshalLogs(t, logs))
	resources, resourceErr := request.ResourceLogs()
	resourceCount := 0
	for resource := range resources {
		resourceCount++
		raw, err := resource.Resource()
		require.NoError(t, err)
		attrs := Resource(raw)

		value, found, err := attrs.StringAttribute("empty")
		require.NoError(t, err)
		require.True(t, found)
		require.NotNil(t, value)
		require.Empty(t, value)

		value, found, err = attrs.StringAttribute("not.string")
		require.NoError(t, err)
		require.False(t, found)
		require.Nil(t, value)
	}
	require.NoError(t, resourceErr())
	require.Equal(t, 1, resourceCount)
}

func TestKeyValueRawAccessors_PreserveFirstEncodedOccurrence(t *testing.T) {
	var firstAnyValue []byte
	firstAnyValue = protowire.AppendTag(firstAnyValue, 1, protowire.BytesType)
	firstAnyValue = protowire.AppendBytes(firstAnyValue, []byte("first"))
	var secondAnyValue []byte
	secondAnyValue = protowire.AppendTag(secondAnyValue, 3, protowire.VarintType)
	secondAnyValue = protowire.AppendVarint(secondAnyValue, 7)

	var raw KeyValue
	raw = protowire.AppendTag(raw, 1, protowire.BytesType)
	raw = protowire.AppendBytes(raw, []byte("first.key"))
	raw = protowire.AppendTag(raw, 1, protowire.BytesType)
	raw = protowire.AppendBytes(raw, []byte("last.key"))
	raw = protowire.AppendTag(raw, 2, protowire.BytesType)
	raw = protowire.AppendBytes(raw, firstAnyValue)
	raw = protowire.AppendTag(raw, 2, protowire.BytesType)
	raw = protowire.AppendBytes(raw, secondAnyValue)

	key, err := raw.Key()
	require.NoError(t, err)
	require.Equal(t, "first.key", string(key))
	valueRaw, err := raw.ValueRaw()
	require.NoError(t, err)
	require.Equal(t, firstAnyValue, valueRaw)

	// StringValue is intentionally separate: it fully parses the merged
	// KeyValue/AnyValue and observes the final non-string oneof member.
	value, found, err := raw.StringValue()
	require.NoError(t, err)
	require.False(t, found)
	require.Nil(t, value)
}

func TestResourceStringAttribute_PdataParity(t *testing.T) {
	stringAnyValue := func(value string) []byte {
		var raw []byte
		raw = protowire.AppendTag(raw, 1, protowire.BytesType)
		return protowire.AppendBytes(raw, []byte(value))
	}
	intAnyValue := func(value uint64) []byte {
		var raw []byte
		raw = protowire.AppendTag(raw, 3, protowire.VarintType)
		return protowire.AppendVarint(raw, value)
	}
	keyValue := func(key string, values ...[]byte) []byte {
		var raw []byte
		raw = protowire.AppendTag(raw, 1, protowire.BytesType)
		raw = protowire.AppendBytes(raw, []byte(key))
		for _, value := range values {
			raw = protowire.AppendTag(raw, 2, protowire.BytesType)
			raw = protowire.AppendBytes(raw, value)
		}
		return raw
	}
	resource := func(attributes ...[]byte) []byte {
		var raw []byte
		for _, attribute := range attributes {
			raw = protowire.AppendTag(raw, 1, protowire.BytesType)
			raw = protowire.AppendBytes(raw, attribute)
		}
		return raw
	}

	tests := []struct {
		name     string
		resource []byte
		want     string
		found    bool
	}{
		{
			name:     "final any value oneof member overrides string",
			resource: resource(keyValue("service.name", append(stringAnyValue("first"), intAnyValue(7)...))),
			found:    false,
		},
		{
			name:     "repeated singular value messages merge in wire order",
			resource: resource(keyValue("service.name", stringAnyValue("first"), intAnyValue(7))),
			found:    false,
		},
		{
			name:     "duplicate resource key first value wins",
			resource: resource(keyValue("service.name", stringAnyValue("first")), keyValue("service.name", intAnyValue(7))),
			want:     "first",
			found:    true,
		},
		{
			name:     "duplicate resource key first non string wins",
			resource: resource(keyValue("service.name", intAnyValue(7)), keyValue("service.name", stringAnyValue("final"))),
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found, err := Resource(tt.resource).StringAttribute("service.name")
			require.NoError(t, err)
			require.Equal(t, tt.found, found)
			if found {
				require.Equal(t, tt.want, string(value))
			}

			logs, err := (&plog.ProtoUnmarshaler{}).UnmarshalLogs(exportLogsWithResource(tt.resource))
			require.NoError(t, err)
			attribute, pdataPresent := logs.ResourceLogs().At(0).Resource().Attributes().Get("service.name")
			pdataFound := pdataPresent && attribute.Type() == pcommon.ValueTypeStr
			require.Equal(t, tt.found, pdataFound)
			if pdataFound {
				require.Equal(t, tt.want, attribute.Str())
			}
		})
	}

	trailingCorruption := resource(keyValue("service.name", append(stringAnyValue("valid"), 0x80)))
	_, _, wireErr := Resource(trailingCorruption).StringAttribute("service.name")
	require.Error(t, wireErr)
	_, pdataErr := (&plog.ProtoUnmarshaler{}).UnmarshalLogs(exportLogsWithResource(trailingCorruption))
	require.Error(t, pdataErr)
}

func TestResourceStringAttribute_EntityRefPdataParity(t *testing.T) {
	resourceWithEntityRef := func(entityRef []byte) []byte {
		var resource []byte
		resource = protowire.AppendTag(resource, 3, protowire.BytesType)
		return protowire.AppendBytes(resource, entityRef)
	}
	validEntityRef := func() []byte {
		var entityRef []byte
		for fieldNum, value := range map[protowire.Number]string{
			1: "https://example.test/schema",
			2: "service",
			3: "service.name",
			4: "service.version",
		} {
			entityRef = protowire.AppendTag(entityRef, fieldNum, protowire.BytesType)
			entityRef = protowire.AppendBytes(entityRef, []byte(value))
		}
		return entityRef
	}

	tests := []struct {
		name      string
		entityRef []byte
		wantError bool
	}{
		{name: "all known fields are strings", entityRef: validEntityRef()},
		{name: "known field wrong wire type", entityRef: []byte{0x08, 0x01}, wantError: true},
		{name: "known field truncated length", entityRef: []byte{0x0a, 0x80}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := resourceWithEntityRef(tt.entityRef)
			_, _, wireErr := Resource(resource).StringAttribute("service.name")
			_, pdataErr := (&plog.ProtoUnmarshaler{}).UnmarshalLogs(exportLogsWithResource(resource))
			require.Equal(t, tt.wantError, wireErr != nil)
			require.Equal(t, tt.wantError, pdataErr != nil)
		})
	}
}

func TestResourceLogsStringAttribute_MergedResourcesPdataParity(t *testing.T) {
	stringAttribute := func(key, value string) []byte {
		var anyValue []byte
		anyValue = protowire.AppendTag(anyValue, 1, protowire.BytesType)
		anyValue = protowire.AppendBytes(anyValue, []byte(value))
		var keyValue []byte
		keyValue = protowire.AppendTag(keyValue, 1, protowire.BytesType)
		keyValue = protowire.AppendBytes(keyValue, []byte(key))
		keyValue = protowire.AppendTag(keyValue, 2, protowire.BytesType)
		keyValue = protowire.AppendBytes(keyValue, anyValue)
		var resource []byte
		resource = protowire.AppendTag(resource, 1, protowire.BytesType)
		return protowire.AppendBytes(resource, keyValue)
	}
	appendResource := func(raw []byte, resource []byte) []byte {
		raw = protowire.AppendTag(raw, 1, protowire.BytesType)
		return protowire.AppendBytes(raw, resource)
	}

	first := stringAttribute("service.name", "checkout")
	second := stringAttribute("service.name", "payments")
	merged := appendResource(appendResource(nil, first), second)
	value, found, err := ResourceLogs(merged).StringAttribute("service.name")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "checkout", string(value), "pdata retains the first duplicate key across merged Resources")

	logs, err := (&plog.ProtoUnmarshaler{}).UnmarshalLogs(exportLogsWithResourceLogs(merged))
	require.NoError(t, err)
	pdataValue, pdataFound := logs.ResourceLogs().At(0).Resource().Attributes().Get("service.name")
	require.True(t, pdataFound)
	require.Equal(t, "checkout", pdataValue.Str())

	malformedLaterResource := appendResource(merged, []byte{0x08, 0x01})
	_, _, wireErr := ResourceLogs(malformedLaterResource).StringAttribute("service.name")
	_, pdataErr := (&plog.ProtoUnmarshaler{}).UnmarshalLogs(exportLogsWithResourceLogs(malformedLaterResource))
	require.Error(t, wireErr)
	require.Error(t, pdataErr)
}

func TestSemanticParserDepthLimit(t *testing.T) {
	var leaf []byte
	leaf = protowire.AppendTag(leaf, 1, protowire.BytesType)
	leaf = protowire.AppendBytes(leaf, []byte("leaf"))
	nestedArray := func(levels int) []byte {
		value := leaf
		for range levels {
			var array []byte
			array = protowire.AppendTag(array, 1, protowire.BytesType)
			array = protowire.AppendBytes(array, value)
			value = nil
			value = protowire.AppendTag(value, 5, protowire.BytesType)
			value = protowire.AppendBytes(value, array)
		}
		return value
	}

	var parsed parsedAnyValue
	require.NoError(t, parseAnyValue(nestedArray(4), &parsed))
	require.NoError(t, parseAnyValueDepth(nestedArray(2), &parsed, semanticParseMaxDepth-2))
	require.ErrorIs(t, parseAnyValueDepth(nestedArray(3), &parsed, semanticParseMaxDepth-2), errSemanticParseDepth)
	require.ErrorIs(t, parseAnyValueDepth(leaf, &parsedAnyValue{}, semanticParseMaxDepth+1), errSemanticParseDepth)
	require.ErrorIs(t, parseAnyValue(nestedArray(semanticParseMaxDepth+2), &parsed), errSemanticParseDepth)
}

func TestUnknownGroupsAreSkippedInLogTraversalAndResourceAttributes(t *testing.T) {
	var record LogRecord
	record = appendUnknownGroup(record, 70)
	record = protowire.AppendTag(record, 2, protowire.VarintType)
	record = protowire.AppendVarint(record, uint64(plog.SeverityNumberInfo))
	record = appendUnknownGroup(record, 71)
	record = protowire.AppendTag(record, 2, protowire.VarintType)
	record = protowire.AppendVarint(record, uint64(plog.SeverityNumberWarn))
	record = appendUnknownGroup(record, 72)

	var scope ScopeLogs
	scope = appendUnknownGroup(scope, 60)
	scope = protowire.AppendTag(scope, 2, protowire.BytesType)
	scope = protowire.AppendBytes(scope, record)
	scope = appendUnknownGroup(scope, 61)
	scope = protowire.AppendTag(scope, 2, protowire.BytesType)
	scope = protowire.AppendBytes(scope, record)

	var resourceLogs ResourceLogs
	resourceLogs = appendUnknownGroup(resourceLogs, 50)
	resourceLogs = protowire.AppendTag(resourceLogs, 2, protowire.BytesType)
	resourceLogs = protowire.AppendBytes(resourceLogs, scope)
	resourceLogs = appendUnknownGroup(resourceLogs, 51)
	resourceLogs = protowire.AppendTag(resourceLogs, 2, protowire.BytesType)
	resourceLogs = protowire.AppendBytes(resourceLogs, scope)

	var resource Resource
	resource = appendUnknownGroup(resource, 40)
	var keyValue []byte
	keyValue = protowire.AppendTag(keyValue, 1, protowire.BytesType)
	keyValue = protowire.AppendBytes(keyValue, []byte("service.name"))
	var anyValue []byte
	anyValue = appendUnknownGroup(anyValue, 30)
	anyValue = protowire.AppendTag(anyValue, 1, protowire.BytesType)
	anyValue = protowire.AppendBytes(anyValue, []byte("checkout"))
	anyValue = appendUnknownGroup(anyValue, 31)
	keyValue = protowire.AppendTag(keyValue, 2, protowire.BytesType)
	keyValue = protowire.AppendBytes(keyValue, anyValue)
	resource = protowire.AppendTag(resource, 1, protowire.BytesType)
	resource = protowire.AppendBytes(resource, keyValue)
	resource = appendUnknownGroup(resource, 41)
	resource = protowire.AppendTag(resource, 1, protowire.BytesType)
	resource = protowire.AppendBytes(resource, keyValue)

	value, found, err := resource.StringAttribute("service.name")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "checkout", string(value))

	scopes, scopeErr := resourceLogs.ScopeLogs()
	records := 0
	for parsedScope := range scopes {
		for parsedRecord, err := range parsedScope.LogRecordsSeq {
			require.NoError(t, err)
			severity, err := parsedRecord.SeverityNumber()
			require.NoError(t, err)
			require.Equal(t, int32(plog.SeverityNumberWarn), severity)
			records++
		}
	}
	require.NoError(t, scopeErr())
	require.Equal(t, 4, records)
}

func TestScopeLogsLogRecords_EarlyStop(t *testing.T) {
	logs := plog.NewLogs()
	records := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	records.AppendEmpty().SetSeverityNumber(plog.SeverityNumberInfo)
	records.AppendEmpty().SetSeverityNumber(plog.SeverityNumberWarn)

	request := ExportLogsServiceRequest(marshalLogs(t, logs))
	resources, resourceErr := request.ResourceLogs()
	resourceCount := 0
	for resource := range resources {
		resourceCount++
		scopes, scopeErr := resource.ScopeLogs()
		for scope := range scopes {
			sequence, recordErr := scope.LogRecords()
			seen := 0
			for range sequence {
				seen++
				break
			}
			require.Equal(t, 1, seen)
			require.NoError(t, recordErr())
		}
		require.NoError(t, scopeErr())
	}
	require.NoError(t, resourceErr())
	require.Equal(t, 1, resourceCount)
}

func TestLogWireAccessors_MalformedAndWrongWire(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "scope logs wrong wire type",
			test: func(t *testing.T) {
				var scope ScopeLogs
				scope = protowire.AppendTag(scope, 2, protowire.VarintType)
				scope = protowire.AppendVarint(scope, 1)
				sequence, errFn := scope.LogRecords()
				for range sequence {
				}
				require.Error(t, errFn())
			},
		},
		{
			name: "scope logs malformed record length",
			test: func(t *testing.T) {
				scope := ScopeLogs{0x12, 0x02, 0x01}
				sawErr := false
				for _, err := range scope.LogRecordsSeq {
					if err != nil {
						sawErr = true
					}
				}
				require.True(t, sawErr)
			},
		},
		{
			name: "severity wrong wire type",
			test: func(t *testing.T) {
				var record LogRecord
				record = protowire.AppendTag(record, 2, protowire.BytesType)
				record = protowire.AppendBytes(record, []byte("bad"))
				_, err := record.SeverityNumber()
				require.Error(t, err)
			},
		},
		{
			name: "severity malformed varint",
			test: func(t *testing.T) {
				record := LogRecord{0x10, 0x80}
				_, err := record.SeverityNumber()
				require.Error(t, err)
			},
		},
		{
			name: "resource attributes wrong wire type",
			test: func(t *testing.T) {
				resource := Resource{0x08, 0x01}
				_, _, err := resource.StringAttribute("service.name")
				require.Error(t, err)
			},
		},
		{
			name: "any value string field wrong wire type",
			test: func(t *testing.T) {
				var keyValue []byte
				keyValue = protowire.AppendTag(keyValue, 1, protowire.BytesType)
				keyValue = protowire.AppendBytes(keyValue, []byte("service.name"))
				var anyValue []byte
				anyValue = protowire.AppendTag(anyValue, 1, protowire.VarintType)
				anyValue = protowire.AppendVarint(anyValue, 1)
				keyValue = protowire.AppendTag(keyValue, 2, protowire.BytesType)
				keyValue = protowire.AppendBytes(keyValue, anyValue)
				var resource []byte
				resource = protowire.AppendTag(resource, 1, protowire.BytesType)
				resource = protowire.AppendBytes(resource, keyValue)

				_, _, err := Resource(resource).StringAttribute("service.name")
				require.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestScopeLogsLogRecordsSeq_ZeroAllocations(t *testing.T) {
	logs := plog.NewLogs()
	records := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	for i := 0; i < 4; i++ {
		records.AppendEmpty().SetSeverityNumber(plog.SeverityNumberInfo)
	}

	request := ExportLogsServiceRequest(marshalLogs(t, logs))
	resources, resourceErr := request.ResourceLogs()
	resource, ok := nextResource(resources)
	require.True(t, ok)
	require.NoError(t, resourceErr())
	scopes, scopeErr := resource.ScopeLogs()
	scope, ok := nextScopeLogs(scopes)
	require.True(t, ok)
	require.NoError(t, scopeErr())

	allocations := testing.AllocsPerRun(1_000, func() {
		count := 0
		for record, err := range scope.LogRecordsSeq {
			if err != nil {
				t.Fatal(err)
			}
			severity, err := record.SeverityNumber()
			if err != nil {
				t.Fatal(err)
			}
			count += int(severity)
		}
		if count == 0 {
			t.Fatal("expected log records")
		}
	})
	require.Zero(t, allocations)
}

func marshalLogs(t *testing.T, logs plog.Logs) []byte {
	t.Helper()
	data, err := (&plog.ProtoMarshaler{}).MarshalLogs(logs)
	require.NoError(t, err)
	return data
}

func exportLogsWithResource(resource []byte) []byte {
	var resourceLogs []byte
	resourceLogs = protowire.AppendTag(resourceLogs, 1, protowire.BytesType)
	resourceLogs = protowire.AppendBytes(resourceLogs, resource)
	return exportLogsWithResourceLogs(resourceLogs)
}

func exportLogsWithRecord(record []byte) []byte {
	var scopeLogs []byte
	scopeLogs = protowire.AppendTag(scopeLogs, 2, protowire.BytesType)
	scopeLogs = protowire.AppendBytes(scopeLogs, record)
	var resourceLogs []byte
	resourceLogs = protowire.AppendTag(resourceLogs, 2, protowire.BytesType)
	resourceLogs = protowire.AppendBytes(resourceLogs, scopeLogs)
	return exportLogsWithResourceLogs(resourceLogs)
}

func exportLogsWithResourceLogs(resourceLogs []byte) []byte {
	var request []byte
	request = protowire.AppendTag(request, 1, protowire.BytesType)
	return protowire.AppendBytes(request, resourceLogs)
}

func appendUnknownGroup(data []byte, fieldNum protowire.Number) []byte {
	data = protowire.AppendTag(data, fieldNum, protowire.StartGroupType)
	data = protowire.AppendTag(data, fieldNum+1, protowire.VarintType)
	data = protowire.AppendVarint(data, 1)
	return protowire.AppendTag(data, fieldNum, protowire.EndGroupType)
}

func onlyLogRecord(t *testing.T, data []byte) LogRecord {
	t.Helper()
	request := ExportLogsServiceRequest(data)
	resources, resourceErr := request.ResourceLogs()
	resource, ok := nextResource(resources)
	require.True(t, ok)
	require.NoError(t, resourceErr())
	scopes, scopeErr := resource.ScopeLogs()
	scope, ok := nextScopeLogs(scopes)
	require.True(t, ok)
	require.NoError(t, scopeErr())
	records, recordErr := scope.LogRecords()
	record, ok := nextLogRecord(records)
	require.True(t, ok)
	require.NoError(t, recordErr())
	return record
}

func nextResource(sequence func(func(ResourceLogs) bool)) (ResourceLogs, bool) {
	var resource ResourceLogs
	found := false
	sequence(func(candidate ResourceLogs) bool {
		resource = candidate
		found = true
		return false
	})
	return resource, found
}

func nextScopeLogs(sequence func(func(ScopeLogs) bool)) (ScopeLogs, bool) {
	var scope ScopeLogs
	found := false
	sequence(func(candidate ScopeLogs) bool {
		scope = candidate
		found = true
		return false
	})
	return scope, found
}

func nextLogRecord(sequence func(func(LogRecord) bool)) (LogRecord, bool) {
	var record LogRecord
	found := false
	sequence(func(candidate LogRecord) bool {
		record = candidate
		found = true
		return false
	})
	return record, found
}
