package otlpwire

// Tests for Scope() and SchemaUrl(). The scope container's InstrumentationScope
// is optional and repeated singular occurrences merge, exactly as for Resource.
// schema_url is a singular *scalar*, so repeated occurrences resolve to the
// last one instead of merging. Fixtures are built with protowire because
// pdata's API cannot emit the omitted, empty, or repeated-singular shapes under
// test; pdata remains the oracle for meaning.

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

// scopeMessage builds an InstrumentationScope carrying name (field 1) and
// version (field 2). Empty strings are omitted rather than encoded.
func scopeMessage(name, version string) []byte {
	var out []byte
	if name != "" {
		out = protowire.AppendTag(out, 1, protowire.BytesType)
		out = protowire.AppendString(out, name)
	}
	if version != "" {
		out = protowire.AppendTag(out, 2, protowire.BytesType)
		out = protowire.AppendString(out, version)
	}
	return out
}

// scopeWithStringAttr builds an InstrumentationScope carrying one string
// attribute. Scope attributes are field 3; Resource attributes are field 1.
func scopeWithStringAttr(key, value string) []byte {
	kv := stringKeyValue(key, value)
	out := protowire.AppendTag(nil, 3, protowire.BytesType)
	out = protowire.AppendVarint(out, uint64(len(kv)))
	return append(out, kv...)
}

// scopeContainer builds a ScopeLogs/ScopeSpans/ScopeMetrics carrying every
// supplied InstrumentationScope occurrence in field 1, in wire order.
func scopeContainer(scopes ...[]byte) []byte {
	var out []byte
	for _, s := range scopes {
		out = protowire.AppendTag(out, 1, protowire.BytesType)
		out = protowire.AppendVarint(out, uint64(len(s)))
		out = append(out, s...)
	}
	return out
}

// scopeContainerWithSchemaURLs appends schema_url occurrences (field 3) after
// the supplied scope occurrences.
func scopeContainerWithSchemaURLs(scopes [][]byte, schemaURLs ...string) []byte {
	out := scopeContainer(scopes...)
	for _, u := range schemaURLs {
		out = protowire.AppendTag(out, 3, protowire.BytesType)
		out = protowire.AppendString(out, u)
	}
	return out
}

// resourceContainerWithScope wraps a scope container as field 2 of a
// ResourceLogs/ResourceMetrics/ResourceSpans, omitting the optional Resource.
func resourceContainerWithScope(sc []byte) []byte {
	out := protowire.AppendTag(nil, 2, protowire.BytesType)
	out = protowire.AppendVarint(out, uint64(len(sc)))
	return append(out, sc...)
}

// resourceContainerWithSchemaURLs wraps a scope container and appends
// schema_url occurrences (field 3) at the resource-container level.
func resourceContainerWithSchemaURLs(sc []byte, schemaURLs ...string) []byte {
	out := resourceContainerWithScope(sc)
	for _, u := range schemaURLs {
		out = protowire.AppendTag(out, 3, protowire.BytesType)
		out = protowire.AppendString(out, u)
	}
	return out
}

// ---------- signal-generic harness ----------

// scopeGetter is satisfied by ScopeLogs, ScopeMetrics, and ScopeSpans.
type scopeGetter interface {
	Scope() (InstrumentationScope, error)
	SchemaUrl() ([]byte, error)
}

// schemaURLGetter is satisfied by every resource and scope container.
type schemaURLGetter interface {
	SchemaUrl() ([]byte, error)
}

// pdataScopeView is what the oracle reports for a single-resource,
// single-scope payload.
type pdataScopeView struct {
	scope             pcommon.InstrumentationScope
	scopeSchemaURL    string
	resourceSchemaURL string
}

type scopeSignalFixture struct {
	name      string
	scopes    func([]byte) scopeGetter
	resources func([]byte) schemaURLGetter
	pdata     func(t *testing.T, payload []byte) pdataScopeView
}

