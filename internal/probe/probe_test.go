package probe

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/chart"
)

func view(objs ...map[string]any) *View {
	var co []chart.Object
	for i, o := range objs {
		co = append(co, chart.Object{Source: "c/templates/t.yaml", Ordinal: i, Obj: o})
	}

	return NewView(co)
}

func deploy(image string, args []any, extra map[string]any) map[string]any {
	c := map[string]any{"name": "app", "image": image, "args": args}
	for k, v := range extra {
		c[k] = v
	}

	return map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "x"},
		"spec":     map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{c}}}},
	}
}

func TestCollectAndGlobs(t *testing.T) {
	vals := map[string]any{
		"image":       map[string]any{"repository": "nginx", "tag": "1.25"},
		"replicas":    float64(2),
		"ratio":       0.5,
		"enabled":     true,
		"annotations": map[string]any{},
		"args":        []any{"--a"},
		"nothing":     nil,
	}

	all := Collect(vals, nil, nil)
	require.Len(t, all, 8)

	kinds := map[string]Kind{}
	for _, c := range all {
		kinds[c.Dotted()] = c.Kind
	}

	require.Equal(t, Int, kinds["replicas"])
	require.Equal(t, Float, kinds["ratio"])
	require.Equal(t, Map, kinds["annotations"])
	require.Equal(t, List, kinds["args"])
	require.Equal(t, Null, kinds["nothing"])

	exp := Collect(vals, []string{"image"}, []string{"image.tag"})
	require.Len(t, exp, 1)
	require.Equal(t, "image.repository", exp[0].Dotted())
	require.True(t, MatchAny([]string{"**.tag"}, "a.b.tag"))
}

func TestProbeValuesAreTypedAndDistinct(t *testing.T) {
	c := Candidate{Kind: Int, Default: float64(3), Index: 7}
	require.Equal(t, float64(100014), c.Value(1))
	require.Equal(t, "100014", Format(c.Value(1))) // no exponent
	s := Candidate{Kind: String, Default: "x", Index: 2}
	require.NotEqual(t, s.Value(1), s.Value(2))
	b := Candidate{Kind: Bool, Default: true}
	require.Equal(t, false, b.Value(1))
}

func TestClassifyInterpolationAndWhole(t *testing.T) {
	c := Candidate{Path: []string{"tag"}, Kind: String, Default: "1.25", Index: 0}
	base := view(deploy("nginx:1.25", []any{}, nil))
	p1 := view(deploy("nginx:"+c.Value(1).(string), []any{}, nil))
	p2 := view(deploy("nginx:"+c.Value(2).(string), []any{}, nil))

	r := Classify(c, base, Noise{}, p1, p2, nil, nil)
	require.Equal(t, Exposed, r.Outcome)
	require.Len(t, r.Targets, 1)
	require.Equal(t, Span, r.Targets[0].Mode)
	require.Equal(t, [][2]int{{6, 10}}, r.Targets[0].Spans)
}

// A list item that appears with a value is structural, never re-attributed by index (audit C3).
func TestClassifyListLengthChangeIsStructural(t *testing.T) {
	c := Candidate{Path: []string{"tls"}, Kind: Bool, Default: false}
	base := view(deploy("img", []any{"--port=1"}, nil))
	flip := view(deploy("img", []any{"--port=1", "--secure"}, nil))

	r := Classify(c, base, Noise{}, flip, flip, nil, nil)
	require.Equal(t, Frozen, r.Outcome)
	require.Equal(t, ReasonToggleFields, r.Reason)
}

func TestClassifyTransformedIsFrozen(t *testing.T) {
	c := Candidate{Path: []string{"name"}, Kind: String, Default: "abc", Index: 0}
	base := view(deploy("ABC", []any{}, nil))
	p1 := view(deploy("C2KV0XA", []any{}, nil))
	p2 := view(deploy("C2KV0YB", []any{}, nil))

	r := Classify(c, base, Noise{}, p1, p2, nil, nil)
	require.Equal(t, Frozen, r.Outcome)
	require.Equal(t, ReasonTransformed, r.Reason)
}

func TestClassifyRenderFailureIsLocal(t *testing.T) {
	c := Candidate{Path: []string{"x"}, Kind: String, Default: "a"}
	r := Classify(c, view(deploy("a", nil, nil)), Noise{}, nil, nil, errors.New("boom\nmore"), nil)
	require.Equal(t, Failed, r.Outcome)
	require.Equal(t, "boom", r.Detail)
}

func TestClassifyBoolObjectsAndTernary(t *testing.T) {
	c := Candidate{Path: []string{"ingress", "enabled"}, Kind: Bool, Default: false}
	base := view(deploy("img", []any{}, map[string]any{"imagePullPolicy": "IfNotPresent"}))
	ing := map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": map[string]any{"name": "x"}}
	flip := view(deploy("img", []any{}, map[string]any{"imagePullPolicy": "Always"}), ing)

	r := Classify(c, base, Noise{}, flip, flip, nil, nil)
	require.Equal(t, Exposed, r.Outcome)
	require.Len(t, r.Added, 1)
	require.Len(t, r.Targets, 1)
	require.Equal(t, Ternary, r.Targets[0].Mode)
	require.Equal(t, "IfNotPresent", r.Targets[0].Default)
	require.Equal(t, "Always", r.Targets[0].Flipped)
}

func TestNoiseAndSecretDecoding(t *testing.T) {
	sec := func(v string) map[string]any {
		return map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "s"}, "data": map[string]any{"p": v}}
	}

	a := view(sec("YWJj")) // abc
	b := view(sec("eHl6")) // xyz
	require.Equal(t, "abc", a.Objects[a.Order[0]].Value["data"].(map[string]any)["p"])

	n := Noise{}
	AddNoise(n, a, b)
	require.True(t, n.Has(a.Order[0], []any{"data", "p"}))
}
