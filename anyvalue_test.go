package otlpwire

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"google.golang.org/protobuf/encoding/protowire"
)

// bodyBytes builds a plog.Logs with a single LogRecord, applies set to its
// Body, marshals it with pdata, and returns the raw AnyValue message bytes
// (LogRecord field 5) plus the pdata Value it was built from, so a test can
// compare ParseAnyValue's result against pdata's own decode of the same
// bytes.
func bodyBytes(t *testing.T, set func(pcommon.Value)) ([]byte, pcommon.Value) {
	t.Helper()
	logs := plog.NewLogs()
	record := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	set(record.Body())

	wireRecord := onlyLogRecord(t, marshalLogs(t, logs))
	var body []byte
	for field, err := range wireRecord.FieldsSeq {
		require.NoError(t, err)
		if field.Number == 5 {
			body = field.Bytes
		}
	}

	roundTrip, err := (&plog.ProtoUnmarshaler{}).UnmarshalLogs(marshalLogs(t, logs))
	require.NoError(t, err)
	pdataValue := roundTrip.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body()
	return body, pdataValue
}

func TestParseAnyValue_AllKinds(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(pcommon.Value) {})
		require.Equal(t, pcommon.ValueTypeEmpty, pdataValue.Type())
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValue{Kind: AnyValueEmpty}, got)
	})

	t.Run("string", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) { v.SetStr("hello") })
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueString, got.Kind)
		require.Equal(t, pdataValue.Str(), string(got.Str))
	})

	t.Run("empty string is present", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) { v.SetStr("") })
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueString, got.Kind)
		require.Equal(t, pdataValue.Str(), string(got.Str))
		require.NotNil(t, got.Str)
	})

	t.Run("bool true", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) { v.SetBool(true) })
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueBool, got.Kind)
		require.Equal(t, pdataValue.Bool(), got.Bool)
	})

	t.Run("bool false", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) { v.SetBool(false) })
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueBool, got.Kind)
		require.Equal(t, pdataValue.Bool(), got.Bool)
	})

	t.Run("int positive", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) { v.SetInt(42) })
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueInt, got.Kind)
		require.Equal(t, pdataValue.Int(), got.Int)
	})

	t.Run("int negative", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) { v.SetInt(-1) })
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueInt, got.Kind)
		require.Equal(t, pdataValue.Int(), got.Int)
	})

	t.Run("double", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) { v.SetDouble(3.5) })
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueDouble, got.Kind)
		require.Equal(t, pdataValue.Double(), got.Double)
	})

	t.Run("bytes", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) {
			v.SetEmptyBytes().FromRaw([]byte{1, 2, 3})
		})
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueBytes, got.Kind)
		require.Equal(t, pdataValue.Bytes().AsRaw(), got.Raw)
	})

	t.Run("array", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) {
			slice := v.SetEmptySlice()
			slice.AppendEmpty().SetStr("a")
			slice.AppendEmpty().SetInt(2)
		})
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueArray, got.Kind)

		// KeyValuesSeq only applies to KeyValueList (verified separately);
		// decode the array elements directly from Raw (field 1, repeated
		// Value) to confirm it holds the raw ArrayValue message body.
		var elements []AnyValue
		forEachRepeatedField(got.Raw, 1, func(item []byte, err error) bool {
			require.NoError(t, err)
			element, elErr := ParseAnyValue(item)
			require.NoError(t, elErr)
			elements = append(elements, element)
			return true
		})
		require.Equal(t, pdataValue.Slice().Len(), len(elements))
		require.Equal(t, AnyValue{Kind: AnyValueString, Str: []byte("a")}, elements[0])
		require.Equal(t, AnyValue{Kind: AnyValueInt, Int: 2}, elements[1])
	})

	t.Run("kvlist", func(t *testing.T) {
		body, pdataValue := bodyBytes(t, func(v pcommon.Value) {
			m := v.SetEmptyMap()
			m.PutStr("k1", "v1")
			m.PutInt("k2", 2)
		})
		got, err := ParseAnyValue(body)
		require.NoError(t, err)
		require.Equal(t, AnyValueKeyValueList, got.Kind)

		var keys []string
		for kv, err := range got.KeyValuesSeq {
			require.NoError(t, err)
			key, err := kv.Key()
			require.NoError(t, err)
			keys = append(keys, string(key))
		}
		require.Equal(t, pdataValue.Map().Len(), len(keys))
		require.Equal(t, []string{"k1", "k2"}, keys)
	})
}

