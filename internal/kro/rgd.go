// Package kro assembles KRO ResourceGraphDefinition resources from
// transformation results.
package kro

import (
	"fmt"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/maputil"
	"github.com/hupe1980/chart2kro/internal/transform"
)

const (
	// APIVersion is the KRO API version.
	APIVersion = "kro.run/v1alpha1"
	// Kind is the KRO resource kind.
	Kind = "ResourceGraphDefinition"
)

// RGD represents a KRO ResourceGraphDefinition.
type RGD struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Spec       Spec     `json:"spec"`
}

// Metadata holds RGD metadata.
type Metadata struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Spec holds the RGD spec.
type Spec struct {
	Schema    *Schema    `json:"schema,omitempty"`
	Resources []Resource `json:"resources"`
}

// Schema holds the SimpleSchema definition.
type Schema struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Spec       map[string]interface{} `json:"spec,omitempty"`
	Status     map[string]interface{} `json:"status,omitempty"`
}

// Resource is a single resource in the RGD.
type Resource struct {
	ID          string                 `json:"id"`
	Template    map[string]interface{} `json:"template"`
	ReadyWhen   []string               `json:"readyWhen,omitempty"`
	IncludeWhen []string               `json:"includeWhen,omitempty"`
	DependsOn   []string               `json:"dependsOn,omitempty"`
}

// GeneratorConfig holds the configuration for RGD generation.
type GeneratorConfig struct {
	// Name is the RGD metadata name.
	Name string
	// ChartName is the Helm chart name (used in labels).
	ChartName string
	// ChartVersion is the Helm chart version (used in labels).
	ChartVersion string
	// SchemaKind overrides the generated CRD kind (default: PascalCase name).
	SchemaKind string
	// SchemaAPIVersion overrides the schema apiVersion (default: "v1alpha1").
	SchemaAPIVersion string
	// SchemaGroup overrides the schema group (default: "kro.run").
	SchemaGroup string
	// SchemaFields are the extracted schema fields for the RGD spec.
	SchemaFields []*transform.SchemaField
	// StatusFields are the status projections to include.
	StatusFields []transform.StatusField
	// CustomReadyConditions are user-supplied readiness conditions keyed by Kind.
	// When set, they override the built-in defaults for matching Kinds.
	CustomReadyConditions map[string][]string
}

// Generator builds a KRO ResourceGraphDefinition from parsed resources.
type Generator struct {
	config GeneratorConfig
}

// NewGenerator creates a new RGD generator.
func NewGenerator(config GeneratorConfig) *Generator {
	return &Generator{config: config}
}

// Generate produces an RGD from the dependency graph.
func (g *Generator) Generate(depGraph *transform.DependencyGraph) (*RGD, error) {
	order, err := depGraph.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("topological sort failed: %w", err)
	}

	resources := make([]Resource, 0, len(order))

	for _, id := range order {
		r := depGraph.Resource(id)
		if r == nil {
			continue
		}

		res, err := g.buildResource(id, r, depGraph)
		if err != nil {
			return nil, fmt.Errorf("building resource %s: %w", id, err)
		}

		resources = append(resources, res)
	}

	rgd := &RGD{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata:   g.buildMetadata(),
		Spec: Spec{
			Schema:    g.buildSchema(),
			Resources: resources,
		},
	}

	return rgd, nil
}

func (g *Generator) buildMetadata() Metadata {
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "chart2kro",
	}

	if g.config.ChartName != "" {
		labels["app.kubernetes.io/name"] = g.config.ChartName
	}

	if g.config.ChartVersion != "" {
		labels["app.kubernetes.io/version"] = g.config.ChartVersion
	}

	annotations := map[string]string{
		"chart2kro.dev/generated": "true",
	}

	return Metadata{
		Name:        g.config.Name,
		Labels:      labels,
		Annotations: annotations,
	}
}

