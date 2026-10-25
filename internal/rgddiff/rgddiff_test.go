package rgddiff

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func rgd(kind string, spec map[string]any, res ...[2]string) map[string]any {
	var rs []any
	for _, r := range res {
		rs = append(rs, map[string]any{"id": r[0], "template": map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": r[1]},
		}})
	}

	return map[string]any{"spec": map[string]any{
		"schema":    map[string]any{"kind": kind, "apiVersion": "v1alpha1", "spec": spec},
		"resources": rs,
	}}
}

func severities(r *Result) map[string]Severity {
	m := map[string]Severity{}
	for _, c := range r.Changes {
		m[c.Path] = c.Severity
	}

	return m
}

func TestCompare(t *testing.T) {
	old := rgd("App", map[string]any{
		"replicas": "integer | default=1",
		"image":    map[string]any{"tag": `string | default="1"`},
		"gone":     "string",
		"opt":      `string | default="x"`,
		"size":     `string | default="1Gi"`,
	}, [2]string{"cm", "${schema.metadata.name}-a"}, [2]string{"old", "${schema.metadata.name}-b"})

	updated := rgd("App2", map[string]any{
		"replicas": `string | default="1"`,
		"image":    map[string]any{"tag": `string | default="2"`},
		"opt":      "string",
		"new":      "string | required=true",
		"extra":    "boolean | default=false",
		"size":     `string | default="1Gi" immutable=true`,
	}, [2]string{"renamedId", "${schema.metadata.name}-a"})

	s := severities(Compare(old, updated))
	require.Equal(t, Breaking, s["spec.schema.kind"])
	require.Equal(t, Breaking, s["spec.replicas"])
	require.Equal(t, Breaking, s["spec.gone"])
	require.Equal(t, Breaking, s["spec.opt"])
	require.Equal(t, Breaking, s["spec.new"])
	require.Equal(t, Compatible, s["spec.extra"])
	require.Equal(t, Compatible, s["spec.image.tag"])
	require.Equal(t, Compatible, s["spec.size"])
	require.Equal(t, Destructive, s["spec.resources.old"])
	_, renamed := s["spec.resources.renamedId"]
	require.False(t, renamed, "an id rename with the same GVK and name is not a change")
}

func TestTemplateChangeIsCompatible(t *testing.T) {
	a := rgd("App", nil, [2]string{"cm", "${schema.metadata.name}-a"})
	b := rgd("App", nil, [2]string{"cm", "${schema.metadata.name}-a"})
	b["spec"].(map[string]any)["resources"].([]any)[0].(map[string]any)["template"].(map[string]any)["data"] = map[string]any{"k": "v"}

	r := Compare(a, b)
	require.Equal(t, Compatible, severities(r)["spec.resources.cm"])
	require.Contains(t, r.Changes[0].Message, "template changed")
}

func TestIncludeWhenChangeIsCompatible(t *testing.T) {
	a := rgd("App", nil, [2]string{"cm", "${schema.metadata.name}-a"})
	b := rgd("App", nil, [2]string{"cm", "${schema.metadata.name}-a"})
	b["spec"].(map[string]any)["resources"].([]any)[0].(map[string]any)["includeWhen"] = []any{"${schema.spec.on}"}

	r := Compare(a, b)
	require.Equal(t, Compatible, severities(r)["spec.resources.cm"])
	require.Contains(t, r.Changes[0].Message, "includeWhen changed")
}

func TestNoChanges(t *testing.T) {
	a := rgd("App", map[string]any{"x": "string"})
	r := Compare(a, a)
	require.Empty(t, r.Changes)
	require.False(t, r.Breaking())
}

func TestParseFieldMarkers(t *testing.T) {
	typ, m := parseField(`string | default="a b \"c\"" immutable=true`)
	require.Equal(t, "string", typ)
	require.Equal(t, `"a b \"c\""`, m["default"])
	require.Equal(t, "true", m["immutable"])

	typ, m = parseField(`[]string | default=["x", "y"] required=true`)
	require.Equal(t, "[]string", typ)
	require.Equal(t, `["x", "y"]`, m["default"])
	require.Equal(t, "true", m["required"])
}