var scopeSignalFixtures = []scopeSignalFixture{
	{
		name:      "metrics",
		scopes:    func(b []byte) scopeGetter { return ScopeMetrics(b) },
		resources: func(b []byte) schemaURLGetter { return ResourceMetrics(b) },
		pdata: func(t *testing.T, payload []byte) pdataScopeView {
			t.Helper()
			req := pmetricotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			require.Equal(t, 1, req.Metrics().ResourceMetrics().Len())
			rm := req.Metrics().ResourceMetrics().At(0)
			require.Equal(t, 1, rm.ScopeMetrics().Len())
			sm := rm.ScopeMetrics().At(0)
			return pdataScopeView{sm.Scope(), sm.SchemaUrl(), rm.SchemaUrl()}
		},
	},
	{
		name:      "logs",
		scopes:    func(b []byte) scopeGetter { return ScopeLogs(b) },
		resources: func(b []byte) schemaURLGetter { return ResourceLogs(b) },
		pdata: func(t *testing.T, payload []byte) pdataScopeView {
			t.Helper()
			req := plogotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			require.Equal(t, 1, req.Logs().ResourceLogs().Len())
			rl := req.Logs().ResourceLogs().At(0)
			require.Equal(t, 1, rl.ScopeLogs().Len())
			sl := rl.ScopeLogs().At(0)
			return pdataScopeView{sl.Scope(), sl.SchemaUrl(), rl.SchemaUrl()}
		},
	},
	{
		name:      "traces",
		scopes:    func(b []byte) scopeGetter { return ScopeSpans(b) },
		resources: func(b []byte) schemaURLGetter { return ResourceSpans(b) },
		pdata: func(t *testing.T, payload []byte) pdataScopeView {
			t.Helper()
			req := ptraceotlp.NewExportRequest()
			require.NoError(t, req.UnmarshalProto(payload))
			require.Equal(t, 1, req.Traces().ResourceSpans().Len())
			rs := req.Traces().ResourceSpans().At(0)
			require.Equal(t, 1, rs.ScopeSpans().Len())
			ss := rs.ScopeSpans().At(0)
			return pdataScopeView{ss.Scope(), ss.SchemaUrl(), rs.SchemaUrl()}
		},
	},
}

// ---------- absence and empty-but-present ----------

// TestScope_AbsentField proves an omitted scope is valid OTLP and that Scope()
// reports (nil, nil) rather than an error, matching pdata's empty scope.
func TestScope_AbsentField(t *testing.T) {
	sc := scopeContainer()
	payload := wrapAsRequest(resourceContainerWithScope(sc))

	for _, sf := range scopeSignalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			view := sf.pdata(t, payload)
			require.Empty(t, view.scope.Name())
			require.Empty(t, view.scope.Version())
			require.Equal(t, 0, view.scope.Attributes().Len())

			got, err := sf.scopes(sc).Scope()
			require.NoError(t, err)
			require.Nil(t, got)
		})
	}
}

// TestScope_PresentButEmpty covers a zero-length scope message: present on the
// wire, empty in pdata.
func TestScope_PresentButEmpty(t *testing.T) {
	sc := scopeContainer(nil)
	payload := wrapAsRequest(resourceContainerWithScope(sc))

	for _, sf := range scopeSignalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			view := sf.pdata(t, payload)
			require.Empty(t, view.scope.Name())

			got, err := sf.scopes(sc).Scope()
			require.NoError(t, err)
			require.Empty(t, got)

			name, err := got.Name()
			require.NoError(t, err)
			require.Nil(t, name)
		})
	}
}

// ---------- populated: name, version, attributes ----------

// TestScope_Populated checks the accessors against pdata for a fully populated
// scope, and pins that scope attributes come from field 3.
func TestScope_Populated(t *testing.T) {
	scope := append(scopeMessage("checkout-instr", "1.2.3"),
		scopeWithStringAttr("library.language", "go")...)
	sc := scopeContainer(scope)
	payload := wrapAsRequest(resourceContainerWithScope(sc))

	for _, sf := range scopeSignalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			view := sf.pdata(t, payload)
			require.Equal(t, "checkout-instr", view.scope.Name())
			require.Equal(t, "1.2.3", view.scope.Version())

			got, err := sf.scopes(sc).Scope()
			require.NoError(t, err)

			name, err := got.Name()
			require.NoError(t, err)
			require.Equal(t, view.scope.Name(), string(name))

			version, err := got.Version()
			require.NoError(t, err)
			require.Equal(t, view.scope.Version(), string(version))

			pdataValue, ok := view.scope.Attributes().Get("library.language")
			require.True(t, ok)
			require.Equal(t, "go", pdataValue.Str())

			count := 0
			seq, errFn := got.Attributes()
			for kv := range seq {
				count++
				key, err := kv.Key()
				require.NoError(t, err)
				require.Equal(t, "library.language", string(key))
				value, found, err := kv.StringValue()
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, "go", string(value))
			}
			require.NoError(t, errFn())
			require.Equal(t, view.scope.Attributes().Len(), count)
		})
	}
}

