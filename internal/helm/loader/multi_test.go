package loader

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiLoader_Load_Directory(t *testing.T) {
	dir := t.TempDir()
	chartDir := createTestChart(t, dir, "ml-dir", "1.0.0")

	ml := NewMultiLoader()
	ch, err := ml.Load(context.Background(), chartDir, LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, "ml-dir", ch.Metadata.Name)
}

func TestMultiLoader_Load_Archive(t *testing.T) {
	dir := t.TempDir()
	archivePath := buildTestArchive(t, dir, "ml-archive", "2.0.0")

	ml := NewMultiLoader()
	ch, err := ml.Load(context.Background(), archivePath, LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, "ml-archive", ch.Metadata.Name)
}

func TestMultiLoader_Load_Repository(t *testing.T) {
	srv := newTestRepoServer(t, "ml-repo", "3.0.0")

	ml := NewMultiLoader()
	ch, err := ml.Load(context.Background(), "myrepo/ml-repo", LoadOptions{
		RepoURL: srv.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "ml-repo", ch.Metadata.Name)
}

func TestMultiLoader_Load_UnknownType(t *testing.T) {
	ml := NewMultiLoader()
	_, err := ml.Load(context.Background(), "just-a-name", LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot determine")
}

func TestMultiLoader_Load_PlainNameWithRepoURL(t *testing.T) {
	srv := newTestRepoServer(t, "plain-chart", "1.0.0")

	ml := NewMultiLoader()
	ch, err := ml.Load(context.Background(), "plain-chart", LoadOptions{
		RepoURL: srv.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "plain-chart", ch.Metadata.Name)
	assert.Equal(t, "1.0.0", ch.Metadata.Version)
}

func TestMultiLoader_Load_Empty(t *testing.T) {
	ml := NewMultiLoader()
	_, err := ml.Load(context.Background(), "", LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty chart reference")
}

func TestNewMultiLoader(t *testing.T) {
	ml := NewMultiLoader()
	assert.NotNil(t, ml.directory)
	assert.NotNil(t, ml.archive)
	assert.NotNil(t, ml.oci)
	assert.NotNil(t, ml.repository)
}
