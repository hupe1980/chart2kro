package kroval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func load(t *testing.T, name string) map[string]any {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, yaml.Unmarshal(b, &m))

	return m
}

func TestGoodAndBad(t *testing.T) {
	v, err := New("", nil)
	require.NoError(t, err)

	_, err = v.Validate(context.Background(), load(t, "good.yaml"))
	require.NoError(t, err)

	bad := map[string]string{
		"bad-dependson.yaml":          "dependsOn",
		"bad-hyphen-id.yaml":          "lower camelCase",
		"bad-schema-apiversion.yaml":  "spec.schema.apiVersion",
		"bad-self-readywhen.yaml":     "self",
		"bad-template-field.yaml":     "typo",
		"bad-type-mismatch.yaml":      "type mismatch",
		"bad-unknown-field-path.yaml": "imagee",
		"bad-unknown-kind.yaml":       "ConfigMapp",
	}

	for f, want := range bad {
		t.Run(f, func(t *testing.T) {
			_, err := v.Validate(context.Background(), load(t, f))
			require.Error(t, err)
			require.Contains(t, err.Error(), want)
		})
	}
}

func TestInputNotModified(t *testing.T) {
	v, err := New("", nil)
	require.NoError(t, err)

	in := load(t, "good.yaml")
	before, _ := yaml.Marshal(in)
	_, _ = v.Validate(context.Background(), in)
	after, _ := yaml.Marshal(in)
	require.Equal(t, string(before), string(after))
}

func TestScope(t *testing.T) {
	v, err := New("", nil)
	require.NoError(t, err)

	ns, known := v.Scope("apps/v1", "Deployment")
	require.True(t, known)
	require.True(t, ns)

	ns, known = v.Scope("rbac.authorization.k8s.io/v1", "ClusterRole")
	require.True(t, known)
	require.False(t, ns)

	_, known = v.Scope("example.com/v1", "Widget")
	require.False(t, known)
}

func TestCRDOverlay(t *testing.T) {
	crd := load(t, "widgets.yaml")
	v, err := New("", []map[string]any{crd})
	require.NoError(t, err)

	ns, known := v.Scope("example.com/v1", "Widget")
	require.True(t, known)
	require.True(t, ns)
}

func TestKubeVersions(t *testing.T) {
	require.NotEmpty(t, KubeVersions())
	require.Equal(t, "1.34", NormalizeKubeVersion("v1.34.2"))
	_, err := New("1.2", nil)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "no validation catalog"))
}

func TestValidateYAMLAggregates(t *testing.T) {
	v, err := New("", nil)
	require.NoError(t, err)

	good, _ := os.ReadFile("testdata/good.yaml")
	bad, _ := os.ReadFile("testdata/bad-hyphen-id.yaml")
	cm := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n")

	require.NoError(t, v.ValidateYAML(context.Background(), [][]byte{good, cm}))

	err = v.ValidateYAML(context.Background(), [][]byte{good, bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lower camelCase")

	require.Error(t, v.ValidateYAML(context.Background(), [][]byte{[]byte(":\n- not yaml")}))

	_, err = ParseCRDs([]map[string]any{{"kind": "CustomResourceDefinition", "spec": "nonsense"}})
	require.Error(t, err)
	require.NotEmpty(t, KroVersion())
	require.Equal(t, NormalizeKubeVersion("1.37"), v.KubeVersion())
}

func TestEveryCatalogValidates(t *testing.T) {
	require.GreaterOrEqual(t, len(KubeVersions()), 3)

	for _, kv := range KubeVersions() {
		v, err := New(kv, nil)
		require.NoError(t, err, kv)

		_, err = v.Validate(context.Background(), load(t, "good.yaml"))
		require.NoError(t, err, kv)
	}
}