// TestScope_AttributesSeqMatchesAttributes proves the zero-allocation variant
// yields the same elements as the closure-based iterator.
func TestScope_AttributesSeqMatchesAttributes(t *testing.T) {
	scope := append(scopeWithStringAttr("a", "1"), scopeWithStringAttr("b", "2")...)
	got := InstrumentationScope(scope)

	var viaSeq []string
	got.AttributesSeq(func(kv KeyValue, err error) bool {
		require.NoError(t, err)
		key, keyErr := kv.Key()
		require.NoError(t, keyErr)
		viaSeq = append(viaSeq, string(key))
		return true
	})

	var viaIter []string
	seq, errFn := got.Attributes()
	for kv := range seq {
		key, err := kv.Key()
		require.NoError(t, err)
		viaIter = append(viaIter, string(key))
	}
	require.NoError(t, errFn())

	require.Equal(t, []string{"a", "b"}, viaSeq)
	require.Equal(t, viaSeq, viaIter)
}

// TestScope_AttributesSeq_EarlyStop proves an early return leaves the rest
// unvisited, matching the documented lazy-iteration contract.
func TestScope_AttributesSeq_EarlyStop(t *testing.T) {
	scope := append(scopeWithStringAttr("a", "1"), scopeWithStringAttr("b", "2")...)

	visited := 0
	InstrumentationScope(scope).AttributesSeq(func(_ KeyValue, err error) bool {
		require.NoError(t, err)
		visited++
		return false
	})
	require.Equal(t, 1, visited)
}

// TestScope_AttributesSeq_ZeroAllocations pins the hot-path gate that matches
// the existing Resource.AttributesSeq guarantee.
func TestScope_AttributesSeq_ZeroAllocations(t *testing.T) {
	scope := InstrumentationScope(append(
		scopeWithStringAttr("service.name", "checkout"),
		scopeWithStringAttr("library.language", "go")...))

	allocs := testing.AllocsPerRun(1000, func() {
		count := 0
		scope.AttributesSeq(func(_ KeyValue, err error) bool {
			if err != nil {
				t.Fatal(err)
			}
			count++
			return true
		})
		if count != 2 {
			t.Fatalf("expected 2 attributes, got %d", count)
		}
	})
	require.Zero(t, allocs)
}

// ---------- single occurrence: the hot, zero-copy path ----------

// TestScope_SingleOccurrence_IsZeroCopy proves the common case returns a slice
// aliasing the source buffer rather than a copy, and that the returned view
// cannot be appended into its neighbours.
func TestScope_SingleOccurrence_IsZeroCopy(t *testing.T) {
	scope := scopeMessage("checkout-instr", "1.2.3")
	baseContainer := scopeContainerWithSchemaURLs([][]byte{scope}, "https://example.test/schema")

	for _, sf := range scopeSignalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			// Fresh copy per subtest since the aliasing check mutates in place.
			sc := append([]byte(nil), baseContainer...)
			got, err := sf.scopes(sc).Scope()
			require.NoError(t, err)
			require.NotEmpty(t, got)

			idx := bytes.Index(sc, scope)
			require.GreaterOrEqual(t, idx, 0, "scope bytes must appear in the container")

			before := append([]byte(nil), got...)
			for i := range scope {
				sc[idx+i] ^= 0xFF
			}
			require.Equal(t, sc[idx:idx+len(scope)], []byte(got),
				"returned slice must alias the whole scope region, not copy it")
			for i := range got {
				require.NotEqual(t, before[i], got[i],
					"byte %d must reflect the source mutation", i)
			}

			// An unclamped capacity would let a caller's append overwrite the
			// sibling schema_url field.
			require.Equal(t, len(got), cap(got),
				"capacity must be clamped so append reallocates instead of corrupting the container")
			tail := append([]byte(nil), sc[idx+len(scope):]...)
			_ = append(got, 0xEE, 0xEE) //nolint:gocritic // deliberately discarded: asserting no in-place growth
			require.Equal(t, tail, sc[idx+len(scope):],
				"appending to the returned view must not touch bytes after the scope")
		})
	}
}

