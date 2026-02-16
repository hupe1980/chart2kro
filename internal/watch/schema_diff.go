package watch

import (
	"fmt"
	"strings"

	"github.com/hupe1980/chart2kro/internal/transform"
)

// SchemaChange describes a single change to the API schema between
// two consecutive generations.
type SchemaChange struct {
	// Kind is one of "added", "removed", or "default-changed".
	Kind string
	// Field is the dotted path of the schema field.
	Field string
	// Detail provides extra information (e.g., old and new default).
	Detail string
}

// SchemaDiff compares two sets of schema fields and returns the changes.
func SchemaDiff(prev, curr []*transform.SchemaField) []SchemaChange {
	prevMap := flattenFields("", prev)
	currMap := flattenFields("", curr)

	var changes []SchemaChange

	// Detect removed fields.
	for path, pf := range prevMap {
		if _, ok := currMap[path]; !ok {
			changes = append(changes, SchemaChange{Kind: "removed", Field: path, Detail: pf.Type})
		}
	}

	// Detect added and default-changed fields.
	for path, cf := range currMap {
		pf, existed := prevMap[path]
		if !existed {
			changes = append(changes, SchemaChange{Kind: "added", Field: path, Detail: cf.Type})
			continue
		}

		if pf.Default != cf.Default {
			changes = append(changes, SchemaChange{
				Kind:   "default-changed",
				Field:  path,
				Detail: fmt.Sprintf("%v -> %v", pf.Default, cf.Default),
			})
		}
	}

	return changes
}

// SchemaDiffSummary returns a human-readable one-line summary.
func SchemaDiffSummary(changes []SchemaChange) string {
	var added, removed, changed int

	for _, c := range changes {
		switch c.Kind {
		case "added":
			added++
		case "removed":
			removed++
		case "default-changed":
			changed++
		}
	}

	if added == 0 && removed == 0 && changed == 0 {
		return "no schema changes"
	}

	parts := make([]string, 0, 3)

	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d field(s) added", added))
	}

	if removed > 0 {
		parts = append(parts, fmt.Sprintf("-%d field(s) removed", removed))
	}

	if changed > 0 {
		parts = append(parts, fmt.Sprintf("~%d default(s) changed", changed))
	}

	return strings.Join(parts, ", ")
}

type flatField struct {
	Type    string
	Default string
}

func flattenFields(prefix string, fields []*transform.SchemaField) map[string]flatField {
	result := make(map[string]flatField)

	for _, f := range fields {
		path := f.Name
		if prefix != "" {
			path = prefix + "." + f.Name
		}

		if len(f.Children) > 0 {
			for k, v := range flattenFields(path, f.Children) {
				result[k] = v
			}
		} else {
			result[path] = flatField{Type: f.Type, Default: f.Default}
		}
	}

	return result
}
