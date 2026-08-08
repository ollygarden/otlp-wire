package otlpwire

import (
	"errors"
	"io"
	"iter"

	"google.golang.org/protobuf/encoding/protowire"
)

// Raw returns the raw datapoint message bytes.
func (d DataPoint) Raw() []byte { return d.raw }

// Type returns the metric type this datapoint came from.
func (d DataPoint) Type() MetricType { return d.typ }

// attributesFieldNum returns the field number of the repeated KeyValue
// attributes for each datapoint message type.
func (d DataPoint) attributesFieldNum() protowire.Number {
	switch d.typ {
	case MetricTypeHistogram:
		return 9
	case MetricTypeExponentialHistogram:
		return 1
	default: // NumberDataPoint (gauge, sum) and SummaryDataPoint
		return 7
	}
}

// Timestamp returns the datapoint's time_unix_nano (field 3, fixed64).
// Returns 0 if the field is not present.
func (d DataPoint) Timestamp() (uint64, error) {
	return extractFixed64Field(d.raw, 3)
}

// Attributes returns an iterator over the datapoint's attribute KeyValues.
// The returned function should be called after iteration to check for errors.
func (d DataPoint) Attributes() (iter.Seq[KeyValue], func() error) {
	return repeatedFieldSeq[KeyValue](d.raw, d.attributesFieldNum())
}

// AttributesSeq is a zero-allocation alternative to Attributes. It has the
// shape of an iter.Seq2[KeyValue, error] and is meant to be ranged over
// directly:
//
//	for kv, err := range dp.AttributesSeq {
//		if err != nil { ... }
//	}
//
// On a parse error it yields a nil KeyValue with a non-nil error and stops.
func (d DataPoint) AttributesSeq(yield func(KeyValue, error) bool) {
	keyValueSeq(d.raw, d.attributesFieldNum(), yield)
}

// DataPointCount returns the total number of metric data points in the batch.
func (m ExportMetricsServiceRequest) DataPointCount() (int, error) {
	return countMetricDataPoints([]byte(m))
}

// ResourceMetrics returns an iterator over ResourceMetrics in the batch.
// The returned function should be called after iteration to check for errors.
func (m ExportMetricsServiceRequest) ResourceMetrics() (iter.Seq[ResourceMetrics], func() error) {
	return repeatedFieldSeq[ResourceMetrics]([]byte(m), 1)
}

// DataPointCount returns the number of metric data points in this resource.
func (r ResourceMetrics) DataPointCount() (int, error) {
	return countInResourceMetrics([]byte(r))
}

// Resource returns the Resource message for this ResourceMetrics. It returns
// (nil, nil) when the field is absent, aliases the input for the single
// occurrence every real producer emits, and merges 2+ occurrences into a new
// buffer. See extractMergedMessage for the full contract.
func (r ResourceMetrics) Resource() (Resource, error) {
	raw, err := extractMergedMessage([]byte(r), 1)
	if err != nil {
		return nil, err
	}
	return Resource(raw), nil
}

// SchemaUrl returns the ResourceMetrics schema_url (field 3) as a view into
// the underlying buffer. Returns nil if the field is not present. Repeated
// occurrences resolve to the last one.
func (r ResourceMetrics) SchemaUrl() ([]byte, error) {
	return extractLastBytesField([]byte(r), 3)
}

// WriteTo writes the ResourceMetrics as a valid ExportMetricsServiceRequest to w.
// Implements io.WriterTo interface.
func (r ResourceMetrics) WriteTo(w io.Writer) (int64, error) {
	return writeResourceMessage(w, []byte(r))
}

// ScopeMetrics returns an iterator over ScopeMetrics in this ResourceMetrics.
// Field 2 in the ResourceMetrics protobuf message.
// The returned function should be called after iteration to check for errors.
func (r ResourceMetrics) ScopeMetrics() (iter.Seq[ScopeMetrics], func() error) {
	return repeatedFieldSeq[ScopeMetrics]([]byte(r), 2)
}

// Scope returns the InstrumentationScope for this ScopeMetrics. It returns
// (nil, nil) when the field is absent, aliases the input for the single
// occurrence every real producer emits, and merges 2+ occurrences into a new
// buffer. See extractMergedMessage for the full contract.
func (s ScopeMetrics) Scope() (InstrumentationScope, error) {
	raw, err := extractMergedMessage([]byte(s), 1)
	if err != nil {
		return nil, err
	}
	return InstrumentationScope(raw), nil
}

// SchemaUrl returns the ScopeMetrics schema_url (field 3) as a view into the
// underlying buffer. Returns nil if the field is not present. Repeated
// occurrences resolve to the last one.
func (s ScopeMetrics) SchemaUrl() ([]byte, error) {
	return extractLastBytesField([]byte(s), 3)
}

// Metrics returns an iterator over Metrics in this ScopeMetrics.
// Field 2 in the ScopeMetrics protobuf message.
// The returned function should be called after iteration to check for errors.
func (s ScopeMetrics) Metrics() (iter.Seq[Metric], func() error) {
	return repeatedFieldSeq[Metric]([]byte(s), 2)
}

