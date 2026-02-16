package output

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdoutWriter_Write(t *testing.T) {
	var buf bytes.Buffer
	w := NewStdoutWriter(&buf)

	data := []byte("apiVersion: kro.run/v1alpha1\nkind: ResourceGraphDefinition\n")
	require.NoError(t, w.Write(data))
	assert.Equal(t, string(data), buf.String())
}

func TestStdoutWriter_NilDefault(t *testing.T) {
	// When nil is passed, it defaults to os.Stdout — just verify it doesn't panic.
	w := NewStdoutWriter(nil)
	assert.NotNil(t, w)
}

func TestFileWriter_Write(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output", "test.yaml")

	w := NewFileWriter(path)
	data := []byte("apiVersion: kro.run/v1alpha1\nkind: ResourceGraphDefinition\n")
	require.NoError(t, w.Write(data))

	// Verify file exists with correct content.
	got, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Equal(t, string(data), string(got))

	// Verify file permissions.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestFileWriter_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "test.yaml")

	w := NewFileWriter(path)
	require.NoError(t, w.Write([]byte("test")))

	_, err := os.Stat(path)
	require.NoError(t, err)
}

func TestFileWriter_CustomPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")

	w := NewFileWriter(path, WithPermissions(0o600))
	require.NoError(t, w.Write([]byte("secret")))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestFileWriter_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.yaml")

	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644)) //nolint:gosec // test

	w := NewFileWriter(path)
	require.NoError(t, w.Write([]byte("new")))

	got, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

func TestFileWriter_Path(t *testing.T) {
	w := NewFileWriter("/tmp/test.yaml")
	assert.Equal(t, "/tmp/test.yaml", w.Path())
}

func TestFileWriter_InvalidPath(t *testing.T) {
	w := NewFileWriter("/dev/null/impossible/path.yaml")
	err := w.Write([]byte("data"))
	assert.Error(t, err)
}
