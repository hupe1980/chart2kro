package loader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	helmloader "helm.sh/helm/v3/pkg/chart/loader"

	"helm.sh/helm/v3/pkg/chart"
)

// ArchiveLoader loads a Helm chart from a .tgz or .tar.gz archive.
type ArchiveLoader struct{}

// NewArchiveLoader creates an ArchiveLoader.
func NewArchiveLoader() *ArchiveLoader {
	return &ArchiveLoader{}
}

// Load reads a chart from an archive file, decompressing entirely in memory.
func (l *ArchiveLoader) Load(_ context.Context, ref string, opts LoadOptions) (*chart.Chart, error) {
	info, err := os.Stat(ref)
	if err != nil {
		return nil, fmt.Errorf("archive %q: %w", ref, err)
	}

	maxSize := opts.effectiveMaxArchiveSize()
	if info.Size() > maxSize {
		return nil, fmt.Errorf("archive %q is %d bytes, exceeding maximum %d bytes", ref, info.Size(), maxSize)
	}

	f, err := os.Open(ref) //nolint:gosec // ref is user-provided chart path
	if err != nil {
		return nil, fmt.Errorf("opening archive %q: %w", ref, err)
	}
	defer func() { _ = f.Close() }()

	return l.LoadFromReader(f, opts)
}

// LoadFromReader loads a chart from an io.Reader containing a .tgz archive.
// This is used by the OCI and repository loaders to delegate archive parsing.
func (l *ArchiveLoader) LoadFromReader(r io.Reader, opts LoadOptions) (*chart.Chart, error) {
	maxSize := opts.effectiveMaxArchiveSize()

	// Read into memory with a size limit.
	lr := io.LimitReader(r, maxSize+1)

	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("reading archive: %w", err)
	}

	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("archive exceeds maximum size of %d bytes", maxSize)
	}

	ch, err := helmloader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("loading chart archive: %w", err)
	}

	return ch, nil
}
