package otlpwire_test

import (
	"fmt"

	"go.olly.garden/otlp-wire"
	"google.golang.org/protobuf/encoding/protowire"
)

func ExampleLogRecord_FieldsSeq() {
	var body []byte
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, "GET /healthz 200")

	var record []byte
	record = protowire.AppendTag(record, 1, protowire.Fixed64Type)
	record = protowire.AppendFixed64(record, 1700000000000000000)
	record = protowire.AppendTag(record, 5, protowire.BytesType)
	record = protowire.AppendBytes(record, body)

	for field, err := range otlpwire.LogRecord(record).FieldsSeq {
		if err != nil {
			fmt.Println(err)
			return
		}
		switch field.Number {
		case 1:
			if field.Type != protowire.Fixed64Type {
				fmt.Println("invalid timestamp type")
				return
			}
			fmt.Println("timestamp:", field.Uint64)
		case 5:
			if field.Type != protowire.BytesType {
				fmt.Println("invalid body type")
				return
			}
			// FieldsSeq checks framing; ParseAnyValue validates the nested
			// AnyValue oneof.
			value, err := otlpwire.ParseAnyValue(field.Bytes)
			if err != nil {
				fmt.Println(err)
				return
			}
			if value.Kind == otlpwire.AnyValueString {
				fmt.Printf("body: %s\n", value.Str)
			}
		}
	}
	// Output:
	// timestamp: 1700000000000000000
	// body: GET /healthz 200
}
