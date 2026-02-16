package loader

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/chart"
)

// MultiLoader implements the Loader interface by auto-detecting the source
// type and delegating to the appropriate specialised loader.
type MultiLoader struct {
	directory  *DirectoryLoader
	archive    *ArchiveLoader
	oci        *OCILoader
	repository *RepositoryLoader
}

// NewMultiLoader creates a MultiLoader with all source-type loaders initialised.
func NewMultiLoader() *MultiLoader {
	return &MultiLoader{
		directory:  NewDirectoryLoader(),
		archive:    NewArchiveLoader(),
		oci:        NewOCILoader(),
		repository: NewRepositoryLoader(),
	}
}

// Load auto-detects the chart source type and delegates to the appropriate loader.
func (m *MultiLoader) Load(ctx context.Context, ref string, opts LoadOptions) (*chart.Chart, error) {
	st, err := Detect(ref)
	if err != nil {
		return nil, err
	}

	switch st {
	case SourceUnknown:
		return nil, fmt.Errorf("unsupported chart source type: %s", st)
	case SourceDirectory:
		return m.directory.Load(ctx, ref, opts)
	case SourceArchive:
		return m.archive.Load(ctx, ref, opts)
	case SourceOCI:
		return m.oci.Load(ctx, ref, opts)
	case SourceRepository:
		return m.repository.Load(ctx, ref, opts)
	default:
		return nil, fmt.Errorf("unsupported chart source type: %s", st)
	}
}
