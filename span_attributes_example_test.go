package otlpwire_test

import (
	"fmt"

	"go.olly.garden/otlp-wire"
	"google.golang.org/protobuf/encoding/protowire"
)

func ExampleSpan_AttributesSeq() {
	attribute := func(key, value string) []byte {
		anyValue := protowire.AppendTag(nil, 1, protowire.BytesType)
		anyValue = protowire.AppendString(anyValue, value)

		kv := protowire.AppendTag(nil, 1, protowire.BytesType)
		kv = protowire.AppendString(kv, key)
		kv = protowire.AppendTag(kv, 2, protowire.BytesType)
		kv = protowire.AppendBytes(kv, anyValue)

		field := protowire.AppendTag(nil, 9, protowire.BytesType)
		return protowire.AppendBytes(field, kv)
	}

	var span []byte
	span = append(span, attribute("http.request.method", "GET")...)
	span = protowire.AppendTag(span, 5, protowire.BytesType)
	span = protowire.AppendString(span, "GET /orders")
	span = append(span, attribute("http.route", "/orders")...)

	for kv, err := range otlpwire.Span(span).AttributesSeq {
		if err != nil {
			fmt.Println(err)
			return
		}
		key, value, err := kv.Fields()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%d-byte key, %d-byte value\n", len(key), len(value))
	}
	// Output:
	// 19-byte key, 5-byte value
	// 10-byte key, 9-byte value
}
