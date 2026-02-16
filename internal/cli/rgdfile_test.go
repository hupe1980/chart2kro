package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRGDFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validRGDYAML), 0o644)) //nolint:gosec // test

	rgdMap, err := loadRGDFile(path, 7)
	require.NoError(t, err)
	assert.Equal(t, "kro.run/v1alpha1", rgdMap["apiVersion"])
	assert.Equal(t, "ResourceGraphDefinition", rgdMap["kind"])
}

func TestLoadRGDFile_NotFound(t *testing.T) {
	_, err := loadRGDFile("/nonexistent/file.yaml", 7)
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.Code)
}

func TestLoadRGDFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("not: valid: yaml: ["), 0o644)) //nolint:gosec // test

	_, err := loadRGDFile(path, 7)
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
}

func TestLoadRGDFile_CustomExitCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("not: valid: yaml: ["), 0o644)) //nolint:gosec // test

	_, err := loadRGDFile(path, 1)
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.Code, "should use the provided syntax error code")
}
