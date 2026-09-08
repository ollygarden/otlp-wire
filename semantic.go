package otlpwire

import (
	"errors"
	"iter"

	"google.golang.org/protobuf/encoding/protowire"
)

// Semantic applies protobuf/pdata singular and oneof resolution and validates
// the complete KeyValue and recursively selected/superseded AnyValues.
func (kv KeyValue) Semantic() (SemanticKeyValue, error) {
	return parseSemanticKeyValueDepth([]byte(kv), 0)
}

func parseSemanticKeyValueDepth(data []byte, depth int) (SemanticKeyValue, error) {
	key, _, _, err := parseKeyValueDepth(data, depth)
	if err != nil {
		return SemanticKeyValue{}, err
	}
	var value AnyValue
	if err := parseAnyValueSemanticField(data, &value, depth); err != nil {
		return SemanticKeyValue{}, err
	}
	return SemanticKeyValue{Key: key, Value: value}, nil
}

func parseAnyValueSemanticField(data []byte, out *AnyValue, depth int) error {
	return walk(data, func(n protowire.Number, typ protowire.Type, value []byte, scalar uint64) error {
		if n != 2 {
			return nil
		}
		if typ != protowire.BytesType {
			return errors.New("wrong wire type for key value value")
		}
		return parseSemanticAnyValue(value, out, depth)
	})
}

func parseSemanticAnyValue(data []byte, out *AnyValue, depth int) error {
	if depth > semanticParseMaxDepth {
		return errSemanticParseDepth
	}
	return walk(data, func(n protowire.Number, typ protowire.Type, b []byte, scalar uint64) error {
		switch n {
		case 1:
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for AnyValue string")
			}
			*out = AnyValue{Type: AnyValueString, String: b}
		case 2:
			if typ != protowire.VarintType {
				return errors.New("wrong wire type for AnyValue bool")
			}
			*out = AnyValue{Type: AnyValueBool, Bool: scalar != 0}
		case 3:
			if typ != protowire.VarintType {
				return errors.New("wrong wire type for AnyValue int")
			}
			*out = AnyValue{Type: AnyValueInt, Int: int64(scalar)}
		case 4:
			if typ != protowire.Fixed64Type {
				return errors.New("wrong wire type for AnyValue double")
			}
			*out = AnyValue{Type: AnyValueDouble, Float64Bits: scalar}
		case 5:
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for AnyValue array")
			}
			if err := validateArraySemantic(b, depth+1); err != nil {
				return err
			}
			if out.Type == AnyValueArray && out.array != nil {
				b = appendMerged(out.array, b)
			}
			*out = AnyValue{Type: AnyValueArray, array: b, depth: depth}
		case 6:
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for AnyValue kvlist")
			}
			if err := validateKVListSemantic(b, depth+1); err != nil {
				return err
			}
			if out.Type == AnyValueKeyValueList && out.kvlist != nil {
				b = appendMerged(out.kvlist, b)
			}
			*out = AnyValue{Type: AnyValueKeyValueList, kvlist: b, depth: depth}
		case 7:
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for AnyValue bytes")
			}
			*out = AnyValue{Type: AnyValueBytes, Bytes: b}
		case 8:
			if typ != protowire.VarintType {
				return errors.New("wrong wire type for AnyValue string index")
			}
			*out = AnyValue{}
		}
		return nil
	})
}

func validateArraySemantic(data []byte, depth int) error {
	return walk(data, func(n protowire.Number, typ protowire.Type, b []byte, _ uint64) error {
		if n != 1 {
			return nil
		}
		if typ != protowire.BytesType {
			return errors.New("wrong wire type for array value")
		}
		return parseSemanticAnyValue(b, &AnyValue{}, depth)
	})
}
func validateKVListSemantic(data []byte, depth int) error {
	return walk(data, func(n protowire.Number, typ protowire.Type, b []byte, _ uint64) error {
		if n != 1 {
			return nil
		}
		if typ != protowire.BytesType {
			return errors.New("wrong wire type for kvlist value")
		}
		_, err := parseSemanticKeyValueDepth(b, depth)
		return err
	})
}

