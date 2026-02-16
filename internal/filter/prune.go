package filter

import (
	"github.com/hupe1980/chart2kro/internal/k8s"
)

// PruneOrphanedFields identifies schema value paths that are only referenced
// by excluded resources. These paths should be removed from the generated
// schema since no remaining resource uses them.
//
// Parameters:
//   - mappings: all field mappings discovered by the parameter detection phase.
//     Each mapping has a ResourceID and a ValuesPath.
//   - excluded: resources that were excluded by the filter chain.
//   - resourceIDs: maps resource pointers to their assigned IDs.
//
// Returns the set of value paths that should be pruned from the schema.
func PruneOrphanedFields(
	mappings []FieldMappingRef,
	excluded []ExcludedResource,
	resourceIDs map[*k8s.Resource]string,
) map[string]bool {
	// Build a set of excluded resource IDs.
	excludedIDs := make(map[string]bool, len(excluded))
	for _, ex := range excluded {
		if id, ok := resourceIDs[ex.Resource]; ok {
			excludedIDs[id] = true
		}
	}

	if len(excludedIDs) == 0 {
		return nil
	}

	// For each values path, track whether it is referenced by at least one
	// included (non-excluded) resource.
	//
	// pathReferencedByIncluded[path] = true  → at least one included resource uses it
	// pathReferencedByIncluded[path] = false → only excluded resources reference it
	pathReferencedByIncluded := make(map[string]bool)

	for _, m := range mappings {
		if excludedIDs[m.ResourceID] {
			// Mark as seen (but only if not already marked as included).
			if _, seen := pathReferencedByIncluded[m.ValuesPath]; !seen {
				pathReferencedByIncluded[m.ValuesPath] = false
			}
		} else {
			// This path is used by an included resource — keep it.
			pathReferencedByIncluded[m.ValuesPath] = true
		}
	}

	// Collect paths that are only referenced by excluded resources.
	orphaned := make(map[string]bool)
	for path, includedRef := range pathReferencedByIncluded {
		if !includedRef {
			orphaned[path] = true
		}
	}

	if len(orphaned) == 0 {
		return nil
	}

	return orphaned
}

// FieldMappingRef is a minimal representation of a field mapping,
// containing only the information needed for orphan detection.
// This avoids a circular dependency on the transform package.
type FieldMappingRef struct {
	// ResourceID is the assigned ID of the resource this mapping targets.
	ResourceID string
	// ValuesPath is the Helm values path (e.g., "image.tag").
	ValuesPath string
}
