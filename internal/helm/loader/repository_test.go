package loader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRepoServer starts a test HTTP server that serves a Helm repository
// index and a chart archive.
func newTestRepoServer(t *testing.T, chartName, chartVersion string) *httptest.Server {
	t.Helper()

	archiveData := buildTestArchiveBytes(t, chartName, chartVersion)

	archiveFileName := fmt.Sprintf("%s-%s.tgz", chartName, chartVersion)

	indexYAML := fmt.Sprintf(`apiVersion: v1
entries:
  %s:
  - name: %s
    version: %s
    urls:
    - %s
    apiVersion: v2
    description: A test chart
    type: application
generated: "2026-01-01T00:00:00Z"
`, chartName, chartName, chartVersion, archiveFileName)

	mux := http.NewServeMux()
	mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(indexYAML))
	})
	mux.HandleFunc("/"+archiveFileName, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archiveData)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

// buildTestArchiveBytes creates a .tgz chart archive in memory.
func buildTestArchiveBytes(t *testing.T, name, version string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	files := map[string]string{
		name + "/Chart.yaml":                "apiVersion: v2\nname: " + name + "\nversion: " + version + "\ndescription: Test chart\ntype: application\n",
		name + "/values.yaml":               "replicaCount: 1\n",
		name + "/templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: test\n",
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

	return buf.Bytes()
}

func TestRepositoryLoader_Load(t *testing.T) {
	srv := newTestRepoServer(t, "test-chart", "1.2.3")

	loader := NewRepositoryLoader()
	ch, err := loader.Load(context.Background(), "myrepo/test-chart", LoadOptions{
		RepoURL: srv.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "test-chart", ch.Metadata.Name)
	assert.Equal(t, "1.2.3", ch.Metadata.Version)
}

func TestRepositoryLoader_Load_NoRepoURL(t *testing.T) {
	loader := NewRepositoryLoader()
	_, err := loader.Load(context.Background(), "myrepo/test-chart", LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository URL")
}

func TestRepositoryLoader_Load_ChartNotFound(t *testing.T) {
	srv := newTestRepoServer(t, "existing-chart", "1.0.0")

	loader := NewRepositoryLoader()
	_, err := loader.Load(context.Background(), "myrepo/nonexistent", LoadOptions{
		RepoURL: srv.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRepositoryLoader_Load_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	loader := NewRepositoryLoader()
	_, err := loader.Load(context.Background(), "repo/chart", LoadOptions{
		RepoURL: srv.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestRepositoryLoader_Load_WithVersion(t *testing.T) {
	srv := newTestRepoServer(t, "versioned-chart", "2.0.0")

	loader := NewRepositoryLoader()
	ch, err := loader.Load(context.Background(), "repo/versioned-chart", LoadOptions{
		RepoURL: srv.URL,
		Version: "2.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", ch.Metadata.Version)
}

func TestRepositoryLoader_Load_VersionNotFound(t *testing.T) {
	srv := newTestRepoServer(t, "versioned-chart", "1.0.0")

	loader := NewRepositoryLoader()
	_, err := loader.Load(context.Background(), "repo/versioned-chart", LoadOptions{
		RepoURL: srv.URL,
		Version: "9.9.9",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRepositoryLoader_Load_WithAuth(t *testing.T) {
	authenticated := false
	mux := http.NewServeMux()

	archiveData := buildTestArchiveBytes(t, "auth-chart", "1.0.0")

	mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		authenticated = true
		indexYAML := `apiVersion: v1
entries:
  auth-chart:
  - name: auth-chart
    version: 1.0.0
    urls:
    - auth-chart-1.0.0.tgz
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`
		_, _ = w.Write([]byte(indexYAML))
	})
	mux.HandleFunc("/auth-chart-1.0.0.tgz", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(archiveData)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	loader := NewRepositoryLoader()
	ch, err := loader.Load(context.Background(), "repo/auth-chart", LoadOptions{
		RepoURL:  srv.URL,
		Username: "admin",
		Password: "secret",
	})
	require.NoError(t, err)
	assert.True(t, authenticated)
	assert.Equal(t, "auth-chart", ch.Metadata.Name)
}

// ---------------------------------------------------------------------------
// Unit tests for internal helpers
// ---------------------------------------------------------------------------

func TestLoadIndex(t *testing.T) {
	raw := []byte(`apiVersion: v1
entries:
  my-chart:
  - name: my-chart
    version: 1.0.0
    urls:
    - my-chart-1.0.0.tgz
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`)
	idx, err := loadIndex(raw)
	require.NoError(t, err)
	assert.Contains(t, idx.Entries, "my-chart")
}

func TestLoadIndex_Invalid(t *testing.T) {
	_, err := loadIndex([]byte("not valid yaml index"))
	require.Error(t, err)
}

func TestResolveChartVersion_Latest(t *testing.T) {
	raw := []byte(`apiVersion: v1
entries:
  nginx:
  - name: nginx
    version: 2.0.0
    urls: [nginx-2.0.0.tgz]
    apiVersion: v2
  - name: nginx
    version: 1.0.0
    urls: [nginx-1.0.0.tgz]
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`)
	idx, err := loadIndex(raw)
	require.NoError(t, err)

	cv, err := resolveChartVersion(idx, "nginx", "")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", cv.Version)
}

func TestResolveChartVersion_Specific(t *testing.T) {
	raw := []byte(`apiVersion: v1
entries:
  nginx:
  - name: nginx
    version: 2.0.0
    urls: [nginx-2.0.0.tgz]
    apiVersion: v2
  - name: nginx
    version: 1.0.0
    urls: [nginx-1.0.0.tgz]
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`)
	idx, err := loadIndex(raw)
	require.NoError(t, err)

	cv, err := resolveChartVersion(idx, "nginx", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", cv.Version)
}

func TestResolveChartVersion_NotFound(t *testing.T) {
	raw := []byte(`apiVersion: v1
entries:
  nginx:
  - name: nginx
    version: 1.0.0
    urls: [nginx-1.0.0.tgz]
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`)
	idx, err := loadIndex(raw)
	require.NoError(t, err)

	_, err = resolveChartVersion(idx, "nonexistent", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestNewRepositoryLoader(t *testing.T) {
	loader := NewRepositoryLoader()
	assert.NotNil(t, loader)
	assert.NotNil(t, loader.archive)
}

func TestHttpClientForOpts_Default(t *testing.T) {
	client, err := httpClientForOpts(LoadOptions{})
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, 60*time.Second, client.Timeout)
}

func TestHttpClientForOpts_InvalidCAFile(t *testing.T) {
	_, err := httpClientForOpts(LoadOptions{CaFile: "/nonexistent/ca.pem"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading CA file")
}

func TestHttpClientForOpts_InvalidCertKeyPair(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, []byte("not-a-cert"), 0o600))
	require.NoError(t, os.WriteFile(keyFile, []byte("not-a-key"), 0o600))

	_, err := httpClientForOpts(LoadOptions{CertFile: certFile, KeyFile: keyFile})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading TLS client certificate")
}

func TestRepositoryLoader_Load_EmptyURLs(t *testing.T) {
	// Chart version found in index but has no download URLs.
	mux := http.NewServeMux()
	mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, _ *http.Request) {
		indexYAML := `apiVersion: v1
entries:
  empty-urls:
  - name: empty-urls
    version: 1.0.0
    urls: []
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`
		_, _ = w.Write([]byte(indexYAML))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	loader := NewRepositoryLoader()
	_, err := loader.Load(context.Background(), "repo/empty-urls", LoadOptions{RepoURL: srv.URL})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no download URLs")
}

func TestRepositoryLoader_Load_DownloadError(t *testing.T) {
	// Index serves fine but chart download returns 404.
	mux := http.NewServeMux()
	mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, _ *http.Request) {
		indexYAML := `apiVersion: v1
entries:
  dl-fail:
  - name: dl-fail
    version: 1.0.0
    urls:
    - dl-fail-1.0.0.tgz
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`
		_, _ = w.Write([]byte(indexYAML))
	})
	mux.HandleFunc("/dl-fail-1.0.0.tgz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	loader := NewRepositoryLoader()
	_, err := loader.Load(context.Background(), "repo/dl-fail", LoadOptions{RepoURL: srv.URL})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
}

func TestRepositoryLoader_Load_AbsoluteChartURL(t *testing.T) {
	// Chart version URLs can be absolute (not relative to repo).
	archiveData := buildTestArchiveBytes(t, "abs-url", "1.0.0")

	// Mirror server hosts the actual archive.
	mirrorMux := http.NewServeMux()
	mirrorMux.HandleFunc("/download/abs-url-1.0.0.tgz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archiveData)
	})
	mirror := httptest.NewServer(mirrorMux)
	t.Cleanup(mirror.Close)

	// Repo server has the index pointing to the absolute mirror URL.
	repoMux := http.NewServeMux()
	repoMux.HandleFunc("/index.yaml", func(w http.ResponseWriter, _ *http.Request) {
		indexYAML := fmt.Sprintf(`apiVersion: v1
entries:
  abs-url:
  - name: abs-url
    version: 1.0.0
    urls:
    - %s/download/abs-url-1.0.0.tgz
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`, mirror.URL)
		_, _ = w.Write([]byte(indexYAML))
	})
	repo := httptest.NewServer(repoMux)
	t.Cleanup(repo.Close)

	loader := NewRepositoryLoader()
	ch, err := loader.Load(context.Background(), "repo/abs-url", LoadOptions{RepoURL: repo.URL})
	require.NoError(t, err)
	assert.Equal(t, "abs-url", ch.Metadata.Name)
}

func TestRepositoryLoader_Load_ContextCancelled(t *testing.T) {
	srv := newTestRepoServer(t, "cancel-chart", "1.0.0")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	loader := NewRepositoryLoader()
	_, err := loader.Load(ctx, "repo/cancel-chart", LoadOptions{RepoURL: srv.URL})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// splitRepoRef tests
// ---------------------------------------------------------------------------

func TestSplitRepoRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantRepo string
		wantName string
	}{
		{"myrepo/nginx", "myrepo", "nginx"},
		{"bitnami/postgresql", "bitnami", "postgresql"},
		{"nginx", "", "nginx"},
		{"a/b/c", "a", "b/c"},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			repo, name := splitRepoRef(tt.ref)
			assert.Equal(t, tt.wantRepo, repo)
			assert.Equal(t, tt.wantName, name)
		})
	}
}

// ---------------------------------------------------------------------------
// defaultRepoConfigPath tests
// ---------------------------------------------------------------------------

func TestDefaultRepoConfigPath_EnvOverride(t *testing.T) {
	customPath := filepath.Join(t.TempDir(), "custom-repos.yaml")
	t.Setenv("HELM_REPOSITORY_CONFIG", customPath)

	got := defaultRepoConfigPath()
	assert.Equal(t, customPath, got)
}

func TestDefaultRepoConfigPath_Default(t *testing.T) {
	t.Setenv("HELM_REPOSITORY_CONFIG", "")

	got := defaultRepoConfigPath()
	assert.Contains(t, got, "repositories.yaml")
}

// ---------------------------------------------------------------------------
// lookupRepoEntry tests
// ---------------------------------------------------------------------------

func TestLookupRepoEntry_Found(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "repositories.yaml")
	content := `apiVersion: ""
generated: "0001-01-01T00:00:00Z"
repositories:
- name: bitnami
  url: https://charts.bitnami.com/bitnami
  username: admin
  password: secret
  caFile: /path/to/ca.pem
  certFile: /path/to/cert.pem
  keyFile: /path/to/key.pem
`
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o600))
	t.Setenv("HELM_REPOSITORY_CONFIG", configFile)

	entry, err := lookupRepoEntry("bitnami")
	require.NoError(t, err)
	assert.Equal(t, "https://charts.bitnami.com/bitnami", entry.URL)
	assert.Equal(t, "admin", entry.Username)
	assert.Equal(t, "secret", entry.Password)
	assert.Equal(t, "/path/to/ca.pem", entry.CAFile)
	assert.Equal(t, "/path/to/cert.pem", entry.CertFile)
	assert.Equal(t, "/path/to/key.pem", entry.KeyFile)
}

func TestLookupRepoEntry_NotFound(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "repositories.yaml")
	content := `apiVersion: ""
generated: "0001-01-01T00:00:00Z"
repositories:
- name: stable
  url: https://charts.helm.sh/stable
`
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o600))
	t.Setenv("HELM_REPOSITORY_CONFIG", configFile)

	_, err := lookupRepoEntry("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLookupRepoEntry_EmptyName(t *testing.T) {
	_, err := lookupRepoEntry("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no repository name")
}

func TestLookupRepoEntry_NoConfigFile(t *testing.T) {
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(t.TempDir(), "nonexistent.yaml"))

	_, err := lookupRepoEntry("myrepo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading Helm repositories config")
}

// ---------------------------------------------------------------------------
// repositories.yaml fallback integration test
// ---------------------------------------------------------------------------

func TestRepositoryLoader_Load_FallbackToRepositoriesYAML(t *testing.T) {
	// Start a test repo server.
	srv := newTestRepoServer(t, "nginx", "1.0.0")

	// Write a repositories.yaml pointing "bitnami" to our test server.
	dir := t.TempDir()
	configFile := filepath.Join(dir, "repositories.yaml")
	content := fmt.Sprintf(`apiVersion: ""
generated: "0001-01-01T00:00:00Z"
repositories:
- name: bitnami
  url: %s
`, srv.URL)
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o600))
	t.Setenv("HELM_REPOSITORY_CONFIG", configFile)

	loader := NewRepositoryLoader()
	// No --repo-url provided; should fall back to repositories.yaml.
	ch, err := loader.Load(context.Background(), "bitnami/nginx", LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, "nginx", ch.Metadata.Name)
	assert.Equal(t, "1.0.0", ch.Metadata.Version)
}