// Values returns array elements in wire order. The error closure must be
// checked, including after early termination.
func (v AnyValue) Values() (iter.Seq[AnyValue], func() error) {
	var final error
	seq := func(yield func(AnyValue) bool) {
		if v.Type != AnyValueArray {
			return
		}
		final = walk(v.array, func(n protowire.Number, typ protowire.Type, b []byte, _ uint64) error {
			if n != 1 {
				return nil
			}
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for array value")
			}
			var item AnyValue
			if err := parseSemanticAnyValue(b, &item, v.depth+1); err != nil {
				return err
			}
			if !yield(item) {
				return errStop
			}
			return nil
		})
		if errors.Is(final, errStop) {
			final = nil
		}
	}
	return seq, func() error { return final }
}

// KeyValues returns kvlist entries in wire order, preserving duplicate keys.
func (v AnyValue) KeyValues() (iter.Seq[SemanticKeyValue], func() error) {
	var final error
	seq := func(yield func(SemanticKeyValue) bool) {
		if v.Type != AnyValueKeyValueList {
			return
		}
		final = walk(v.kvlist, func(n protowire.Number, typ protowire.Type, b []byte, _ uint64) error {
			if n != 1 {
				return nil
			}
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for kvlist value")
			}
			kv, err := parseSemanticKeyValueDepth(b, v.depth+1)
			if err != nil {
				return err
			}
			if !yield(kv) {
				return errStop
			}
			return nil
		})
		if errors.Is(final, errStop) {
			final = nil
		}
	}
	return seq, func() error { return final }
}

var errStop = errors.New("iteration stopped")

func appendMerged(first, second []byte) []byte {
	merged := make([]byte, 0, len(first)+len(second))
	merged = append(merged, first...)
	return append(merged, second...)
}

// Semantic parses the complete Metric, applies last-one-wins scalar and oneof
// resolution, and validates the selected body and all of its datapoints.
func (m Metric) Semantic() (SemanticMetric, error) {
	var out SemanticMetric
	err := walk([]byte(m), func(n protowire.Number, typ protowire.Type, b []byte, _ uint64) error {
		switch n {
		case 1, 2, 3:
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for metric text")
			}
			if n == 1 {
				out.Name = b
			} else if n == 2 {
				out.Description = b
			} else {
				out.Unit = b
			}
		case 5, 7, 9, 10, 11:
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for metric body")
			}
			kind := MetricType(n)
			// Generated protobuf unmarshalling parses every encountered oneof
			// message before a later member can supersede it. Validate each body
			// now, then parse the resolved (and possibly merged) body below.
			if err := parseMetricBody(b, &SemanticMetric{Kind: kind}, nil); err != nil {
				return err
			}
			if out.Kind == kind && out.body != nil {
				out.body = appendMerged(out.body, b)
			} else {
				out.Kind = kind
				out.body = b
			}
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if out.Kind != 0 {
		if err := parseMetricBody(out.body, &out, nil); err != nil {
			return SemanticMetric{}, err
		}
	}
	return out, nil
}

// DataPoints returns fully validated semantic datapoints from the selected
// oneof body. Metric.Semantic prevalidates these, so iteration errors are only
// possible if the aliased source bytes are mutated afterward.
func (m SemanticMetric) DataPoints() (iter.Seq[SemanticDataPoint], func() error) {
	var final error
	seq := func(yield func(SemanticDataPoint) bool) {
		final = parseMetricBody(m.body, &m, func(dp SemanticDataPoint) bool { return yield(dp) })
		if errors.Is(final, errStop) {
			final = nil
		}
	}
	return seq, func() error { return final }
}

func parseMetricBody(data []byte, metric *SemanticMetric, yield func(SemanticDataPoint) bool) error {
	return walk(data, func(n protowire.Number, typ protowire.Type, b []byte, scalar uint64) error {
		if n == 1 {
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for datapoint")
			}
			dp, err := parseSemanticDataPoint(b, metric.Kind)
			if err != nil {
				return err
			}
			if yield != nil && !yield(dp) {
				return errStop
			}
			return nil
		}
		if n == 2 && (metric.Kind == MetricTypeSum || metric.Kind == MetricTypeHistogram || metric.Kind == MetricTypeExponentialHistogram) {
			if typ != protowire.VarintType {
				return errors.New("wrong wire type for aggregation temporality")
			}
			metric.AggregationTemporality = int32(scalar)
		}
		if n == 3 && metric.Kind == MetricTypeSum {
			if typ != protowire.VarintType {
				return errors.New("wrong wire type for monotonic")
			}
			metric.Monotonic = scalar != 0
		}
		return nil
	})
}

