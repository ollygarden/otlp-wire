package otlpwire

import (
	"errors"
	"io"
	"iter"

	"google.golang.org/protobuf/encoding/protowire"
)

func repeatedFieldSeq[T ~[]byte](data []byte, fieldNum protowire.Number) (iter.Seq[T], func() error) {
	var iterErr error
	seq := func(yield func(T) bool) {
		forEachRepeatedField(data, fieldNum, func(item []byte, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(T(item))
		})
	}
	return seq, func() error { return iterErr }
}

// skipField skips a field value after its tag has been consumed. The field
// number is required for groups so ConsumeFieldValue can verify the matching
// end-group marker and recursively validate its contents.
func skipField(data []byte, fieldNum protowire.Number, wireType protowire.Type) int {
	return protowire.ConsumeFieldValue(fieldNum, wireType, data)
}

// countRepeatedField counts items in a repeated field by delegating to countFunc
// for each occurrence of the specified field.
func countRepeatedField(data []byte, fieldNum protowire.Number, countFunc func([]byte) (int, error)) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return 0, errors.New("wrong wire type for field")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in repeated field")
			}
			pos += n

			c, err := countFunc(msgBytes)
			if err != nil {
				return 0, err
			}
			count += c
		} else {
			n := skipField(data[pos:], num, wireType)
			if n < 0 {
				return 0, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return count, nil
}

// countOccurrences counts direct occurrences of a specific field.
func countOccurrences(data []byte, fieldNum protowire.Number) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return 0, errors.New("wrong wire type for field")
			}
			_, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in field")
			}
			pos += n
			count++
		} else {
			n := skipField(data[pos:], num, wireType)
			if n < 0 {
				return 0, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return count, nil
}

// forEachRepeatedField iterates over a repeated field, calling fn for each occurrence.
// The callback receives field bytes or an error. Return false to stop iteration.
func forEachRepeatedField(data []byte, fieldNum protowire.Number, fn func([]byte, error) bool) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			fn(nil, errors.New("malformed protobuf tag"))
			return
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				fn(nil, errors.New("wrong wire type for field"))
				return
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				fn(nil, errors.New("invalid bytes in repeated field"))
				return
			}
			pos += n

			if !fn(msgBytes, nil) {
				return
			}
		} else {
			n := skipField(data[pos:], num, wireType)
			if n < 0 {
				fn(nil, errors.New("failed to skip field"))
				return
			}
			pos += n
		}
	}
}

// validateMessageFraming reports whether data is a structurally well-formed
// protobuf message: every tag parses, every value lies fully inside data, and
// the walk lands exactly on the end. It does not interpret field numbers or
// validate semantics, and it allocates nothing.
func validateMessageFraming(data []byte) error {
	for pos := 0; pos < len(data); {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in resource occurrence")
		}
		pos += tagLen

		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return errors.New("malformed field in resource occurrence")
		}
		pos += n
	}
	return nil
}

