package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExport_YAML(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)

	stdout, _, err := executeCommand("export", path)
	require.NoError(t, err)
	assert.Contains(t, stdout, "apiVersion: kro.run/v1alpha1")
	assert.Contains(t, stdout, "kind: ResourceGraphDefinition")
}

func TestExport_YAMLExplicitFormat(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)

	stdout, _, err := executeCommand("export", "--format", "yaml", path)
	require.NoError(t, err)
	assert.Contains(t, stdout, "apiVersion: kro.run/v1alpha1")
}

func TestExport_JSON(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)

	stdout, _, err := executeCommand("export", "--format", "json", path)
	require.NoError(t, err)
	assert.Contains(t, stdout, `"apiVersion"`)
	assert.Contains(t, stdout, `"kro.run/v1alpha1"`)
}

func TestExport_YAMLToFile(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)
	outPath := filepath.Join(t.TempDir(), "exported.yaml")

	_, _, err := executeCommand("export", "--format", "yaml", "-o", outPath, path)
	require.NoError(t, err)

	data, err := os.ReadFile(outPath) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "apiVersion: kro.run/v1alpha1")
}

func TestExport_JSONToFile(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)
	outPath := filepath.Join(t.TempDir(), "exported.json")

	_, _, err := executeCommand("export", "--format", "json", "-o", outPath, path)
	require.NoError(t, err)

	data, err := os.ReadFile(outPath) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), `"apiVersion"`)
}

func TestExport_Kustomize(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)
	outDir := filepath.Join(t.TempDir(), "kustomize-out")

	_, _, err := executeCommand("export", "--format", "kustomize", "--output-dir", outDir, path)
	require.NoError(t, err)

	// Verify directory contents.
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	assert.Contains(t, names, "kustomization.yaml")
	assert.Contains(t, names, "test.yaml")

	// Verify kustomization references the RGD file.
	kData, err := os.ReadFile(filepath.Join(outDir, "kustomization.yaml")) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(kData), "test.yaml")
}

func TestExport_KustomizeRequiresOutputDir(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)

	_, _, err := executeCommand("export", "--format", "kustomize", path)
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 2, exitErr.Code)
}

func TestExport_UnsupportedFormat(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)

	_, _, err := executeCommand("export", "--format", "toml", path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestExport_WithComments(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)

	stdout, _, err := executeCommand("export", "--comments", path)
	require.NoError(t, err)
	assert.Contains(t, stdout, "# From Helm values: .Values.replicaCount")
}

func TestExport_FileNotFound(t *testing.T) {
	_, _, err := executeCommand("export", "/nonexistent/file.yaml")
	require.Error(t, err)
}

func TestExport_Help(t *testing.T) {
	stdout, _, err := executeCommand("export", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Export a generated ResourceGraphDefinition")
	assert.Contains(t, stdout, "--format")
	assert.Contains(t, stdout, "--output-dir")
}
