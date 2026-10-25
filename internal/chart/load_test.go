package chart

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

// repoServer serves a classic Helm repository containing the simple fixture.
func repoServer(t *testing.T) string {
	t.Helper()

	l, err := Load(context.Background(), "../../testdata/charts/simple", LoadOptions{})
	require.NoError(t, err)

	dir := t.TempDir()
	tgz, err := chartutil.Save(l.Chart, dir)
	require.NoError(t, err)

	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)

	idx := repo.NewIndexFile()
	require.NoError(t, idx.MustAdd(l.Chart.Metadata, filepath.Base(tgz), srv.URL, "sha256:0"))
	require.NoError(t, idx.WriteFile(filepath.Join(dir, "index.yaml"), 0o600))

	return srv.URL
}

func TestLoadFromRepository(t *testing.T) {
	url := repoServer(t)

	l, err := Load(context.Background(), "simple", LoadOptions{RepoURL: url, Version: "1.0.0"})
	require.NoError(t, err)
	require.Equal(t, "simple", l.Chart.Name())
	require.NotEmpty(t, l.Digest)

	_, err = Load(context.Background(), "simple", LoadOptions{RepoURL: url, Version: "9.9.9"})
	require.Error(t, err)
}

func TestLoadFromRepositoriesYAML(t *testing.T) {
	url := repoServer(t)

	cfg := filepath.Join(t.TempDir(), "repositories.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("apiVersion: \"\"\nrepositories:\n  - name: fixtures\n    url: "+url+"\n"), 0o600))
	t.Setenv("HELM_REPOSITORY_CONFIG", cfg)

	l, err := Load(context.Background(), "fixtures/simple", LoadOptions{})
	require.NoError(t, err)
	require.Equal(t, "simple", l.Chart.Name())

	_, err = Load(context.Background(), "unknown/simple", LoadOptions{})
	require.ErrorContains(t, err, "not found")
}

func TestLoadArchiveAndErrors(t *testing.T) {
	l, err := Load(context.Background(), "../../testdata/charts/simple", LoadOptions{})
	require.NoError(t, err)

	tgz, err := chartutil.Save(l.Chart, t.TempDir())
	require.NoError(t, err)

	a, err := Load(context.Background(), tgz, LoadOptions{})
	require.NoError(t, err)
	require.Contains(t, a.Digest, "sha256:")

	_, err = Load(context.Background(), "plainname", LoadOptions{})
	require.ErrorContains(t, err, "cannot resolve chart")

	_, err = httpClient(LoadOptions{CAFile: "/does/not/exist"})
	require.Error(t, err)

	bad := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(bad, []byte("not a cert"), 0o600))
	_, err = httpClient(LoadOptions{CAFile: bad})
	require.ErrorContains(t, err, "no certificates")

	_, err = httpClient(LoadOptions{CertFile: bad, KeyFile: bad})
	require.Error(t, err)
}

func TestRendererOptions(t *testing.T) {
	l, err := Load(context.Background(), "../../testdata/charts/simple", LoadOptions{})
	require.NoError(t, err)

	_, err = NewRenderer(l.Chart, RenderOptions{KubeVersion: "not-a-version"})
	require.Error(t, err)

	l.Chart.Metadata.KubeVersion = ">=1.99.0"
	_, err = NewRenderer(l.Chart, RenderOptions{KubeVersion: "1.30"})
	require.ErrorContains(t, err, "requires kubeVersion")

	require.False(t, IsTestHook([]string{"pre-install"}))
	require.True(t, IsTestHook([]string{"test"}))
}

func TestUserValues(t *testing.T) {
	v, err := UserValues(nil, []string{"a.b=1", "c=x"}, []string{"d=007"}, nil)
	require.NoError(t, err)
	require.Equal(t, "007", v["d"])

	_, err = UserValues([]string{"/does/not/exist.yaml"}, nil, nil, nil)
	require.Error(t, err)
}

func TestLoadMissingDependencies(t *testing.T) {
	l, err := Load(context.Background(), "../../testdata/charts/simple", LoadOptions{})
	require.NoError(t, err)

	l.Chart.Metadata.Dependencies = append(l.Chart.Metadata.Dependencies, &chartv2.Dependency{Name: "missing"})
	require.ErrorContains(t, checkDependencies(l.Chart), "helm dependency build")
}

func TestLoadRejectsOversizedArchive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "huge.tgz")
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(MaxArchiveSize+1)) // sparse: no real disk use
	require.NoError(t, f.Close())

	_, err = Load(context.Background(), p, LoadOptions{})
	require.ErrorContains(t, err, "larger than")
}
