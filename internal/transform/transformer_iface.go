package transform

import (
	"context"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// TransformerOutput holds the results from a transformer's per-resource
// transformation. This is the engine-facing type that decouples the
// engine from the transformer package.
type TransformerOutput struct {
	// ReadyWhen are the readiness conditions (raw CEL-style strings).
	ReadyWhen []string

	// StatusFields are the status projections for this resource.
	StatusFields []StatusField

	// IncludeWhen is an optional conditional inclusion CEL expression.
	IncludeWhen string
}

// TransformerRegistry is the interface the engine uses to dispatch
// per-resource transformation. The concrete implementation lives in
// internal/transform/transformer to avoid circular imports.
type TransformerRegistry interface {
	// TransformResource applies the matching transformer to a resource.
	TransformResource(
		ctx context.Context,
		resource *k8s.Resource,
		resourceID string,
		fieldMappings []FieldMapping,
		values map[string]interface{},
	) (*TransformerOutput, error)
}
