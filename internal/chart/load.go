// Package chart loads Helm charts and renders them in memory with the Helm v4
// SDK, reproducing `helm template` without a cluster or a helm binary.
package chart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/helmpath"
	"helm.sh/helm/v4/pkg/registry"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

// MaxArchiveSize bounds chart archives read from any source.
const MaxArchiveSize = 100 << 20

// LoadOptions selects and authenticates a chart source.
type LoadOptions struct {
	Version  string // chart version for OCI and repository sources
	RepoURL  string // repository URL for "repo/chart" references
	Username string
	Password string
	CAFile   string
	CertFile string
	KeyFile  string
}

// Loaded is a chart plus where it came from.
type Loaded struct {
	Chart  *chartv2.Chart
	Ref    string
	Digest string // sha256 of the archive bytes, or empty for directories
}

// Load resolves ref (directory, .tgz, oci://…, or repo/chart) and loads the chart.
// Declared dependencies must be present (as `helm template` requires); otherwise the
// error tells the user to run `helm dependency build`.
func Load(ctx context.Context, ref string, o LoadOptions) (*Loaded, error) {
	var (
		ch     *chartv2.Chart
		digest string
		err    error
	)

	switch {
	case strings.HasPrefix(ref, "oci://"):
		var data []byte
		if data, err = pullOCI(ref, o); err == nil {
			ch, digest, err = fromArchive(data)
		}
	case isLocal(ref):
		if fi, serr := os.Stat(ref); serr == nil && !fi.IsDir() && fi.Size() > MaxArchiveSize {
			return nil, fmt.Errorf("loading chart %q: archive is larger than %d bytes", ref, MaxArchiveSize)
		}

		ch, err = loader.Load(ref)
		if err == nil && !isDir(ref) {
			data, rerr := os.ReadFile(ref) //nolint:gosec // user-selected chart path
			if rerr == nil {
				digest = sha(data)
			}
		}
	case strings.Contains(ref, "/") || o.RepoURL != "":
		var data []byte
		if data, err = fetchFromRepo(ctx, ref, o); err == nil {
			ch, digest, err = fromArchive(data)
		}
	default:
		return nil, fmt.Errorf("cannot resolve chart %q: not a directory, archive, oci:// reference or repo/chart", ref)
	}

	if err != nil {
		return nil, fmt.Errorf("loading chart %q: %w", ref, err)
	}

	if err := checkDependencies(ch); err != nil {
		return nil, err
	}

	return &Loaded{Chart: ch, Ref: ref, Digest: digest}, nil
}

func isLocal(ref string) bool {
	_, err := os.Stat(ref)
	return err == nil
}

func isDir(ref string) bool {
	fi, err := os.Stat(ref)
	return err == nil && fi.IsDir()
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:])
}

func fromArchive(data []byte) (*chartv2.Chart, string, error) {
	if len(data) > MaxArchiveSize {
		return nil, "", fmt.Errorf("chart archive is larger than %d bytes", MaxArchiveSize)
	}

	ch, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}

	return ch, sha(data), nil
}

// checkDependencies mirrors `helm template`: every declared dependency must be
// vendored under charts/.
func checkDependencies(ch *chartv2.Chart) error {
	if ch.Metadata == nil || len(ch.Metadata.Dependencies) == 0 {
		return nil
	}

	have := map[string]bool{}
	for _, d := range ch.Dependencies() {
		have[d.Name()] = true
	}

	var missing []string

	for _, d := range ch.Metadata.Dependencies {
		if !have[d.Name] {
			missing = append(missing, d.Name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("chart %q is missing dependencies %s: run `helm dependency build` first",
			ch.Name(), strings.Join(missing, ", "))
	}

	return nil
}

func pullOCI(ref string, o LoadOptions) ([]byte, error) {
	opts := []registry.ClientOption{registry.ClientOptEnableCache(true)}

	if o.CAFile != "" || o.CertFile != "" {
		hc, err := httpClient(o)
		if err != nil {
			return nil, err
		}

		opts = append(opts, registry.ClientOptHTTPClient(hc))
	}

	if o.Username != "" {
		opts = append(opts, registry.ClientOptBasicAuth(o.Username, o.Password))
	}

	client, err := registry.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	pullRef := strings.TrimPrefix(ref, "oci://")
	if o.Version != "" && !strings.Contains(lastSegment(pullRef), ":") && !strings.Contains(pullRef, "@") {
		pullRef += ":" + o.Version
	}

	res, err := client.Pull(pullRef, registry.PullOptWithChart(true))
	if err != nil {
		return nil, err
	}

	if res.Chart == nil || len(res.Chart.Data) == 0 {
		return nil, errors.New("registry returned no chart layer")
	}

	return res.Chart.Data, nil
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}

	return s
}

// fetchFromRepo resolves "repo/chart" via --repo or Helm's repositories.yaml and
// downloads the archive with Helm's getters.
func fetchFromRepo(ctx context.Context, ref string, o LoadOptions) ([]byte, error) {
	name := ref
	repoURL := o.RepoURL

	if repoURL == "" {
		repoName, chartName, ok := strings.Cut(ref, "/")
		if !ok {
			return nil, fmt.Errorf("reference %q is not repo/chart; use --repo", ref)
		}

		entry, err := repoEntry(repoName)
		if err != nil {
			return nil, err
		}

		name, repoURL = chartName, entry.URL
		o.Username = firstNonEmpty(o.Username, entry.Username)
		o.Password = firstNonEmpty(o.Password, entry.Password)
		o.CAFile = firstNonEmpty(o.CAFile, entry.CAFile)
		o.CertFile = firstNonEmpty(o.CertFile, entry.CertFile)
		o.KeyFile = firstNonEmpty(o.KeyFile, entry.KeyFile)
	}

	providers := getter.All(cli.New())

	url, err := repo.FindChartInRepoURL(repoURL, name, providers,
		repo.WithChartVersion(o.Version),
		repo.WithUsernamePassword(o.Username, o.Password),
		repo.WithClientTLS(o.CertFile, o.KeyFile, o.CAFile))
	if err != nil {
		return nil, err
	}

	hc, err := httpClient(o)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if o.Username != "" {
		req.SetBasicAuth(o.Username, o.Password)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: HTTP %d", url, resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, MaxArchiveSize+1))
}

func repoEntry(name string) (*repo.Entry, error) {
	path := os.Getenv("HELM_REPOSITORY_CONFIG")
	if path == "" {
		path = helmpath.ConfigPath("repositories.yaml")
	}

	f, err := repo.LoadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("repository %q: reading %s: %w (or pass --repo)", name, path, err)
	}

	for _, e := range f.Repositories {
		if e.Name == name {
			return e, nil
		}
	}

	return nil, fmt.Errorf("repository %q not found in %s (run `helm repo add` or pass --repo)", name, path)
}

func httpClient(o LoadOptions) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if o.CAFile != "" {
		pem, err := os.ReadFile(o.CAFile) //nolint:gosec // user-selected CA file
		if err != nil {
			return nil, fmt.Errorf("reading CA file: %w", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("CA file %q has no certificates", o.CAFile)
		}

		cfg.RootCAs = pool
	}

	if o.CertFile != "" && o.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(o.CertFile, o.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading client certificate: %w", err)
		}

		cfg.Certificates = []tls.Certificate{cert}
	}

	tr.TLSClientConfig = cfg

	return &http.Client{Transport: tr, Timeout: 60 * time.Second}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}

	return b
}
