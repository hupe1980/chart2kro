package probe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hupe1980/chart2kro/internal/manifest"
)

// Outcome of a value's classification.
type Outcome string

// Outcomes.
const (
	Exposed Outcome = "exposed"
	Frozen  Outcome = "frozen"
	Failed  Outcome = "failed"
	Unused  Outcome = "unused"
)

// Reason codes for values that are not exposed (stable, part of the report contract).
const (
	ReasonRenderFailed      = "render-failed"
	ReasonStructure         = "changes-structure"
	ReasonTransformed       = "transformed"
	ReasonObjects           = "adds-or-removes-objects"
	ReasonToggleFields      = "toggles-fields"
	ReasonTernaryTypes      = "branch-types-differ"
	ReasonNull              = "null-default"
	ReasonKeyNotAddressable = "key-not-addressable"
	ReasonConflict          = "shares-field"
	ReasonOracle            = "verification-mismatch"
	ReasonKroType           = "kro-type-check"
	ReasonOnlyDropped       = "affects-only-dropped-objects"
	ReasonSecretUser        = "user-secret"
	ReasonUnknownKind       = "unknown-kind"
)

// Mode says how a value reaches a field.
type Mode int

// Modes.
const (
	Whole     Mode = iota // the field is exactly the value
	Span                  // the value occupies spans of a string field
	Ternary               // a boolean selects between two field values
	WholeTree             // a list/map value is the field's subtree
)

// Target is one field a value controls, in an object of the baseline (or of the
// object set selected by a boolean).
type Target struct {
	Key  string
	Path manifest.Path
	Mode Mode
	// Spans are [start,end) byte ranges in the baseline string (Mode Span).
	Spans [][2]int
	// Stringify: a non-string value rendered into a string field.
	Stringify bool
	// When (Mode Ternary): field values with the default and the flipped boolean.
	Default, Flipped any
}

// Result is the classification of one candidate.
type Result struct {
	Candidate Candidate
	Outcome   Outcome
	Reason    string
	Detail    string
	Targets   []Target
	// Booleans: objects present only when flipped (Added) or only at the default (Removed).
	Added, Removed []string
}

// Classify compares the probe renders of c against the baseline. p1 and p2 are nil
// when the corresponding render failed (err1/err2 hold the error).
func Classify(c Candidate, base *View, noise Noise, p1, p2 *View, err1, err2 error) Result {
	r := Result{Candidate: c}

	switch {
	case c.Kind == Null:
		return r.freeze(ReasonNull, "default is null; the value's type is unknown")
	case err1 != nil || err2 != nil:
		e := err1
		if e == nil {
			e = err2
		}

		r.Outcome, r.Reason, r.Detail = Failed, ReasonRenderFailed, firstLine(e.Error())

		return r
	}

	added, removed := objectDelta(base, p1)

	changes1 := changes(base, p1, noise)
	if len(changes1) == 0 && len(added) == 0 && len(removed) == 0 {
		r.Outcome = Unused
		return r
	}

	if c.Kind == Bool {
		return classifyBool(r, base, p1, changes1, added, removed)
	}

	if len(added) > 0 || len(removed) > 0 {
		return r.freeze(ReasonObjects, "objects "+strings.Join(append(added, removed...), ", "))
	}

	if c.Kind == List || c.Kind == Map {
		return classifyTree(r, base, p1, p2, changes1)
	}

	return classifyScalar(r, base, p1, p2, changes1, noise)
}

func (r Result) freeze(reason, detail string) Result {
	r.Outcome, r.Reason, r.Detail, r.Targets = Frozen, reason, detail, nil
	return r
}

type objChange struct {
	key string
	manifest.Change
}

func objectDelta(base, p *View) (added, removed []string) {
	for k, o := range p.Objects {
		if _, ok := base.Objects[k]; !ok && !isTestHook(o) {
			added = append(added, k)
		}
	}

	for k, o := range base.Objects {
		if _, ok := p.Objects[k]; !ok && !isTestHook(o) {
			removed = append(removed, k)
		}
	}

	sort.Strings(added)
	sort.Strings(removed)

	return added, removed
}

func isTestHook(o *Obj) bool {
	for _, e := range o.Hook {
		if strings.HasPrefix(e, "test") {
			return true
		}
	}

	return false
}

// changes lists differences in objects present in both views, minus noise and checksums.
func changes(base, p *View, noise Noise) []objChange {
	var out []objChange

	for _, k := range base.Order {
		bo := base.Objects[k]

		po, ok := p.Objects[k]
		if !ok {
			continue
		}

		for _, c := range manifest.Diff(bo.Value, po.Value) {
			if noise.Has(k, c.Path) || IsChecksumAnnotation(c.Path) {
				continue
			}

			out = append(out, objChange{key: k, Change: c})
		}
	}

	return out
}

