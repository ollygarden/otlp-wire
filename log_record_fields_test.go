package otlpwire

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestLogRecordFields(t *testing.T) {
	var raw []byte
	raw = protowire.AppendTag(raw, 1, protowire.Fixed64Type)
	raw = protowire.AppendFixed64(raw, 123)
	raw = protowire.AppendTag(raw, 6, protowire.BytesType)
	bytesOffset := len(raw) + 1 // +1 for the length-prefix varint of the 1-byte payload
	raw = protowire.AppendBytes(raw, []byte{0x80})
	raw = protowire.AppendTag(raw, 1, protowire.Fixed64Type)
	raw = protowire.AppendFixed64(raw, 456)
	raw = protowire.AppendTag(raw, 99, protowire.Fixed32Type)
	raw = protowire.AppendFixed32(raw, 789)
	raw = protowire.AppendTag(raw, 2, protowire.VarintType)
	raw = protowire.AppendVarint(raw, 42)

	record := LogRecord(raw)
	var fields []LogRecordField
	for field, err := range record.FieldsSeq {
		require.NoError(t, err)
		fields = append(fields, field)
	}
	require.Equal(t, []LogRecordField{
		{Number: 1, Type: protowire.Fixed64Type, Uint64: 123},
		{Number: 6, Type: protowire.BytesType, Bytes: []byte{0x80}},
		{Number: 1, Type: protowire.Fixed64Type, Uint64: 456},
		{Number: 99, Type: protowire.Fixed32Type, Uint64: 789},
		{Number: 2, Type: protowire.VarintType, Uint64: 42},
	}, fields)
	require.Equal(t, len(fields[1].Bytes), cap(fields[1].Bytes))
	require.Same(t, &raw[bytesOffset], &fields[1].Bytes[0])
}

func TestLogRecordFieldsFraming(t *testing.T) {
	for name, raw := range map[string][]byte{
		"tag": {0x80}, "zero tag": {0}, "varint": {8, 0x80},
		"fixed64": {9, 1}, "fixed32": {13, 1}, "bytes": {10, 2, 1},
		"unclosed group": {11}, "wrong end group": {11, 20},
		"unexpected end group": {12}, "invalid type": {14},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			for field, err := range LogRecord(raw).FieldsSeq {
				calls++
				require.Error(t, err)
				require.Equal(t, LogRecordField{}, field)
			}
			require.Equal(t, 1, calls)
		})
	}
	var fields []LogRecordField
	for field, err := range LogRecord([]byte{11, 16, 1, 12}).FieldsSeq {
		require.NoError(t, err)
		fields = append(fields, field)
	}
	require.Equal(t, []LogRecordField{{Number: 1, Type: protowire.StartGroupType}}, fields)
}

func TestLogRecordFieldsEarlyStopAndEmpty(t *testing.T) {
	for _, raw := range [][]byte{nil, {8, 1, 0x80}} {
		calls := 0
		for _, err := range LogRecord(raw).FieldsSeq {
			require.NoError(t, err)
			calls++
			break
		}
		if len(raw) == 0 {
			require.Zero(t, calls)
		} else {
			require.Equal(t, 1, calls)
		}
	}
}

func TestLogRecordFieldsZeroAllocations(t *testing.T) {
	record := LogRecord([]byte{8, 1, 18, 2, 3, 4})
	allocs := testing.AllocsPerRun(100, func() {
		for _, err := range record.FieldsSeq {
			if err != nil {
				panic(err)
			}
		}
	})
	require.Zero(t, allocs)
}

func TestLogRecordFieldsStopsAfterError(t *testing.T) {
	calls := 0
	for field, err := range LogRecord([]byte{8, 1, 0, 8, 2}).FieldsSeq {
		calls++
		if calls == 1 {
			require.NoError(t, err)
			require.Equal(t, uint64(1), field.Uint64)
		} else {
			require.Error(t, err)
			require.Equal(t, LogRecordField{}, field)
		}
	}
	require.Equal(t, 2, calls)
}

