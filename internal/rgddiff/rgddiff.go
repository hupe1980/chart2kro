// Package rgddiff classifies changes between two ResourceGraphDefinitions the way they
// affect existing instances: breaking (instances may stop validating, or kro rejects
// the update), destructive (objects of instances are deleted) or compatible.
package rgddiff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Severity of a change.
type Severity string

// Severities.
const (
	Breaking    Severity = "breaking"
	Destructive Severity = "destructive"
	Compatible  Severity = "compatible"
)

// Change is one classified difference.
type Change struct {
	Severity Severity `json:"severity"`
	Path     string   `json:"path"`
	Message  string   `json:"message"`
}

// Result lists changes, most severe first.
type Result struct {
	SchemaVersion string   `json:"schemaVersion"`
	Changes       []Change `json:"changes"`
}

// Breaking reports whether any change is breaking or destructive.
func (r *Result) Breaking() bool {
	for _, c := range r.Changes {
		if c.Severity != Compatible {
			return true
		}
	}

	return false
}

// Compare classifies the changes from old to updated.
func Compare(old, updated map[string]any) *Result {
	r := &Result{}

	os, ns := get(old, "spec", "schema"), get(updated, "spec", "schema")

	// kro marks these immutable: changing them is rejected for existing RGDs.
	for _, f := range []string{"kind", "apiVersion", "group", "scope"} {
		a, b := str(os, f), str(ns, f)
		if f == "group" {
			a, b = orDefault(a, "kro.run"), orDefault(b, "kro.run")
		}

		if f == "scope" {
			a, b = orDefault(a, "Namespaced"), orDefault(b, "Namespaced")
		}

		if a != b {
			r.add(Breaking, "spec.schema."+f, fmt.Sprintf("%s changed from %q to %q (immutable in kro)", f, a, b))
		}
	}

	compareFields(r, "spec", asMap(get(os, "spec")), asMap(get(ns, "spec")))
	compareResources(r, old, updated)

	sort.SliceStable(r.Changes, func(i, j int) bool {
		return rank(r.Changes[i].Severity) < rank(r.Changes[j].Severity)
	})

	return r
}

func compareFields(r *Result, prefix string, a, b map[string]any) {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}

	for k := range b {
		keys[k] = true
	}

	for _, k := range sorted(keys) {
		p := prefix + "." + k
		av, aok := a[k]
		bv, bok := b[k]

		am, aIsMap := av.(map[string]any)
		bm, bIsMap := bv.(map[string]any)

		switch {
		case aok && !bok:
			r.add(Breaking, p, "field removed: instances that set it are rejected")
		case !aok && bok:
			if bIsMap {
				compareFields(r, p, nil, bm)
				continue
			}

			t, m := parseField(fmt.Sprint(bv))
			if m["required"] == "true" {
				r.add(Breaking, p, "required field added: existing instances do not set it")
			} else {
				r.add(Compatible, p, "field added ("+t+")")
			}
		case aIsMap && bIsMap:
			compareFields(r, p, am, bm)
		case aIsMap != bIsMap:
			r.add(Breaking, p, "field changed between object and value")
		default:
			at, amk := parseField(fmt.Sprint(av))
			bt, bmk := parseField(fmt.Sprint(bv))

			switch {
			case at != bt:
				r.add(Breaking, p, fmt.Sprintf("type changed from %s to %s", at, bt))
			case amk["required"] != "true" && bmk["required"] == "true":
				r.add(Breaking, p, "field became required")
			case amk["default"] != "" && bmk["default"] == "" && bmk["required"] != "true":
				r.add(Breaking, p, "default removed: instances relying on it change behaviour")
			case amk["default"] != bmk["default"]:
				r.add(Compatible, p, fmt.Sprintf("default changed from %s to %s", orDash(amk["default"]), orDash(bmk["default"])))
			}

			if at == bt && (amk["immutable"] == "true") != (bmk["immutable"] == "true") {
				// kro v0.9.4 skips CRD updates that only change validation rules.
				r.add(Compatible, p, fmt.Sprintf("immutable changed to %t; kro applies validation-only changes to the CRD only together with another schema change", bmk["immutable"] == "true"))
			}
		}
	}
}