func classifyBool(r Result, base, p *View, ch []objChange, added, removed []string) Result {
	for _, c := range ch {
		if c.Kind == manifest.Structural {
			return r.freeze(ReasonToggleFields, fmt.Sprintf("%s %s", c.key, c.Path))
		}

		if jsonType(c.A) != jsonType(c.B) {
			return r.freeze(ReasonTernaryTypes, fmt.Sprintf("%s %s", c.key, c.Path))
		}

		r.Targets = append(r.Targets, Target{Key: c.key, Path: c.Path, Mode: Ternary, Default: c.A, Flipped: c.B})
	}

	r.Outcome, r.Added, r.Removed = Exposed, added, removed

	return r
}

func classifyTree(r Result, base, p1, p2 *View, ch []objChange) Result {
	v1, v2 := r.Candidate.Value(1), r.Candidate.Value(2)
	roots := map[string]Target{}

	for _, c := range ch {
		root, ok := treeRoot(p1.Objects[c.key].Value, c.Path, v1)
		if !ok {
			return r.freeze(ReasonTransformed, fmt.Sprintf("%s %s", c.key, c.Path))
		}

		if bv, ok := manifest.Get(base.Objects[c.key].Value, root); !ok || !manifest.Equal(bv, r.Candidate.Default) {
			return r.freeze(ReasonStructure, fmt.Sprintf("%s %s is absent or different at the default", c.key, root))
		}

		if pv, ok := manifest.Get(p2.Objects[c.key].Value, root); !ok || !manifest.Equal(pv, v2) {
			return r.freeze(ReasonTransformed, fmt.Sprintf("%s %s", c.key, root))
		}

		roots[c.key+"|"+root.Key()] = Target{Key: c.key, Path: root, Mode: WholeTree}
	}

	keys := make([]string, 0, len(roots))
	for k := range roots {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		r.Targets = append(r.Targets, roots[k])
	}

	r.Outcome = Exposed

	return r
}

// treeRoot finds the nearest ancestor (or self) of p whose value equals want.
func treeRoot(obj any, p manifest.Path, want any) (manifest.Path, bool) {
	for i := len(p); i >= 0; i-- {
		if v, ok := manifest.Get(obj, p[:i]); ok && manifest.Equal(v, want) {
			return append(manifest.Path{}, p[:i]...), true
		}
	}

	return nil, false
}

func classifyScalar(r Result, base, p1, p2 *View, ch []objChange, noise Noise) Result {
	c := r.Candidate
	t1, t2 := c.Value(1), c.Value(2)
	s1, s2, def := Format(t1), Format(t2), Format(c.Default)

	for _, x := range ch {
		if x.Kind == manifest.Structural {
			return r.freeze(ReasonStructure, fmt.Sprintf("%s %s", x.key, x.Path))
		}

		bv := x.A
		v1 := x.B

		v2, ok := manifest.Get(p2.Objects[x.key].Value, x.Path)
		if !ok {
			return r.freeze(ReasonStructure, fmt.Sprintf("%s %s", x.key, x.Path))
		}

		switch {
		case manifest.Equal(v1, t1) && manifest.Equal(v2, t2) && manifest.Equal(bv, c.Default):
			r.Targets = append(r.Targets, Target{Key: x.key, Path: x.Path, Mode: Whole})
		case isString(v1) && v1 == s1 && v2 == s2 && bv == def:
			r.Targets = append(r.Targets, Target{Key: x.key, Path: x.Path, Mode: Whole, Stringify: c.Kind != String})
		case isString(v1) && isString(v2) && isString(bv):
			spans, ok := spansFor(bv.(string), v1.(string), v2.(string), def, s1, s2)
			if !ok {
				return r.freeze(ReasonTransformed, fmt.Sprintf("%s %s", x.key, x.Path))
			}

			r.Targets = append(r.Targets, Target{Key: x.key, Path: x.Path, Mode: Span, Spans: spans, Stringify: c.Kind != String})
		default:
			return r.freeze(ReasonTransformed, fmt.Sprintf("%s %s", x.key, x.Path))
		}
	}

	_ = noise
	r.Outcome = Exposed

	return r
}

// spansFor checks that x1 and x2 are base with the default's occurrences replaced by
// the probe strings, and returns those occurrences' ranges in base.
func spansFor(base, x1, x2, def, s1, s2 string) ([][2]int, bool) {
	if !strings.Contains(x1, s1) {
		return nil, false
	}

	parts := strings.Split(x1, s1)
	if strings.Join(parts, def) != base || strings.Join(parts, s2) != x2 {
		return nil, false
	}

	var spans [][2]int

	pos := 0

	for i, p := range parts {
		pos += len(p)
		if i < len(parts)-1 {
			spans = append(spans, [2]int{pos, pos + len(def)})
			pos += len(def)
		}
	}

	return spans, true
}

func isString(v any) bool { _, ok := v.(string); return ok }

func jsonType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "bool"
	case int64, float64:
		return "number"
	case nil:
		return "null"
	default:
		return "container"
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}

	return s
}
