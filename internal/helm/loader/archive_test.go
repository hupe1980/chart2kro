package loader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestArchive creates a .tgz chart archive in dir and returns the path.
func buildTestArchive(t *testing.T, dir, name, version string) string {
	t.Helper()

	archivePath := filepath.Join(dir, name+"-"+version+".tgz")

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	files := map[string]string{
		name + "/Chart.yaml":  "apiVersion: v2\nname: " + name + "\nversion: " + version + "\ndescription: Test chart\ntype: application\n",
		name + "/values.yaml": "replicaCount: 1\n",
		name + "/templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-` + name + `
spec:
  replicas: {{ .Values.replicaCount }}
`,
	}

	for path, content := range files {
		hdr := &tar.Header{
			Name: path,
			Mode: 0o600,
			Size: int64(len(content)),
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	require.NoError(t, os.WriteFile(archivePath, buf.Bytes(), 0o600))

	return archivePath
}

func TestArchiveLoader_Load(t *testing.T) {
	dir := t.TempDir()
	archivePath := buildTestArchive(t, dir, "my-chart", "1.0.0")

	loader := NewArchiveLoader()
	ch, err := loader.Load(context.Background(), archivePath, LoadOptions{})
	require.NoError(t, err)

	assert.Equal(t, "my-chart", ch.Metadata.Name)
	assert.Equal(t, "1.0.0", ch.Metadata.Version)
	assert.NotEmpty(t, ch.Templates)
}

func TestArchiveLoader_Load_FileNotFound(t *testing.T) {
	loader := NewArchiveLoader()
	_, err := loader.Load(context.Background(), "/nonexistent/chart.tgz", LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "archive")
}

func TestArchiveLoader_Load_ExceedsMaxSize(t *testing.T) {
	dir := t.TempDir()
	archivePath := buildTestArchive(t, dir, "big-chart", "0.1.0")

	loader := NewArchiveLoader()
	_, err := loader.Load(context.Background(), archivePath, LoadOptions{
		MaxArchiveSize: 10, // 10 bytes — way too small
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeding maximum")
}

func TestArchiveLoader_LoadFromReader(t *testing.T) {
	dir := t.TempDir()
	archivePath := buildTestArchive(t, dir, "reader-chart", "2.0.0")

	data, err := os.ReadFile(archivePath) //nolint:gosec // archivePath is a test-created path
	require.NoError(t, err)

	loader := NewArchiveLoader()
	ch, err := loader.LoadFromReader(bytes.NewReader(data), LoadOptions{})
	require.NoError(t, err)

	assert.Equal(t, "reader-chart", ch.Metadata.Name)
}

func TestArchiveLoader_LoadFromReader_ExceedsMaxSize(t *testing.T) {
	dir := t.TempDir()
	archivePath := buildTestArchive(t, dir, "oversized", "1.0.0")

	data, err := os.ReadFile(archivePath) //nolint:gosec // archivePath is a test-created path
	require.NoError(t, err)

	loader := NewArchiveLoader()
	_, err = loader.LoadFromReader(bytes.NewReader(data), LoadOptions{
		MaxArchiveSize: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}

func TestArchiveLoader_LoadFromReader_Corrupted(t *testing.T) {
	loader := NewArchiveLoader()
	_, err := loader.LoadFromReader(bytes.NewReader([]byte("not a valid archive")), LoadOptions{})
	require.Error(t, err)
}
