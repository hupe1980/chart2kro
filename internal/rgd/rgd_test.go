package rgd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/kroval"
	"github.com/hupe1980/chart2kro/internal/oracle"
)

func rgdWith(spec map[string]any, resources []any) map[string]any {
	return map[string]any{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata":   map[string]any{"name": "t"},
		"spec": map[string]any{
			"schema":    map[string]any{"apiVersion": "v1alpha1", "kind": "T", "group": "kro.run", "spec": spec},
			"resources": resources,
		},
	}
}

func configMap(data map[string]any) []any {
	return []any{map[string]any{"id": "cm", "template": map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": "${schema.metadata.name}"},
		"data":     data,
	}}}
}

// Defaults must round-trip through kro's SimpleSchema parser exactly.
func TestDefaultEncodingRoundTrip(t *testing.T) {
	values := []string{
		"plain", `say "hi" | pipe`, `back\slash`, "multi\nline", "${VAR:-x}", "tab\there", "ünïcödé", "",
		`{"json": true}`, "a=b c=d", `\"`,
	}

	spec := map[string]any{}
	data := map[string]any{}

	for i, v := range values {
		k := "v" + string(rune('a'+i))
		spec[k] = marker(Field{Path: []string{k}, Type: "string", Default: v})
		data[k] = "${schema.spec." + k + "}"
	}

	val, err := kroval.New("", nil)
	require.NoError(t, err)

	g, err := val.Validate(context.Background(), rgdWith(spec, configMap(data)))
	require.NoError(t, err)

	nodes, err := oracle.Instantiate(context.Background(), g, "x", "ns", nil)
	require.NoError(t, err)

	got := nodes[0].Objects[0]["data"].(map[string]any)
	for i, v := range values {
		require.Equal(t, v, got["v"+string(rune('a'+i))], "default %q", v)
	}
}

func TestEscapedLiteralAndTemplates(t *testing.T) {
	data := map[string]any{
		"literal": escapeLiteral("echo ${HOME} ${A:-${B}}"),
		"mixed":   template(segments("pre-NAME-post", []hole{{start: 4, end: 8, expr: refName, str: true}})),
		"number":  template(segments("port=80", []hole{{start: 5, end: 7, expr: "schema.spec.port", str: false}})),
	}

	val, err := kroval.New("", nil)
	require.NoError(t, err)

	g, err := val.Validate(context.Background(), rgdWith(map[string]any{"port": "integer | default=80"}, configMap(data)))
	require.NoError(t, err)

	nodes, err := oracle.Instantiate(context.Background(), g, "inst", "ns", nil)
	require.NoError(t, err)

	got := nodes[0].Objects[0]["data"].(map[string]any)
	require.Equal(t, "echo ${HOME} ${A:-${B}}", got["literal"])
	require.Equal(t, "pre-inst-post", got["mixed"])
	require.Equal(t, "port=80", got["number"])
}

// The reserved-id mirror must agree with kro's builder.
func TestReservedIDsMatchKro(t *testing.T) {
	val, err := kroval.New("", nil)
	require.NoError(t, err)

	for id := range reservedIDs {
		if !lowerCamel(id) {
			continue // kro rejects these by pattern anyway
		}

		res := configMap(map[string]any{"a": "b"})
		res[0].(map[string]any)["id"] = id

		_, err := val.Validate(context.Background(), rgdWith(map[string]any{}, res))
		require.Error(t, err, "kro should reject reserved id %q", id)
	}

	res := configMap(map[string]any{"a": "b"})
	res[0].(map[string]any)["id"] = "serviceHeadless"
	_, err = val.Validate(context.Background(), rgdWith(map[string]any{}, res))
	require.NoError(t, err)
}

func lowerCamel(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}

	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}

	return true
}

func TestAssignIDs(t *testing.T) {
	ids := assignIDs([]idInput{
		{key: "a", kind: "Service", name: "headless"},
		{key: "b", kind: "Service", name: ""},
		{key: "c", kind: "Deployment", name: ""},
		{key: "d", kind: "Namespace", name: ""},
		{key: "e", kind: "HTTPRoute", name: ""},
	})

	require.Equal(t, "serviceHeadless", ids["a"])
	require.Equal(t, "service", ids["b"])
	require.Equal(t, "deployment", ids["c"])
	require.Equal(t, "namespaceRes", ids["d"])
	require.Equal(t, "httpRoute", ids["e"])
}

func TestSpecRefEscaping(t *testing.T) {
	ref, ok := SpecRef([]string{"serviceMonitor", "namespace"})
	require.True(t, ok)
	require.Equal(t, "schema.spec.serviceMonitor.__namespace__", ref)

	ref, ok = SpecRef([]string{"my-key", "redis.conf"})
	require.True(t, ok)
	require.Equal(t, "schema.spec.my__dash__key.redis__dot__conf", ref)

	_, ok = SpecRef([]string{"1st"})
	require.False(t, ok)
}

// kro accepts the markers chart2kro emits together.
func TestMarkersAcceptedByKro(t *testing.T) {
	spec := map[string]any{
		"a": marker(Field{Type: "string", Default: "x", Immutable: true}),
		"b": marker(Field{Type: "integer", Default: float64(3), Immutable: true}),
		"c": marker(Field{Type: "string", Required: true, Immutable: true}),
		"d": marker(Field{Type: "[]string", Default: []any{}}),
		"e": marker(Field{Type: "object", Default: map[string]any{}}),
	}

	data := map[string]any{"a": "${schema.spec.a}", "b": "${string(schema.spec.b)}", "c": "${schema.spec.c}"}

	val, err := kroval.New("", nil)
	require.NoError(t, err)

	g, err := val.Validate(context.Background(), rgdWith(spec, configMap(data)))
	require.NoError(t, err)

	nodes, err := oracle.Instantiate(context.Background(), g, "x", "ns", map[string]any{"c": "given"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"a": "x", "b": "3", "c": "given"}, nodes[0].Objects[0]["data"])

	_, err = oracle.Instantiate(context.Background(), g, "x", "ns", nil)
	require.Error(t, err, "required field missing")
}

// Descriptions must reach the generated CRD unchanged, whatever the comment contains.
func TestDescriptionMarkerReachesCRD(t *testing.T) {
	texts := []string{"Number of replicas", `Say "hi" | pipe`, `back\slash`, "Literal ${schema.spec.x} stays text", "ünïcödé…"}

	spec := map[string]any{}
	for i, d := range texts {
		k := "v" + string(rune('a'+i))
		spec[k] = marker(Field{Path: []string{k}, Type: "string", Default: "x", Description: d})
	}

	val, err := kroval.New("", nil)
	require.NoError(t, err)

	g, err := val.Validate(context.Background(), rgdWith(spec, configMap(map[string]any{"k": "v"})))
	require.NoError(t, err)

	props := g.CRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties
	for i, d := range texts {
		require.Equal(t, d, props["v"+string(rune('a'+i))].Description)
	}
}
