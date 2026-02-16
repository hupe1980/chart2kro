// Package transformer defines the Transformer interface and Registry
// for per-resource-kind transformation logic. Transformers allow
// resource-specific customization of CEL expressions, readiness
// conditions, status projections, and conditional inclusion.
package transformer

import (
	"context"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TransformInput holds all context needed for resource transformation.
type TransformInput struct {
	// Resource is the parsed Kubernetes resource.
	Resource *k8s.Resource

	// ResourceID is the assigned resource ID.
	ResourceID string

	// Values is the merged Helm values.
	Values map[string]interface{}

	// FieldMappings are the detected parameter mappings for this resource.
	FieldMappings []transform.FieldMapping
}

// TransformOutput holds the transformation results for a single resource.
type TransformOutput struct {
	// ReadyWhen are the readiness conditions (CEL expressions).
	ReadyWhen []string

	// StatusFields are the status projections.
	StatusFields []transform.StatusField

	// IncludeWhen is an optional conditional inclusion CEL expression.
	IncludeWhen string
}

// Transformer is the interface for per-resource-kind transformation logic.
// Implementations produce readiness conditions, status projections, and
// conditional inclusion expressions for specific resource types.
type Transformer interface {
	// Name returns the transformer's name for logging.
	Name() string

	// Matches returns true if this transformer handles the given GVK.
	Matches(gvk schema.GroupVersionKind) bool

	// Transform applies resource-specific transformation logic.
	Transform(ctx context.Context, input TransformInput) (*TransformOutput, error)
}
