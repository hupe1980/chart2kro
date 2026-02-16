package filter

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// KindFilter excludes resources whose kind matches any of the specified kinds.
type KindFilter struct {
	kinds map[string]bool
}

// NewKindFilter creates a filter that excludes resources matching any of the
// given Kubernetes kinds. Matching is case-insensitive.
func NewKindFilter(kinds []string) *KindFilter {
	m := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		m[strings.ToLower(k)] = true
	}

	return &KindFilter{kinds: m}
}

// Apply filters out resources whose kind matches.
func (f *KindFilter) Apply(_ context.Context, resources []*k8s.Resource) (*Result, error) {
	r := NewResult()

	for _, res := range resources {
		if f.kinds[strings.ToLower(res.Kind())] {
			r.Excluded = append(r.Excluded, ExcludedResource{
				Resource: res,
				Reason:   fmt.Sprintf("excluded by kind: %s", res.Kind()),
			})
		} else {
			r.Included = append(r.Included, res)
		}
	}

	return r, nil
}

// ResourceIDFilter excludes resources whose assigned ID matches any of the
// specified IDs. The resource IDs must be assigned before this filter runs.
type ResourceIDFilter struct {
	ids      map[string]bool
	idLookup map[*k8s.Resource]string
}

// NewResourceIDFilter creates a filter that excludes resources by assigned ID.
func NewResourceIDFilter(ids []string, resourceIDs map[*k8s.Resource]string) *ResourceIDFilter {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}

	return &ResourceIDFilter{ids: m, idLookup: resourceIDs}
}

// Apply filters out resources whose assigned ID matches.
func (f *ResourceIDFilter) Apply(_ context.Context, resources []*k8s.Resource) (*Result, error) {
	r := NewResult()

	for _, res := range resources {
		id := f.idLookup[res]
		if f.ids[id] {
			r.Excluded = append(r.Excluded, ExcludedResource{
				Resource: res,
				Reason:   fmt.Sprintf("excluded by resource ID: %s", id),
			})
		} else {
			r.Included = append(r.Included, res)
		}
	}

	return r, nil
}

// SubchartFilter excludes resources originating from the named subcharts.
// It inspects the SourcePath of each resource.
type SubchartFilter struct {
	subcharts map[string]bool
}

// NewSubchartFilter creates a filter that excludes resources from named subcharts.
func NewSubchartFilter(subcharts []string) *SubchartFilter {
	m := make(map[string]bool, len(subcharts))
	for _, s := range subcharts {
		m[strings.ToLower(s)] = true
	}

	return &SubchartFilter{subcharts: m}
}

// Apply filters out resources from matching subcharts.
func (f *SubchartFilter) Apply(_ context.Context, resources []*k8s.Resource) (*Result, error) {
	r := NewResult()

	for _, res := range resources {
		sc := res.SourceChart()
		if sc != "" && f.subcharts[strings.ToLower(sc)] {
			r.Excluded = append(r.Excluded, ExcludedResource{
				Resource: res,
				Reason:   fmt.Sprintf("excluded by subchart: %s", sc),
			})
		} else {
			r.Included = append(r.Included, res)
		}
	}

	return r, nil
}

// LabelFilter excludes resources matching a label selector.
// Supports key=value (equality), key!=value (inequality), and
// key in (v1,v2) (set membership).
type LabelFilter struct {
	selectors []labelSelector
}

type labelSelector struct {
	key    string
	op     labelOp
	values []string
}

type labelOp int

const (
	labelOpEqual labelOp = iota
	labelOpNotEqual
	labelOpIn
)

// NewLabelFilter creates a filter from a comma-separated label selector string.
// Supported syntax: "key=value", "key!=value", "key in (v1,v2)".
func NewLabelFilter(selectorExpr string) (*LabelFilter, error) {
	parts := splitSelectors(selectorExpr)
	selectors := make([]labelSelector, 0, len(parts))

	for _, part := range parts {
		sel, err := parseLabelSelector(part)
		if err != nil {
			return nil, err
		}

		selectors = append(selectors, sel)
	}

	return &LabelFilter{selectors: selectors}, nil
}

// Apply filters out resources whose labels match ALL selectors (AND semantics).
func (f *LabelFilter) Apply(_ context.Context, resources []*k8s.Resource) (*Result, error) {
	r := NewResult()

	for _, res := range resources {
		if f.matches(res.Labels) {
			r.Excluded = append(r.Excluded, ExcludedResource{
				Resource: res,
				Reason:   "excluded by label match",
			})
		} else {
			r.Included = append(r.Included, res)
		}
	}

	return r, nil
}

func (f *LabelFilter) matches(labels map[string]string) bool {
	for _, sel := range f.selectors {
		val, exists := labels[sel.key]

		switch sel.op {
		case labelOpEqual:
			if !exists || val != sel.values[0] {
				return false
			}
		case labelOpNotEqual:
			if exists && val == sel.values[0] {
				return false
			}
		case labelOpIn:
			if !exists {
				return false
			}

			found := false
			for _, v := range sel.values {
				if val == v {
					found = true
					break
				}
			}

			if !found {
				return false
			}
		}
	}

	return true
}

// splitSelectors splits a selector expression on commas, but not inside parentheses.
func splitSelectors(expr string) []string {
	var parts []string

	depth := 0
	start := 0

	for i, ch := range expr {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(expr[start:i]))
				start = i + 1
			}
		}
	}

	if start < len(expr) {
		parts = append(parts, strings.TrimSpace(expr[start:]))
	}

	return parts
}

// parseLabelSelector parses a single label selector expression.
func parseLabelSelector(expr string) (labelSelector, error) {
	expr = strings.TrimSpace(expr)

	// Check for "key in (v1,v2)" syntax.
	if inIdx := strings.Index(expr, " in ("); inIdx > 0 {
		key := strings.TrimSpace(expr[:inIdx])
		valStr := expr[inIdx+5:]
		valStr = strings.TrimSuffix(valStr, ")")

		values := strings.Split(valStr, ",")
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}

		return labelSelector{key: key, op: labelOpIn, values: values}, nil
	}

	// Check for "key!=value".
	if neqIdx := strings.Index(expr, "!="); neqIdx > 0 {
		return labelSelector{
			key:    strings.TrimSpace(expr[:neqIdx]),
			op:     labelOpNotEqual,
			values: []string{strings.TrimSpace(expr[neqIdx+2:])},
		}, nil
	}

	// Check for "key=value".
	if eqIdx := strings.Index(expr, "="); eqIdx > 0 {
		return labelSelector{
			key:    strings.TrimSpace(expr[:eqIdx]),
			op:     labelOpEqual,
			values: []string{strings.TrimSpace(expr[eqIdx+1:])},
		}, nil
	}

	return labelSelector{}, fmt.Errorf("invalid label selector: %q", expr)
}
