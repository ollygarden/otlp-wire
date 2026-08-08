package otlpwire

import (
	"errors"
	"iter"

	"google.golang.org/protobuf/encoding/protowire"
)

// Key returns the attribute key (field 1) as a view into the underlying
// buffer. Returns nil if the field is not present.
func (kv KeyValue) Key() ([]byte, error) {
	return extractBytesField([]byte(kv), 1)
}

// ValueRaw returns the raw AnyValue message bytes (field 2) as a view into
// the underlying buffer, suitable for type-tagged hashing.
// Returns nil if the field is not present.
func (kv KeyValue) ValueRaw() ([]byte, error) {
	return extractBytesField([]byte(kv), 2)
}

// StringValue returns the string value in the KeyValue's AnyValue, if it has
// one. The bool reports whether a string value was present; an empty string is
// present and is returned as a non-nil, zero-length slice. The returned slice
// aliases the underlying buffer.
func (kv KeyValue) StringValue() ([]byte, bool, error) {
	_, _, value, err := parseKeyValue([]byte(kv))
	if err != nil || value.kind != anyValueString {
		return nil, false, err
	}
	return value.stringValue, true, nil
}

// keyValueSeq walks the repeated KeyValue field at fieldNum, yielding each
// element inline. It backs every AttributesSeq method; the field number
// differs per container (Resource 1, InstrumentationScope 3, and per
// data-point type for DataPoint). On a parse error it yields a nil KeyValue
// with a non-nil error and stops. Nothing escapes, so iterating allocates
// nothing.
func keyValueSeq(data []byte, fieldNum protowire.Number, yield func(KeyValue, error) bool) {
	forEachRepeatedField(data, fieldNum, func(rb []byte, err error) bool {
		if err != nil {
			yield(nil, err)
			return false
		}
		return yield(KeyValue(rb), nil)
	})
}

// Attributes returns an iterator over the Resource's attribute KeyValues.
// The returned function should be called after iteration to check for errors.
func (r Resource) Attributes() (iter.Seq[KeyValue], func() error) {
	return repeatedFieldSeq[KeyValue]([]byte(r), 1)
}

// AttributesSeq is a zero-allocation alternative to Attributes. It has the
// shape of an iter.Seq2[KeyValue, error] and is meant to be ranged over
// directly. On a parse error it yields a nil KeyValue with a non-nil error and
// stops.
func (r Resource) AttributesSeq(yield func(KeyValue, error) bool) {
	keyValueSeq([]byte(r), 1, yield)
}

// StringAttribute returns the string value of the first resource attribute
// named key. The bool reports whether that attribute had a string value; it
// distinguishes an absent attribute from a present-but-empty string. The
// returned slice aliases the underlying buffer.
func (r Resource) StringAttribute(key string) ([]byte, bool, error) {
	var state resourceStringAttributeState
	if err := scanResourceStringAttribute([]byte(r), key, &state); err != nil {
		return nil, false, err
	}
	return state.value, state.found, nil
}

type anyValueKind uint8

const (
	anyValueUnset anyValueKind = iota
	anyValueString
	anyValueBool
	anyValueInt
	anyValueDouble
	anyValueArray
	anyValueKeyValueList
	anyValueBytes
	anyValueStringIndex
)

type parsedAnyValue struct {
	kind        anyValueKind
	stringValue []byte
}

const semanticParseMaxDepth = 64

var errSemanticParseDepth = errors.New("protobuf semantic parse nesting limit exceeded")

// parseKeyValue parses the complete KeyValue message using pdata's singular
// field behavior: scalar key fields and AnyValue oneofs are last-value-wins,
// while repeated Value messages merge by processing their contents in order.
func parseKeyValue(data []byte) ([]byte, []byte, parsedAnyValue, error) {
	return parseKeyValueDepth(data, 0)
}

func parseKeyValueDepth(data []byte, depth int) ([]byte, []byte, parsedAnyValue, error) {
	if depth > semanticParseMaxDepth {
		return nil, nil, parsedAnyValue{}, errSemanticParseDepth
	}
	var key []byte
	var valueRaw []byte
	var value parsedAnyValue
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, nil, parsedAnyValue{}, errors.New("malformed protobuf tag in key value")
		}
		pos += tagLen

		switch fieldNum {
		case 1: // KeyValue.key
			if wireType != protowire.BytesType {
				return nil, nil, parsedAnyValue{}, errors.New("wrong wire type for key value key")
			}
			var n int
			key, n = protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, nil, parsedAnyValue{}, errors.New("invalid bytes in key value key")
			}
			pos += n
		case 2: // KeyValue.value
			if wireType != protowire.BytesType {
				return nil, nil, parsedAnyValue{}, errors.New("wrong wire type for key value value")
			}
			var n int
			valueRaw, n = protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, nil, parsedAnyValue{}, errors.New("invalid bytes in key value value")
			}
			pos += n
			if err := parseAnyValueDepth(valueRaw, &value, depth); err != nil {
				return nil, nil, parsedAnyValue{}, err
			}
		case 3: // KeyValue.key_strindex
			if wireType != protowire.VarintType {
				return nil, nil, parsedAnyValue{}, errors.New("wrong wire type for key value key string index")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return nil, nil, parsedAnyValue{}, errors.New("invalid varint in key value key string index")
			}
			pos += n
		default:
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				return nil, nil, parsedAnyValue{}, errors.New("failed to skip field in key value")
			}
			pos += n
		}
	}

	return key, valueRaw, value, nil
}

// parseAnyValue applies the AnyValue oneof in wire order. It parses every
// field, including fields superseded by a later oneof member, so malformed
// trailing data is never hidden by an earlier string value.
func parseAnyValue(data []byte, value *parsedAnyValue) error {
	return parseAnyValueDepth(data, value, 0)
}