func TestAnyValue_KeyValuesSeqNoOpForNonKeyValueList(t *testing.T) {
	values := []AnyValue{
		{Kind: AnyValueEmpty},
		{Kind: AnyValueString, Str: []byte("x")},
		{Kind: AnyValueBool, Bool: true},
		{Kind: AnyValueInt, Int: 1},
		{Kind: AnyValueDouble, Double: 1.5},
		{Kind: AnyValueArray, Raw: []byte{1, 2, 3}},
		{Kind: AnyValueBytes, Raw: []byte{1, 2, 3}},
	}
	for _, v := range values {
		calls := 0
		for range v.KeyValuesSeq {
			calls++
		}
		require.Zero(t, calls)
	}
}

func TestParseAnyValue_LastValueWins(t *testing.T) {
	var data []byte
	data = protowire.AppendTag(data, 1, protowire.BytesType)
	data = protowire.AppendString(data, "first")
	data = protowire.AppendTag(data, 3, protowire.VarintType)
	data = protowire.AppendVarint(data, 7)

	got, err := ParseAnyValue(data)
	require.NoError(t, err)
	require.Equal(t, AnyValue{Kind: AnyValueInt, Int: 7}, got)
}

func TestParseAnyValue_MalformedFraming(t *testing.T) {
	for name, data := range map[string][]byte{
		"bad tag":          {0x80},
		"truncated string": append(protowire.AppendTag(nil, 1, protowire.BytesType), 4, 'a'),
		"wrong wire type":  protowire.AppendTag(nil, 3, protowire.BytesType),
		"malformed array":  protowire.AppendBytes(protowire.AppendTag(nil, 5, protowire.BytesType), []byte{0x80}),
		"malformed kvlist": protowire.AppendBytes(protowire.AppendTag(nil, 6, protowire.BytesType), []byte{0x80}),
		"nested kvlist error": protowire.AppendBytes(
			protowire.AppendTag(nil, 6, protowire.BytesType),
			protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), []byte{0x80}),
		),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseAnyValue(data)
			require.Error(t, err)
		})
	}
}

func TestKeyValue_Value(t *testing.T) {
	var kv []byte
	kv = keyValueBytesField(kv, 1, "http.method")
	var value []byte
	value = protowire.AppendTag(value, 1, protowire.BytesType)
	value = protowire.AppendString(value, "GET")
	kv = protowire.AppendTag(kv, 2, protowire.BytesType)
	kv = protowire.AppendBytes(kv, value)

	got, err := KeyValue(kv).Value()
	require.NoError(t, err)
	require.Equal(t, AnyValueString, got.Kind)
	require.Equal(t, "GET", string(got.Str))
}

func TestKeyValue_ValueAbsent(t *testing.T) {
	kv := keyValueBytesField(nil, 1, "http.method")
	got, err := KeyValue(kv).Value()
	require.NoError(t, err)
	require.Equal(t, AnyValue{}, got)
}

func TestKeyValue_ValueLastWinsMatchesStringValue(t *testing.T) {
	var kv []byte
	kv = keyValueBytesField(kv, 1, "k")
	var first []byte
	first = protowire.AppendTag(first, 3, protowire.VarintType)
	first = protowire.AppendVarint(first, 1)
	kv = protowire.AppendTag(kv, 2, protowire.BytesType)
	kv = protowire.AppendBytes(kv, first)
	var second []byte
	second = protowire.AppendTag(second, 1, protowire.BytesType)
	second = protowire.AppendString(second, "wins")
	kv = protowire.AppendTag(kv, 2, protowire.BytesType)
	kv = protowire.AppendBytes(kv, second)

	got, err := KeyValue(kv).Value()
	require.NoError(t, err)
	require.Equal(t, AnyValueString, got.Kind)
	require.Equal(t, "wins", string(got.Str))

	str, found, err := KeyValue(kv).StringValue()
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "wins", string(str))
}
