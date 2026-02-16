package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/hupe1980/chart2kro/internal/transform"
)

// Result holds the complete plan for an RGD generation.
type Result struct {
	Name                string           `json:"name"`
	SchemaFields        []SchemaField    `json:"schemaFields"`
	Resources           []Resource       `json:"resources"`
	StatusFields        []StatusField    `json:"statusFields"`
	HasBreakingChanges  bool             `json:"hasBreakingChanges,omitempty"`
	BreakingChangeCount int              `json:"breakingChangeCount,omitempty"`
	Evolution           *EvolutionResult `json:"evolution,omitempty"`
}

// SchemaField represents a schema field in the plan output.
type SchemaField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Default  string `json:"default,omitempty"`
	Required bool   `json:"required"`
	Path     string `json:"path,omitempty"`
}

// Resource represents a managed resource in the plan output.
type Resource struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	APIVersion  string   `json:"apiVersion,omitempty"`
	Conditional bool     `json:"conditional,omitempty"`
	DependsOn   []string `json:"dependsOn,omitempty"`
}

// StatusField represents a status projection in the plan output.
type StatusField struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
}

// BuildPlan constructs a Result from a transform.Result and the generated RGD map.
func BuildPlan(result *transform.Result, rgdMap map[string]interface{}) *Result {
	plan := &Result{
		Name: extractName(rgdMap),
	}

	// Build schema fields.
	for _, sf := range result.SchemaFields {
		plan.SchemaFields = append(plan.SchemaFields, flattenSchemaField("", sf)...)
	}

	// Build resources.
	for _, res := range result.Resources {
		id := result.ResourceIDs[res]
		pr := Resource{
			ID:         id,
			Kind:       res.Kind(),
			APIVersion: res.APIVersion(),
		}

		plan.Resources = append(plan.Resources, pr)
	}

	// If resources came out empty (no resource pointers), try from RGD map directly.
	if len(plan.Resources) == 0 {
		plan.Resources = buildResourcesFromMap(rgdMap)
	}

	// Build status fields.
	for _, sf := range result.StatusFields {
		plan.StatusFields = append(plan.StatusFields, StatusField{
			Name:       sf.Name,
			Expression: sf.CELExpression,
		})
	}

	return plan
}

// extractName pulls the RGD name from the metadata.
func extractName(rgdMap map[string]interface{}) string {
	meta, ok := rgdMap["metadata"].(map[string]interface{})
	if !ok {
		return ""
	}

	name, _ := meta["name"].(string)

	return name
}

// flattenSchemaField recursively flattens a SchemaField tree into a flat list.
func flattenSchemaField(prefix string, field *transform.SchemaField) []SchemaField {
	fullName := field.Name
	if prefix != "" {
		fullName = prefix + "." + field.Name
	}

	var fields []SchemaField

	fields = append(fields, SchemaField{
		Name:     fullName,
		Type:     field.Type,
		Default:  field.Default,
		Required: field.Default == "",
		Path:     field.Path,
	})

	for _, child := range field.Children {
		fields = append(fields, flattenSchemaField(fullName, child)...)
	}

	return fields
}

// buildResourcesFromMap extracts resource info directly from the RGD map.
func buildResourcesFromMap(rgdMap map[string]interface{}) []Resource {
	spec, ok := rgdMap["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	resources, ok := spec["resources"].([]interface{})
	if !ok {
		return nil
	}

	var planResources []Resource

	for _, r := range resources {
		rm, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := rm["id"].(string)
		kind := extractResourceKind(rm)

		pr := Resource{
			ID:   id,
			Kind: kind,
		}

		if rm["includeWhen"] != nil {
			pr.Conditional = true
		}

		if deps, ok := rm["dependsOn"].([]interface{}); ok {
			for _, d := range deps {
				if ds, ok := d.(string); ok {
					pr.DependsOn = append(pr.DependsOn, ds)
				}
			}
		}

		planResources = append(planResources, pr)
	}

	return planResources
}

// ApplyEvolution merges evolution analysis into the plan result.
func ApplyEvolution(plan *Result, evolution *EvolutionResult) {
	plan.Evolution = evolution
	plan.HasBreakingChanges = evolution.HasBreakingChanges()
	plan.BreakingChangeCount = evolution.BreakingCount()
}

// FormatPlan writes a human-readable plan to the given writer.
func FormatPlan(w io.Writer, plan *Result) {
	_, _ = fmt.Fprintf(w, "Plan: %s\n", plan.Name)
	_, _ = fmt.Fprintln(w, strings.Repeat("=", 60))

	// Schema fields.
	if len(plan.SchemaFields) > 0 {
		_, _ = fmt.Fprintln(w, "\nSchema Fields:")
		_, _ = fmt.Fprintln(w, strings.Repeat("-", 40))

		for _, f := range plan.SchemaFields {
			req := "optional"
			if f.Required {
				req = "required"
			}

			if f.Default != "" {
				_, _ = fmt.Fprintf(w, "  %-25s %s (default: %s) [%s]\n", f.Name, f.Type, f.Default, req)
			} else {
				_, _ = fmt.Fprintf(w, "  %-25s %s [%s]\n", f.Name, f.Type, req)
			}
		}
	}

	// Resources.
	if len(plan.Resources) > 0 {
		_, _ = fmt.Fprintln(w, "\nResources:")
		_, _ = fmt.Fprintln(w, strings.Repeat("-", 40))

		for _, r := range plan.Resources {
			cond := ""
			if r.Conditional {
				cond = " (conditional)"
			}

			_, _ = fmt.Fprintf(w, "  %-20s %s%s\n", r.ID, r.Kind, cond)

			if len(r.DependsOn) > 0 {
				_, _ = fmt.Fprintf(w, "    depends on: %s\n", strings.Join(r.DependsOn, ", "))
			}
		}
	}

	// Status fields.
	if len(plan.StatusFields) > 0 {
		_, _ = fmt.Fprintln(w, "\nStatus Projections:")
		_, _ = fmt.Fprintln(w, strings.Repeat("-", 40))

		for _, s := range plan.StatusFields {
			_, _ = fmt.Fprintf(w, "  %-25s %s\n", s.Name, s.Expression)
		}
	}

	// Summary footer.
	_, _ = fmt.Fprintf(w, "\nSummary: %d schema fields, %d resources, %d status projections\n",
		len(plan.SchemaFields), len(plan.Resources), len(plan.StatusFields))

	// Evolution summary.
	if plan.Evolution != nil && plan.Evolution.HasChanges() {
		_, _ = fmt.Fprintln(w)
		FormatTable(w, plan.Evolution)
	}

	_, _ = fmt.Fprintln(w)
}

// FormatPlanJSON writes the plan as JSON.
func FormatPlanJSON(w io.Writer, plan *Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(plan)
}

// FormatPlanCompact writes a compact summary of the plan.
func FormatPlanCompact(w io.Writer, plan *Result) {
	_, _ = fmt.Fprintf(w, "Plan: %s -- %d schema fields, %d resources, %d status projections\n",
		plan.Name,
		len(plan.SchemaFields),
		len(plan.Resources),
		len(plan.StatusFields),
	)

	if plan.Evolution != nil && plan.Evolution.HasChanges() {
		_, _ = fmt.Fprintf(w, "Evolution: %s\n", FormatCompactSummary(plan.Evolution))
	}

	if plan.HasBreakingChanges {
		_, _ = fmt.Fprintf(w, "WARNING: %d breaking changes detected!\n", plan.BreakingChangeCount)
	}
}
