package otlpwire

// Tests for Resource(): the container's single Resource is optional, and
// repeated singular occurrences merge, matching pdata. Fixtures are built with
// protowire because pdata's API cannot emit the omitted, empty, or
// repeated-singular shapes under test; pdata remains the oracle for meaning.

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/protobuf/encoding/protowire"
)

// ---------- wire-fixture builders ----------

// wrapAsRequest wraps one resource container as an export request. All three
// export request types carry the container in repeated field 1.
func wrapAsRequest(container []byte) []byte {
	out := protowire.AppendTag(nil, 1, protowire.BytesType)
	out = protowire.AppendVarint(out, uint64(len(container)))
	return append(out, container...)
}

// scopeContainerNoResource builds a ResourceSpans/ResourceLogs/ResourceMetrics
// that omits the optional Resource field (field 1) and carries only an empty
// scope container (field 2).
func scopeContainerNoResource() []byte {
	out := protowire.AppendTag(nil, 2, protowire.BytesType)
	return protowire.AppendVarint(out, 0)
}

// containerWithResource builds a container whose Resource field holds the
// supplied raw Resource bytes, followed by an empty scope container.
func containerWithResource(resource []byte) []byte {
	return containerWithResources(resource)
}

// containerWithResources builds a container carrying every supplied Resource
// occurrence, in wire order, followed by an empty scope container.
func containerWithResources(resources ...[]byte) []byte {
	var out []byte
	for _, r := range resources {
		out = protowire.AppendTag(out, 1, protowire.BytesType)
		out = protowire.AppendVarint(out, uint64(len(r)))
		out = append(out, r...)
	}
	out = protowire.AppendTag(out, 2, protowire.BytesType)
	return protowire.AppendVarint(out, 0)
}

// stringKeyValue builds a KeyValue message with a string value (KeyValue.key
// is field 1, KeyValue.value field 2, AnyValue.string_value field 1).
func stringKeyValue(key, value string) []byte {
	anyValue := protowire.AppendTag(nil, 1, protowire.BytesType)
	anyValue = protowire.AppendString(anyValue, value)

	kv := protowire.AppendTag(nil, 1, protowire.BytesType)
	kv = protowire.AppendString(kv, key)
	kv = protowire.AppendTag(kv, 2, protowire.BytesType)
	kv = protowire.AppendVarint(kv, uint64(len(anyValue)))
	return append(kv, anyValue...)
}

// resourceWithStringAttr builds a Resource message carrying one string
// attribute. Resource.attributes is field 1.
func resourceWithStringAttr(key, value string) []byte {
	kv := stringKeyValue(key, value)
	res := protowire.AppendTag(nil, 1, protowire.BytesType)
	res = protowire.AppendVarint(res, uint64(len(kv)))
	return append(res, kv...)
}

// ---------- signal-generic harness ----------

// resourceGetter is satisfied by ResourceLogs, ResourceMetrics, and
// ResourceSpans, letting one test body cover all three signals.
type resourceGetter interface {
	Resource() (Resource, error)
	SchemaUrl() ([]byte, error)
}

type signalFixture struct {
	name string
	wrap func([]byte) resourceGetter
	// attrs returns the container's Resource attributes via pdata, the oracle
	// for merge and absence behavior.
	attrs func(t *testing.T, payload []byte) pcommon.Map
	// schemaURL returns the container's schema_url via pdata, the oracle for
	// singular-scalar resolution.
	schemaURL func(t *testing.T, payload []byte) string
}

var signalFixtures = []signalFixture{
	{
		name: "metrics",
		wrap: func(b []byte) resourceGetter { return ResourceMetrics(b) },
		attrs: func(t *testing.T, payload []byte) pcommon.Map {
			t.Helper()
			req := pmetricotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			require.Equal(t, 1, req.Metrics().ResourceMetrics().Len())
			return req.Metrics().ResourceMetrics().At(0).Resource().Attributes()
		},
		schemaURL: func(t *testing.T, payload []byte) string {
			t.Helper()
			req := pmetricotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			return req.Metrics().ResourceMetrics().At(0).SchemaUrl()
		},
	},
	{
		name: "logs",
		wrap: func(b []byte) resourceGetter { return ResourceLogs(b) },
		attrs: func(t *testing.T, payload []byte) pcommon.Map {
			t.Helper()
			req := plogotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			require.Equal(t, 1, req.Logs().ResourceLogs().Len())
			return req.Logs().ResourceLogs().At(0).Resource().Attributes()
		},
		schemaURL: func(t *testing.T, payload []byte) string {
			t.Helper()
			req := plogotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			return req.Logs().ResourceLogs().At(0).SchemaUrl()
		},
	},
	{
		name: "traces",
		wrap: func(b []byte) resourceGetter { return ResourceSpans(b) },
		attrs: func(t *testing.T, payload []byte) pcommon.Map {
			t.Helper()
			req := ptraceotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			require.Equal(t, 1, req.Traces().ResourceSpans().Len())
			return req.Traces().ResourceSpans().At(0).Resource().Attributes()
		},
		schemaURL: func(t *testing.T, payload []byte) string {
			t.Helper()
			req := ptraceotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			return req.Traces().ResourceSpans().At(0).SchemaUrl()
		},
	},
}

