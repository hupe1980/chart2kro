// Package probe finds which chart values control which rendered fields: it collects
// candidate values, renders typed probes and classifies each value's effect.
package probe

import (
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
)

// Kind is the type of a candidate value.
type Kind int

// Candidate kinds.
const (
	Bool Kind = iota
	Int
	Float
	String
	List
	Map
	Null
)

func (k Kind) String() string {
	return [...]string{"bool", "int", "float", "string", "list", "map", "null"}[k]
}

// Candidate is one chart value that may become an input.
type Candidate struct {
	Path    []string
	Kind    Kind
	Default any
	Index   int    // stable index used to make probe tokens unique
	Elem    string // lists: "string" or "object" (element type of probes and schema)
}

// Dotted renders the path with "." separators (keys keep their spelling).
func (c Candidate) Dotted() string { return strings.Join(c.Path, ".") }

// Collect walks merged values and returns candidates in path order: scalar leaves,
// lists, and maps that are empty by default. Non-empty maps are descended into.
// expose (if non-empty) restricts candidates to matching paths; freeze removes paths.
func Collect(values map[string]any, expose, freeze []string) []Candidate {
	var out []Candidate

	var walk func(v any, p []string)

	walk = func(v any, p []string) {
		switch t := v.(type) {
		case map[string]any:
			if len(t) == 0 && len(p) > 0 {
				out = append(out, Candidate{Path: p, Kind: Map, Default: t})
				return
			}

			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}

			sort.Strings(keys)

			for _, k := range keys {
				walk(t[k], append(append([]string{}, p...), k))
			}
		case []any:
			elem := "object"
			if len(t) > 0 {
				elem = "string"
				for _, e := range t {
					if _, ok := e.(string); !ok {
						elem = "object"
					}
				}
			}

			out = append(out, Candidate{Path: p, Kind: List, Default: t, Elem: elem})
		case bool:
			out = append(out, Candidate{Path: p, Kind: Bool, Default: t})
		case float64:
			k := Float
			if t == math.Trunc(t) {
				k = Int
			}

			out = append(out, Candidate{Path: p, Kind: k, Default: t})
		case int64, int:
			out = append(out, Candidate{Path: p, Kind: Int, Default: toFloat(t)})
		case string:
			out = append(out, Candidate{Path: p, Kind: String, Default: t})
		case nil:
			out = append(out, Candidate{Path: p, Kind: Null})
		}
	}

	walk(values, nil)

	var kept []Candidate

	for _, c := range out {
		d := c.Dotted()
		if len(expose) > 0 && !MatchAny(expose, d) {
			continue
		}

		if MatchAny(freeze, d) {
			continue
		}

		kept = append(kept, c)
	}

	for i := range kept {
		kept[i].Index = i
	}

	return kept
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case float64:
		return t
	}

	return 0
}

// MatchAny reports whether dotted path p matches any glob. Segments use path.Match;
// "**" matches any number of segments; a pattern also matches every path below it.
func MatchAny(globs []string, p string) bool {
	for _, g := range globs {
		if matchGlob(strings.Split(g, "."), strings.Split(p, ".")) {
			return true
		}
	}

	return false
}

func matchGlob(g, p []string) bool {
	if len(g) == 0 {
		return true // prefix match: a pattern covers its subtree
	}

	if g[0] == "**" {
		for i := 0; i <= len(p); i++ {
			if matchGlob(g[1:], p[i:]) {
				return true
			}
		}

		return false
	}

	if len(p) == 0 {
		return false
	}

	ok, err := path.Match(g[0], p[0])

	return err == nil && ok && matchGlob(g[1:], p[1:])
}

// Value returns the probe value for a candidate at point 1 or 2. Numbers stay below
// 1e6 so Go templates print them without exponents; strings are mixed case so case
// functions show up as transforms.
func (c Candidate) Value(point int) any {
	k := c.Index

	switch c.Kind {
	case Bool:
		return !c.Default.(bool)
	case Int:
		return float64(100000 + 2*k + point - 1)
	case Float:
		return 0.25 + float64(k) + 0.5*float64(point-1)
	case String:
		return fmt.Sprintf("C2kv%d%s", k, []string{"Xa", "Yb"}[point-1])
	case List:
		v := fmt.Sprintf("c2kv%d%s", k, []string{"a", "b"}[point-1])
		if c.Elem == "string" {
			return []any{v}
		}

		return []any{map[string]any{fmt.Sprintf("c2kk%d", k): v}}
	case Map:
		return map[string]any{fmt.Sprintf("c2kk%d", k): fmt.Sprintf("c2kv%d%s", k, []string{"a", "b"}[point-1])}
	default:
		return nil
	}
}

// Format renders a scalar the way Go templates print it with %v (Helm values are
// float64, so large integers print with exponents, as in Helm).
func Format(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprint(t)
	case int64:
		return fmt.Sprint(float64(t))
	default:
		return fmt.Sprint(t)
	}
}

// With returns a deep copy of values with the value at p replaced.
func With(values map[string]any, p []string, v any) map[string]any {
	out := copyMap(values)
	cur := out

	for i, s := range p {
		if i == len(p)-1 {
			cur[s] = v
			break
		}

		next, ok := cur[s].(map[string]any)
		if !ok {
			next = map[string]any{}
		}

		next = copyMap(next)
		cur[s] = next
		cur = next
	}

	return out
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}

	return out
}
