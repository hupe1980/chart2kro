package probe

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/chart"
)

type doc struct {
	source string
	obj    map[string]any
}

// render builds a view as Helm would: ordinals count per source file.
func render(docs ...doc) *View {
	ord := map[string]int{}

	var co []chart.Object

	for _, d := range docs {
		co = append(co, chart.Object{Source: d.source, Ordinal: ord[d.source], Obj: d.obj})
		ord[d.source]++
	}

	return NewView(co)
}

func cm(name string, data map[string]any) map[string]any {
	return map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": name}, "data": data}
}

const (
	loopSrc  = "c/templates/tenants.yaml"
	otherSrc = "c/templates/svc.yaml"
)

var svc = doc{otherSrc, map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "svc"}}}

// lengths renders n copies of an entry; per(i) returns the documents of entry i.
func lengths(per func(i int) []doc) [LoopLengths]*View {
	var out [LoopLengths]*View

	for n := 0; n < LoopLengths; n++ {
		docs := []doc{svc}
		for i := 0; i < n; i++ {
			docs = append(docs, per(i)...)
		}

		out[n] = render(docs...)
	}

	return out
}

func TestDetectLoopOnePerEntry(t *testing.T) {
	l, reason, _ := DetectLoop(lengths(func(int) []doc {
		return []doc{{loopSrc, cm("r-a", nil)}}
	}), Noise{})

	require.Empty(t, reason)
	require.Equal(t, []Position{{Source: loopSrc, Ordinal: 0, PerEntry: 1, GVK: "v1/ConfigMap"}}, l.Positions)
	require.Empty(t, l.Index)
}

func TestDetectLoopTwoDocumentsPerEntry(t *testing.T) {
	l, reason, _ := DetectLoop(lengths(func(int) []doc {
		return []doc{{loopSrc, cm("r-a", nil)}, {loopSrc, map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "r-a"}}}}
	}), Noise{})

	require.Empty(t, reason)
	require.Len(t, l.Positions, 2)
	require.Equal(t, "v1/Secret", l.Positions[1].GVK)
	require.Equal(t, loopSrc+"#3#v1/Secret", l.Positions[1].Key(1))
}

func TestDetectLoopIndex(t *testing.T) {
	l, reason, _ := DetectLoop(lengths(func(i int) []doc {
		return []doc{{loopSrc, cm("r-worker-"+string(rune('0'+i)), map[string]any{"slot": float64(i)})}}
	}), Noise{})

	require.Empty(t, reason)
	require.Len(t, l.Index, 2)

	for _, tg := range l.Index {
		if tg.Mode == Span {
			require.Equal(t, [][2]int{{9, 10}}, tg.Spans)
		}
	}
}

func TestDetectLoopRejections(t *testing.T) {
	cases := []struct {
		name   string
		views  [LoopLengths]*View
		reason string
	}{
		{"shares template", func() [LoopLengths]*View {
			v := lengths(func(int) []doc { return []doc{{loopSrc, cm("r-a", nil)}} })
			for n := range v {
				docs := []doc{svc, {loopSrc, cm("static", nil)}}
				for i := 0; i < n; i++ {
					docs = append(docs, doc{loopSrc, cm("r-a", nil)})
				}

				v[n] = render(docs...)
			}

			return v
		}(), ReasonLoopShares},
		{"conditional body", func() [LoopLengths]*View {
			var v [LoopLengths]*View
			for n := range v {
				docs := []doc{svc}
				for i := 0; i < n && i < 2; i++ {
					docs = append(docs, doc{loopSrc, cm("r-a", nil)})
				}

				v[n] = render(docs...)
			}

			return v
		}(), ReasonLoopCondition},
		{"changes another object", func() [LoopLengths]*View {
			var v [LoopLengths]*View
			for n := range v {
				other := doc{otherSrc, map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "svc", "annotations": map[string]any{"count": float64(n)}}}}
				docs := []doc{other}
				for i := 0; i < n; i++ {
					docs = append(docs, doc{loopSrc, cm("r-a", nil)})
				}

				v[n] = render(docs...)
			}

			return v
		}(), ReasonLoopElement},
		{"entries differ beyond the index", lengths(func(i int) []doc {
			return []doc{{loopSrc, cm("r-a", map[string]any{"x": []string{"p", "q", "r"}[i]})}}
		}), ReasonLoopElement},
	}

	for _, c := range cases {
		_, reason, _ := DetectLoop(c.views, Noise{})
		require.Equal(t, c.reason, reason, c.name)
	}
}

func TestEntryFields(t *testing.T) {
	list := Candidate{Path: []string{"tenants"}, Kind: List, Index: 2}

	fs, reason, _ := EntryFields(list, []any{map[string]any{"name": "a", "quota": float64(1), "tags": []any{"x"}}})
	require.Empty(t, reason)
	require.Len(t, fs, 3)
	require.Equal(t, []string{"tenants", "[0]", "name"}, fs[0].Path)
	require.Equal(t, "integer", FieldType(fs[1]))
	require.Equal(t, "[]string", FieldType(fs[2]))

	fs, reason, _ = EntryFields(list, []any{"a", "b"})
	require.Empty(t, reason)
	require.Equal(t, String, fs[0].Kind)

	_, reason, _ = EntryFields(list, []any{map[string]any{"a": "1"}, map[string]any{"b": "2"}})
	require.Equal(t, ReasonLoopElement, reason)

	_, reason, _ = EntryFields(list, []any{map[string]any{"a": nil}})
	require.Equal(t, ReasonLoopElement, reason)
}

func TestClassifyLoopField(t *testing.T) {
	l := &Loop{Positions: []Position{{Source: loopSrc, PerEntry: 1, GVK: "v1/ConfigMap"}}}
	base := render(svc, doc{loopSrc, cm("r-a", map[string]any{"q": "1"})})
	field := Candidate{Path: []string{"tenants", "[0]", "name"}, Kind: String, Default: "a", Index: 7}

	p := func(point int) *View {
		return render(svc, doc{loopSrc, cm("r-"+Format(field.Value(point)), map[string]any{"q": "1"})})
	}

	ts, reason, _ := ClassifyLoopField(l, field, base, Noise{}, p(1), p(2), nil, nil)
	require.Empty(t, reason)
	require.Len(t, ts, 1)
	require.Equal(t, Span, ts[0].Mode)

	// A field that changes another object is not a pure loop field.
	shared := render(doc{otherSrc, map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "svc-a"}}}, doc{loopSrc, cm("r-a", map[string]any{"q": "1"})})
	q := func(point int) *View {
		other := doc{otherSrc, map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "svc-" + Format(field.Value(point))}}}
		return render(other, doc{loopSrc, cm("r-a", map[string]any{"q": "1"})})
	}

	_, reason, _ = ClassifyLoopField(l, field, shared, Noise{}, q(1), q(2), nil, nil)
	require.Equal(t, ReasonLoopElement, reason)
}