func TestRepositoryLoader_Load_FallbackCredentials(t *testing.T) {
	// Verify that credentials from repositories.yaml are used as defaults,
	// and CLI flags override them.
	authChecked := false
	mux := http.NewServeMux()
	archiveData := buildTestArchiveBytes(t, "secure-chart", "1.0.0")

	mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "cli-user" || pass != "cli-pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authChecked = true
		indexYAML := `apiVersion: v1
entries:
  secure-chart:
  - name: secure-chart
    version: 1.0.0
    urls:
    - secure-chart-1.0.0.tgz
    apiVersion: v2
generated: "2026-01-01T00:00:00Z"
`
		_, _ = w.Write([]byte(indexYAML))
	})
	mux.HandleFunc("/secure-chart-1.0.0.tgz", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "cli-user" || pass != "cli-pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(archiveData)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Write repos.yaml with different credentials (should be overridden by CLI).
	dir := t.TempDir()
	configFile := filepath.Join(dir, "repositories.yaml")
	content := fmt.Sprintf(`apiVersion: ""
generated: "0001-01-01T00:00:00Z"
repositories:
- name: secure
  url: %s
  username: repo-user
  password: repo-pass
`, srv.URL)
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o600))
	t.Setenv("HELM_REPOSITORY_CONFIG", configFile)

	loader := NewRepositoryLoader()
	// CLI flags override repos.yaml credentials.
	ch, err := loader.Load(context.Background(), "secure/secure-chart", LoadOptions{
		Username: "cli-user",
		Password: "cli-pass",
	})
	require.NoError(t, err)
	assert.True(t, authChecked)
	assert.Equal(t, "secure-chart", ch.Metadata.Name)
}

func TestRepositoryLoader_Load_NoRepoURL_NoConfig(t *testing.T) {
	// No --repo-url and no valid repositories.yaml → descriptive error.
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(t.TempDir(), "nonexistent.yaml"))

	loader := NewRepositoryLoader()
	_, err := loader.Load(context.Background(), "myrepo/chart", LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository URL (--repo-url) is required")
}
