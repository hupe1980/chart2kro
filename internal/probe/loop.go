package probe

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hupe1980/chart2kro/internal/manifest"
)

// Reason codes for lists whose loop cannot become a kro collection.
const (
	ReasonRangeOverMap  = "range-over-map"
	ReasonLoopCondition = "conditional-loop-body"
	ReasonLoopShares    = "loop-shares-template"
	ReasonNestedLoop    = "nested-loop"
	ReasonLoopElement   = "loop-element-structure"
	ReasonLoopShape     = "loop-shape-unknown"
)

// LoopLengths are the list lengths rendered to detect an object loop (copies of the
// first entry). Index 0 is the empty list.
const LoopLengths = 4

// Position is one document of a loop body: entry i produces the object at ordinal
// i·PerEntry + Ordinal of Source.
type Position struct {
	Source   string
	Ordinal  int // within one entry
	PerEntry int
	GVK      string // apiVersion/kind
}

// Key returns the object key of entry i at this position.
func (p Position) Key(i int) string {
	return p.Source + "#" + strconv.Itoa(i*p.PerEntry+p.Ordinal) + "#" + p.GVK
}

// LoopField is one field of a list entry ("" for string entries).
type LoopField struct {
	Name    string
	Type    string // SimpleSchema type of the field
	Targets []Target
}

// Loop describes a list value whose range loop emits whole objects.
type Loop struct {
	Positions []Position
	Elem      string // "string" or "object"
	Fields    []LoopField
	Index     []Target // fields computed from the entry's position
	Key       string   // the entry field that alone names the objects (object lists)
	// Template holds the entry-0 objects (keys Position.Key(0)) the collection is built
	// from: the default render, or the render with Example when the default is empty.
	Template *View
	Example  any
	// Witnesses are list values with Helm's render for each, used by verification.
	Witnesses []LoopWitness
}

// LoopWitness is a list value and Helm's render of it.
type LoopWitness struct {
	Value any
	View  *View
}

// Entry0Keys returns the template keys of the first entry, one per position.
func (l *Loop) Entry0Keys() []string {
	out := make([]string, len(l.Positions))
	for i, p := range l.Positions {
		out[i] = p.Key(0)
	}

	return out
}

// PositionOf returns the position index of an object key produced by this loop.
func (l *Loop) PositionOf(key string) (int, bool) {
	src, ord, gvk, ok := SplitKey(key)
	if !ok {
		return 0, false
	}

	for j, p := range l.Positions {
		if p.Source == src && p.GVK == gvk && ord%p.PerEntry == p.Ordinal {
			return j, true
		}
	}

	return 0, false
}

// SplitKey parses "source#ordinal#apiVersion/kind".
func SplitKey(key string) (source string, ordinal int, gvk string, ok bool) {
	parts := strings.SplitN(key, "#", 3)
	if len(parts) != 3 {
		return "", 0, "", false
	}

	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, "", false
	}

	return parts[0], n, parts[2], true
}