// TestScopeSingleOccurrence_ZeroAllocations pins the performance gate: Scope()
// on the common single-occurrence container allocates nothing.
func TestScopeSingleOccurrence_ZeroAllocations(t *testing.T) {
	sm := ScopeMetrics(scopeContainer(scopeMessage("checkout-instr", "1.2.3")))

	allocs := testing.AllocsPerRun(1000, func() {
		got, err := sm.Scope()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("expected a non-empty scope")
		}
	})
	require.Zero(t, allocs)
}

// ---------- merge: 2+ scope occurrences ----------

// TestScope_Merge covers protobuf merge behavior for repeated occurrences of
// the singular scope field. Attributes accumulate, while name and version are
// scalars, so the later occurrence wins — pdata is the oracle for both.
func TestScope_Merge(t *testing.T) {
	tests := []struct {
		name        string
		occurrences [][]byte
	}{
		{
			"attributes accumulate",
			[][]byte{scopeWithStringAttr("a", "1"), scopeWithStringAttr("b", "2")},
		},
		{
			"later name and version win",
			[][]byte{scopeMessage("first", "1.0.0"), scopeMessage("second", "2.0.0")},
		},
		{
			"later occurrence fills omitted version",
			[][]byte{scopeMessage("first", ""), scopeMessage("", "2.0.0")},
		},
		{
			"three occurrences",
			[][]byte{
				scopeMessage("first", "1.0.0"),
				scopeWithStringAttr("a", "1"),
				scopeMessage("third", "3.0.0"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := scopeContainer(tt.occurrences...)
			payload := wrapAsRequest(resourceContainerWithScope(sc))

			for _, sf := range scopeSignalFixtures {
				t.Run(sf.name, func(t *testing.T) {
					view := sf.pdata(t, payload)

					got, err := sf.scopes(sc).Scope()
					require.NoError(t, err)

					name, err := got.Name()
					require.NoError(t, err)
					require.Equal(t, view.scope.Name(), string(name))

					version, err := got.Version()
					require.NoError(t, err)
					require.Equal(t, view.scope.Version(), string(version))

					count := 0
					seq, errFn := got.Attributes()
					for range seq {
						count++
					}
					require.NoError(t, errFn())
					require.Equal(t, view.scope.Attributes().Len(), count)
				})
			}
		})
	}
}

// TestScope_MergeMultipleOccurrencesAllocates documents the one accepted
// allocation: merging 2+ occurrences concatenates into a new buffer.
func TestScope_MergeMultipleOccurrencesAllocates(t *testing.T) {
	sm := ScopeMetrics(scopeContainer(scopeMessage("first", ""), scopeMessage("", "2.0.0")))

	allocs := testing.AllocsPerRun(100, func() {
		got, err := sm.Scope()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("expected a non-empty scope")
		}
	})
	require.Positive(t, allocs, "merging 2+ occurrences is documented to allocate")
}

// TestScope_SplicedOccurrencesRejected proves concatenation cannot manufacture
// validity: two occurrences that each fail to parse alone must not reassemble
// into a scope pdata would reject.
func TestScope_SplicedOccurrencesRejected(t *testing.T) {
	// A name field declaring 8 bytes but carrying only 3, with the remaining
	// 5 in the next occurrence.
	whole := protowire.AppendTag(nil, 1, protowire.BytesType)
	whole = protowire.AppendString(whole, "checkout")
	split := len(whole) - 5

	sc := scopeContainer(whole[:split], whole[split:])
	payload := wrapAsRequest(resourceContainerWithScope(sc))

	// The three signals share these field numbers, so one oracle covers them.
	req := pmetricotlp.NewExportRequest()
	require.Error(t, req.UnmarshalProto(payload), "pdata must reject the spliced payload")

	for _, sf := range scopeSignalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			_, err := sf.scopes(sc).Scope()
			require.Error(t, err)
		})
	}
}

