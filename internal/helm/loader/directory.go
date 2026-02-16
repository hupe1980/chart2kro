package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	helmloader "helm.sh/helm/v3/pkg/chart/loader"

	"helm.sh/helm/v3/pkg/chart"
)

// DirectoryLoader loads a Helm chart from a local directory.
type DirectoryLoader struct{}

// NewDirectoryLoader creates a DirectoryLoader.
func NewDirectoryLoader() *DirectoryLoader {
	return &DirectoryLoader{}
}

// Load reads a chart from a local directory.
func (l *DirectoryLoader) Load(_ context.Context, ref string, _ LoadOptions) (*chart.Chart, error) {
	info, err := os.Stat(ref)
	if err != nil {
		return nil, fmt.Errorf("chart directory %q: %w", ref, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("chart reference %q is not a directory", ref)
	}

	chartYAMLPath := filepath.Join(ref, "Chart.yaml")
	if _, err := os.Stat(chartYAMLPath); err != nil {
		return nil, fmt.Errorf("chart directory %q has no Chart.yaml: %w", ref, err)
	}

	ch, err := helmloader.LoadDir(ref)
	if err != nil {
		return nil, fmt.Errorf("loading chart from %q: %w", ref, err)
	}

	return ch, nil
}