// Name returns the metric name (field 1) as a view into the underlying
// buffer. Returns nil if the field is not present. Repeated occurrences
// resolve to the last one; see extractLastBytesField.
func (m Metric) Name() ([]byte, error) {
	return extractLastBytesField([]byte(m), 1)
}

// DataPoints returns an iterator over datapoints in this Metric, descending
// whichever oneof body is present (gauge 5, sum 7, histogram 9,
// exponential_histogram 10, summary 11). Each body holds its datapoints in
// field 1. If a malformed metric carries more than one oneof body,
// datapoints from each are yielded, each tagged with its own type.
// The returned function should be called after iteration to check for errors.
// DataPoints is a thin adapter over DataPointsSeq.
func (m Metric) DataPoints() (iter.Seq[DataPoint], func() error) {
	var iterErr error

	seq := func(yield func(DataPoint) bool) {
		m.DataPointsSeq(func(dp DataPoint, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			return yield(dp)
		})
	}

	errFunc := func() error {
		return iterErr
	}

	return seq, errFunc
}

// DataPointsSeq is a zero-allocation alternative to DataPoints. It has the
// shape of an iter.Seq2[DataPoint, error] and is meant to be ranged over
// directly:
//
//	for dp, err := range m.DataPointsSeq {
//		if err != nil { ... }
//	}
//
// On a parse error it yields a zero DataPoint with a non-nil error and
// stops. Unlike DataPoints, no closures escape, so iterating allocates
// nothing. If a malformed metric carries more than one oneof body,
// datapoints from each are yielded, each tagged with its own type.
func (m Metric) DataPointsSeq(yield func(DataPoint, error) bool) {
	data := []byte(m)
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			yield(DataPoint{}, errors.New("malformed protobuf tag in metric"))
			return
		}
		pos += tagLen

		typ := MetricType(fieldNum)
		isBody := typ == MetricTypeGauge || typ == MetricTypeSum ||
			typ == MetricTypeHistogram || typ == MetricTypeExponentialHistogram ||
			typ == MetricTypeSummary
		if isBody && wireType != protowire.BytesType {
			yield(DataPoint{}, errors.New("wrong wire type for metric data"))
			return
		}
		if isBody {
			body, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				yield(DataPoint{}, errors.New("invalid bytes in metric data"))
				return
			}
			pos += n

			done := false
			forEachRepeatedField(body, 1, func(dpBytes []byte, err error) bool {
				if err != nil {
					done = true
					yield(DataPoint{}, err)
					return false
				}
				if !yield(DataPoint{raw: dpBytes, typ: typ}, nil) {
					done = true
					return false
				}
				return true
			})
			if done {
				return
			}
		} else {
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				yield(DataPoint{}, errors.New("failed to skip field"))
				return
			}
			pos += n
		}
	}
}

// countMetricDataPoints counts the number of metric data points in an OTLP
// ExportMetricsServiceRequest protobuf message without unmarshaling it.
//
// Wire format structure:
//
//	ExportMetricsServiceRequest
//	  └─ field 1: ResourceMetrics[] (repeated message)
//	      └─ field 2: ScopeMetrics[] (repeated message)
//	          └─ field 2: Metric[] (repeated message)
//	              └─ field 5: Gauge | field 7: Sum | field 9: Histogram | etc.
//	                  └─ field 1: DataPoints[] (repeated message) ← count these
func countMetricDataPoints(data []byte) (int, error) {
	return countRepeatedField(data, 1, countInResourceMetrics)
}

func countInResourceMetrics(data []byte) (int, error) {
	return countRepeatedField(data, 2, countInScopeMetrics)
}

func countInScopeMetrics(data []byte) (int, error) {
	return countRepeatedField(data, 2, countInMetric)
}

func countInMetric(data []byte) (int, error) {
	count := 0
	pos := 0

	for pos < len(data) {
		fieldNum, wireType, tagLen := protowire.ConsumeTag(data[pos:])
		if tagLen < 0 {
			return 0, errors.New("malformed protobuf tag in metric")
		}
		pos += tagLen

		// Metric types: field 5=Gauge, 7=Sum, 9=Histogram, 10=ExponentialHistogram, 11=Summary
		isBody := fieldNum == 5 || fieldNum == 7 || fieldNum == 9 || fieldNum == 10 || fieldNum == 11
		if isBody && wireType != protowire.BytesType {
			return 0, errors.New("wrong wire type for metric data")
		}
		if isBody {
			msgBytes, n := protowire.ConsumeBytes(data[pos:])
			if n < 0 {
				return 0, errors.New("invalid bytes in metric data")
			}
			pos += n

			c, err := countDataPoints(msgBytes)
			if err != nil {
				return 0, err
			}
			count += c
		} else {
			n := skipField(data[pos:], fieldNum, wireType)
			if n < 0 {
				return 0, errors.New("failed to skip field")
			}
			pos += n
		}
	}

	return count, nil
}

func countDataPoints(data []byte) (int, error) {
	return countOccurrences(data, 1)
}