func parseSemanticDataPoint(data []byte, kind MetricType) (SemanticDataPoint, error) {
	d := SemanticDataPoint{Kind: kind}
	attrs := protowire.Number(7)
	if kind == MetricTypeHistogram {
		attrs = 9
	}
	if kind == MetricTypeExponentialHistogram {
		attrs = 1
	}
	err := walk(data, func(n protowire.Number, typ protowire.Type, b []byte, scalar uint64) error {
		if n == attrs {
			if typ != protowire.BytesType {
				return errors.New("wrong wire type for datapoint attribute")
			}
			kv, e := KeyValue(b).Semantic()
			if e != nil {
				return e
			}
			d.Attributes = append(d.Attributes, kv)
			return nil
		}
		if n == 2 {
			if typ != protowire.Fixed64Type {
				return errors.New("wrong wire type for start timestamp")
			}
			d.StartTimestamp = scalar
			return nil
		}
		if n == 3 {
			if typ != protowire.Fixed64Type {
				return errors.New("wrong wire type for timestamp")
			}
			d.Timestamp = scalar
			return nil
		}
		switch kind {
		case MetricTypeGauge, MetricTypeSum:
			if n == 4 {
				if typ != protowire.Fixed64Type {
					return errors.New("wrong wire type for double value")
				}
				d.NumberType = NumberValueDouble
				d.NumberDoubleBits = scalar
			}
			if n == 6 {
				if typ != protowire.Fixed64Type {
					return errors.New("wrong wire type for int value")
				}
				d.NumberType = NumberValueInt
				d.NumberInt = int64(scalar)
			}
			if n == 8 {
				if typ != protowire.VarintType {
					return errors.New("wrong wire type for flags")
				}
				d.Flags = uint32(scalar)
			}
		case MetricTypeHistogram:
			return parseHistogramField(&d, n, typ, b, scalar)
		case MetricTypeExponentialHistogram:
			return parseExpField(&d, n, typ, b, scalar)
		case MetricTypeSummary:
			return parseSummaryField(&d, n, typ, b, scalar)
		}
		return nil
	})
	return d, err
}

