package otlpwire

import (
	"errors"

	"google.golang.org/protobuf/encoding/protowire"
)

// DataPointField is a decoded top-level datapoint field. Bytes aliases the
// request for length-delimited fields; Uint64 contains varint or fixed bits.
// Interpret values according to Type and the datapoint's MetricType.
type DataPointField struct {
	Number protowire.Number
	Type   protowire.Type
	Bytes  []byte
	Uint64 uint64
}

// NewDataPoint wraps raw datapoint bytes without copying or validating them.
// The caller must supply the enclosing metric type and keep raw unchanged.
func NewDataPoint(raw []byte, typ MetricType) DataPoint {
	return DataPoint{raw: raw, typ: typ}
}

// FieldsSeq walks the top-level wire fields once, in encoded order. It checks
// framing, not schema: callers validate known field types and nested contents.
// Groups are checked and yielded with empty values. Repeated fields are not resolved.
// Tag validity follows protowire.ConsumeTag, which accepts numbers through
// MaxInt32, including numbers above protobuf's MaxValidNumber.
// Returning false stops immediately, leaving the remaining bytes unvalidated.
// An error is yielded once with a zero field, then iteration stops.
func (d DataPoint) FieldsSeq(yield func(DataPointField, error) bool) {
	data := d.raw
	for len(data) > 0 {
		number, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			yield(DataPointField{}, errors.New("malformed protobuf tag in datapoint"))
			return
		}
		data = data[n:]
		field := DataPointField{Number: number, Type: typ}
		switch typ {
		case protowire.VarintType:
			field.Uint64, n = protowire.ConsumeVarint(data)
		case protowire.Fixed32Type:
			var value uint32
			value, n = protowire.ConsumeFixed32(data)
			field.Uint64 = uint64(value)
		case protowire.Fixed64Type:
			field.Uint64, n = protowire.ConsumeFixed64(data)
		case protowire.BytesType:
			field.Bytes, n = protowire.ConsumeBytes(data)
			if n >= 0 {
				field.Bytes = field.Bytes[:len(field.Bytes):len(field.Bytes)]
			}
		default:
			n = protowire.ConsumeFieldValue(number, typ, data)
		}
		if n < 0 {
			yield(DataPointField{}, errors.New("malformed protobuf field in datapoint"))
			return
		}
		data = data[n:]
		if !yield(field, nil) {
			return
		}
	}
}
