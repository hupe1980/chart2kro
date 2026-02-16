package loader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryLoader_Load(t *testing.T) {
	dir := t.TempDir()
	chartDir := createTestChart(t, dir, "my-chart", "1.0.0")

	loader := NewDirectoryLoader()
	ch, err := loader.Load(context.Background(), chartDir, LoadOptions{})
	require.NoError(t, err)

	assert.Equal(t, "my-chart", ch.Metadata.Name)
	assert.Equal(t, "1.0.0", ch.Metadata.Version)
	assert.NotEmpty(t, ch.Templates)
	assert.NotNil(t, ch.Values)
}

func TestDirectoryLoader_Load_InvalidPath(t *testing.T) {
	loader := NewDirectoryLoader()
	_, err := loader.Load(context.Background(), "/nonexistent/path", LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chart directory")
}

func TestDirectoryLoader_Load_NotADirectory(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-dir-*")
	require.NoError(t, err)
	_ = f.Close()

	loader := NewDirectoryLoader()
	_, err = loader.Load(context.Background(), f.Name(), LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a directory")
}

func TestDirectoryLoader_Load_NoChartYAML(t *testing.T) {
	dir := t.TempDir()
	loader := NewDirectoryLoader()
	_, err := loader.Load(context.Background(), dir, LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Chart.yaml")
}

func TestDirectoryLoader_Load_WithCRDs(t *testing.T) {
	dir := t.TempDir()
	chartDir := createTestChart(t, dir, "crd-chart", "0.1.0")

	// Add a CRD file.
	crdsDir := filepath.Join(chartDir, "crds")
	require.NoError(t, os.MkdirAll(crdsDir, 0o750))

	crd := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: myresources.example.com
`
	require.NoError(t, os.WriteFile(filepath.Join(crdsDir, "myresource.yaml"), []byte(crd), 0o600))

	loader := NewDirectoryLoader()
	ch, err := loader.Load(context.Background(), chartDir, LoadOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, ch.CRDObjects())
}

func TestDirectoryLoader_Load_WithSchema(t *testing.T) {
	dir := t.TempDir()
	chartDir := createTestChart(t, dir, "schema-chart", "0.1.0")

	schema := `{"$schema": "http://json-schema.org/schema#", "type": "object"}`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.schema.json"), []byte(schema), 0o600))

	loader := NewDirectoryLoader()
	ch, err := loader.Load(context.Background(), chartDir, LoadOptions{})
	require.NoError(t, err)
	assert.NotNil(t, ch.Schema)
}