func parseHistogramField(d *SemanticDataPoint, n protowire.Number, t protowire.Type, b []byte, s uint64) error {
	switch n {
	case 4:
		if t != protowire.Fixed64Type {
			return errors.New("wrong histogram count")
		}
		d.Count = s
	case 5, 11, 12:
		if t != protowire.Fixed64Type {
			return errors.New("wrong histogram optional")
		}
		o := OptionalFloat64{true, s}
		if n == 5 {
			d.Sum = o
		} else if n == 11 {
			d.Min = o
		} else {
			d.Max = o
		}
	case 6:
		return appendRepeated(&d.BucketCounts, t, b, s, protowire.Fixed64Type)
	case 7:
		return appendRepeated(&d.ExplicitBoundsBits, t, b, s, protowire.Fixed64Type)
	case 10:
		if t != protowire.VarintType {
			return errors.New("wrong histogram flags")
		}
		d.Flags = uint32(s)
	}
	return nil
}
func parseExpField(d *SemanticDataPoint, n protowire.Number, t protowire.Type, b []byte, s uint64) error {
	switch n {
	case 4:
		if t != protowire.Fixed64Type {
			return errors.New("wrong exponential count")
		}
		d.Count = s
	case 5, 12, 13:
		if t != protowire.Fixed64Type {
			return errors.New("wrong exponential optional")
		}
		o := OptionalFloat64{true, s}
		if n == 5 {
			d.Sum = o
		} else if n == 12 {
			d.Min = o
		} else {
			d.Max = o
		}
	case 6:
		if t != protowire.VarintType {
			return errors.New("wrong exponential scale")
		}
		d.Scale = int32(protowire.DecodeZigZag(s))
	case 7:
		if t != protowire.Fixed64Type {
			return errors.New("wrong exponential zero count")
		}
		d.ZeroCount = s
	case 8, 9:
		if t != protowire.BytesType {
			return errors.New("wrong exponential buckets")
		}
		if n == 8 {
			return parseBuckets(b, &d.Positive)
		} else {
			return parseBuckets(b, &d.Negative)
		}
	case 10:
		if t != protowire.VarintType {
			return errors.New("wrong exponential flags")
		}
		d.Flags = uint32(s)
	case 14:
		if t != protowire.Fixed64Type {
			return errors.New("wrong zero threshold")
		}
		d.ZeroThresholdBits = s
	}
	return nil
}
func parseBuckets(data []byte, x *ExponentialHistogramBuckets) error {
	err := walk(data, func(n protowire.Number, t protowire.Type, b []byte, s uint64) error {
		if n == 1 {
			if t != protowire.VarintType {
				return errors.New("wrong bucket offset")
			}
			x.Offset = int32(protowire.DecodeZigZag(s))
		}
		if n == 2 {
			return appendRepeated(&x.BucketCounts, t, b, s, protowire.VarintType)
		}
		return nil
	})
	return err
}
func parseSummaryField(d *SemanticDataPoint, n protowire.Number, t protowire.Type, b []byte, s uint64) error {
	switch n {
	case 4:
		if t != protowire.Fixed64Type {
			return errors.New("wrong summary count")
		}
		d.Count = s
	case 5:
		if t != protowire.Fixed64Type {
			return errors.New("wrong summary sum")
		}
		d.SummarySumBits = s
	case 6:
		if t != protowire.BytesType {
			return errors.New("wrong quantile")
		}
		var q QuantileValue
		if err := walk(b, func(fn protowire.Number, ft protowire.Type, _ []byte, v uint64) error {
			if (fn == 1 || fn == 2) && ft != protowire.Fixed64Type {
				return errors.New("wrong quantile value")
			}
			if fn == 1 {
				q.QuantileBits = v
			}
			if fn == 2 {
				q.ValueBits = v
			}
			return nil
		}); err != nil {
			return err
		}
		d.Quantiles = append(d.Quantiles, q)
	case 8:
		if t != protowire.VarintType {
			return errors.New("wrong summary flags")
		}
		d.Flags = uint32(s)
	}
	return nil
}

func appendRepeated(dst *[]uint64, t protowire.Type, b []byte, s uint64, element protowire.Type) error {
	if t == element {
		*dst = append(*dst, s)
		return nil
	}
	if t != protowire.BytesType {
		return errors.New("wrong wire type for repeated primitive")
	}
	for len(b) > 0 {
		var v uint64
		var n int
		if element == protowire.VarintType {
			v, n = protowire.ConsumeVarint(b)
		} else {
			v, n = protowire.ConsumeFixed64(b)
		}
		if n < 0 {
			return errors.New("malformed packed primitive")
		}
		*dst = append(*dst, v)
		b = b[n:]
	}
	return nil
}