// DetectLoop examines renders of the list at lengths 0..3 (copies of the first entry)
// and returns the loop's positions and index targets, a reason the loop cannot become
// a collection, or neither when no template emits objects per entry.
func DetectLoop(lengths [LoopLengths]*View, noise Noise) (*Loop, string, string) {
	for n, v := range lengths {
		if v == nil {
			return nil, ReasonRenderFailed, fmt.Sprintf("the chart does not render with %d entries", n)
		}
	}

	counts := make([]map[string]int, LoopLengths)
	sources := map[string]bool{}

	for n, v := range lengths {
		counts[n] = map[string]int{}

		for _, k := range v.Order {
			if o := v.Objects[k]; !isTestHook(o) {
				counts[n][o.Source]++
				sources[o.Source] = true
			}
		}
	}

	var loopSources []string

	for s := range sources {
		if counts[0][s] != counts[3][s] || counts[1][s] != counts[2][s] {
			loopSources = append(loopSources, s)
		}
	}

	sort.Strings(loopSources)

	if len(loopSources) == 0 {
		return nil, "", "" // not a loop: the list's effect is elsewhere
	}

	l := &Loop{}

	for _, s := range loopSources {
		k := counts[1][s]

		switch {
		case counts[0][s] != 0:
			return nil, ReasonLoopShares, s + " has documents outside the loop"
		case k == 0 || counts[2][s] != 2*k || counts[3][s] != 3*k:
			return nil, ReasonLoopCondition, fmt.Sprintf("%s emits %d, %d, %d objects for 1, 2, 3 entries", s, counts[1][s], counts[2][s], counts[3][s])
		}

		for j := 0; j < k; j++ {
			gvk := ""

			for i := 0; i < 3; i++ {
				o := findOrdinal(lengths[3], s, i*k+j)
				if o == nil {
					return nil, ReasonLoopCondition, s + " does not emit entries in order"
				}

				_, _, g, _ := SplitKey(o.Key)
				if i == 0 {
					gvk = g
				} else if g != gvk {
					return nil, ReasonLoopElement, fmt.Sprintf("%s: entries emit different kinds (%s, %s)", s, gvk, g)
				}
			}

			l.Positions = append(l.Positions, Position{Source: s, Ordinal: j, PerEntry: k, GVK: gvk})
		}
	}

	isLoop := map[string]bool{}
	for _, s := range loopSources {
		isLoop[s] = true
	}

	// The list must not change anything outside its loops.
	for _, k := range lengths[0].Order {
		o := lengths[0].Objects[k]
		if isLoop[o.Source] || isTestHook(o) {
			continue
		}

		o3, ok := lengths[3].Objects[k]
		if !ok {
			return nil, ReasonLoopElement, "the list also removes " + k
		}

		for _, c := range manifest.Diff(o.Value, o3.Value) {
			if !noise.Has(k, c.Path) && !IsChecksumAnnotation(c.Path) {
				return nil, ReasonLoopElement, fmt.Sprintf("the list also changes %s %s", k, c.Path)
			}
		}
	}

	for _, k := range lengths[3].Order {
		o := lengths[3].Objects[k]
		if _, ok := lengths[0].Objects[k]; !ok && !isLoop[o.Source] && !isTestHook(o) {
			return nil, ReasonLoopElement, "the list also adds " + k
		}
	}

	idx, reason, detail := indexTargets(l, lengths[3], noise)
	if reason != "" {
		return nil, reason, detail
	}

	l.Index = idx

	return l, "", ""
}

func findOrdinal(v *View, source string, ordinal int) *Obj {
	for _, k := range v.Order {
		if s, n, _, ok := SplitKey(k); ok && s == source && n == ordinal {
			return v.Objects[k]
		}
	}

	return nil
}

// indexTargets compares identical entries: any difference between their objects can
// only come from the loop index.
func indexTargets(l *Loop, v *View, noise Noise) ([]Target, string, string) {
	var out []Target

	for _, p := range l.Positions {
		o0 := v.Objects[p.Key(0)]
		found := map[string]Target{}

		for i := 1; i < 3; i++ {
			oi := v.Objects[p.Key(i)]

			for _, c := range manifest.Diff(o0.Value, oi.Value) {
				if noise.Has(p.Key(0), c.Path) || IsChecksumAnnotation(c.Path) {
					continue
				}

				if c.Kind == manifest.Structural {
					return nil, ReasonLoopElement, fmt.Sprintf("%s %s differs between identical entries", p.Key(0), c.Path)
				}

				t, ok := indexTarget(p.Key(0), c.Path, c.A, c.B, i)
				if !ok {
					return nil, ReasonLoopElement, fmt.Sprintf("%s %s differs between identical entries", p.Key(0), c.Path)
				}

				if prev, seen := found[c.Path.Key()]; seen && !sameTarget(prev, t) {
					return nil, ReasonLoopElement, fmt.Sprintf("%s %s is not the loop index", p.Key(0), c.Path)
				}

				found[c.Path.Key()] = t
			}
		}

		keys := make([]string, 0, len(found))
		for k := range found {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			out = append(out, found[k])
		}
	}

	return out, "", ""
}

// indexTarget checks that b (entry i) is a (entry 0) with the index 0 replaced by i.
func indexTarget(key string, p manifest.Path, a, b any, i int) (Target, bool) {
	if fa, ok := number(a); ok {
		fb, ok := number(b)

		return Target{Key: key, Path: p, Mode: Whole}, ok && fa == 0 && fb == float64(i)
	}

	sa, ok1 := a.(string)
	sb, ok2 := b.(string)

	if !ok1 || !ok2 {
		return Target{}, false
	}

	pre := commonPrefix(sa, sb)
	suf := commonSuffix(sa[pre:], sb[pre:])

	if sa[pre:len(sa)-suf] != "0" || sb[pre:len(sb)-suf] != strconv.Itoa(i) {
		return Target{}, false
	}

	return Target{Key: key, Path: p, Mode: Span, Spans: [][2]int{{pre, pre + 1}}, Stringify: true}, true
}