// extractResourceMessage extracts and merges the Resource message (field 1)
// from ResourceMetrics/ResourceLogs/ResourceSpans messages.
//
// OTLP declares the Resource field optional: a container that omits it is
// valid and pdata reports it as an empty Resource, so absence returns
// (nil, nil) rather than an error, matching extractBytesField and
// extractFixedBytesField elsewhere in this file. A malformed occurrence
// (wrong wire type, bad length) still returns an error.
//
// Protobuf also merges repeated occurrences of a singular message field, and
// pdata performs that merge. A single occurrence is the case every real
// producer emits and returns a slice aliasing data with no extra allocation.
// Two or more occurrences require a new buffer: this function concatenates
// the encoded bodies in wire order, which has been verified to be
// byte-equivalent to protobuf's field-by-field merge for singular message
// fields (distinct keys, duplicate keys, and 3+ occurrences), so
// concatenation is correct without a general recursive merge.
//
// Validation-scope note: finding every occurrence to merge correctly means
// this function must scan the complete container instead of returning as
// soon as the first Resource field is found (the previous behavior, and the
// pattern extractBytesField/extractFixed64Field/extractFixedBytesField still
// use for their single-value fields). Consequently a malformed field located
// after the last Resource occurrence is now reported as an error, where a
// shallow single-field extractor would never have reached it. This is an
// intentional, documented expansion of validation scope for this one
// accessor; see docs/specification.md.
//
// Cost note, stated plainly because it is a complexity change and not just a
// constant: the previous implementation returned at the first Resource field,
// which in practice is the container's first field, so it was O(1) regardless
// of container size. This one is O(number of top-level fields). Each skipped
// field is still cheap — protowire.ConsumeFieldValue on a length-delimited
// field reads the length prefix and steps over the body without descending
// into it — so the walk is over the container's tags, never its payload. But
// the total grows with the scope count: measured at roughly 17ns for one
// scope entry, 88ns for ten and 412ns for fifty, against a flat ~7ns before,
// all still zero-allocation. See BenchmarkResource_ScanScaling in
// benchmark_comparison_test.go and the table in docs/BENCHMARKS.md. Scanning
// fully is the price of matching pdata, which also parses the whole message;
// there is no way to prove a second occurrence is absent without looking.
func extractResourceMessage(data []byte) ([]byte, error) {
	var first []byte
	var found bool
	var rest [][]byte
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		// Field 1 = Resource (message)
		if fieldNum == 1 {
			if wireType != protowire.BytesType {
				return nil, errors.New("resource field has wrong wire type")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in resource field")
			}
			pos += n

			if !found {
				first = msgBytes
				found = true
			} else {
				rest = append(rest, msgBytes)
			}
			continue
		}

		// Skip other fields
		n := skipField(data[pos:], fieldNum, wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	if !found {
		return nil, nil
	}
	if len(rest) == 0 {
		// Zero-copy: aliases data. The capacity is clamped to the length so a
		// caller that appends to the returned view reallocates instead of
		// writing over whatever follows the Resource in the container.
		// protowire.ConsumeBytes yields a two-index slice whose capacity runs
		// to the end of the enclosing buffer, so without this an append would
		// silently corrupt the sibling scope or schema_url field.
		return first[:len(first):len(first)], nil
	}

	// Concatenation must not manufacture validity that was not on the wire.
	// pdata parses each occurrence of a repeated singular message field on its
	// own, so a message spliced across two occurrences - the first declaring a
	// length it does not carry, the second supplying the remainder - is
	// rejected there. Concatenating first would silently reassemble it into a
	// well-formed Resource and report success, accepting a payload the pdata
	// fallback rejects. Require every occurrence to stand alone before joining
	// them.
	if err := validateMessageFraming(first); err != nil {
		return nil, err
	}
	for _, r := range rest {
		if err := validateMessageFraming(r); err != nil {
			return nil, err
		}
	}

	total := len(first)
	for _, r := range rest {
		total += len(r)
	}
	merged := make([]byte, 0, total)
	merged = append(merged, first...)
	for _, r := range rest {
		merged = append(merged, r...)
	}
	return merged, nil
}

// extractBytesField extracts the first occurrence of a length-delimited
// field from protobuf data. Returns nil (not an error) if absent.
// The returned slice aliases data; no copy is made.
func extractBytesField(data []byte, fieldNum protowire.Number) ([]byte, error) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return nil, errors.New("wrong wire type for field")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in field")
			}
			return msgBytes, nil
		}

		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	return nil, nil
}

// extractFixed64Field extracts the first occurrence of a fixed64 field from
// protobuf data. Returns 0 (not an error) if absent.
func extractFixed64Field(data []byte, fieldNum protowire.Number) (uint64, error) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.Fixed64Type {
				return 0, errors.New("wrong wire type for field")
			}
			v, n := protowire.ConsumeFixed64(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid fixed64 in field")
			}
			return v, nil
		}

		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return 0, errors.New("failed to skip field")
		}
		pos += n
	}

	return 0, nil
}

// writeResourceMessage writes resource data as a valid OTLP export request message.
// Wraps the resource bytes with field tag 1 and length prefix.
func writeResourceMessage(w io.Writer, data []byte) (int64, error) {
	buf := make([]byte, 0, 11) // tag + length varint
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(len(data)))

	n1, err := w.Write(buf)
	if err != nil {
		return int64(n1), err
	}

	n2, err := w.Write(data)
	return int64(n1 + n2), err
}

// extractFixedBytesField extracts a bytes field of known size from protobuf data.
// Returns nil (not an error) if the field is not present.
func extractFixedBytesField(data []byte, fieldNum protowire.Number, size int) ([]byte, error) {
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return nil, errors.New("wrong wire type for field")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in field")
			}
			if len(msgBytes) == 0 {
				return nil, nil // proto3 zero-value encoding, treat as absent
			}
			if len(msgBytes) != size {
				return nil, errors.New("field has unexpected size")
			}
			return msgBytes, nil
		}

		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	return nil, nil
}
