// Package loader provides chart loading from multiple source types
// (directory, archive, OCI registry, Helm repository) with automatic
// source-type detection.
package loader

import (
	"context"
	"fmt"
	"os"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
)

// SourceType identifies the origin of a Helm chart reference.
type SourceType int

const (
	// SourceUnknown indicates the source type could not be determined.
	SourceUnknown SourceType = iota
	// SourceDirectory is a local directory containing Chart.yaml.
	SourceDirectory
	// SourceArchive is a .tgz or .tar.gz packaged chart.
	SourceArchive
	// SourceOCI is an oci:// registry reference.
	SourceOCI
	// SourceRepository is a repo/chart reference requiring --repo-url.
	SourceRepository
)

// String returns a human-readable name for the source type.
func (s SourceType) String() string {
	switch s {
	case SourceUnknown:
		return "unknown"
	case SourceDirectory:
		return "directory"
	case SourceArchive:
		return "archive"
	case SourceOCI:
		return "oci"
	case SourceRepository:
		return "repository"
	default:
		return "unknown"
	}
}

// LoadOptions configures chart loading behaviour.
type LoadOptions struct {
	// Version is a SemVer constraint for repository/OCI lookups.
	Version string
	// RepoURL is the Helm repository URL for SourceRepository.
	RepoURL string
	// Username for repository or OCI authentication.
	Username string
	// Password for repository or OCI authentication.
	Password string
	// CaFile is the path to a CA certificate bundle.
	CaFile string
	// CertFile is the path to a TLS client certificate.
	CertFile string
	// KeyFile is the path to a TLS client key.
	KeyFile string
	// MaxArchiveSize is the maximum allowed archive size in bytes.
	// Zero means use the default (100 MB).
	MaxArchiveSize int64
}

// DefaultMaxArchiveSize is 100 MB.
const DefaultMaxArchiveSize int64 = 100 * 1024 * 1024

// effectiveMaxArchiveSize returns the archive size limit, falling back to
// DefaultMaxArchiveSize when not configured.
func (o *LoadOptions) effectiveMaxArchiveSize() int64 {
	if o.MaxArchiveSize > 0 {
		return o.MaxArchiveSize
	}

	return DefaultMaxArchiveSize
}

// Loader loads a Helm chart from a given reference.
type Loader interface {
	// Load resolves ref according to opts and returns the in-memory chart.
	Load(ctx context.Context, ref string, opts LoadOptions) (*chart.Chart, error)
}

// Detect classifies the chart reference string and returns the appropriate
// SourceType based on URI scheme, path, and naming heuristics.
func Detect(ref string) (SourceType, error) {
	if ref == "" {
		return SourceUnknown, fmt.Errorf("empty chart reference")
	}

	if strings.HasPrefix(ref, "oci://") {
		return SourceOCI, nil
	}

	if strings.HasSuffix(ref, ".tgz") || strings.HasSuffix(ref, ".tar.gz") {
		return SourceArchive, nil
	}

	if info, err := os.Stat(ref); err == nil && info.IsDir() {
		return SourceDirectory, nil
	}

	if strings.Contains(ref, "/") {
		return SourceRepository, nil
	}

	return SourceUnknown, fmt.Errorf("cannot determine chart source type for %q", ref)
}
