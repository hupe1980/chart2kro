// Package transform - engine.go orchestrates the full transformation pipeline
// that converts parsed Kubernetes resources and Helm values into a KRO
// ResourceGraphDefinition.
package transform

import (
	"context"
	"fmt"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// Result holds all artifacts produced by the transformation pipeline.
type Result struct {
	// Resources is the list of parsed Kubernetes resources.
	Resources []*k8s.Resource

	// ResourceIDs maps each resource to its assigned ID.
	ResourceIDs map[*k8s.Resource]string

	// SchemaFields are the extracted schema fields.
	SchemaFields []*SchemaField

	// StatusFields are the generated status projections.
	StatusFields []StatusField

	// DependencyGraph is the constructed dependency graph.
	DependencyGraph *DependencyGraph

	// FieldMappings are the detected parameter mappings.
	FieldMappings []FieldMapping
}

// EngineConfig configures the transformation engine.
type EngineConfig struct {
	// IncludeAllValues includes all values in the schema, even unreferenced ones.
	IncludeAllValues bool

	// FlatSchema uses flat camelCase field names instead of nested objects.
	FlatSchema bool

	// ResourceIDOverrides allows manual ID assignment by qualified name.
	ResourceIDOverrides map[string]string

	// FieldMappings are pre-computed sentinel-based parameter mappings.
	// When non-nil, the engine applies these to resource templates,
	// replacing hardcoded values with CEL expressions.
	FieldMappings []FieldMapping

	// ReferencedPaths is the set of Helm value paths detected as referenced.
	// When non-nil and IncludeAllValues is false, only these paths are
	// included in the schema.
	ReferencedPaths map[string]bool

	// JSONSchemaBytes is the raw values.schema.json from the chart.
	// When non-nil, enriches schema type inference with explicit JSON Schema types.
	JSONSchemaBytes []byte

	// SchemaOverrides override inferred schema field types and defaults.
	// Keys are dotted Helm value paths (e.g., "replicaCount", "image.tag").
	SchemaOverrides map[string]SchemaOverride

	// TransformerRegistry is an optional pluggable transformer registry.
	// When non-nil, the engine dispatches per-resource transformation
	// through the registry to produce readiness conditions and status
	// projections. When nil, DefaultStatusProjections is used.
	TransformerRegistry TransformerRegistry
}

// Engine orchestrates the full transformation pipeline.
type Engine struct {
	config EngineConfig
}

// NewEngine creates a new transformation engine.
func NewEngine(config EngineConfig) *Engine {
	return &Engine{config: config}
}

// Transform runs the full transformation pipeline:
// 1. Assign resource IDs
// 2. Apply field mappings to resource templates (CEL expression injection)
// 3. Extract schema from values (with optional pruning)
// 4. Build dependency graph
// 5. Generate status projections via transformer registry
func (e *Engine) Transform(
	ctx context.Context,
	resources []*k8s.Resource,
	values map[string]interface{},
) (*Result, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources to transform")
	}

	// 1. Assign resource IDs.
	resourceIDs, err := AssignResourceIDs(resources, e.config.ResourceIDOverrides)
	if err != nil {
		return nil, fmt.Errorf("assigning resource IDs: %w", err)
	}

	// 2. Apply field mappings to resource templates.
	if len(e.config.FieldMappings) > 0 {
		ApplyFieldMappings(resources, resourceIDs, e.config.FieldMappings)
	}

	// 3. Extract schema from Helm values (enriched with JSON Schema when available).
	jsonSchemaResolver, jsonSchemaErr := NewJSONSchemaResolver(e.config.JSONSchemaBytes)
	if jsonSchemaErr != nil {
		return nil, fmt.Errorf("parsing values.schema.json: %w", jsonSchemaErr)
	}

	extractor := NewSchemaExtractor(e.config.IncludeAllValues, e.config.FlatSchema, jsonSchemaResolver)

	var refs map[string]bool
	if !e.config.IncludeAllValues && e.config.ReferencedPaths != nil {
		refs = e.config.ReferencedPaths
	}

	schemaFields := extractor.Extract(values, refs)

	// 3b. Apply schema overrides from config.
	if len(e.config.SchemaOverrides) > 0 {
		ApplySchemaOverrides(schemaFields, e.config.SchemaOverrides)
	}

	// 4. Build dependency graph.
	depGraph := BuildDependencyGraph(resourceIDs)

	// 5. Validate: check for cycles.
	cycles := depGraph.DetectCycles()
	if len(cycles) > 0 {
		return nil, &CycleError{Cycles: cycles}
	}

	// 6. Generate status projections via transformer registry.
	var statusFields []StatusField

	for _, r := range resources {
		id := resourceIDs[r]

		if e.config.TransformerRegistry != nil {
			output, transformErr := e.config.TransformerRegistry.TransformResource(ctx, r, id, e.config.FieldMappings, values)
			if transformErr != nil {
				return nil, fmt.Errorf("transformer for %s/%s: %w", r.GVK.Kind, id, transformErr)
			}

			statusFields = append(statusFields, output.StatusFields...)
		} else {
			projections := DefaultStatusProjections(r.GVK, id)
			statusFields = append(statusFields, projections...)
		}
	}

	return &Result{
		Resources:       resources,
		ResourceIDs:     resourceIDs,
		SchemaFields:    schemaFields,
		StatusFields:    statusFields,
		DependencyGraph: depGraph,
		FieldMappings:   e.config.FieldMappings,
	}, nil
}

// CycleError is returned when the dependency graph contains cycles.
type CycleError struct {
	Cycles [][]string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("dependency graph contains %d cycle(s): %v", len(e.Cycles), e.Cycles)
}
