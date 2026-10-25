package chart2kro

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/chart"
	"github.com/hupe1980/chart2kro/internal/manifest"
	"github.com/hupe1980/chart2kro/internal/yamlx"
)

func yamlUnmarshal(b []byte, v any) error { return yaml.Unmarshal(b, v) }

// whyNot is the outcome and reason of the nearest reported value at or above keys.
func whyNot(why map[string]string, keys []string) string {
	for i := len(keys); i > 0; i-- {
		if w := why[strings.Join(keys[:i], ".")]; w != "" {
			return w
		}
	}

	return "not a chart value"
}

var dns1123 = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// InstanceOptions select the Helm values an instance should reproduce.
type InstanceOptions struct {
	Name, Namespace string
	ValueFiles      []string // Helm values of the existing release
	Set             []string
	// AllowUnexposed drops settings at paths the API does not expose instead of failing.
	AllowUnexposed bool
}

// Instance converts the chart (o) and returns an instance manifest whose spec sets
// exactly the exposed values where the given Helm values differ from the API's
// defaults. Settings at paths the API does not expose fail with ErrConstraint
// unless AllowUnexposed is set.
func Instance(ctx context.Context, o Options, in InstanceOptions) ([]byte, error) {
	if len(in.Name) > 253 || !dns1123.MatchString(in.Name) {
		return nil, fmt.Errorf("%w: instance name %q must be a DNS-1123 subdomain (lowercase letters, digits, '-', '.')", ErrUsage, in.Name)
	}

	c, err := newConversion(ctx, o)
	if err != nil {
		return nil, err
	}

	res, err := c.run(ctx)
	if err != nil {
		return nil, err
	}

	from, err := chart.UserValues(in.ValueFiles, in.Set, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: values: %w", ErrUsage, err)
	}

	user, err := chart.UserValues(o.ValueFiles, o.Set, o.SetString, o.SetFile)
	if err != nil {
		return nil, fmt.Errorf("%w: values: %w", ErrUsage, err)
	}

	// The release's effective values: chart defaults ⊕ API defaults ⊕ release values.
	want, err := c.r.Values(mergeValues(user, from))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRender, err)
	}

	exposed := map[string]ValueReport{}
	why := map[string]string{}

	for _, v := range res.Report.Values {
		if v.Outcome == "exposed" {
			exposed[v.Path] = v
		} else {
			why[v.Path] = strings.TrimSuffix(v.Outcome+": "+v.Reason, ": ")
		}
	}

	spec := map[string]any{}
	set := map[string]any{} // exposed path → value the instance sets

	var unexposed []string

	manifest.Leaves(manifest.Normalize(want), func(p manifest.Path, v any) {
		keys := make([]string, len(p))
		for i, s := range p {
			keys[i] = fmt.Sprint(s)
		}

		def, ok := lookup(c.values, keys)
		if ok && manifest.Equal(def, v) {
			return
		}

		// Whole-value inputs (lists, maps) are matched at their own path.
		for i := len(keys); i > 0; i-- {
			path := strings.Join(keys[:i], ".")
			if _, ok := exposed[path]; ok {
				full, _ := lookup(want, keys[:i])
				setPath(spec, keys[:i], full)
				set[path] = full

				return
			}
		}

		unexposed = append(unexposed, strings.Join(keys, ".")+" ("+whyNot(why, keys)+")")
	})

	if len(unexposed) > 0 && in.AllowUnexposed {
		for _, u := range unexposed {
			o.logger().Warn("dropping a value the API does not expose", "value", u)
		}
	}

	if len(unexposed) > 0 && !in.AllowUnexposed {
		sort.Strings(unexposed)

		return nil, fmt.Errorf("%w: these values are set but not exposed by the generated API:\n  %s\n"+
			"re-run convert with them in --expose, or pass --allow-unexposed to drop them",
			ErrConstraint, strings.Join(unexposed, "\n  "))
	}

	if err := checkTypes(set, exposed, elementTypes(res.RGD)); err != nil {
		return nil, err
	}

	secretValues(set, exposed, res.Report.Resources, o.logger())

	md := map[string]any{"name": in.Name}
	if in.Namespace != "" {
		md["namespace"] = in.Namespace
	}

	return yamlx.Marshal(map[string]any{
		"apiVersion": res.Report.Group + "/" + res.Report.APIVersion,
		"kind":       res.Report.Kind,
		"metadata":   md,
		"spec":       manifest.Normalize(spec),
	})
}