func parseAnyValueDepth(data []byte, value *parsedAnyValue, depth int) error {
	if depth > semanticParseMaxDepth {
		return errSemanticParseDepth
	}
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in any value")
		}
		pos += tagLen

		switch fieldNum {
		case 1: // string_value
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for any value string")
			}
			stringValue, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in any value string")
			}
			value.kind = anyValueString
			value.stringValue = stringValue
			pos += n
		case 2, 3, 8: // bool_value, int_value, string_value_strindex
			if wireType != protowire.VarintType {
				return errors.New("wrong wire type for any value varint")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return errors.New("invalid varint in any value")
			}
			switch fieldNum {
			case 2:
				value.kind = anyValueBool
			case 3:
				value.kind = anyValueInt
			case 8:
				value.kind = anyValueStringIndex
			}
			value.stringValue = nil
			pos += n
		case 4: // double_value
			if wireType != protowire.Fixed64Type {
				return errors.New("wrong wire type for any value double")
			}
			_, n := protowire.ConsumeFixed64(data[pos:])
			if n < 0 {
				return errors.New("invalid fixed64 in any value double")
			}
			value.kind = anyValueDouble
			value.stringValue = nil
			pos += n
		case 5, 6, 7: // array_value, kvlist_value, bytes_value
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for any value bytes")
			}
			message, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in any value")
			}
			switch fieldNum {
			case 5:
				if err := validateArrayValueDepth(message, depth+1); err != nil {
					return err
				}
				value.kind = anyValueArray
			case 6:
				if err := validateKeyValueListDepth(message, depth+1); err != nil {
					return err
				}
				value.kind = anyValueKeyValueList
			case 7:
				value.kind = anyValueBytes
			}
			value.stringValue = nil
			pos += n
		default:
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				return errors.New("failed to skip field in any value")
			}
			pos += n
		}
	}
	return nil
}

func validateArrayValueDepth(data []byte, depth int) error {
	if depth > semanticParseMaxDepth {
		return errSemanticParseDepth
	}
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in array value")
		}
		pos += tagLen
		if fieldNum == 1 {
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for array value values")
			}
			item, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in array value values")
			}
			if err := parseAnyValueDepth(item, &parsedAnyValue{}, depth); err != nil {
				return err
			}
			pos += n
			continue
		}
		n := skipField(data[pos:], fieldNum, wireType)
		if n < 0 {
			return errors.New("failed to skip field in array value")
		}
		pos += n
	}
	return nil
}

func validateKeyValueListDepth(data []byte, depth int) error {
	if depth > semanticParseMaxDepth {
		return errSemanticParseDepth
	}
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in key value list")
		}
		pos += tagLen
		if fieldNum == 1 {
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for key value list values")
			}
			item, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in key value list values")
			}
			if _, _, _, err := parseKeyValueDepth(item, depth); err != nil {
				return err
			}
			pos += n
			continue
		}
		n := skipField(data[pos:], fieldNum, wireType)
		if n < 0 {
			return errors.New("failed to skip field in key value list")
		}
		pos += n
	}
	return nil
}

type resourceStringAttributeState struct {
	value   []byte
	found   bool
	matched bool
}

// scanResourceStringAttribute validates a complete Resource message and
// records the first matching attribute only. pdata's resource map keeps that
// first duplicate key. When the caller's Resource value came from merging 2+
// wire occurrences (see extractMergedMessage), the merged bytes already
// hold every occurrence's attributes concatenated in wire order, so this
// single pass reproduces the merge's first-value-wins behavior without any
// extra merge-aware logic here.
func scanResourceStringAttribute(data []byte, key string, state *resourceStringAttributeState) error {
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in resource")
		}
		pos += tagLen

		switch fieldNum {
		case 1: // Resource.attributes
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for resource attributes")
			}
			attribute, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in resource attributes")
			}
			attributeKey, _, anyValue, err := parseKeyValue(attribute)
			if err != nil {
				return err
			}
			if !state.matched && equalBytesString(attributeKey, key) {
				state.matched = true
				state.found = anyValue.kind == anyValueString
				state.value = nil
				if state.found {
					state.value = anyValue.stringValue
				}
			}
			pos += n
		case 2: // Resource.dropped_attributes_count
			if wireType != protowire.VarintType {
				return errors.New("wrong wire type for resource dropped attributes count")
			}
			_, n := protowire.ConsumeVarint(data[pos:])
			if n < 0 {
				return errors.New("invalid varint in resource dropped attributes count")
			}
			pos += n
		case 3: // Resource.entity_refs
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for resource entity refs")
			}
			entityRef, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in resource entity refs")
			}
			if err := validateEntityRef(entityRef); err != nil {
				return err
			}
			pos += n
		default:
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				return errors.New("failed to skip field in resource")
			}
			pos += n
		}
	}
	return nil
}

// validateEntityRef matches pdata v1.64.0's EntityRef wire contract. Its four
// known fields (schema_url, type, id_keys, and description_keys) are all
// length-delimited strings; unknown fields remain forward-compatible and are
// skipped with full group validation.
func validateEntityRef(data []byte) error {
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in entity reference")
		}
		pos += tagLen

		if fieldNum >= 1 && fieldNum <= 4 {
			if wireType != protowire.BytesType {
				return errors.New("wrong wire type for entity reference field")
			}
			_, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return errors.New("invalid bytes in entity reference field")
			}
			pos += n
			continue
		}

		n := skipField(data[pos:], fieldNum, wireType)
		if n < 0 {
			return errors.New("failed to skip field in entity reference")
		}
		pos += n
	}
	return nil
}

// equalBytesString reports whether b contains exactly s without converting b
// to a string or allocating a temporary byte slice for s.
func equalBytesString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}