// ---------- absence and empty-but-present ----------

// TestResource_AbsentField proves the omitted-Resource payload is valid OTLP
// and that Resource() now agrees: (nil, nil), not an error.
func TestResource_AbsentField(t *testing.T) {
	container := scopeContainerNoResource()
	payload := wrapAsRequest(container)

	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			attrs := sf.attrs(t, payload)
			require.Equal(t, 0, attrs.Len(), "pdata accepts the omitted field")

			got, err := sf.wrap(container).Resource()
			require.NoError(t, err)
			require.Nil(t, got)
		})
	}
}

// TestResource_PresentButEmpty shows the present-but-zero-length Resource
// case, which already worked before this change and must keep working.
func TestResource_PresentButEmpty(t *testing.T) {
	container := containerWithResource(nil)
	payload := wrapAsRequest(container)

	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			attrs := sf.attrs(t, payload)
			require.Equal(t, 0, attrs.Len())

			got, err := sf.wrap(container).Resource()
			require.NoError(t, err)
			require.Empty(t, got)
		})
	}
}

// ---------- single occurrence: the hot, zero-copy path ----------

// TestResource_SingleOccurrence_IsZeroCopy proves the common case returns a
// slice aliasing the source buffer rather than a copy, and that the returned
// view cannot be appended into its neighbours.
func TestResource_SingleOccurrence_IsZeroCopy(t *testing.T) {
	res := resourceWithStringAttr("service.name", "checkout")
	baseContainer := containerWithResource(res)
	payload := wrapAsRequest(baseContainer)

	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			attrs := sf.attrs(t, payload)
			value, ok := attrs.Get("service.name")
			require.True(t, ok)
			require.Equal(t, "checkout", value.Str())

			// Fresh copy per subtest since the aliasing check below mutates
			// the container in place.
			container := append([]byte(nil), baseContainer...)
			got, err := sf.wrap(container).Resource()
			require.NoError(t, err)
			require.NotEmpty(t, got)

			strVal, found, err := got.StringAttribute("service.name")
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, "checkout", string(strVal))

			idx := bytes.Index(container, res)
			require.GreaterOrEqual(t, idx, 0, "resource bytes must appear in the container")

			// Flip every byte, not just the first: proves the whole region
			// aliases rather than one lucky offset.
			before := append([]byte(nil), got...)
			for i := range res {
				container[idx+i] ^= 0xFF
			}
			require.Equal(t, container[idx:idx+len(res)], []byte(got),
				"returned slice must alias the whole resource region, not copy it")
			for i := range got {
				require.NotEqual(t, before[i], got[i],
					"byte %d must reflect the source mutation", i)
			}

			// An unclamped capacity would let a caller's append overwrite the
			// sibling scope field.
			require.Equal(t, len(got), cap(got),
				"capacity must be clamped so append reallocates instead of corrupting the container")
			tail := append([]byte(nil), container[idx+len(res):]...)
			_ = append(got, 0xEE, 0xEE) //nolint:gocritic // deliberately discarded: asserting no in-place growth
			require.Equal(t, tail, container[idx+len(res):],
				"appending to the returned view must not touch bytes after the resource")
		})
	}
}

// TestResourceSingleOccurrence_ZeroAllocations pins the hard performance gate:
// Resource() on the common single-occurrence container allocates nothing.
func TestResourceSingleOccurrence_ZeroAllocations(t *testing.T) {
	container := containerWithResource(resourceWithStringAttr("service.name", "checkout"))
	rm := ResourceMetrics(container)

	allocs := testing.AllocsPerRun(1000, func() {
		got, err := rm.Resource()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("expected a non-empty resource")
		}
	})
	require.Zero(t, allocs)
}

// ---------- merge: 2+ occurrences ----------