func mergeValues(a, b map[string]any) map[string]any {
	out := chart.DeepCopy(a)

	for k, v := range b {
		if bm, ok := v.(map[string]any); ok {
			if am, ok := out[k].(map[string]any); ok {
				out[k] = mergeValues(am, bm)
				continue
			}
		}

		out[k] = v
	}

	return out
}

// checkTypes reports values whose type does not match the exposed field's
// SimpleSchema type: the API server would reject the instance.
func checkTypes(set map[string]any, exposed map[string]ValueReport, types map[string]map[string]string) error {
	var bad []string

	for _, p := range sortedKeys(set) {
		if want := exposed[p].Type; want != "" && !hasType(set[p], want, types) {
			bad = append(bad, fmt.Sprintf("%s: want %s, got %s", p, want, typeName(set[p])))
		}
	}

	if len(bad) > 0 {
		return fmt.Errorf("%w: these values do not match the API's types:\n  %s", ErrConstraint, strings.Join(bad, "\n  "))
	}

	return nil
}

func hasType(v any, t string, types map[string]map[string]string) bool {
	if fields, ok := types[strings.TrimPrefix(t, "[]")]; ok && strings.HasPrefix(t, "[]") {
		l, ok := v.([]any)
		if !ok {
			return false
		}

		for _, x := range l {
			m, ok := x.(map[string]any)
			if !ok || len(m) != len(fields) {
				return false
			}

			for k, ft := range fields {
				if fv, ok := m[k]; !ok || !hasType(fv, ft, types) {
					return false
				}
			}
		}

		return true
	}

	switch t {
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		switch n := v.(type) {
		case int, int64:
			return true
		case float64:
			return n == float64(int64(n))
		}

		return false
	case "float":
		switch v.(type) {
		case int, int64, float64:
			return true
		}

		return false
	case "string":
		_, ok := v.(string)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "[]string", "[]object":
		l, ok := v.([]any)
		if !ok {
			return false
		}

		for _, x := range l {
			if !hasType(x, strings.TrimPrefix(t, "[]"), types) {
				return false
			}
		}

		return true
	}

	return true
}

func typeName(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case int, int64, float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "list"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	}

	return fmt.Sprintf("%T", v)
}

// secretValues warns about values that land in Secrets: they are written into the
// instance manifest, so it must be handled like a secret.
func secretValues(set map[string]any, exposed map[string]ValueReport, resources []ResourceReport, log *slog.Logger) {
	kind := map[string]string{}
	for _, r := range resources {
		kind[r.ID] = r.Kind
	}

	for _, p := range sortedKeys(set) {
		for _, t := range exposed[p].Targets {
			id, _, _ := strings.Cut(t, ".")
			if kind[id] == "Secret" {
				log.Warn("this value lands in a Secret and is written into the instance manifest; treat the file as a secret", "value", p)
				break
			}
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// elementTypes reads the element types of collection lists from an RGD:
// type name → field → SimpleSchema type.
func elementTypes(rgd []byte) map[string]map[string]string {
	var m struct {
		Spec struct {
			Schema struct {
				Types map[string]map[string]string `json:"types"`
			} `json:"schema"`
		} `json:"spec"`
	}

	if err := yamlUnmarshal(rgd, &m); err != nil {
		return nil
	}

	out := map[string]map[string]string{}

	for name, fields := range m.Spec.Schema.Types {
		out[name] = map[string]string{}
		for f, marker := range fields {
			out[name][f] = strings.TrimSpace(strings.SplitN(marker, "|", 2)[0])
		}
	}

	return out
}
