package filter

import (
	"context"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// Filter is the interface for all resource filters.
// Filters are stateless — they receive a set of resources and return
// a result without modifying shared state.
type Filter interface {
	// Apply runs the filter on the given resources and returns a result.
	// The context allows cancellation of long-running filter operations.
	Apply(ctx context.Context, resources []*k8s.Resource) (*Result, error)
}

// ExcludedResource records a resource that was excluded by a filter.
type ExcludedResource struct {
	// Resource is the excluded resource.
	Resource *k8s.Resource
	// Reason is a human-readable explanation for the exclusion.
	Reason string
}

// ExternalizedResource records a resource promoted to an externalRef.
type ExternalizedResource struct {
	// Resource is the original excluded resource.
	Resource *k8s.Resource
	// ExternalRef is the generated KRO externalRef entry.
	ExternalRef map[string]interface{}
	// SchemaFields are additional schema fields added for the external resource.
	SchemaFields map[string]string
	// Rewirings maps original reference values to their CEL replacements.
	Rewirings map[string]string
}

// Result holds the outcome of a filter application.
type Result struct {
	// Included are the resources that passed the filter.
	Included []*k8s.Resource
	// Excluded are the resources removed by the filter.
	Excluded []ExcludedResource
	// Externalized are the resources promoted to externalRef entries.
	Externalized []ExternalizedResource
	// SchemaAdditions are new schema fields to add to the RGD.
	SchemaAdditions map[string]string
}

// NewResult creates an empty Result.
func NewResult() *Result {
	return &Result{
		SchemaAdditions: make(map[string]string),
	}
}

// Chain applies multiple filters sequentially, passing the included
// resources from each filter as input to the next.
type Chain struct {
	filters []Filter
}

// NewChain creates a filter chain from the given filters.
func NewChain(filters ...Filter) *Chain {
	return &Chain{filters: filters}
}

// Apply runs all filters in order, accumulating excluded and externalized
// resources. Returns the combined result.
func (c *Chain) Apply(ctx context.Context, resources []*k8s.Resource) (*Result, error) {
	combined := NewResult()
	current := resources

	for _, f := range c.filters {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		r, err := f.Apply(ctx, current)
		if err != nil {
			return nil, err
		}

		current = r.Included

		combined.Excluded = append(combined.Excluded, r.Excluded...)
		combined.Externalized = append(combined.Externalized, r.Externalized...)

		for k, v := range r.SchemaAdditions {
			combined.SchemaAdditions[k] = v
		}
	}

	combined.Included = current

	return combined, nil
}
