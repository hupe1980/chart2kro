package transform_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/transform"
)

func TestLoadCustomReadyConditions(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		content := `Deployment:
  - "${self.status.conditions != null}"
Service:
  - "${self.status.loadBalancer.ingress != null}"
`
		path := writeTemp(t, content)

		conditions, err := transform.LoadCustomReadyConditions(path)
		require.NoError(t, err)
		require.Len(t, conditions, 2)
		assert.Equal(t, []string{`${self.status.conditions != null}`}, conditions["Deployment"])
		assert.Equal(t, []string{`${self.status.loadBalancer.ingress != null}`}, conditions["Service"])
	})

	t.Run("multiple conditions per kind", func(t *testing.T) {
		content := `Job:
  - "${self.status.succeeded > 0}"
  - "${self.status.failed == 0}"
`
		path := writeTemp(t, content)

		conditions, err := transform.LoadCustomReadyConditions(path)
		require.NoError(t, err)
		require.Len(t, conditions["Job"], 2)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := transform.LoadCustomReadyConditions("/nonexistent/file.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reading ready-conditions file")
	})

	t.Run("invalid YAML", func(t *testing.T) {
		path := writeTemp(t, "{{invalid}}")

		_, err := transform.LoadCustomReadyConditions(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parsing ready-conditions file")
	})

	t.Run("empty file", func(t *testing.T) {
		path := writeTemp(t, "")

		_, err := transform.LoadCustomReadyConditions(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is empty")
	})
}

func TestResolveReadyWhen(t *testing.T) {
	custom := map[string][]string{
		"Deployment": {"${self.status.conditions != null}"},
	}

	t.Run("custom overrides default", func(t *testing.T) {
		gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
		result := transform.ResolveReadyWhen(gvk, custom)
		require.Len(t, result, 1)
		assert.Equal(t, "${self.status.conditions != null}", result[0])
	})

	t.Run("falls back to default", func(t *testing.T) {
		gvk := schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}
		result := transform.ResolveReadyWhen(gvk, custom)
		require.Len(t, result, 1)
		assert.Equal(t, "${self.status.succeeded > 0}", result[0])
	})

	t.Run("nil custom uses defaults", func(t *testing.T) {
		gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
		result := transform.ResolveReadyWhen(gvk, nil)
		require.Len(t, result, 1)
		assert.Contains(t, result[0], "availableReplicas")
	})

	t.Run("unknown kind returns nil", func(t *testing.T) {
		gvk := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "MyResource"}
		result := transform.ResolveReadyWhen(gvk, nil)
		assert.Empty(t, result)
	})
}

// writeTemp creates a temporary file with the given content and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "conditions.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}
