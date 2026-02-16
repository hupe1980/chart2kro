package loader

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/helmpath"
	"helm.sh/helm/v3/pkg/repo"
)

// RepositoryLoader loads a Helm chart from a classic Helm repository.
type RepositoryLoader struct {
	archive *ArchiveLoader
}

// NewRepositoryLoader creates a RepositoryLoader.
func NewRepositoryLoader() *RepositoryLoader {
	return &RepositoryLoader{
		archive: NewArchiveLoader(),
	}
}

// httpClientForOpts builds an *http.Client with TLS configuration from LoadOptions.
func httpClientForOpts(opts LoadOptions) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if opts.CaFile != "" || opts.CertFile != "" {
		tlsCfg := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		if opts.CaFile != "" {
			caCert, err := os.ReadFile(opts.CaFile) //nolint:gosec // user-provided CA path
			if err != nil {
				return nil, fmt.Errorf("reading CA file %q: %w", opts.CaFile, err)
			}

			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("CA file %q contains no valid certificates", opts.CaFile)
			}

			tlsCfg.RootCAs = pool
		}

		if opts.CertFile != "" && opts.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("loading TLS client certificate: %w", err)
			}

			tlsCfg.Certificates = []tls.Certificate{cert}
		}

		transport.TLSClientConfig = tlsCfg
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}, nil
}

// Load resolves a repo/chart style reference and downloads the chart archive.
func (l *RepositoryLoader) Load(ctx context.Context, ref string, opts LoadOptions) (*chart.Chart, error) {
	// Extract repo name and chart name from "repo/chart" reference.
	repoName, chartName := splitRepoRef(ref)

	// If RepoURL is not provided, fall back to repositories.yaml.
	if opts.RepoURL == "" {
		entry, err := lookupRepoEntry(repoName)
		if err != nil {
			return nil, err
		}

		opts.RepoURL = entry.URL

		// Apply credentials from repositories.yaml as defaults — CLI flags take precedence.
		if opts.Username == "" {
			opts.Username = entry.Username
		}

		if opts.Password == "" {
			opts.Password = entry.Password
		}

		if opts.CaFile == "" {
			opts.CaFile = entry.CAFile
		}

		if opts.CertFile == "" {
			opts.CertFile = entry.CertFile
		}

		if opts.KeyFile == "" {
			opts.KeyFile = entry.KeyFile
		}
	}

	httpClient, err := httpClientForOpts(opts)
	if err != nil {
		return nil, fmt.Errorf("configuring HTTP client: %w", err)
	}

	indexURL := strings.TrimSuffix(opts.RepoURL, "/") + "/index.yaml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating index request: %w", err)
	}

	if opts.Username != "" && opts.Password != "" {
		req.SetBasicAuth(opts.Username, opts.Password)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching repository index from %q: %w", indexURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repository index %q returned status %d", indexURL, resp.StatusCode)
	}

	// Limit index size to 256 MB to prevent OOM from very large repository indices.
	const maxIndexSize = 256 << 20 // 256 MB

	indexData, err := io.ReadAll(io.LimitReader(resp.Body, maxIndexSize))
	if err != nil {
		return nil, fmt.Errorf("reading repository index: %w", err)
	}

	index, err := loadIndex(indexData)
	if err != nil {
		return nil, fmt.Errorf("parsing repository index: %w", err)
	}

	cv, err := resolveChartVersion(index, chartName, opts.Version)
	if err != nil {
		return nil, err
	}

	if len(cv.URLs) == 0 {
		return nil, fmt.Errorf("chart %q version %q has no download URLs", chartName, cv.Version)
	}

	chartURL := cv.URLs[0]
	if !strings.HasPrefix(chartURL, "http://") && !strings.HasPrefix(chartURL, "https://") {
		chartURL = strings.TrimSuffix(opts.RepoURL, "/") + "/" + chartURL
	}

	return l.downloadChart(ctx, httpClient, chartURL, opts)
}

// loadIndex parses a Helm repository index from raw YAML bytes.
func loadIndex(data []byte) (*repo.IndexFile, error) {
	tmpDir, err := os.MkdirTemp("", "chart2kro-index-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir for index: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	indexPath := filepath.Join(tmpDir, "index.yaml")
	if err := os.WriteFile(indexPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("writing index file: %w", err)
	}

	idx, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("loading repository index: %w", err)
	}

	idx.SortEntries()

	return idx, nil
}

// resolveChartVersion finds the matching chart version in the index.
func resolveChartVersion(idx *repo.IndexFile, name, version string) (*repo.ChartVersion, error) {
	versions, ok := idx.Entries[name]
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("chart %q not found in repository index", name)
	}

	if version == "" {
		// Return the latest version (index is sorted, first entry is latest).
		return versions[0], nil
	}

	cv, err := idx.Get(name, version)
	if err != nil {
		return nil, fmt.Errorf("chart %q version %q not found: %w", name, version, err)
	}

	return cv, nil
}

// downloadChart fetches the chart archive from the given URL.
func (l *RepositoryLoader) downloadChart(ctx context.Context, httpClient *http.Client, url string, opts LoadOptions) (*chart.Chart, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating chart download request: %w", err)
	}

	if opts.Username != "" && opts.Password != "" {
		req.SetBasicAuth(opts.Username, opts.Password)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading chart from %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chart download from %q returned status %d", url, resp.StatusCode)
	}

	// Stream directly to the archive loader which applies its own size limit.
	return l.archive.LoadFromReader(resp.Body, opts)
}

// splitRepoRef splits a "repo/chart" reference into repo name and chart name.
// If no "/" is present, the entire ref is treated as the chart name with an
// empty repo name.
func splitRepoRef(ref string) (repoName, chartName string) {
	if idx := strings.Index(ref, "/"); idx >= 0 {
		return ref[:idx], ref[idx+1:]
	}

	return "", ref
}

// defaultRepoConfigPath returns the path to Helm's repositories.yaml file.
// It respects the HELM_REPOSITORY_CONFIG environment variable; if unset it
// falls back to Helm's standard config directory.
func defaultRepoConfigPath() string {
	if p := os.Getenv("HELM_REPOSITORY_CONFIG"); p != "" {
		return p
	}

	return helmpath.ConfigPath("repositories.yaml")
}

// lookupRepoEntry reads Helm's repositories.yaml and looks up a repo by name.
func lookupRepoEntry(name string) (*repo.Entry, error) {
	if name == "" {
		return nil, fmt.Errorf("no repository name in chart reference")
	}

	configPath := defaultRepoConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf(
			"repository %q not configured (no Helm repositories file found at %s)\n"+
				"  Use --repo-url to specify the repository URL, or run:\n"+
				"  helm repo add %s <repository-url>",
			name, configPath, name,
		)
	}

	repoFile, err := repo.LoadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("loading Helm repositories config %q: %w", configPath, err)
	}

	entry := repoFile.Get(name)
	if entry == nil {
		return nil, fmt.Errorf(
			"repository %q not found in %s\n"+
				"  Use --repo-url to specify the repository URL, or run:\n"+
				"  helm repo add %s <repository-url>",
			name, configPath, name,
		)
	}

	return entry, nil
}