// ValidateSemantic validates every field exposed by semantic metrics traversal
// throughout the request before a stateful consumer begins mutation.
func (r ExportMetricsServiceRequest) ValidateSemantic() error {
	return walk([]byte(r), func(n protowire.Number, t protowire.Type, b []byte, _ uint64) error {
		if n != 1 {
			return nil
		}
		if t != protowire.BytesType {
			return errors.New("wrong resource metrics")
		}
		return validateResourceMetricsSemantic(b)
	})
}
func validateResourceMetricsSemantic(data []byte) error {
	return walk(data, func(n protowire.Number, t protowire.Type, b []byte, _ uint64) error {
		if n == 1 {
			if t != protowire.BytesType {
				return errors.New("wrong resource")
			}
			return validateResourceSemantic(b)
		}
		if n == 2 {
			if t != protowire.BytesType {
				return errors.New("wrong scope metrics")
			}
			return validateScopeMetricsSemantic(b)
		}
		if n == 3 && t != protowire.BytesType {
			return errors.New("wrong resource metrics schema URL")
		}
		return nil
	})
}
func validateResourceSemantic(data []byte) error {
	return walk(data, func(n protowire.Number, t protowire.Type, b []byte, _ uint64) error {
		if n == 1 {
			if t != protowire.BytesType {
				return errors.New("wrong resource attribute")
			}
			_, err := KeyValue(b).Semantic()
			return err
		}
		if n == 2 && t != protowire.VarintType {
			return errors.New("wrong resource dropped attributes count")
		}
		return nil
	})
}
func validateScopeMetricsSemantic(data []byte) error {
	return walk(data, func(n protowire.Number, t protowire.Type, b []byte, _ uint64) error {
		if n == 1 {
			if t != protowire.BytesType {
				return errors.New("wrong scope")
			}
			return validateScopeSemantic(b)
		}
		if n == 2 {
			if t != protowire.BytesType {
				return errors.New("wrong metric")
			}
			_, e := Metric(b).Semantic()
			return e
		}
		if n == 3 && t != protowire.BytesType {
			return errors.New("wrong scope metrics schema URL")
		}
		return nil
	})
}
func validateScopeSemantic(data []byte) error {
	return walk(data, func(n protowire.Number, t protowire.Type, b []byte, _ uint64) error {
		if n == 1 || n == 2 {
			if t != protowire.BytesType {
				return errors.New("wrong instrumentation scope text")
			}
		}
		if n == 3 {
			if t != protowire.BytesType {
				return errors.New("wrong instrumentation scope attribute")
			}
			_, err := KeyValue(b).Semantic()
			return err
		}
		if n == 4 && t != protowire.VarintType {
			return errors.New("wrong instrumentation scope dropped attributes count")
		}
		return nil
	})
}

// walk validates complete protobuf framing and invokes fn for each field.
func walk(data []byte, fn func(protowire.Number, protowire.Type, []byte, uint64) error) error {
	for len(data) > 0 {
		num, t, n := protowire.ConsumeTag(data)
		if n < 0 {
			return errors.New("malformed protobuf tag")
		}
		data = data[n:]
		var b []byte
		var scalar uint64
		var m int
		switch t {
		case protowire.VarintType:
			scalar, m = protowire.ConsumeVarint(data)
		case protowire.Fixed32Type:
			var v uint32
			v, m = protowire.ConsumeFixed32(data)
			scalar = uint64(v)
		case protowire.Fixed64Type:
			scalar, m = protowire.ConsumeFixed64(data)
		case protowire.BytesType:
			b, m = protowire.ConsumeBytes(data)
		default:
			m = skipField(data, num, t)
		}
		if m < 0 {
			return errors.New("malformed protobuf field")
		}
		if b != nil {
			b = b[:len(b):len(b)]
		}
		if err := fn(num, t, b, scalar); err != nil {
			return err
		}
		data = data[m:]
	}
	return nil
}
