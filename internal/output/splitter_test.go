package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRGDWithResources() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata":   map[string]interface{}{"name": "nginx"},
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"apiVersion": "v1alpha1",
				"kind":       "NginxApp",
				"spec":       map[string]interface{}{"replicaCount": "integer"},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"kind": "Deployment"},
				},
				map[string]interface{}{
					"id":       "service",
					"template": map[string]interface{}{"kind": "Service"},
				},
			},
		},
	}
}

func TestSplit_ProducesCorrectFileCount(t *testing.T) {
	result, err := Split(testRGDWithResources(), DefaultSerializeOptions())
	require.NoError(t, err)
	assert.Len(t, result.Files, 2)
	assert.Contains(t, result.Files, "deployment.yaml")
	assert.Contains(t, result.Files, "service.yaml")
}

func TestSplit_EachFileContainsSingleResource(t *testing.T) {
	result, err := Split(testRGDWithResources(), DefaultSerializeOptions())
	require.NoError(t, err)

	for name, data := range result.Files {
		content := string(data)
		assert.Contains(t, content, "apiVersion: kro.run/v1alpha1", "file %s missing apiVersion", name)
		assert.Contains(t, content, "kind: ResourceGraphDefinition", "file %s missing kind", name)

		// Schema is preserved in each split file.
		assert.Contains(t, content, "kind: NginxApp", "file %s missing schema", name)
	}
}

func TestSplit_KustomizationListsAllFiles(t *testing.T) {
	result, err := Split(testRGDWithResources(), DefaultSerializeOptions())
	require.NoError(t, err)

	kContent := string(result.Kustomization)
	assert.Contains(t, kContent, "deployment.yaml")
	assert.Contains(t, kContent, "service.yaml")
	assert.Contains(t, kContent, "kind: Kustomization")
	assert.Contains(t, kContent, "chart2kro")
}

func TestSplit_NoResources(t *testing.T) {
	rgd := testRGD() // has empty resources
	_, err := Split(rgd, DefaultSerializeOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no resources")
}

func TestSplit_NoSpec(t *testing.T) {
	rgd := map[string]interface{}{"apiVersion": "v1"}
	_, err := Split(rgd, DefaultSerializeOptions())
	assert.Error(t, err)
}

func TestWriteSplit(t *testing.T) {
	dir := t.TempDir()
	result, err := Split(testRGDWithResources(), DefaultSerializeOptions())
	require.NoError(t, err)

	require.NoError(t, WriteSplit(dir, result))

	// Verify files exist.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	assert.Contains(t, names, "deployment.yaml")
	assert.Contains(t, names, "service.yaml")
	assert.Contains(t, names, "kustomization.yaml")
}

func TestWriteSplit_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "nested")
	result, err := Split(testRGDWithResources(), DefaultSerializeOptions())
	require.NoError(t, err)

	require.NoError(t, WriteSplit(dir, result))

	_, err = os.Stat(filepath.Join(dir, "kustomization.yaml"))
	require.NoError(t, err)
}

func TestValidateSplitFlags(t *testing.T) {
	assert.NoError(t, ValidateSplitFlags(false, ""))
	assert.NoError(t, ValidateSplitFlags(true, "/tmp/out"))
	assert.Error(t, ValidateSplitFlags(true, ""))
	assert.Error(t, ValidateSplitFlags(false, "/tmp/out"))
}

func TestFormatKustomizeDir(t *testing.T) {
	dir := t.TempDir()
	rgd := testRGDWithResources()

	require.NoError(t, FormatKustomizeDir(dir, rgd, DefaultSerializeOptions()))

	// Should have two files: nginx.yaml and kustomization.yaml.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	assert.Contains(t, names, "nginx.yaml")
	assert.Contains(t, names, "kustomization.yaml")

	// Verify kustomization references the RGD file.
	kData, err := os.ReadFile(filepath.Join(dir, "kustomization.yaml")) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(kData), "nginx.yaml")
}
