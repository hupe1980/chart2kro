package loader

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/registry"
)

// OCILoader loads a Helm chart from an OCI registry.
type OCILoader struct {
	archive *ArchiveLoader
}

// NewOCILoader creates an OCILoader.
func NewOCILoader() *OCILoader {
	return &OCILoader{archive: NewArchiveLoader()}
}

// Load pulls a chart from an OCI registry and returns the in-memory chart.
func (l *OCILoader) Load(_ context.Context, ref string, opts LoadOptions) (*chart.Chart, error) {
	if !strings.HasPrefix(ref, "oci://") {
		return nil, fmt.Errorf("OCI reference must start with oci://, got %q", ref)
	}

	registryOpts := []registry.ClientOption{
		registry.ClientOptEnableCache(true),
	}

	if opts.CaFile != "" || opts.CertFile != "" || opts.KeyFile != "" {
		httpClient, err := httpClientForOpts(opts)
		if err != nil {
			return nil, fmt.Errorf("configuring OCI HTTP client: %w", err)
		}

		registryOpts = append(registryOpts,
			registry.ClientOptHTTPClient(httpClient),
		)
	}

	client, err := registry.NewClient(registryOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating OCI registry client: %w", err)
	}

	// Authenticate if credentials are provided.
	if opts.Username != "" && opts.Password != "" {
		host := extractHost(ref)
		if err := client.Login(host,
			registry.LoginOptBasicAuth(opts.Username, opts.Password),
		); err != nil {
			return nil, fmt.Errorf("authenticating to OCI registry %q: %w", host, err)
		}
	}

	// Strip the oci:// prefix for the pull API.
	pullRef := strings.TrimPrefix(ref, "oci://")

	pullOpts := []registry.PullOption{
		registry.PullOptWithChart(true),
	}

	result, err := client.Pull(pullRef, pullOpts...)
	if err != nil {
		return nil, fmt.Errorf("pulling chart from %q: %w", ref, err)
	}

	if result.Chart == nil || result.Chart.Data == nil {
		return nil, fmt.Errorf("no chart data in OCI pull result for %q", ref)
	}

	return l.archive.LoadFromReader(bytes.NewReader(result.Chart.Data), opts)
}

// extractHost extracts the registry host from an oci:// reference.
func extractHost(ref string) string {
	// oci://host/path → host
	trimmed := strings.TrimPrefix(ref, "oci://")

	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[:idx]
	}

	return trimmed
}