// TestScope_ValidOccurrencesStillMerge guards against the splice check
// becoming over-strict.
func TestScope_ValidOccurrencesStillMerge(t *testing.T) {
	sc := scopeContainer(scopeMessage("first", ""), scopeMessage("", "2.0.0"))

	for _, sf := range scopeSignalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			got, err := sf.scopes(sc).Scope()
			require.NoError(t, err)

			name, err := got.Name()
			require.NoError(t, err)
			require.Equal(t, "first", string(name))

			version, err := got.Version()
			require.NoError(t, err)
			require.Equal(t, "2.0.0", string(version))
		})
	}
}

// ---------- malformed input ----------

func TestScope_Malformed(t *testing.T) {
	tests := []struct {
		name string
		sc   []byte
	}{
		{
			"wrong wire type for scope field",
			func() []byte {
				out := protowire.AppendTag(nil, 1, protowire.VarintType)
				return protowire.AppendVarint(out, 42)
			}(),
		},
		{
			"truncated scope length",
			func() []byte {
				out := protowire.AppendTag(nil, 1, protowire.BytesType)
				out = protowire.AppendVarint(out, 32)
				return append(out, 0x01, 0x02)
			}(),
		},
		{
			"malformed field after the last scope occurrence",
			func() []byte {
				out := scopeContainer(scopeMessage("checkout-instr", ""))
				out = protowire.AppendTag(out, 9, protowire.BytesType)
				return protowire.AppendVarint(out, 64)
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, sf := range scopeSignalFixtures {
				t.Run(sf.name, func(t *testing.T) {
					_, err := sf.scopes(tt.sc).Scope()
					require.Error(t, err)
				})
			}
		})
	}
}

// TestScope_MalformedAttributeSurfacesOnIteration pins the operation-scoped
// validation level: Name() walks the scope structurally and succeeds, while
// iterating the corrupt attribute reports the error.
func TestScope_MalformedAttributeSurfacesOnIteration(t *testing.T) {
	badKV := protowire.AppendTag(nil, 1, protowire.BytesType)
	badKV = protowire.AppendVarint(badKV, 64) // declares more than it carries

	scope := protowire.AppendTag(nil, 3, protowire.BytesType)
	scope = protowire.AppendVarint(scope, uint64(len(badKV)))
	scope = append(scope, badKV...)
	scope = append(scopeMessage("checkout-instr", ""), scope...)

	got := InstrumentationScope(scope)

	name, err := got.Name()
	require.NoError(t, err)
	require.Equal(t, "checkout-instr", string(name))

	seq, errFn := got.Attributes()
	for kv := range seq {
		_, _, err := kv.StringValue()
		require.Error(t, err)
	}
	require.NoError(t, errFn())
}

// TestScope_UnknownAndOutOfOrderFieldsSkipped proves forward compatibility:
// unknown fields and a scope field emitted after the records field still work.
func TestScope_UnknownAndOutOfOrderFieldsSkipped(t *testing.T) {
	sc := protowire.AppendTag(nil, 7, protowire.VarintType)
	sc = protowire.AppendVarint(sc, 99)
	sc = protowire.AppendTag(sc, 2, protowire.BytesType)
	sc = protowire.AppendVarint(sc, 0)
	scope := scopeMessage("checkout-instr", "1.2.3")
	sc = protowire.AppendTag(sc, 1, protowire.BytesType)
	sc = protowire.AppendVarint(sc, uint64(len(scope)))
	sc = append(sc, scope...)

	for _, sf := range scopeSignalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			got, err := sf.scopes(sc).Scope()
			require.NoError(t, err)

			name, err := got.Name()
			require.NoError(t, err)
			require.Equal(t, "checkout-instr", string(name))
		})
	}
}

// ---------- schema_url ----------

