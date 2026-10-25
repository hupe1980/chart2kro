package rgd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/chart"
	"github.com/hupe1980/chart2kro/internal/kroval"
	"github.com/hupe1980/chart2kro/internal/manifest"
	"github.com/hupe1980/chart2kro/internal/oracle"
	"github.com/hupe1980/chart2kro/internal/probe"
)

const loopSrc = "c/templates/tenants.yaml"

// loopInput builds an assembly input whose list "tenants" (object entries with name
// and quota) emits one ConfigMap per entry named "<release>-<name>".
func loopInput(entries []any, fields []probe.LoopField, index []probe.Target, elem string, objs ...map[string]any) Input {
	var co []chart.Object
	for i, o := range objs {
		co = append(co, chart.Object{Source: loopSrc, Ordinal: i, Obj: o})
	}

	base := probe.NewView(co)

	l := &probe.Loop{
		Positions: []probe.Position{{Source: loopSrc, PerEntry: 1, GVK: "v1/ConfigMap"}},
		Elem:      elem, Fields: fields, Index: index, Template: base,
	}

	cand := probe.Candidate{Path: []string{"tenants"}, Kind: probe.List, Default: entries, Elem: elem}

	return Input{Base: base, Base2: base, Noise: probe.Noise{}, Results: []probe.Result{{Candidate: cand, Outcome: probe.Exposed, Loop: l}}}
}

func key0() string { return loopSrc + "#0#v1/ConfigMap" }

func instantiate(t *testing.T, rgd map[string]any, spec map[string]any) ([]map[string]any, error) {
	t.Helper()

	v, err := kroval.New("", nil)
	require.NoError(t, err)

	g, err := v.Validate(context.Background(), rgd)
	require.NoError(t, err, "kro accepts the RGD")

	if _, err := oracle.Instance(context.Background(), g, "inst", "ns", spec); err != nil {
		return nil, err
	}

	nodes, err := oracle.Instantiate(context.Background(), g, "inst", "ns", spec)
	require.NoError(t, err)

	var out []map[string]any
	for _, n := range nodes {
		out = append(out, n.Objects...)
	}

	return out, nil
}

func names(objs []map[string]any) []string {
	var out []string
	for _, o := range objs {
		out = append(out, o["metadata"].(map[string]any)["name"].(string))
	}

	return out
}

func TestCollectionOfObjects(t *testing.T) {
	in := loopInput(
		[]any{map[string]any{"name": "a", "quota": "1"}},
		[]probe.LoopField{
			{Name: "name", Type: "string", Targets: []probe.Target{{Key: key0(), Path: manifest.Path{"metadata", "name"}, Mode: probe.Span, Spans: [][2]int{{6, 7}}}}},
			{Name: "quota", Type: "string", Targets: []probe.Target{{Key: key0(), Path: manifest.Path{"data", "quota"}, Mode: probe.Whole}}},
		}, nil, "object",
		cm("c2kra-a", map[string]any{"quota": "1"}),
	)

	out, err := Assemble(in, opts())
	require.NoError(t, err)
	require.Empty(t, out.Demote)
	require.Contains(t, out.Collections, "tenantConfigMap")

	for _, n := range []int{0, 1, 3} {
		var list []any
		for i := 0; i < n; i++ {
			list = append(list, map[string]any{"name": string(rune('p' + i)), "quota": "9"})
		}

		objs, err := instantiate(t, out.RGD, map[string]any{"tenants": list})
		require.NoError(t, err)
		require.Len(t, objs, n)

		if n == 3 {
			require.Equal(t, []string{"inst-p", "inst-q", "inst-r"}, names(objs))
			require.Equal(t, "9", objs[1]["data"].(map[string]any)["quota"])
			require.Equal(t, "1", objs[1]["metadata"].(map[string]any)["labels"].(map[string]any)["kro.run/collection-index"])
		}
	}

	_, err = instantiate(t, out.RGD, map[string]any{"tenants": []any{map[string]any{"name": "x", "quota": "1", "extra": "y"}}})
	require.Error(t, err, "unknown entry fields are rejected")

	_, err = instantiate(t, out.RGD, map[string]any{"tenants": []any{map[string]any{"name": "x", "quota": "1"}, map[string]any{"name": "x", "quota": "2"}}})
	require.Error(t, err, "duplicate names are rejected")
}

func TestCollectionOfStringsWithIndex(t *testing.T) {
	in := loopInput(
		[]any{"a"},
		[]probe.LoopField{{Name: "", Type: "string", Targets: []probe.Target{{Key: key0(), Path: manifest.Path{"data", "queue"}, Mode: probe.Whole}}}},
		[]probe.Target{{Key: key0(), Path: manifest.Path{"metadata", "name"}, Mode: probe.Span, Spans: [][2]int{{13, 14}}, Stringify: true}},
		"string",
		cm("c2kra-worker-0", map[string]any{"queue": "a"}),
	)

	out, err := Assemble(in, opts())
	require.NoError(t, err)
	require.Empty(t, out.Demote)

	objs, err := instantiate(t, out.RGD, map[string]any{"tenants": []any{"p", "q", "r"}})
	require.NoError(t, err)
	require.Equal(t, []string{"inst-worker-0", "inst-worker-1", "inst-worker-2"}, names(objs))
	require.Equal(t, "r", objs[2]["data"].(map[string]any)["queue"])

	_, err = instantiate(t, out.RGD, map[string]any{"tenants": []any{"p", "p"}})
	require.Error(t, err, "duplicate entries are rejected")
}

func TestCollectionNeedsDistinctNames(t *testing.T) {
	in := loopInput(
		[]any{map[string]any{"quota": "1"}},
		[]probe.LoopField{{Name: "quota", Type: "string", Targets: []probe.Target{{Key: key0(), Path: manifest.Path{"data", "quota"}, Mode: probe.Whole}}}},
		nil, "object",
		cm("c2kra-fixed", map[string]any{"quota": "1"}),
	)

	out, err := Assemble(in, opts())
	require.NoError(t, err)
	require.Equal(t, probe.ReasonLoopElement, out.Demote["tenants"])
}

func TestIteratorName(t *testing.T) {
	require.Equal(t, "tenant", iteratorName([]string{"tenants"}, nil))
	require.Equal(t, "itemEntry", iteratorName([]string{"items"}, nil), "kro reserves item")
	require.Equal(t, "queueEntry", iteratorName([]string{"queues"}, map[string]bool{"queue": true}))
	require.Equal(t, "class", iteratorName([]string{"class"}, nil))
	require.Equal(t, "TenantsEntry", elementTypeName([]string{"tenants"}))
}

func cm(name string, data map[string]any) map[string]any {
	return map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": name}, "data": data}
}

func opts() Options {
	return Options{Name: "t", Kind: "T", Group: "kro.run", APIVersion: "v1alpha1", ChartName: "c", ChartVersion: "1", Release: "c2kra", Namespace: "c2kna"}
}