func (g *Generator) buildSchema() *Schema {
	if len(g.config.SchemaFields) == 0 && len(g.config.StatusFields) == 0 {
		return nil
	}

	// Determine schema kind.
	kind := g.config.SchemaKind
	if kind == "" {
		kind = transform.ToPascalCase(g.config.Name)
	}

	// Determine schema group and version.
	group := g.config.SchemaGroup
	if group == "" {
		group = "kro.run"
	}

	version := g.config.SchemaAPIVersion
	if version == "" {
		version = "v1alpha1"
	}

	apiVersion := g.config.Name + "." + group + "/" + version

	s := &Schema{
		APIVersion: apiVersion,
		Kind:       kind,
	}

	if len(g.config.SchemaFields) > 0 {
		s.Spec = transform.BuildSimpleSchema(g.config.SchemaFields)
	}

	if len(g.config.StatusFields) > 0 {
		status := make(map[string]interface{})
		for _, sf := range g.config.StatusFields {
			status[sf.Name] = sf.CELExpression
		}

		s.Status = status
	}

	return s
}

func (g *Generator) buildResource(id string, r *k8s.Resource, depGraph *transform.DependencyGraph) (Resource, error) {
	template := make(map[string]interface{})

	if r.Object != nil {
		template = maputil.DeepCopyMap(r.Object.Object)
	} else {
		template["apiVersion"] = transform.GVKToAPIVersion(r.GVK)
		template["kind"] = r.Kind()
		template["metadata"] = map[string]interface{}{"name": r.Name}
	}

	res := Resource{
		ID:       id,
		Template: template,
	}

	// Add readyWhen conditions.
	readyWhen := transform.ResolveReadyWhen(r.GVK, g.config.CustomReadyConditions)
	res.ReadyWhen = readyWhen

	// Add dependsOn from the graph.
	deps := depGraph.DependenciesOf(id)
	if len(deps) > 0 {
		res.DependsOn = deps
	}

	return res, nil
}

// ToMap converts an RGD to a map[string]interface{} for YAML serialization.
func (r *RGD) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"apiVersion": r.APIVersion,
		"kind":       r.Kind,
		"metadata":   metadataToMap(r.Metadata),
		"spec":       specToMap(r.Spec),
	}

	return result
}

func metadataToMap(m Metadata) map[string]interface{} {
	result := map[string]interface{}{
		"name": m.Name,
	}

	if len(m.Labels) > 0 {
		labels := make(map[string]interface{}, len(m.Labels))
		for k, v := range m.Labels {
			labels[k] = v
		}

		result["labels"] = labels
	}

	if len(m.Annotations) > 0 {
		annotations := make(map[string]interface{}, len(m.Annotations))
		for k, v := range m.Annotations {
			annotations[k] = v
		}

		result["annotations"] = annotations
	}

	return result
}

func specToMap(s Spec) map[string]interface{} {
	result := make(map[string]interface{})

	if s.Schema != nil {
		schema := map[string]interface{}{
			"apiVersion": s.Schema.APIVersion,
			"kind":       s.Schema.Kind,
		}

		if len(s.Schema.Spec) > 0 {
			schema["spec"] = s.Schema.Spec
		}

		if len(s.Schema.Status) > 0 {
			schema["status"] = s.Schema.Status
		}

		result["schema"] = schema
	}

	if len(s.Resources) > 0 {
		resources := make([]interface{}, len(s.Resources))

		for i, r := range s.Resources {
			res := map[string]interface{}{
				"id":       r.ID,
				"template": r.Template,
			}

			if len(r.ReadyWhen) > 0 {
				readyWhen := make([]interface{}, len(r.ReadyWhen))
				for j, rw := range r.ReadyWhen {
					readyWhen[j] = rw
				}

				res["readyWhen"] = readyWhen
			}

			if len(r.IncludeWhen) > 0 {
				includeWhen := make([]interface{}, len(r.IncludeWhen))
				for j, iw := range r.IncludeWhen {
					includeWhen[j] = iw
				}

				res["includeWhen"] = includeWhen
			}

			if len(r.DependsOn) > 0 {
				dependsOn := make([]interface{}, len(r.DependsOn))
				for j, d := range r.DependsOn {
					dependsOn[j] = d
				}

				res["dependsOn"] = dependsOn
			}

			resources[i] = res
		}

		result["resources"] = resources
	}

	return result
}
