package otlpwire_test

import (
	"fmt"

	"go.olly.garden/otlp-wire"
	"google.golang.org/protobuf/encoding/protowire"
)

func ExampleDataPoint_FieldsSeq() {
	raw := protowire.AppendTag(nil, 3, protowire.Fixed64Type)
	raw = protowire.AppendFixed64(raw, 123)
	raw = protowire.AppendTag(raw, 7, protowire.BytesType)
	raw = protowire.AppendBytes(raw, []byte{10, 1, 'k', 18, 3, 10, 1, 'v'})
	point := otlpwire.NewDataPoint(raw, otlpwire.MetricTypeGauge)
	for field, err := range point.FieldsSeq {
		if err != nil {
			fmt.Println(err)
			return
		}
		switch field.Number {
		case 3:
			if field.Type != protowire.Fixed64Type {
				fmt.Println("invalid timestamp type")
				return
			}
			fmt.Println("timestamp:", field.Uint64)
		case 7:
			if field.Type != protowire.BytesType {
				fmt.Println("invalid attribute type")
				return
			}
			// FieldsSeq checks framing; StringValue validates the nested KeyValue.
			value, found, err := otlpwire.KeyValue(field.Bytes).StringValue()
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Printf("string attribute: %s (%t)\n", value, found)
		}
	}
	// Output:
	// timestamp: 123
	// string attribute: v (true)
}
