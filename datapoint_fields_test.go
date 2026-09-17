package otlpwire

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestDataPointFields(t *testing.T) {
	var raw []byte
	raw = protowire.AppendTag(raw, 3, protowire.Fixed64Type)
	raw = protowire.AppendFixed64(raw, 123)
	raw = protowire.AppendTag(raw, 7, protowire.BytesType)
	raw = protowire.AppendBytes(raw, []byte{0x80})
	raw = protowire.AppendTag(raw, 3, protowire.Fixed64Type)
	raw = protowire.AppendFixed64(raw, 456)
	raw = protowire.AppendTag(raw, 99, protowire.Fixed32Type)
	raw = protowire.AppendFixed32(raw, 789)
	raw = protowire.AppendTag(raw, 8, protowire.VarintType)
	raw = protowire.AppendVarint(raw, 42)
	for _, kind := range []MetricType{MetricTypeGauge, MetricTypeSum, MetricTypeHistogram, MetricTypeExponentialHistogram, MetricTypeSummary} {
		dp := NewDataPoint(raw, kind)
		require.Equal(t, kind, dp.Type())
		var fields []DataPointField
		for field, err := range dp.FieldsSeq {
			require.NoError(t, err)
			fields = append(fields, field)
		}
		require.Equal(t, []DataPointField{
			{Number: 3, Type: protowire.Fixed64Type, Uint64: 123},
			{Number: 7, Type: protowire.BytesType, Bytes: []byte{0x80}},
			{Number: 3, Type: protowire.Fixed64Type, Uint64: 456},
			{Number: 99, Type: protowire.Fixed32Type, Uint64: 789},
			{Number: 8, Type: protowire.VarintType, Uint64: 42},
		}, fields)
		require.Equal(t, len(fields[1].Bytes), cap(fields[1].Bytes))
		require.Same(t, &raw[11], &fields[1].Bytes[0])
	}
}

func TestDataPointFieldsFraming(t *testing.T) {
	for name, raw := range map[string][]byte{
		"tag": {0x80}, "zero tag": {0}, "varint": {8, 0x80},
		"fixed64": {9, 1}, "fixed32": {13, 1}, "bytes": {10, 2, 1},
		"unclosed group": {11}, "wrong end group": {11, 20},
		"unexpected end group": {12}, "invalid type": {14},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			for field, err := range NewDataPoint(raw, MetricTypeGauge).FieldsSeq {
				calls++
				require.Error(t, err)
				require.Equal(t, DataPointField{}, field)
			}
			require.Equal(t, 1, calls)
		})
	}
	var fields []DataPointField
	for field, err := range NewDataPoint([]byte{11, 16, 1, 12}, MetricTypeGauge).FieldsSeq {
		require.NoError(t, err)
		fields = append(fields, field)
	}
	require.Equal(t, []DataPointField{{Number: 1, Type: protowire.StartGroupType}}, fields)
}

func TestDataPointFieldsEarlyStopAndEmpty(t *testing.T) {
	for _, raw := range [][]byte{nil, {8, 1, 0x80}} {
		calls := 0
		for _, err := range NewDataPoint(raw, MetricTypeGauge).FieldsSeq {
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

func TestDataPointFieldsZeroAllocations(t *testing.T) {
	dp := NewDataPoint([]byte{8, 1, 18, 2, 3, 4}, MetricTypeGauge)
	allocs := testing.AllocsPerRun(100, func() {
		for _, err := range dp.FieldsSeq {
			if err != nil {
				panic(err)
			}
		}
	})
	require.Zero(t, allocs)
}

func TestDataPointFieldsStopsAfterError(t *testing.T) {
	calls := 0
	for field, err := range NewDataPoint([]byte{8, 1, 0, 8, 2}, MetricTypeGauge).FieldsSeq {
		calls++
		if calls == 1 {
			require.NoError(t, err)
			require.Equal(t, uint64(1), field.Uint64)
		} else {
			require.Error(t, err)
			require.Equal(t, DataPointField{}, field)
		}
	}
	require.Equal(t, 2, calls)
}

func TestDataPointFieldsInheritsProtowireTagRange(t *testing.T) {
	raw := []byte{0x80, 0x80, 0x80, 0x80, 0x10, 0}
	calls := 0
	for field, err := range NewDataPoint(raw, MetricTypeGauge).FieldsSeq {
		calls++
		require.NoError(t, err)
		require.Equal(t, protowire.MaxValidNumber+1, field.Number)
	}
	require.Equal(t, 1, calls)
}

var datapointFieldsSink uint64

func BenchmarkDataPointFields(b *testing.B) {
	var raw []byte
	raw = protowire.AppendTag(raw, 3, protowire.Fixed64Type)
	raw = protowire.AppendFixed64(raw, 123)
	for range 12 {
		raw = protowire.AppendTag(raw, 7, protowire.BytesType)
		raw = protowire.AppendString(raw, "attribute payload")
	}
	dp := NewDataPoint(raw, MetricTypeGauge)
	for _, passes := range []int{1, 2} {
		name := "SinglePass"
		if passes == 2 {
			name = "SeparatePasses"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			var sum uint64
			for range b.N {
				for pass := range passes {
					for field, err := range dp.FieldsSeq {
						if err != nil {
							b.Fatal(err)
						}
						if field.Number == 3 && pass == 0 {
							sum += field.Uint64
						}
						if field.Number == 7 && (passes == 1 || pass == 1) {
							sum += uint64(len(field.Bytes))
						}
					}
				}
			}
			datapointFieldsSink = sum
		})
	}
}

func FuzzDataPointFields(f *testing.F) {
	f.Add([]byte{8, 1, 18, 2, 3, 4})
	f.Add([]byte{11, 16, 1, 12})
	f.Fuzz(func(t *testing.T, raw []byte) {
		pos := 0
		failed := false
		for field, err := range NewDataPoint(raw, MetricTypeGauge).FieldsSeq {
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
