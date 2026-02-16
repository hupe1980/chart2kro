package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface conformance checks.
var (
	_ Loader = (*DirectoryLoader)(nil)
	_ Loader = (*ArchiveLoader)(nil)
	_ Loader = (*OCILoader)(nil)
	_ Loader = (*RepositoryLoader)(nil)
	_ Loader = (*MultiLoader)(nil)
)

func TestSourceType_String(t *testing.T) {
	tests := []struct {
		st   SourceType
		want string
	}{
		{SourceDirectory, "directory"},
		{SourceArchive, "archive"},
		{SourceOCI, "oci"},
		{SourceRepository, "repository"},
		{SourceUnknown, "unknown"},
		{SourceType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.st.String())
		})
	}
}

func TestDetect_OCI(t *testing.T) {
	tests := []string{
		"oci://ghcr.io/org/my-chart:1.0.0",
		"oci://registry.example.com/charts/nginx",
		"oci://localhost:5000/test",
	}
	for _, ref := range tests {
		st, err := Detect(ref)
		require.NoError(t, err)
		assert.Equal(t, SourceOCI, st, "ref=%q", ref)
	}
}

func TestDetect_Archive(t *testing.T) {
	tests := []string{
		"my-chart-1.0.0.tgz",
		"./charts/my-chart-1.0.0.tgz",
		"/tmp/chart.tar.gz",
	}
	for _, ref := range tests {
		st, err := Detect(ref)
		require.NoError(t, err)
		assert.Equal(t, SourceArchive, st, "ref=%q", ref)
	}
}

func TestDetect_Directory(t *testing.T) {
	dir := t.TempDir()

	st, err := Detect(dir)
	require.NoError(t, err)
	assert.Equal(t, SourceDirectory, st)
}

func TestDetect_Repository(t *testing.T) {
	tests := []string{
		"bitnami/nginx",
		"stable/grafana",
		"myrepo/my-chart",
	}
	for _, ref := range tests {
		st, err := Detect(ref)
		require.NoError(t, err)
		assert.Equal(t, SourceRepository, st, "ref=%q", ref)
	}
}

func TestDetect_EmptyString(t *testing.T) {
	_, err := Detect("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty chart reference")
}

func TestDetect_Unknown(t *testing.T) {
	_, err := Detect("just-a-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot determine chart source type")
}

func TestDetect_AbsolutePathDirectory(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	require.NoError(t, err)

	st, err := Detect(abs)
	require.NoError(t, err)
	assert.Equal(t, SourceDirectory, st)
}

func TestDetect_TildePathFallsToRepo(t *testing.T) {
	st, err := Detect("~/my-charts/nginx")
	require.NoError(t, err)
	assert.Equal(t, SourceRepository, st)
}

func TestLoadOptions_EffectiveMaxArchiveSize_Default(t *testing.T) {
	opts := &LoadOptions{}
	assert.Equal(t, DefaultMaxArchiveSize, opts.effectiveMaxArchiveSize())
}

func TestLoadOptions_EffectiveMaxArchiveSize_Custom(t *testing.T) {
	opts := &LoadOptions{MaxArchiveSize: 50 * 1024 * 1024}
	assert.Equal(t, int64(50*1024*1024), opts.effectiveMaxArchiveSize())
}

func TestLoadOptions_EffectiveMaxArchiveSize_Negative(t *testing.T) {
	opts := &LoadOptions{MaxArchiveSize: -1}
	assert.Equal(t, DefaultMaxArchiveSize, opts.effectiveMaxArchiveSize(),
		"negative MaxArchiveSize should fall back to default")
}

func TestDetect_NonExistentAbsPath(t *testing.T) {
	// A non-existent absolute path with a "/" will be detected as repository.
	st, err := Detect("/nonexistent/path/chart")
	require.NoError(t, err)
	assert.Equal(t, SourceRepository, st)
}

func TestDetect_TarGzSuffix(t *testing.T) {
	st, err := Detect("my-chart-1.0.0.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, SourceArchive, st)
}

// createTestChart creates a minimal valid chart directory for testing.
func createTestChart(t *testing.T, dir, name, version string) string {
	t.Helper()

	chartDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "templates"), 0o750))

	chartYAML := "apiVersion: v2\nname: " + name + "\nversion: " + version + "\ndescription: A test chart\ntype: application\n"
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYAML), 0o600))

	valuesYAML := "replicaCount: 1\nimage:\n  repository: nginx\n  tag: latest\n"
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYAML), 0o600))

	deployTmpl := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: {{ .Release.Name }}-{{ .Chart.Name }}\n  namespace: {{ .Release.Namespace }}\nspec:\n  replicas: {{ .Values.replicaCount }}\n  selector:\n    matchLabels:\n      app: {{ .Chart.Name }}\n  template:\n    metadata:\n      labels:\n        app: {{ .Chart.Name }}\n    spec:\n      containers:\n      - name: {{ .Chart.Name }}\n        image: \"{{ .Values.image.repository }}:{{ .Values.image.tag }}\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(deployTmpl), 0o600))

	svcTmpl := "apiVersion: v1\nkind: Service\nmetadata:\n  name: {{ .Release.Name }}-{{ .Chart.Name }}\n  namespace: {{ .Release.Namespace }}\nspec:\n  type: ClusterIP\n  ports:\n  - port: 80\n    targetPort: 80\n  selector:\n    app: {{ .Chart.Name }}\n"
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "templates", "service.yaml"), []byte(svcTmpl), 0o600))

	return chartDir
}
