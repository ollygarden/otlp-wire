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

// repeatedFieldSeq2 is the allocation-free form of repeatedFieldSeq. Public
// methods with this shape are meant to be ranged over directly as method
// values, keeping the consumer's loop body on the stack.
func repeatedFieldSeq2[T ~[]byte](data []byte, fieldNum protowire.Number, yield func(T, error) bool) {
	forEachRepeatedField(data, fieldNum, func(item []byte, err error) bool {
		if err != nil {
			yield(nil, err)
			return false
		}
		return yield(T(item), nil)
	})
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
//
// Yielded slices have their capacity clamped to their length. ConsumeBytes
// hands back a slice whose capacity runs to the end of the enclosing message,
// so an unclamped view would let a caller's append overwrite the sibling
// fields that follow it in a buffer the library does not own. Every accessor
// built on this helper inherits the guarantee.
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

			if !fn(msgBytes[:len(msgBytes):len(msgBytes)], nil) {
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

// validateMessageFraming reports whether data is structurally well-formed:
// every tag parses, every value is contained, and the walk ends exactly at the
// end. Structure only, no semantics.
func validateMessageFraming(data []byte) error {
	for pos := 0; pos < len(data); {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return errors.New("malformed protobuf tag in message occurrence")
		}
		pos += tagLen

		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return errors.New("malformed field in message occurrence")
		}
		pos += n
	}
	return nil
}

// extractMergedMessage returns the singular embedded message at fieldNum,
// merging repeated occurrences the way protobuf and pdata do. It backs both
// the Resource field (field 1) of a ResourceMetrics/ResourceLogs/ResourceSpans
// and the InstrumentationScope field (field 1) of a
// ScopeMetrics/ScopeLogs/ScopeSpans: pdata unmarshals every occurrence of each
// into the same struct, so both accumulate rather than replace.
//
// An absent field returns (nil, nil); malformed framing returns an error. One
// occurrence aliases data; two or more are each validated, then concatenated
// into a new buffer. Locating every occurrence requires scanning the whole
// container, so a malformed field after the last occurrence is an error too,
// and the cost grows with the container's field count instead of being
// constant.
//
// docs/DESIGN.md covers why concatenation is a correct merge and why each
// occurrence is validated first; docs/BENCHMARKS.md has the scan cost.
func extractMergedMessage(data []byte, fieldNum protowire.Number) ([]byte, error) {
	var first []byte
	var found bool
	var rest [][]byte
	pos := 0

	for pos < len(data) {
		num, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return nil, errors.New("malformed protobuf tag")
		}
		pos += tagLen

		if num == fieldNum {
			if wireType != protowire.BytesType {
				return nil, errors.New("embedded message field has wrong wire type")
			}
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return nil, errors.New("invalid bytes in embedded message field")
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
		n := skipField(data[pos:], num, wireType)
		if n < 0 {
			return nil, errors.New("failed to skip field")
		}
		pos += n
	}

	if !found {
		return nil, nil
	}
	if len(rest) == 0 {
		// Aliases data. Capacity is clamped so a caller's append reallocates:
		// ConsumeBytes hands back a slice whose capacity runs to the end of
		// the container, which would otherwise let append overwrite the
		// sibling scope field.
		return first[:len(first):len(first)], nil
	}

	// Concatenation must not manufacture validity. A message spliced across
	// two occurrences, neither parseable alone, would otherwise reassemble
	// into a valid message that pdata rejects.
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

// extractLastBytesField extracts a singular length-delimited scalar field,
// resolving repeated occurrences by last-value-wins the way protobuf and
// pdata do. Returns nil (not an error) if absent; the returned slice aliases
// data.
//
// Reaching the last occurrence means walking the whole message, so unlike
// extractBytesField this reports a malformed field after that occurrence.
// docs/DESIGN.md records why scalars resolve differently from the messages
// extractMergedMessage merges.
func extractLastBytesField(data []byte, fieldNum protowire.Number) ([]byte, error) {
	var last []byte
	var walkErr error

	forEachRepeatedField(data, fieldNum, func(item []byte, err error) bool {
		if err != nil {
			walkErr = err
			return false
		}
		last = item
		return true
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return last, nil
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
