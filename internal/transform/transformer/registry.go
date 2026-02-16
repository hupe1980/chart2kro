package transformer

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
)

// Registry is a priority-ordered collection of transformers.
// The first transformer whose Matches method returns true for a given
// GVK is used. A DefaultTransformer is always registered as the final
// fallback.
type Registry struct {
	mu           sync.RWMutex
	transformers []Transformer
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a transformer to the registry. Transformers are matched
// in registration order, so register more specific transformers first.
func (r *Registry) Register(t Transformer) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.transformers = append(r.transformers, t)
}

// Prepend inserts a transformer at the front of the registry, giving it
// the highest priority. This is used for config-based transformers.
func (r *Registry) Prepend(t Transformer) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.transformers = append([]Transformer{t}, r.transformers...)
}

// TransformerFor returns the first transformer that matches the given GVK.
// Returns nil if no transformer matches (should not happen when
// DefaultTransformer is registered).
func (r *Registry) TransformerFor(gvk schema.GroupVersionKind) Transformer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.transformers {
		if t.Matches(gvk) {
			return t
		}
	}

	return nil
}

// All returns a copy of the registered transformers.
func (r *Registry) All() []Transformer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Transformer, len(r.transformers))
	copy(out, r.transformers)

	return out
}

// DefaultRegistry returns a registry pre-populated with the built-in
// transformers in priority order.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&DeploymentTransformer{})
	r.Register(&ServiceTransformer{})
	r.Register(&ConfigMapTransformer{})
	r.Register(&DefaultTransformer{})

	return r
}

// TransformResource implements transform.TransformerRegistry. It finds
// the first matching transformer for the resource's GVK and delegates.
func (r *Registry) TransformResource(
	ctx context.Context,
	resource *k8s.Resource,
	resourceID string,
	fieldMappings []transform.FieldMapping,
	values map[string]interface{},
) (*transform.TransformerOutput, error) {
	t := r.TransformerFor(resource.GVK)
	if t == nil {
		// No transformer matched — fall back to default projections.
		projections := transform.DefaultStatusProjections(resource.GVK, resourceID)
		return &transform.TransformerOutput{StatusFields: projections}, nil
	}

	input := TransformInput{
		Resource:      resource,
		ResourceID:    resourceID,
		Values:        values,
		FieldMappings: fieldMappings,
	}

	output, err := t.Transform(ctx, input)
	if err != nil {
		return nil, err
	}

	return &transform.TransformerOutput{
		ReadyWhen:    output.ReadyWhen,
		StatusFields: output.StatusFields,
		IncludeWhen:  output.IncludeWhen,
	}, nil
}

// Compile-time check that *Registry implements transform.TransformerRegistry.
var _ transform.TransformerRegistry = (*Registry)(nil)
