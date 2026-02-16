package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// EvolutionResult holds the full comparison result between two RGD versions.
type EvolutionResult struct {
	SchemaChanges   []SchemaChange   `json:"schemaChanges"`
	ResourceChanges []ResourceChange `json:"resourceChanges"`
}

// HasBreakingChanges returns true if any change is breaking.
func (e *EvolutionResult) HasBreakingChanges() bool {
	for _, c := range e.SchemaChanges {
		if c.Breaking {
			return true
		}
	}

	for _, c := range e.ResourceChanges {
		if c.Breaking {
			return true
		}
	}

	return false
}

// BreakingCount returns the number of breaking changes.
func (e *EvolutionResult) BreakingCount() int {
	count := 0

	for _, c := range e.SchemaChanges {
		if c.Breaking {
			count++
		}
	}

	for _, c := range e.ResourceChanges {
		if c.Breaking {
			count++
		}
	}

	return count
}

// NonBreakingCount returns the number of non-breaking changes.
func (e *EvolutionResult) NonBreakingCount() int {
	count := 0

	for _, c := range e.SchemaChanges {
		if !c.Breaking {
			count++
		}
	}

	for _, c := range e.ResourceChanges {
		if !c.Breaking {
			count++
		}
	}

	return count
}

// HasChanges returns true if there are any changes.
func (e *EvolutionResult) HasChanges() bool {
	return len(e.SchemaChanges) > 0 || len(e.ResourceChanges) > 0
}

// Analyze compares two RGD maps and returns the full evolution result.
func Analyze(oldRGD, newRGD map[string]interface{}) *EvolutionResult {
	oldSchema := extractSchemaSpec(oldRGD)
	newSchema := extractSchemaSpec(newRGD)

	oldResources := extractResources(oldRGD)
	newResources := extractResources(newRGD)

	return &EvolutionResult{
		SchemaChanges:   CompareSchemas(oldSchema, newSchema),
		ResourceChanges: CompareResources(oldResources, newResources),
	}
}

// extractSchemaSpec extracts the spec.schema.spec section from an RGD map.
func extractSchemaSpec(rgd map[string]interface{}) map[string]interface{} {
	spec, ok := rgd["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	schema, ok := spec["schema"].(map[string]interface{})
	if !ok {
		return nil
	}

	schemaSpec, ok := schema["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	return schemaSpec
}

// extractResources extracts the spec.resources array from an RGD map.
func extractResources(rgd map[string]interface{}) []interface{} {
	spec, ok := rgd["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	resources, ok := spec["resources"].([]interface{})
	if !ok {
		return nil
	}

	return resources
}

// FormatTable writes the evolution result as a human-readable table.
func FormatTable(w io.Writer, result *EvolutionResult) {
	if !result.HasChanges() {
		_, _ = fmt.Fprintln(w, "No changes detected.")
		return
	}

	if len(result.SchemaChanges) > 0 {
		_, _ = fmt.Fprintln(w, "Schema Changes:")
		_, _ = fmt.Fprintln(w, strings.Repeat("-", 60))

		for _, c := range result.SchemaChanges {
			icon := changeIcon(c.Type, c.Breaking)
			_, _ = fmt.Fprintf(w, "  %s %-30s %s\n", icon, c.Field, c.Details)

			if c.Impact != "" {
				_, _ = fmt.Fprintf(w, "    Impact: %s\n", c.Impact)
			}
		}

		_, _ = fmt.Fprintln(w)
	}

	if len(result.ResourceChanges) > 0 {
		_, _ = fmt.Fprintln(w, "Resource Changes:")
		_, _ = fmt.Fprintln(w, strings.Repeat("-", 60))

		for _, c := range result.ResourceChanges {
			icon := changeIcon(c.Type, c.Breaking)
			ref := formatResourceRef(c.ID, c.Kind)
			_, _ = fmt.Fprintf(w, "  %s %-30s %s\n", icon, ref, c.Details)
		}

		_, _ = fmt.Fprintln(w)
	}

	// Summary.
	breaking := result.BreakingCount()
	nonBreaking := result.NonBreakingCount()

	_, _ = fmt.Fprintf(w, "Breaking changes: %d, Non-breaking changes: %d\n", breaking, nonBreaking)

	if breaking > 0 {
		_, _ = fmt.Fprintln(w, "\nWARNING: Breaking changes detected! Review carefully before upgrading.")
	}
}

// FormatJSON writes the evolution result as JSON.
func FormatJSON(w io.Writer, result *EvolutionResult) error {
	output := struct {
		SchemaChanges   []SchemaChange   `json:"schemaChanges"`
		ResourceChanges []ResourceChange `json:"resourceChanges"`
		Summary         struct {
			Breaking    int  `json:"breaking"`
			NonBreaking int  `json:"nonBreaking"`
			HasBreaking bool `json:"hasBreaking"`
		} `json:"summary"`
	}{
		SchemaChanges:   result.SchemaChanges,
		ResourceChanges: result.ResourceChanges,
	}
	output.Summary.Breaking = result.BreakingCount()
	output.Summary.NonBreaking = result.NonBreakingCount()
	output.Summary.HasBreaking = result.HasBreakingChanges()

	if output.SchemaChanges == nil {
		output.SchemaChanges = []SchemaChange{}
	}

	if output.ResourceChanges == nil {
		output.ResourceChanges = []ResourceChange{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(output)
}

// changeIcon returns an icon/prefix for a change type.
func changeIcon(ct ChangeType, breaking bool) string {
	if breaking {
		return "! "
	}

	switch ct {
	case ChangeAdded:
		return "+ "
	case ChangeRemoved:
		return "- "
	case ChangeModified:
		return "~ "
	default:
		return "  "
	}
}

// FormatCompactSummary returns a single-line summary of the evolution result.
func FormatCompactSummary(result *EvolutionResult) string {
	if !result.HasChanges() {
		return "No changes detected."
	}

	var parts []string

	sAdded, sRemoved, sModified := countByType(result.SchemaChanges)
	if sAdded > 0 {
		parts = append(parts, fmt.Sprintf("%d schema fields added", sAdded))
	}

	if sRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d schema fields removed", sRemoved))
	}

	if sModified > 0 {
		parts = append(parts, fmt.Sprintf("%d schema fields modified", sModified))
	}

	rAdded, rRemoved, rModified := countResourcesByType(result.ResourceChanges)
	if rAdded > 0 {
		parts = append(parts, fmt.Sprintf("%d resources added", rAdded))
	}

	if rRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d resources removed", rRemoved))
	}

	if rModified > 0 {
		parts = append(parts, fmt.Sprintf("%d resources modified", rModified))
	}

	return strings.Join(parts, ", ")
}

// countByType counts schema changes by type.
func countByType(changes []SchemaChange) (added, removed, modified int) {
	for _, c := range changes {
		switch c.Type {
		case ChangeAdded:
			added++
		case ChangeRemoved:
			removed++
		case ChangeModified:
			modified++
		}
	}

	return
}