// TestSchemaUrl_Absent proves an omitted schema_url is (nil, nil), matching
// pdata's empty string.
func TestSchemaUrl_Absent(t *testing.T) {
	sc := scopeContainer(scopeMessage("checkout-instr", ""))
	container := resourceContainerWithScope(sc)
	payload := wrapAsRequest(container)

	for _, sf := range scopeSignalFixtures {
		t.Run(sf.name, func(t *testing.T) {
			view := sf.pdata(t, payload)
			require.Empty(t, view.scopeSchemaURL)
			require.Empty(t, view.resourceSchemaURL)

			got, err := sf.scopes(sc).SchemaUrl()
			require.NoError(t, err)
			require.Nil(t, got)

			got, err = sf.resources(container).SchemaUrl()
			require.NoError(t, err)
			require.Nil(t, got)
		})
	}
}

// TestSchemaUrl_LastValueWins is the core differential for the scalar
// contract: protobuf and pdata resolve a repeated singular string to the last
// occurrence, so a first-match extractor would diverge.
func TestSchemaUrl_LastValueWins(t *testing.T) {
	tests := []struct {
		name string
		urls []string
		want string
	}{
		{"single", []string{"https://example.test/v1"}, "https://example.test/v1"},
		{"repeated", []string{"https://example.test/v1", "https://example.test/v2"}, "https://example.test/v2"},
		{"three", []string{"a", "b", "c"}, "c"},
		{"last is empty", []string{"https://example.test/v1", ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := scopeContainerWithSchemaURLs([][]byte{scopeMessage("s", "")}, tt.urls...)
			container := resourceContainerWithSchemaURLs(sc, tt.urls...)
			payload := wrapAsRequest(container)

			for _, sf := range scopeSignalFixtures {
				t.Run(sf.name, func(t *testing.T) {
					view := sf.pdata(t, payload)
					require.Equal(t, tt.want, view.scopeSchemaURL, "pdata scope schema_url")
					require.Equal(t, tt.want, view.resourceSchemaURL, "pdata resource schema_url")

					got, err := sf.scopes(sc).SchemaUrl()
					require.NoError(t, err)
					require.Equal(t, tt.want, string(got))

					got, err = sf.resources(container).SchemaUrl()
					require.NoError(t, err)
					require.Equal(t, tt.want, string(got))
				})
			}
		})
	}
}

// TestSchemaUrl_Malformed covers the wrong-wire-type and trailing-corruption
// cases the full scan is responsible for reporting.
func TestSchemaUrl_Malformed(t *testing.T) {
	tests := []struct {
		name string
		sc   []byte
	}{
		{
			"wrong wire type",
			func() []byte {
				out := protowire.AppendTag(nil, 3, protowire.VarintType)
				return protowire.AppendVarint(out, 42)
			}(),
		},
		{
			"truncated length",
			func() []byte {
				out := protowire.AppendTag(nil, 3, protowire.BytesType)
				out = protowire.AppendVarint(out, 32)
				return append(out, 0x01)
			}(),
		},
		{
			"malformed field after schema_url",
			func() []byte {
				out := protowire.AppendTag(nil, 3, protowire.BytesType)
				out = protowire.AppendString(out, "https://example.test/v1")
				out = protowire.AppendTag(out, 9, protowire.BytesType)
				return protowire.AppendVarint(out, 64)
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, sf := range scopeSignalFixtures {
				t.Run(sf.name, func(t *testing.T) {
					_, err := sf.scopes(tt.sc).SchemaUrl()
					require.Error(t, err)

					_, err = sf.resources(resourceContainerWithScope(nil)).SchemaUrl()
					require.NoError(t, err, "control: a clean container must still parse")
				})
			}
		})
	}
}

// TestSchemaUrl_ZeroAllocations keeps the accessor off the allocating path.
func TestSchemaUrl_ZeroAllocations(t *testing.T) {
	sm := ScopeMetrics(scopeContainerWithSchemaURLs(
		[][]byte{scopeMessage("checkout-instr", "")}, "https://example.test/v1"))

	allocs := testing.AllocsPerRun(1000, func() {
		got, err := sm.SchemaUrl()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("expected a schema URL")
		}
	})
	require.Zero(t, allocs)
}
