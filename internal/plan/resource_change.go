package plan

import (
	"fmt"
	"reflect"
	"sort"
)

// ResourceChange represents a change to a managed resource between two RGD versions.
type ResourceChange struct {
	Type     ChangeType `json:"type"`
	ID       string     `json:"id"`
	Kind     string     `json:"kind,omitempty"`
	Details  string     `json:"details"`
	Breaking bool       `json:"breaking"`
}

// CompareResources compares the resource lists of two RGD maps and returns a list of changes.
func CompareResources(oldResources, newResources []interface{}) []ResourceChange {
	oldIdx := indexResourcesByID(oldResources)
	newIdx := indexResourcesByID(newResources)

	var changes []ResourceChange

	// Detect removed resources.
	for _, id := range sortedKeys(oldIdx) {
		if _, exists := newIdx[id]; !exists {
			changes = append(changes, ResourceChange{
				Type:     ChangeRemoved,
				ID:       id,
				Kind:     extractResourceKind(oldIdx[id]),
				Details:  "resource removed",
				Breaking: true,
			})
		}
	}

	// Detect added and modified resources.
	for _, id := range sortedKeys(newIdx) {
		oldRes, exists := oldIdx[id]
		if !exists {
			changes = append(changes, ResourceChange{
				Type:     ChangeAdded,
				ID:       id,
				Kind:     extractResourceKind(newIdx[id]),
				Details:  "resource added",
				Breaking: false,
			})

			continue
		}

		// Both exist — compare.
		if !mapsEqual(oldRes, newIdx[id]) {
			changes = append(changes, ResourceChange{
				Type:     ChangeModified,
				ID:       id,
				Kind:     extractResourceKind(newIdx[id]),
				Details:  "resource modified",
				Breaking: false,
			})
		}
	}

	// Sort: breaking first, then by ID.
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Breaking != changes[j].Breaking {
			return changes[i].Breaking
		}

		return changes[i].ID < changes[j].ID
	})

	return changes
}

// indexResourcesByID builds a lookup map of resource ID to its map representation.
func indexResourcesByID(resources []interface{}) map[string]map[string]interface{} {
	idx := make(map[string]map[string]interface{})

	for _, r := range resources {
		rm, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		id, ok := rm["id"].(string)
		if !ok {
			continue
		}

		idx[id] = rm
	}

	return idx
}

// extractResourceKind extracts the kind from a resource's template.
func extractResourceKind(resource map[string]interface{}) string {
	tmpl, ok := resource["template"]
	if !ok {
		return ""
	}

	tmplMap, ok := tmpl.(map[string]interface{})
	if !ok {
		return ""
	}

	kind, ok := tmplMap["kind"].(string)
	if !ok {
		return ""
	}

	return kind
}

// mapsEqual does a deep comparison of two maps.
func mapsEqual(a, b map[string]interface{}) bool {
	return reflect.DeepEqual(a, b)
}

// sortedKeys returns sorted keys for a map.
func sortedKeys(m map[string]map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// formatResourceRef returns a human-readable resource reference.
func formatResourceRef(id, kind string) string {
	if kind != "" {
		return fmt.Sprintf("%s (%s)", id, kind)
	}

	return id
}

// countResourcesByType counts resource changes of each type.
func countResourcesByType(changes []ResourceChange) (added, removed, modified int) {
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
