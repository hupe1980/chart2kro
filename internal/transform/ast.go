// Package transform - ast.go implements Go template AST analysis for fast mode.
// Instead of sentinel rendering (O(N+1) renders), it parses template files
// once to find .Values.* references and matches rendered field values against
// known Helm values to produce FieldMappings in a single render pass.
package transform

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"text/template/parse"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// AnalyzeTemplates parses Go template files and extracts all .Values.* paths
// referenced in template actions. This returns the set of referenced value
// paths without requiring sentinel rendering.
//
// templateFiles is a map of template filename to template content (e.g.,
// from chart.Chart.Templates). Only files with .yaml/.yml/.tpl extensions
// are parsed.
func AnalyzeTemplates(templateFiles map[string]string) (map[string]bool, error) {
	referencedPaths := make(map[string]bool)

	for name, content := range templateFiles {
		// Only parse template files (skip NOTES.txt, etc.).
		lower := strings.ToLower(name)
		if !isTemplateFile(lower) {
			continue
		}

		paths, err := extractValuesRefs(name, content)
		if err != nil {
			// Template parse errors are non-fatal — skip the file.
			continue
		}

		for _, p := range paths {
			referencedPaths[p] = true
		}
	}

	return referencedPaths, nil
}

// isTemplateFile returns true if the filename looks like a Helm template file.
func isTemplateFile(name string) bool {
	return strings.HasSuffix(name, ".yaml") ||
		strings.HasSuffix(name, ".yml") ||
		strings.HasSuffix(name, ".tpl")
}

// extractValuesRefs parses a single Go template and returns all .Values.*
// dotted paths found.
func extractValuesRefs(name, content string) ([]string, error) {
	tree, err := parse.Parse(name, content, "{{", "}}")
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", name, err)
	}

	var paths []string

	for _, t := range tree {
		if t.Root == nil {
			continue
		}

		walkNode(t.Root, &paths)
	}

	return paths, nil
}

// walkNode recursively walks a parse tree node, collecting .Values.* paths
// from FieldNodes and PipeNodes.
func walkNode(node parse.Node, paths *[]string) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}

		for _, child := range n.Nodes {
			walkNode(child, paths)
		}

	case *parse.ActionNode:
		if n.Pipe != nil {
			walkNode(n.Pipe, paths)
		}

	case *parse.PipeNode:
		if n == nil {
			return
		}

		for _, cmd := range n.Cmds {
			walkNode(cmd, paths)
		}

	case *parse.CommandNode:
		if n == nil {
			return
		}

		for _, arg := range n.Args {
			walkNode(arg, paths)
		}

	case *parse.FieldNode:
		// FieldNode represents .X.Y.Z (starts with dot).
		// We're looking for patterns starting with "Values".
		if len(n.Ident) >= 2 && n.Ident[0] == "Values" {
			path := strings.Join(n.Ident[1:], ".")
			*paths = append(*paths, path)
		}

	case *parse.IfNode:
		walkBranchNode(&n.BranchNode, paths)

	case *parse.RangeNode:
		walkBranchNode(&n.BranchNode, paths)

	case *parse.WithNode:
		walkBranchNode(&n.BranchNode, paths)

	case *parse.TemplateNode:
		if n.Pipe != nil {
			walkNode(n.Pipe, paths)
		}
	}
}

// walkBranchNode walks the pipe, list, and else list of a branch node.
func walkBranchNode(n *parse.BranchNode, paths *[]string) {
	if n.Pipe != nil {
		walkNode(n.Pipe, paths)
	}

	if n.List != nil {
		walkNode(n.List, paths)
	}

	if n.ElseList != nil {
		walkNode(n.ElseList, paths)
	}
}

// MatchFieldsByValue walks rendered resources and matches field values against
// known Helm values to produce FieldMappings without sentinel rendering.
// This is the field-mapping phase for fast mode.
//
// It flattens Helm values to a map of path→value, then walks each resource's
// fields. If a field value matches a known value exactly, a FieldMapping with
// MatchExact is created. If a field value contains known string values as
// substrings, a FieldMapping with MatchSubstring is created.
//
// Only paths present in referencedPaths (from AST analysis) are considered.
func MatchFieldsByValue(
	resources []*k8s.Resource,
	resourceIDs map[*k8s.Resource]string,
	values map[string]interface{},
	referencedPaths map[string]bool,
) []FieldMapping {
	// Flatten values to path→value map, filtered to referenced paths only.
	flatValues := flattenValues(values, "", referencedPaths)
	if len(flatValues) == 0 {
		return nil
	}

	// Build reverse index: string(value) → []path for substring matching.
	stringIndex := make(map[string][]string)

	for path, val := range flatValues {
		s := fmt.Sprintf("%v", val)
		if s != "" && s != "<nil>" {
			stringIndex[s] = append(stringIndex[s], path)
		}
	}

	var mappings []FieldMapping

	for _, r := range resources {
		if r.Object == nil {
			continue
		}

		id := resourceIDs[r]
		resourceMappings := matchFieldsRecursive(r.Object.Object, id, "", flatValues, stringIndex)
		mappings = append(mappings, resourceMappings...)
	}

	return mappings
}