func TestLogRecordFieldsInheritsProtowireTagRange(t *testing.T) {
	raw := []byte{0x80, 0x80, 0x80, 0x80, 0x10, 0}
	calls := 0
	for field, err := range LogRecord(raw).FieldsSeq {
		calls++
		require.NoError(t, err)
		require.Equal(t, protowire.MaxValidNumber+1, field.Number)
	}
	require.Equal(t, 1, calls)
}

// TestLogRecordFieldsMatchesPdataOrder builds a fully populated LogRecord with
// pdata, marshals it, and verifies FieldsSeq walks every top-level field
// pdata's marshaler produced, in the same order pdata wrote them. pdata
// writes generated-struct fields back-to-front (see AGENTS.md), so the wire
// order is the reverse of proto declaration order; trace_id (9) and span_id
// (10) are always emitted even when left unset (zero-value fixed-size byte
// arrays). This pins that order for the fields Bale reads: time_unix_nano
// (1), body (5), attributes (6), and observed_time_unix_nano (11, excluded
// from identity hashing by the consumer, not by this package).
func TestLogRecordFieldsMatchesPdataOrder(t *testing.T) {
	logs := plog.NewLogs()
	record := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	record.SetTimestamp(123)
	record.SetObservedTimestamp(456)
	record.Body().SetStr("hello")
	record.Attributes().PutStr("http.method", "GET")

	wireRecord := onlyLogRecord(t, marshalLogs(t, logs))

	var numbers []protowire.Number
	for field, err := range wireRecord.FieldsSeq {
		require.NoError(t, err)
		numbers = append(numbers, field.Number)
	}
	require.Equal(t, []protowire.Number{10, 9, 6, 5, 11, 1}, numbers)
}

func BenchmarkLogRecordFields(b *testing.B) {
	logs := plog.NewLogs()
	record := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	record.SetTimestamp(123)
	record.SetObservedTimestamp(456)
	record.SetSeverityNumber(plog.SeverityNumberInfo)
	record.SetSeverityText("INFO")
	record.Body().SetStr("GET /healthz 200 12ms")
	attrs := record.Attributes()
	attrs.PutStr("http.method", "GET")
	attrs.PutStr("http.route", "/healthz")
	attrs.PutInt("http.status_code", 200)
	attrs.PutStr("net.peer.ip", "10.0.0.1")

	data, err := (&plog.ProtoMarshaler{}).MarshalLogs(logs)
	if err != nil {
		b.Fatal(err)
	}
	request := ExportLogsServiceRequest(data)
	resources, resourceErr := request.ResourceLogs()
	resource, ok := nextResource(resources)
	if !ok || resourceErr() != nil {
		b.Fatal("missing resource")
	}
	scopes, scopeErr := resource.ScopeLogs()
	scope, ok := nextScopeLogs(scopes)
	if !ok || scopeErr() != nil {
		b.Fatal("missing scope")
	}
	records, recordErr := scope.LogRecords()
	wireRecord, ok := nextLogRecord(records)
	if !ok || recordErr() != nil {
		b.Fatal("missing record")
	}

	b.ReportAllocs()
	var sum uint64
	for b.Loop() {
		for field, err := range wireRecord.FieldsSeq {
			if err != nil {
				b.Fatal(err)
			}
			sum += field.Uint64 + uint64(len(field.Bytes))
		}
	}
	logRecordFieldsSink = sum
}

var logRecordFieldsSink uint64

func FuzzLogRecordFields(f *testing.F) {
	f.Add([]byte{8, 1, 18, 2, 3, 4})
	f.Add([]byte{11, 16, 1, 12})
	f.Fuzz(func(t *testing.T, raw []byte) {
		pos := 0
		failed := false
		for field, err := range LogRecord(raw).FieldsSeq {
			require.False(t, failed)
			number, typ, n := protowire.ConsumeTag(raw[pos:])
			if n >= 0 {
				m := protowire.ConsumeFieldValue(number, typ, raw[pos+n:])
				if m < 0 {
					n = m
				} else {
					n += m
				}
			}
			if n < 0 {
				require.Error(t, err)
				failed = true
				continue
			}
			require.NoError(t, err)
			require.Equal(t, number, field.Number)
			require.Equal(t, typ, field.Type)
			pos += n
		}
		require.True(t, failed || pos == len(raw))
	})
}
