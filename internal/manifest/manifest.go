// Package manifest provides the tree operations conversion is built on: JSON
// normalization, paths, leaf walking and structural diffs between rendered objects.
package manifest

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Normalize converts a value tree to the types the Kubernetes API server stores:
// integral numbers become int64, other numbers float64; maps and slices are copied.
func Normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = Normalize(x)
		}

		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = Normalize(x)
		}

		return out
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case float32:
		return Normalize(float64(t))
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1<<53 {
			return int64(t)
		}

		return t
	default:
		return v
	}
}

// NormalizeMap is Normalize for maps.
func NormalizeMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}

	return Normalize(m).(map[string]any)
}

// Path addresses a node inside an object: map keys are strings, list indexes ints.
type Path []any

// String renders a path like spec.containers[0].image (keys containing dots are quoted).
func (p Path) String() string {
	var b strings.Builder

	for i, s := range p {
		switch t := s.(type) {
		case int:
			fmt.Fprintf(&b, "[%d]", t)
		case string:
			if i > 0 {
				b.WriteByte('.')
			}

			if strings.ContainsAny(t, ".[]\"") {
				b.WriteString(strconv.Quote(t))
			} else {
				b.WriteString(t)
			}
		}
	}

	return b.String()
}

// Key is a comparable form of the path.
func (p Path) Key() string { return p.String() }

// Append returns a new path with s appended.
func (p Path) Append(s any) Path {
	out := make(Path, len(p), len(p)+1)
	copy(out, p)

	return append(out, s)
}

// HasPrefix reports whether q is a prefix of p.
func (p Path) HasPrefix(q Path) bool {
	if len(q) > len(p) {
		return false
	}

	for i := range q {
		if p[i] != q[i] {
			return false
		}
	}

	return true
}

// Get returns the node at path.
func Get(root any, p Path) (any, bool) {
	cur := root

	for _, s := range p {
		switch t := s.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}

			cur, ok = m[t]
			if !ok {
				return nil, false
			}
		case int:
			l, ok := cur.([]any)
			if !ok || t < 0 || t >= len(l) {
				return nil, false
			}

			cur = l[t]
		}
	}

	return cur, true
}

// Set replaces the node at path; the path must exist.
func Set(root any, p Path, v any) bool {
	if len(p) == 0 {
		return false
	}

	parent, ok := Get(root, p[:len(p)-1])
	if !ok {
		return false
	}

	switch t := p[len(p)-1].(type) {
	case string:
		m, ok := parent.(map[string]any)
		if !ok {
			return false
		}

		if _, exists := m[t]; !exists {
			return false
		}

		m[t] = v
	case int:
		l, ok := parent.([]any)
		if !ok || t < 0 || t >= len(l) {
			return false
		}

		l[t] = v
	}

	return true
}

// Delete removes a map key at path (lists are not shortened).
func Delete(root any, p Path) bool {
	if len(p) == 0 {
		return false
	}

	parent, ok := Get(root, p[:len(p)-1])
	if !ok {
		return false
	}

	m, ok := parent.(map[string]any)
	k, isKey := p[len(p)-1].(string)

	if !ok || !isKey {
		return false
	}

	if _, exists := m[k]; !exists {
		return false
	}

	delete(m, k)

	return true
}

// IsScalar reports whether v is a leaf value (not a map or list).
func IsScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
}

// Leaves calls fn for every scalar node (including nil) in deterministic order.
func Leaves(root any, fn func(Path, any)) { walk(root, nil, fn) }

func walk(v any, p Path, fn func(Path, any)) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			walk(t[k], p.Append(k), fn)
		}
	case []any:
		for i, x := range t {
			walk(x, p.Append(i), fn)
		}
	default:
		fn(p, v)
	}
}

// ChangeKind classifies a difference between two trees.
type ChangeKind int

const (
	// Changed means a scalar at the same path has a different value.
	Changed ChangeKind = iota
	// Structural means a node exists on one side only, changes between scalar and container, or a list changes length.
	Structural
)

// Change is one difference at a path.
type Change struct {
	Path Path
	Kind ChangeKind
	A, B any // values on both sides (nil when absent)
}

// Diff lists differences from a to b. Lists are compared by index; a length change is
// a single Structural change at the list path. Empty containers are values, not absences.
func Diff(a, b any) []Change {
	var out []Change

	diff(a, b, nil, &out)

	return out
}

func diff(a, b any, p Path, out *[]Change) {
	am, aIsMap := a.(map[string]any)
	bm, bIsMap := b.(map[string]any)

	if aIsMap && bIsMap {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}

		for k := range bm {
			keys[k] = true
		}

		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}

		sort.Strings(sorted)

		for _, k := range sorted {
			av, aok := am[k]
			bv, bok := bm[k]

			if aok != bok {
				*out = append(*out, Change{Path: p.Append(k), Kind: Structural, A: av, B: bv})
				continue
			}

			diff(av, bv, p.Append(k), out)
		}

		return
	}

	al, aIsList := a.([]any)
	bl, bIsList := b.([]any)

	if aIsList && bIsList {
		if len(al) != len(bl) {
			*out = append(*out, Change{Path: p, Kind: Structural, A: a, B: b})
			return
		}

		for i := range al {
			diff(al[i], bl[i], p.Append(i), out)
		}

		return
	}

	if aIsMap || bIsMap || aIsList || bIsList {
		*out = append(*out, Change{Path: p, Kind: Structural, A: a, B: b})
		return
	}

	if !equalScalar(a, b) {
		*out = append(*out, Change{Path: p, Kind: Changed, A: a, B: b})
	}
}

func equalScalar(a, b any) bool {
	return fmt.Sprintf("%T:%v", Normalize(a), Normalize(a)) == fmt.Sprintf("%T:%v", Normalize(b), Normalize(b))
}

// Equal reports deep equality after normalization.
func Equal(a, b any) bool { return len(Diff(Normalize(a), Normalize(b))) == 0 }