// TestResource_Merge covers protobuf's merge behavior for repeated
// occurrences of the singular Resource field: distinct keys, a duplicate key
// (pdata keeps the first value), and 3+ occurrences. otlp-wire merges by
// concatenating the encoded Resource bodies; this is verified byte-equivalent
// to pdata's merge for these shapes.
func TestResource_Merge(t *testing.T) {
	first := resourceWithStringAttr("service.name", "checkout")
	second := resourceWithStringAttr("deployment.environment", "prod")
	dup := resourceWithStringAttr("service.name", "shadow")

	tests := []struct {
		name        string
		occurrences [][]byte
		wantLen     int // pdata's raw attribute count (duplicates are not deduplicated)
	}{
		{"two distinct keys", [][]byte{first, second}, 2},
		{"duplicate key", [][]byte{first, dup}, 2},
		{"three occurrences", [][]byte{first, second, dup}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := containerWithResources(tt.occurrences...)
			payload := wrapAsRequest(container)

			for _, sf := range signalFixtures {
				t.Run(sf.name, func(t *testing.T) {
					attrs := sf.attrs(t, payload)
					require.Equal(t, tt.wantLen, attrs.Len(), "pdata merge result")
					pdataValue, ok := attrs.Get("service.name")
					require.True(t, ok)

					got, err := sf.wrap(container).Resource()
					require.NoError(t, err)

					count := 0
					seq, errFn := got.Attributes()
					for range seq {
						count++
					}
					require.NoError(t, errFn())
					require.Equal(t, tt.wantLen, count, "otlp-wire must see every merged attribute occurrence")

					value, found, err := got.StringAttribute("service.name")
					require.NoError(t, err)
					require.True(t, found)
					require.Equal(t, pdataValue.Str(), string(value), "first-value-wins must match pdata")
				})
			}
		})
	}
}

// TestResource_Merge_MultipleOccurrencesAllocate documents the one
// intentional exception to the zero-copy contract: 2+ occurrences require a
// concatenated buffer and must not alias the source.
func TestResource_Merge_MultipleOccurrencesAllocate(t *testing.T) {
	container := containerWithResources(
		resourceWithStringAttr("service.name", "checkout"),
		resourceWithStringAttr("deployment.environment", "prod"),
	)
	rm := ResourceMetrics(container)

	got, err := rm.Resource()
	require.NoError(t, err)
	require.NotEmpty(t, got)

	allocs := testing.AllocsPerRun(200, func() {
		merged, err := rm.Resource()
		if err != nil {
			t.Fatal(err)
		}
		if len(merged) == 0 {
			t.Fatal("expected merged resource bytes")
		}
	})
	require.Greater(t, allocs, 0.0, "2+ occurrences must allocate a new buffer, not alias the source")
}

// ---------- malformed and structural coverage ----------

// TestResource_MalformedWrongWireType keeps malformed-wire coverage distinct
// from absence: an incorrect wire type on the Resource field must stay an
// error regardless of occurrence count.
func TestResource_MalformedWrongWireType(t *testing.T) {
	container := protowire.AppendTag(nil, 1, protowire.VarintType)
	container = protowire.AppendVarint(container, 42)

	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			got, err := sf.wrap(container).Resource()
			require.Error(t, err)
			require.Contains(t, err.Error(), "wrong wire type")
			require.Nil(t, got)
		})
	}
}

// TestResource_MalformedTruncatedLength covers a Resource field whose length
// prefix claims more bytes than the buffer holds.
func TestResource_MalformedTruncatedLength(t *testing.T) {
	container := protowire.AppendTag(nil, 1, protowire.BytesType)
	container = protowire.AppendVarint(container, 50) // claims 50 bytes; none follow

	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			got, err := sf.wrap(container).Resource()
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}

// TestResource_MalformedTrailingFieldAfterResource exercises the
// validation-scope change: merging requires scanning the complete container
// to find every Resource occurrence, so a malformed field located after the
// (only) Resource occurrence is now detected, where the previous
// first-match-and-return implementation never reached it.
func TestResource_MalformedTrailingFieldAfterResource(t *testing.T) {
	container := containerWithResource(resourceWithStringAttr("service.name", "checkout"))
	// Append a malformed trailing field: a truncated tag byte with the
	// continuation bit set and nothing after it.
	container = append(container, 0x80)

	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			got, err := sf.wrap(container).Resource()
			require.Error(t, err, "scanning the full container for merge now surfaces this corruption")
			require.Nil(t, got)
		})
	}
}

