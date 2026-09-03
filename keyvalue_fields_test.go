package otlpwire

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func keyValueBytesField(dst []byte, number protowire.Number, value string) []byte {
	dst = protowire.AppendTag(dst, number, protowire.BytesType)
	return protowire.AppendString(dst, value)
}

func keyValueVarintField(dst []byte, number protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, number, protowire.VarintType)
	return protowire.AppendVarint(dst, value)
}

func separateKeyValueFields(kv KeyValue) ([]byte, []byte, error) {
	key, err := kv.Key()
	if err != nil {
		return nil, nil, err
	}
	value, err := kv.ValueRaw()
	return key, value, err
}

func TestKeyValueFieldsMatchesSeparateAccessors(t *testing.T) {
	valid := func(fields ...func([]byte) []byte) []byte {
		var data []byte
		for _, field := range fields {
			data = field(data)
		}
		return data
	}
	key := func(value string) func([]byte) []byte {
		return func(dst []byte) []byte { return keyValueBytesField(dst, 1, value) }
	}
	value := func(raw string) func([]byte) []byte {
		return func(dst []byte) []byte { return keyValueBytesField(dst, 2, raw) }
	}
	unknown := func(dst []byte) []byte { return keyValueVarintField(dst, 99, 7) }
	wrongKey := func(dst []byte) []byte { return keyValueVarintField(dst, 1, 7) }
	wrongValue := func(dst []byte) []byte { return keyValueVarintField(dst, 2, 7) }
	malformed := func(dst []byte) []byte { return append(dst, 0x80) }
	truncatedValue := func(dst []byte) []byte {
		dst = protowire.AppendTag(dst, 2, protowire.BytesType)
		return append(dst, 4, 'x')
	}

	tests := map[string][]byte{
		"normal":                          valid(key("method"), value("GET")),
		"reversed":                        valid(value("GET"), key("method")),
		"absent key":                      valid(value("GET")),
		"absent value":                    valid(key("method")),
		"both absent":                     valid(unknown),
		"unknown before fields":           valid(unknown, key("method"), value("GET")),
		"first duplicate key wins":        valid(key("first"), key("second"), value("GET")),
		"first duplicate value wins":      valid(key("method"), value("first"), value("second")),
		"resolved wrong key is skipped":   valid(key("method"), wrongKey, value("GET")),
		"resolved wrong value is skipped": valid(value("GET"), wrongValue, key("method")),
		"wrong key":                       valid(wrongKey, value("GET")),
		"wrong value":                     valid(key("method"), wrongValue),
		"truncated value":                 valid(key("method"), truncatedValue),
		"malformed between fields":        valid(key("method"), malformed, value("GET")),
		"malformed after both":            valid(key("method"), value("GET"), malformed),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			wantKey, wantValue, wantErr := separateKeyValueFields(KeyValue(data))
			gotKey, gotValue, gotErr := KeyValue(data).Fields()
			assert.Equal(t, wantErr != nil, gotErr != nil)
			if wantErr == nil {
				assert.Equal(t, wantKey, gotKey)
				assert.Equal(t, wantValue, gotValue)
			}
		})
	}
}

func TestKeyValueFieldsReturnsClampedViews(t *testing.T) {
	data := keyValueBytesField(nil, 1, "method")
	data = keyValueBytesField(data, 2, "GET")
	original := append([]byte(nil), data...)

	key, value, err := KeyValue(data).Fields()
	require.NoError(t, err)
	require.Equal(t, len(key), cap(key))
	require.Equal(t, len(value), cap(value))
	_ = append(key, '!')
	_ = append(value, '!')
	assert.Equal(t, original, data)
}

func TestKeyValueFieldsAllocations(t *testing.T) {
	data := keyValueBytesField(nil, 1, "method")
	data = keyValueBytesField(data, 2, "GET")
	kv := KeyValue(data)

	allocs := testing.AllocsPerRun(1000, func() {
		key, value, err := kv.Fields()
		if err != nil || len(key) == 0 || len(value) == 0 {
			panic("unexpected result")
		}
	})
	assert.Zero(t, allocs)
}

var keyValueFieldsSink int

func BenchmarkKeyValueFields(b *testing.B) {
	data := keyValueBytesField(nil, 1, "deployment.environment.name")
	data = keyValueBytesField(data, 2, "production")
	kv := KeyValue(data)

	b.Run("separate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			key, err := kv.Key()
			if err != nil {
				b.Fatal(err)
			}
			value, err := kv.ValueRaw()
			if err != nil {
				b.Fatal(err)
			}
			keyValueFieldsSink = len(key) + len(value)
		}
	})

	b.Run("combined", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			key, value, err := kv.Fields()
			if err != nil {
				b.Fatal(err)
			}
			keyValueFieldsSink = len(key) + len(value)
		}
	})
}
