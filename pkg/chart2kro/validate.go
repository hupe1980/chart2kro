package chart2kro

import (
	"context"
	"fmt"

	"github.com/hupe1980/chart2kro/internal/kroval"
)

// ValidateOptions configure Validate.
type ValidateOptions struct {
	KubeVersion string   // default: newest embedded catalog
	CRDFiles    []string // CRDs used by the RGDs' resources
}

// Validate checks every ResourceGraphDefinition in a YAML stream with kro's own
// admission schema and graph builder, offline. It returns an error wrapping
// ErrInvalid listing every problem.
func Validate(ctx context.Context, stream []byte, o ValidateOptions) error {
	var crds []map[string]any

	for _, f := range o.CRDFiles {
		docs, err := readYAMLDocs(f)
		if err != nil {
			return fmt.Errorf("%w: --crd %s: %w", ErrUsage, f, err)
		}

		crds = append(crds, docs...)
	}

	docs := splitDocs(stream)

	// CRDs in the same stream (e.g. prerequisites) are used too.
	for _, d := range docs {
		var m map[string]any
		if err := yamlUnmarshal(d, &m); err == nil && m["kind"] == "CustomResourceDefinition" {
			crds = append(crds, m)
		}
	}

	v, err := kroval.New(o.KubeVersion, crds)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}

	if err := v.ValidateYAML(ctx, docs); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	return nil
}