func sameTarget(a, b Target) bool {
	return a.Mode == b.Mode && fmt.Sprint(a.Spans) == fmt.Sprint(b.Spans)
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}

	return n
}

func commonSuffix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}

	return n
}

// EntryFields returns the fields of a list's first entry with their candidates for
// probing, or a reason the entries cannot be typed.
func EntryFields(list Candidate, entries []any) ([]Candidate, string, string) {
	first := entries[0]

	if s, ok := first.(string); ok {
		for _, e := range entries {
			if _, ok := e.(string); !ok {
				return nil, ReasonLoopElement, "entries mix strings and objects"
			}
		}

		return []Candidate{{Path: append(append([]string{}, list.Path...), "[0]"), Kind: String, Default: s, Index: list.Index}}, "", ""
	}

	m, ok := first.(map[string]any)
	if !ok {
		return nil, ReasonLoopElement, fmt.Sprintf("entries of type %T are not supported", first)
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, e := range entries[1:] {
		em, ok := e.(map[string]any)
		if !ok || !sameKeys(m, em) {
			return nil, ReasonLoopElement, "entries have different fields"
		}
	}

	var out []Candidate

	for i, k := range keys {
		c := Candidate{Path: append(append([]string{}, list.Path...), "[0]", k), Default: m[k], Index: list.Index*1000 + i}

		switch v := m[k].(type) {
		case bool:
			c.Kind = Bool
		case float64:
			c.Kind = Float
			if v == math.Trunc(v) {
				c.Kind = Int
			}
		case string:
			c.Kind = String
		case []any:
			c.Kind, c.Elem = List, "object"
			if len(v) > 0 {
				c.Elem = "string"

				for _, x := range v {
					if _, ok := x.(string); !ok {
						c.Elem = "object"
					}
				}
			}
		case map[string]any:
			c.Kind = Map
		default:
			return nil, ReasonLoopElement, fmt.Sprintf("entry field %q has no type (null)", k)
		}

		out = append(out, c)
	}

	return out, "", ""
}

func sameKeys(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}

	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}

	return true
}

// ClassifyLoopField classifies one entry field from renders with only that field of the
// first entry changed. Every effect must stay inside the loop's first-entry objects.
func ClassifyLoopField(l *Loop, f Candidate, base *View, noise Noise, p1, p2 *View, err1, err2 error) ([]Target, string, string) {
	name := f.Path[len(f.Path)-1]

	r := Classify(f, base, noise, p1, p2, err1, err2)

	switch {
	case r.Outcome == Unused:
		return nil, "", ""
	case r.Outcome == Failed:
		return nil, r.Reason, "entry field " + name + ": " + r.Detail
	case r.Reason == ReasonObjects:
		if f.Kind == List {
			return nil, ReasonNestedLoop, "entry field " + name + " adds or removes objects"
		}

		return nil, ReasonLoopCondition, "entry field " + name + " adds or removes objects"
	case r.Outcome == Frozen:
		return nil, r.Reason, "entry field " + name + ": " + r.Detail
	case len(r.Added) > 0 || len(r.Removed) > 0:
		return nil, ReasonLoopCondition, "entry field " + name + " adds or removes objects"
	case f.Kind == Bool && len(r.Targets) > 0:
		return nil, ReasonLoopElement, "boolean entry field " + name + " changes the output"
	}

	own := map[string]bool{}
	for _, k := range l.Entry0Keys() {
		own[k] = true
	}

	for _, t := range r.Targets {
		if !own[t.Key] {
			return nil, ReasonLoopElement, fmt.Sprintf("entry field %s also changes %s", name, t.Key)
		}
	}

	return r.Targets, "", ""
}

// FieldType is the SimpleSchema type of an entry field.
func FieldType(c Candidate) string {
	switch c.Kind {
	case Bool:
		return "boolean"
	case Int:
		return "integer"
	case Float:
		return "float"
	case List:
		if c.Elem == "string" {
			return "[]string"
		}

		return "[]object"
	case Map:
		return "object"
	default:
		return "string"
	}
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}

	return 0, false
}