// TestResource_UnknownAndOutOfOrderFieldsSkipped confirms unknown top-level
// fields and an out-of-order Resource occurrence (appearing after the scope
// container rather than before it) are both handled correctly.
func TestResource_UnknownAndOutOfOrderFieldsSkipped(t *testing.T) {
	res := resourceWithStringAttr("service.name", "checkout")

	var container []byte
	container = appendUnknownGroup(container, 90) // unknown field before anything else
	container = protowire.AppendTag(container, 2, protowire.BytesType)
	container = protowire.AppendVarint(container, 0) // empty scope container, field 2
	container = appendUnknownGroup(container, 91)    // unknown field between scope and resource
	container = protowire.AppendTag(container, 1, protowire.BytesType)
	container = protowire.AppendVarint(container, uint64(len(res)))
	container = append(container, res...) // resource arrives out of order, after the scope field
	container = appendUnknownGroup(container, 92)

	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			got, err := sf.wrap(container).Resource()
			require.NoError(t, err)
			value, found, err := got.StringAttribute("service.name")
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, "checkout", string(value))
		})
	}
}

// TestResource_SplicedOccurrencesRejected pins the boundary that makes merging
// by concatenation safe. Neither half of a spliced Resource parses alone, but
// their concatenation does; pdata rejects such a payload, so merging without
// checking would make the wire path accept what its pdata fallback refuses.
func TestResource_SplicedOccurrencesRejected(t *testing.T) {
	full := resourceWithStringAttr("service.name", "checkout")
	cut := 12 // lands inside the attributes field, after its length prefix
	occ1, occ2 := full[:cut], full[cut:]

	require.True(t, bytes.Equal(append(append([]byte{}, occ1...), occ2...), full),
		"fixture must split a valid Resource into two halves")

	container := containerWithResources(occ1, occ2)

	// pdata rejects the payload outright.
	req := plogotlp.NewExportRequest()
	require.Error(t, req.UnmarshalProto(wrapAsRequest(container)),
		"pdata must reject a spliced Resource")

	// otlp-wire must reject it too, rather than silently reassembling it.
	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			got, err := sf.wrap(container).Resource()
			require.Error(t, err, "spliced occurrences must not be merged into a valid Resource")
			require.Nil(t, got)
		})
	}
}

// TestResource_ValidOccurrencesStillMerge guards the fix above from becoming
// over-strict: occurrences that each stand alone must still merge.
func TestResource_ValidOccurrencesStillMerge(t *testing.T) {
	a := resourceWithStringAttr("service.name", "checkout")
	b := resourceWithStringAttr("deployment.environment", "prod")
	container := containerWithResources(a, b)

	for _, sf := range signalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			got, err := sf.wrap(container).Resource()
			require.NoError(t, err)

			for key, want := range map[string]string{
				"service.name":           "checkout",
				"deployment.environment": "prod",
			} {
				value, found, err := got.StringAttribute(key)
				require.NoError(t, err)
				require.True(t, found, "attribute %q must survive the merge", key)
				require.Equal(t, want, string(value))
			}
		})
	}
}

// ---------- schema_url: a singular scalar, not a merged message ----------

// TestResourceSchemaUrl covers the container-level schema_url (field 3).
// Unlike Resource, it is a singular *scalar*: protobuf and pdata resolve a
// repeated occurrence to the last one rather than merging.
func TestResourceSchemaUrl(t *testing.T) {
	tests := []struct {
		name string
		urls []string
		want string
	}{
		{"absent", nil, ""},
		{"single", []string{"https://example.test/v1"}, "https://example.test/v1"},
		{"repeated", []string{"https://example.test/v1", "https://example.test/v2"}, "https://example.test/v2"},
		{"last is empty", []string{"https://example.test/v1", ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := containerWithResource(resourceWithStringAttr("service.name", "checkout"))
			for _, u := range tt.urls {
				container = protowire.AppendTag(container, 3, protowire.BytesType)
				container = protowire.AppendString(container, u)
			}
			payload := wrapAsRequest(container)

			for _, sf := range signalFixtures {
				t.Run(sf.name, func(t *testing.T) {
					require.Equal(t, tt.want, sf.schemaURL(t, payload), "pdata schema_url")

					got, err := sf.wrap(container).SchemaUrl()
					require.NoError(t, err)
					require.Equal(t, tt.want, string(got))
					if tt.urls == nil {
						require.Nil(t, got, "an absent schema_url is (nil, nil)")
					}
				})
			}
		})
	}
}
