package otlpwire

import (
	"iter"
)

// Name returns the scope name (field 1) as a view into the underlying buffer.
// Returns nil if the field is not present. Repeated occurrences resolve to
// the last one; see extractLastBytesField.
//
// Validation is structural and operation-scoped: the whole scope message is
// walked, so trailing corruption is reported, but nested attribute values are
// only parsed when Attributes is iterated.
func (s InstrumentationScope) Name() ([]byte, error) {
	return extractLastBytesField([]byte(s), 1)
}

// Version returns the scope version (field 2) as a view into the underlying
// buffer. Returns nil if the field is not present. Repeated occurrences
// resolve to the last one, as for Name.
func (s InstrumentationScope) Version() ([]byte, error) {
	return extractLastBytesField([]byte(s), 2)
}

// Attributes returns an iterator over the scope's attribute KeyValues.
// The returned function should be called after iteration to check for errors.
//
// Scope attributes are field 3, unlike Resource attributes, which are field 1.
func (s InstrumentationScope) Attributes() (iter.Seq[KeyValue], func() error) {
	return repeatedFieldSeq[KeyValue]([]byte(s), 3)
}

// AttributesSeq is a zero-allocation alternative to Attributes. It has the
// shape of an iter.Seq2[KeyValue, error] and is meant to be ranged over
// directly. On a parse error it yields a nil KeyValue with a non-nil error and
// stops.
func (s InstrumentationScope) AttributesSeq(yield func(KeyValue, error) bool) {
	repeatedFieldSeq2([]byte(s), 3, yield)
}