// flattenValues recursively flattens a values map to path→value pairs,
// filtered to only include paths in referencedPaths.
func flattenValues(values map[string]interface{}, prefix string, referencedPaths map[string]bool) map[string]interface{} {
	result := make(map[string]interface{})

	for key, val := range values {
		path := joinFieldPath(prefix, key)

		switch v := val.(type) {
		case map[string]interface{}:
			for k, v := range flattenValues(v, path, referencedPaths) {
				result[k] = v
			}
		default:
			if referencedPaths[path] {
				result[path] = val
			}
		}
	}

	return result
}

// matchFieldsRecursive walks a resource's fields and matches values.
func matchFieldsRecursive(
	obj map[string]interface{},
	resourceID, prefix string,
	flatValues map[string]interface{},
	stringIndex map[string][]string,
) []FieldMapping {
	var mappings []FieldMapping

	for key, val := range obj {
		fieldPath := joinFieldPath(prefix, key)

		switch v := val.(type) {
		case map[string]interface{}:
			mappings = append(mappings, matchFieldsRecursive(v, resourceID, fieldPath, flatValues, stringIndex)...)

		case []interface{}:
			for i, item := range v {
				itemPath := fmt.Sprintf("%s[%d]", fieldPath, i)

				if m, ok := item.(map[string]interface{}); ok {
					mappings = append(mappings, matchFieldsRecursive(m, resourceID, itemPath, flatValues, stringIndex)...)
				} else {
					mappings = append(mappings, matchFieldValue(item, resourceID, itemPath, flatValues, stringIndex)...)
				}
			}

		default:
			mappings = append(mappings, matchFieldValue(val, resourceID, fieldPath, flatValues, stringIndex)...)
		}
	}

	return mappings
}

// matchFieldValue checks if a single field value matches any known Helm value.
func matchFieldValue(
	val interface{},
	resourceID, fieldPath string,
	flatValues map[string]interface{},
	stringIndex map[string][]string,
) []FieldMapping {
	// Sort flatValues keys for deterministic matching when multiple values
	// have the same value (e.g., image.repo and backup.image.repo both = "nginx").
	sortedPaths := make([]string, 0, len(flatValues))
	for k := range flatValues {
		sortedPaths = append(sortedPaths, k)
	}

	sort.Strings(sortedPaths)

	// Check for exact matches first.
	for _, valPath := range sortedPaths {
		knownVal := flatValues[valPath]
		if reflect.DeepEqual(val, knownVal) {
			return []FieldMapping{{
				ValuesPath: valPath,
				ResourceID: resourceID,
				FieldPath:  fieldPath,
				MatchType:  MatchExact,
			}}
		}
	}

	// Check for substring matches in string fields.
	s, ok := val.(string)
	if !ok || s == "" {
		return nil
	}

	var mappings []FieldMapping

	seen := make(map[string]bool)

	// Sort stringIndex keys for deterministic iteration order.
	sortedStrVals := make([]string, 0, len(stringIndex))
	for strVal := range stringIndex {
		sortedStrVals = append(sortedStrVals, strVal)
	}

	sort.Strings(sortedStrVals)

	for _, strVal := range sortedStrVals {
		valPaths := stringIndex[strVal]

		if !strings.Contains(s, strVal) {
			continue
		}

		// Don't match very short strings (likely false positives).
		if len(strVal) < 2 {
			continue
		}

		for _, valPath := range valPaths {
			if seen[valPath] {
				continue
			}

			seen[valPath] = true

			mappings = append(mappings, FieldMapping{
				ValuesPath:       valPath,
				ResourceID:       resourceID,
				FieldPath:        fieldPath,
				MatchType:        MatchSubstring,
				SentinelRendered: buildFastModeSentinel(s, stringIndex),
			})
		}
	}

	return mappings
}

// buildFastModeSentinel creates a synthetic sentinel-rendered string by
// replacing known value substrings with sentinel markers. This allows the
// existing BuildInterpolatedCELFromSentinel function to produce the correct
// interpolated CEL expression.
func buildFastModeSentinel(original string, stringIndex map[string][]string) string {
	result := original

	// Sort by longest value first to avoid partial replacements of shorter substrings.
	type entry struct {
		strVal string
		path   string
	}

	var entries []entry

	for strVal, paths := range stringIndex {
		if len(strVal) < 2 {
			continue
		}

		if strings.Contains(original, strVal) {
			for _, p := range paths {
				entries = append(entries, entry{strVal: strVal, path: p})
			}
		}
	}

	// Sort longest first; for equal-length entries, sort by path for determinism.
	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i].strVal) != len(entries[j].strVal) {
			return len(entries[i].strVal) > len(entries[j].strVal)
		}

		return entries[i].path < entries[j].path
	})

	for _, e := range entries {
		sentinel := SentinelForString(e.path)
		result = strings.Replace(result, e.strVal, sentinel, 1)
	}

	return result
}