// parseField splits "type | k=v | k=v".
func parseField(s string) (string, map[string]string) {
	parts := strings.SplitN(s, "|", 2)
	m := map[string]string{}

	if len(parts) == 2 {
		rest := strings.TrimSpace(parts[1])
		for _, kv := range splitMarkers(rest) {
			k, v, _ := strings.Cut(kv, "=")
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}

	return strings.TrimSpace(parts[0]), m
}

// splitMarkers splits kro markers ("k=v k=v") on whitespace outside quotes and brackets.
func splitMarkers(s string) []string {
	var (
		out   []string
		cur   strings.Builder
		quote bool
		esc   bool
		depth int
	)

	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}

	for _, r := range s {
		switch {
		case esc:
			esc = false
		case r == '\\' && quote:
			esc = true
		case r == '"':
			quote = !quote
		case !quote && (r == '[' || r == '{'):
			depth++
		case !quote && (r == ']' || r == '}'):
			depth--
		case !quote && depth == 0 && (r == ' ' || r == '\t'):
			flush()
			continue
		}

		cur.WriteRune(r)
	}

	flush()

	return out
}

type res struct {
	id, gvk, name string
	body          map[string]any // the resource entry without its id
}

func resources(rgd map[string]any) map[string]res {
	out := map[string]res{}

	list, _ := get(rgd, "spec", "resources").([]any)
	for _, x := range list {
		m, _ := x.(map[string]any)
		t, _ := m["template"].(map[string]any)
		md, _ := t["metadata"].(map[string]any)
		id, _ := m["id"].(string)

		body := map[string]any{}
		for k, v := range m {
			if k != "id" {
				body[k] = v
			}
		}

		r := res{id: id, gvk: fmt.Sprint(t["apiVersion"], "/", t["kind"]), name: fmt.Sprint(md["name"]), body: body}
		out[r.gvk+"|"+r.name] = r
	}

	return out
}

// compareResources matches resources by GVK and name template, not by id: kro tracks
// objects by identity, so renaming an id alone does not delete anything.
func compareResources(r *Result, old, updated map[string]any) {
	a, b := resources(old), resources(updated)

	for _, k := range sortedKeys(a) {
		if _, ok := b[k]; !ok {
			r.add(Destructive, "spec.resources."+a[k].id, fmt.Sprintf("%s %s removed: kro deletes it from every instance", a[k].gvk, a[k].name))
		}
	}

	for _, k := range sortedKeys(b) {
		prev, ok := a[k]

		switch {
		case !ok:
			r.add(Compatible, "spec.resources."+b[k].id, fmt.Sprintf("%s %s added", b[k].gvk, b[k].name))
		case !reflect.DeepEqual(prev.body["includeWhen"], b[k].body["includeWhen"]):
			r.add(Compatible, "spec.resources."+b[k].id, fmt.Sprintf("%s %s: includeWhen changed; instances may gain or lose it", b[k].gvk, b[k].name))
		case !reflect.DeepEqual(prev.body, b[k].body):
			r.add(Compatible, "spec.resources."+b[k].id, fmt.Sprintf("%s %s: template changed; kro updates it in every instance", b[k].gvk, b[k].name))
		}
	}
}

func (r *Result) add(s Severity, p, m string) {
	r.Changes = append(r.Changes, Change{Severity: s, Path: p, Message: m})
}

func rank(s Severity) int {
	switch s {
	case Breaking:
		return 0
	case Destructive:
		return 1
	default:
		return 2
	}
}

func get(m any, path ...string) any {
	cur := m

	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}

		cur = mm[p]
	}

	return cur
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func str(m any, k string) string {
	s, _ := get(m, k).(string)
	return s
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}

	return s
}

func orDash(s string) string { return orDefault(s, "-") }

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

func sortedKeys(m map[string]res) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}
