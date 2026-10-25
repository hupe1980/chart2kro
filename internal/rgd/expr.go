package rgd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	apiservercel "k8s.io/apiserver/pkg/cel"
)

// SpecRef returns the CEL reference schema.spec.<path> with Kubernetes CEL escaping,
// or false when a segment cannot be addressed (e.g. it starts with a digit).
func SpecRef(path []string) (string, bool) {
	parts := []string{"schema", "spec"}

	for _, p := range path {
		e, ok := apiservercel.Escape(p)
		if !ok {
			return "", false
		}

		parts = append(parts, e)
	}

	return strings.Join(parts, "."), true
}

const (
	refName      = "schema.metadata.name"
	refNamespace = "schema.metadata.namespace"
)

// seg is one piece of a string field: a literal or a CEL expression.
type seg struct {
	lit    string
	expr   string
	isExpr bool
	str    bool // expression already has type string
}

func (s seg) cel() string {
	if !s.isExpr {
		return strconv.Quote(s.lit)
	}

	if s.str {
		return s.expr
	}

	return "string(" + s.expr + ")"
}

// hole is an expression occupying [start,end) of a base string.
type hole struct {
	start, end int
	expr       string
	str        bool
	owner      string // value path (or "" for identity/generated) for conflict reporting
}

// segments splits base by holes; holes must not overlap (caller checks).
func segments(base string, holes []hole) []seg {
	sort.Slice(holes, func(i, j int) bool { return holes[i].start < holes[j].start })

	var out []seg

	pos := 0

	for _, h := range holes {
		if h.start > pos {
			out = append(out, seg{lit: base[pos:h.start]})
		}

		out = append(out, seg{expr: h.expr, isExpr: true, str: h.str})
		pos = h.end
	}

	if pos < len(base) {
		out = append(out, seg{lit: base[pos:]})
	}

	return out
}

// overlaps returns the owner of a hole that overlaps another, if any.
func overlaps(holes []hole) (string, bool) {
	sort.Slice(holes, func(i, j int) bool { return holes[i].start < holes[j].start })

	for i := 1; i < len(holes); i++ {
		if holes[i].start < holes[i-1].end {
			return holes[i].owner, true
		}
	}

	return "", false
}

// celConcat builds one CEL string expression from segments.
func celConcat(segs []seg) string {
	if len(segs) == 0 {
		return `""`
	}

	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = s.cel()
	}

	return strings.Join(parts, " + ")
}

// escapeLiteral makes a literal safe inside a kro field: every "${" becomes ${"${"}.
func escapeLiteral(s string) string {
	return strings.ReplaceAll(s, "${", `${"${"}`)
}

// template renders segments as a kro field value: a single expression becomes
// "${expr}"; otherwise a kro string template.
func template(segs []seg) string {
	if len(segs) == 1 && segs[0].isExpr {
		s := segs[0]
		if s.str {
			return "${" + s.expr + "}"
		}

		return "${string(" + s.expr + ")}"
	}

	var b strings.Builder

	for _, s := range segs {
		if !s.isExpr {
			b.WriteString(escapeLiteral(s.lit))
			continue
		}

		b.WriteString("${" + s.cel() + "}")
	}

	return b.String()
}

// celLiteral renders a JSON scalar as a CEL literal.
func celLiteral(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return strconv.Quote(t), nil
	case bool:
		return strconv.FormatBool(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case float64:
		s := strconv.FormatFloat(t, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}

		return s, nil
	case nil:
		return "null", nil
	default:
		return "", fmt.Errorf("unsupported literal %T", v)
	}
}
