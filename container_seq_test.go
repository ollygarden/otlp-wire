package otlpwire

import (
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestContainerSeq_AdapterParity(t *testing.T) {
	first := []byte{0x08, 0x01}
	second := []byte{0x08, 0x02}
	valid := appendRepeatedMessages(nil, 1, first, second)
	malformed := append(append([]byte(nil), valid...), 0x80)

	t.Run("logs resources", func(t *testing.T) {
		assertSeqParity(t, ExportLogsServiceRequest(malformed).ResourceLogs,
			ExportLogsServiceRequest(malformed).ResourceLogsSeq, first, second)
	})
	t.Run("logs scopes", func(t *testing.T) {
		container := appendRepeatedMessages(nil, 2, first, second)
		container = append(container, 0x80)
		assertSeqParity(t, ResourceLogs(container).ScopeLogs,
			ResourceLogs(container).ScopeLogsSeq, first, second)
	})
	t.Run("metrics resources", func(t *testing.T) {
		assertSeqParity(t, ExportMetricsServiceRequest(malformed).ResourceMetrics,
			ExportMetricsServiceRequest(malformed).ResourceMetricsSeq, first, second)
	})
	t.Run("metrics scopes", func(t *testing.T) {
		container := appendRepeatedMessages(nil, 2, first, second)
		container = append(container, 0x80)
		assertSeqParity(t, ResourceMetrics(container).ScopeMetrics,
			ResourceMetrics(container).ScopeMetricsSeq, first, second)
	})
	t.Run("traces resources", func(t *testing.T) {
		assertSeqParity(t, ExportTracesServiceRequest(malformed).ResourceSpans,
			ExportTracesServiceRequest(malformed).ResourceSpansSeq, first, second)
	})
	t.Run("traces scopes", func(t *testing.T) {
		container := appendRepeatedMessages(nil, 2, first, second)
		container = append(container, 0x80)
		assertSeqParity(t, ResourceSpans(container).ScopeSpans,
			ResourceSpans(container).ScopeSpansSeq, first, second)
	})
}

func TestContainerSeq_EarlyStopLeavesLaterCorruptionUnvisited(t *testing.T) {
	first := []byte{0x08, 0x01}
	payload := appendRepeatedMessages(nil, 1, first)
	payload = append(payload, 0x80)

	yields := 0
	for resource, err := range ExportLogsServiceRequest(payload).ResourceLogsSeq {
		require.NoError(t, err)
		require.Equal(t, ResourceLogs(first), resource)
		yields++
		break
	}
	require.Equal(t, 1, yields)

	resource := appendRepeatedMessages(nil, 2, first)
	resource = append(resource, 0x80)
	yields = 0
	for scope, err := range ResourceLogs(resource).ScopeLogsSeq {
		require.NoError(t, err)
		require.Equal(t, ScopeLogs(first), scope)
		yields++
		break
	}
	require.Equal(t, 1, yields)
}

func TestContainerSeq_MalformedWire(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "selected field has wrong wire type", payload: protowire.AppendTag(nil, 1, protowire.VarintType)},
		{name: "selected field has truncated length", payload: append(protowire.AppendTag(nil, 1, protowire.BytesType), 0x02, 0x01)},
		{name: "unknown group is malformed", payload: protowire.AppendTag(nil, 7, protowire.StartGroupType)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yields := 0
			for value, err := range ExportLogsServiceRequest(tt.payload).ResourceLogsSeq {
				require.Nil(t, value)
				require.Error(t, err)
				yields++
			}
			require.Equal(t, 1, yields)
		})
	}
}

func TestContainerSeq_UnknownGroupAndCapacityClamp(t *testing.T) {
	unknown := protowire.AppendTag(nil, 7, protowire.StartGroupType)
	unknown = protowire.AppendTag(unknown, 8, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	unknown = protowire.AppendTag(unknown, 7, protowire.EndGroupType)

	first := []byte{0x08, 0x01}
	second := []byte{0x08, 0x02}
	payload := append(unknown, appendRepeatedMessages(nil, 1, first, second)...)

	var values []ResourceLogs
	for resource, err := range ExportLogsServiceRequest(payload).ResourceLogsSeq {
		require.NoError(t, err)
		require.Equal(t, len(resource), cap(resource))
		values = append(values, resource)
	}
	require.Len(t, values, 2)

	firstBefore := append([]byte(nil), values[0]...)
	_ = append(values[0], 0xff)
	require.Equal(t, firstBefore, []byte(values[0]))
	require.Equal(t, second, []byte(values[1]))
}

func TestContainerSeq_ZeroAllocations(t *testing.T) {
	resource := appendRepeatedMessages(nil, 2, []byte{0x08, 0x01})
	payload := appendRepeatedMessages(nil, 1, resource)

	allocs := testing.AllocsPerRun(1000, func() {
		for resource, err := range ExportLogsServiceRequest(payload).ResourceLogsSeq {
			if err != nil {
				panic(err)
			}
			for _, err := range resource.ScopeLogsSeq {
				if err != nil {
					panic(err)
				}
			}
		}
	})
	require.Zero(t, allocs)

	allocs = testing.AllocsPerRun(1000, func() {
		for resource, err := range ExportMetricsServiceRequest(payload).ResourceMetricsSeq {
			if err != nil {
				panic(err)
			}
			for _, err := range resource.ScopeMetricsSeq {
				if err != nil {
					panic(err)
				}
			}
		}
	})
	require.Zero(t, allocs)

	allocs = testing.AllocsPerRun(1000, func() {
		for resource, err := range ExportTracesServiceRequest(payload).ResourceSpansSeq {
			if err != nil {
				panic(err)
			}
			for _, err := range resource.ScopeSpansSeq {
				if err != nil {
					panic(err)
				}
			}
		}
	})
	require.Zero(t, allocs)
}

var containerSeqBenchmarkSink int

func BenchmarkContainerSeq(b *testing.B) {
	for _, shape := range []struct {
		name              string
		resources, scopes int
		records           int
	}{
		{name: "record-heavy", resources: 5, scopes: 2, records: 100},
		{name: "resource-heavy", resources: 20, scopes: 1, records: 5},
	} {
		payload := containerSeqBenchmarkPayload(shape.resources, shape.scopes, shape.records)

		b.Run(shape.name+"/Ordinary", func(b *testing.B) {
			for b.Loop() {
				total := 0
				resources, resourcesErr := ExportLogsServiceRequest(payload).ResourceLogs()
				for resource := range resources {
					scopes, scopesErr := resource.ScopeLogs()
					for scope := range scopes {
						for _, err := range scope.LogRecordsSeq {
							if err != nil {
								b.Fatal(err)
							}
							total++
						}
					}
					if err := scopesErr(); err != nil {
						b.Fatal(err)
					}
				}
				if err := resourcesErr(); err != nil {
					b.Fatal(err)
				}
				containerSeqBenchmarkSink = total
			}
		})

		b.Run(shape.name+"/Seq", func(b *testing.B) {
			for b.Loop() {
				total := 0
				for resource, err := range ExportLogsServiceRequest(payload).ResourceLogsSeq {
					if err != nil {
						b.Fatal(err)
					}
					for scope, err := range resource.ScopeLogsSeq {
						if err != nil {
							b.Fatal(err)
						}
						for _, err := range scope.LogRecordsSeq {
							if err != nil {
								b.Fatal(err)
							}
							total++
						}
					}
				}
				containerSeqBenchmarkSink = total
			}
		})
	}
}

func containerSeqBenchmarkPayload(resources, scopes, records int) []byte {
	record := []byte{0x08, 0x01}
	scope := make([]byte, 0, records*len(record))
	for range records {
		scope = appendRepeatedMessages(scope, 2, record)
	}
	resource := make([]byte, 0, scopes*len(scope))
	for range scopes {
		resource = appendRepeatedMessages(resource, 2, scope)
	}
	payload := make([]byte, 0, resources*len(resource))
	for range resources {
		payload = appendRepeatedMessages(payload, 1, resource)
	}
	return payload
}

func assertSeqParity[T ~[]byte](
	t *testing.T,
	adapter func() (iter.Seq[T], func() error),
	seq func(func(T, error) bool),
	want ...[]byte,
) {
	t.Helper()

	ordinary, ordinaryErr := adapter()
	var ordinaryValues [][]byte
	for value := range ordinary {
		ordinaryValues = append(ordinaryValues, append([]byte(nil), value...))
	}
	adapterErr := ordinaryErr()
	require.Error(t, adapterErr)

	var seqValues [][]byte
	var seqErr error
	errYields := 0
	for value, err := range seq {
		if err != nil {
			seqErr = err
			errYields++
			continue
		}
		seqValues = append(seqValues, append([]byte(nil), value...))
	}
	require.Error(t, seqErr)
	require.Equal(t, 1, errYields)
	require.Equal(t, adapterErr.Error(), seqErr.Error())
	require.Equal(t, want, ordinaryValues)
	require.Equal(t, ordinaryValues, seqValues)
}

func appendRepeatedMessages(dst []byte, field protowire.Number, messages ...[]byte) []byte {
	for _, message := range messages {
		dst = protowire.AppendTag(dst, field, protowire.BytesType)
		dst = protowire.AppendBytes(dst, message)
	}
	return dst
}
