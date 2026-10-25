package rgd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hupe1980/chart2kro/internal/chart"
	"github.com/hupe1980/chart2kro/internal/manifest"
	"github.com/hupe1980/chart2kro/internal/probe"
)

// InstanceLabel is added to selectors that would otherwise match every instance's pods.
const InstanceLabel = "app.kubernetes.io/instance"

// GeneratedRoot is the schema key holding inputs for values the chart generates at render time.
const GeneratedRoot = "chart2kroGenerated"

// Options configure assembly.
type Options struct {
	Name, Kind, Group, APIVersion string // RGD name and generated kind's identity
	ChartName, ChartVersion       string
	Release, Namespace            string // probe identity used in the baseline render
	IncludeHooks                  bool
	Exclude, ExcludeSubcharts     []string
	// Scope reports whether a kind is namespaced; known=false for unresolvable kinds.
	Scope func(apiVersion, kind string) (namespaced, known bool)
}

// Input is a classified baseline.
type Input struct {
	Base, Base2 *probe.View // two identical baseline renders (noise = their difference)
	Noise       probe.Noise
	Results     []probe.Result // only Exposed results are used
	// Conditional holds objects that exist only when a boolean is flipped, from that render.
	Conditional map[string]*probe.Obj
	// UserSecrets are value paths (dotted) supplied by the user that land in Secret data.
	UserSecrets map[string]bool
	// Descriptions are value descriptions from values.yaml comments, by dotted path.
	Descriptions map[string]string
}

// Field describes one schema field.
type Field struct {
	Path     []string
	Type     string
	Default  any
	Required bool
	Origin   string // "value" or "generated"
	// Immutable fields land in object fields Kubernetes forbids changing after creation.
	Immutable bool
	// Markers are extra SimpleSchema markers (e.g. listType=map listMapKey=name).
	Markers []string
	// Description comes from the chart's values.yaml comments.
	Description string
	Targets     []string
}

// Dropped is an object left out of the RGD.
type Dropped struct {
	Key, Kind, Name, Source, Reason string
}

// Output is the assembled RGD plus everything conversion and the oracle need.
type Output struct {
	RGD           map[string]any
	IDs           map[string]string // object key → resource id
	Keys          map[string]string // resource id → object key
	Injected      map[string][]manifest.Path
	Prerequisites []map[string]any
	PrereqKeys    []string
	Dropped       []Dropped
	Fields        []Field
	// Demote lists value paths (dotted) that must not be exposed, with reasons;
	// the caller demotes them and assembles again.
	Demote   map[string]string
	Warnings []string
	Isolated []string // ids whose selectors received the instance label
	// Collections maps forEach resource ids to their list and loop position.
	Collections map[string]Collection
	// Types are custom schema types (element types of collection lists).
	Types map[string]any

	excludedIDs []string // collection ids excluded while their list is demoted
}

type edit struct {
	path      manifest.Path
	whole     string // replacement for the whole leaf (already a kro field value)
	holes     []hole
	owner     string
	isTernary bool
}

// Assemble builds the RGD.
func Assemble(in Input, o Options) (*Output, error) {
	out := &Output{IDs: map[string]string{}, Keys: map[string]string{}, Injected: map[string][]manifest.Path{}, Demote: map[string]string{}, Collections: map[string]Collection{}}

	exposed := map[string]probe.Result{}
	for _, r := range in.Results {
		if r.Outcome == probe.Exposed {
			exposed[r.Candidate.Dotted()] = r
		}
	}

	refs := map[string]string{}
	for p, r := range exposed {
		ref, ok := SpecRef(r.Candidate.Path)
		if !ok {
			out.Demote[p] = probe.ReasonKeyNotAddressable
			continue
		}

		refs[p] = ref
	}

	// Lists exposed as collections: their objects become forEach templates, and every
	// target in a loop's objects moves to the collection's template.
	var loops []loopInfo

	for _, p := range sortedKeys(exposed) {
		if r := exposed[p]; r.Loop != nil && refs[p] != "" {
			loops = append(loops, loopInfo{path: p, ref: refs[p], l: r.Loop, r: r})
		}
	}

	for p, r := range exposed {
		exposed[p] = retarget(loops, r)
	}

	// Collections excluded by id stay excluded after their list is demoted (it then has
	// nothing left to control): their objects are dropped, not kept as single resources.
	var excludedLoops []loopInfo

	for _, r := range in.Results {
		if r.Loop == nil || r.Outcome == probe.Exposed {
			continue
		}

		li := loopInfo{path: r.Candidate.Dotted(), l: r.Loop}
		v := iteratorName(r.Candidate.Path, nil)

		for _, pos := range r.Loop.Positions {
			if id := camel(v, kindOf(pos.GVK)); contains(o.Exclude, id) {
				excludedLoops = append(excludedLoops, li)
				out.excludedIDs = append(out.excludedIDs, id)

				break
			}
		}
	}

	// 1. Object set.
	objs := map[string]*probe.Obj{}

	for _, k := range in.Base.Order {
		if translate(excludedLoops, k) != k {
			out.Dropped = append(out.Dropped, dropped(in.Base.Objects[k], "excluded"))
			continue
		}

		if translate(loops, k) == k {
			objs[k] = in.Base.Objects[k]
		}
	}

	for _, li := range loops {
		for j, k := range li.l.Entry0Keys() {
			if t := li.l.Template.Objects[k]; t != nil {
				c := *t
				c.Key = loopKey(li.path, j)
				objs[c.Key] = &c
			}
		}
	}

	gates := map[string][]string{} // key → includeWhen expressions

	for _, p := range sortedKeys(exposed) {
		r := exposed[p]
		if r.Candidate.Kind != probe.Bool || refs[p] == "" {
			continue
		}

		def := r.Candidate.Default.(bool)
		onFlip, onDefault := "${"+refs[p]+"}", "${!"+refs[p]+"}"

		if def {
			onFlip, onDefault = onDefault, onFlip
		}

		for _, k := range r.Added {
			if strings.HasPrefix(k, loopKeyPrefix) && objs[k] != nil {
				gates[k] = append(gates[k], onFlip)
				continue
			}

			if c, ok := in.Conditional[k]; ok {
				objs[k] = c
				gates[k] = append(gates[k], onFlip)
			}
		}

		for _, k := range r.Removed {
			gates[k] = append(gates[k], onDefault)
		}
	}

	keys := sortedKeys(objs)

	var kept []string

	var idIn []idInput

	for _, k := range keys {
		ob := objs[k]

		switch {
		case ob.CRD || ob.Kind() == "CustomResourceDefinition":
			out.Prerequisites = append(out.Prerequisites, restore(ob))
			out.PrereqKeys = append(out.PrereqKeys, k)

			continue
		case chart.IsTestHook(ob.Hook):
			out.Dropped = append(out.Dropped, dropped(ob, "test hook"))
			continue
		case len(ob.Hook) > 0 && !o.IncludeHooks:
			out.Dropped = append(out.Dropped, dropped(ob, "hook "+strings.Join(ob.Hook, ",")))
			continue
		case inSubchart(ob.Source, o.ChartName, o.ExcludeSubcharts):
			out.Dropped = append(out.Dropped, dropped(ob, "excluded subchart"))
			continue
		}

		if o.Scope != nil {
			if ns, known := o.Scope(ob.APIVersion(), ob.Kind()); known && !ns && !strings.Contains(ob.Name(), o.Release) && !strings.HasPrefix(k, loopKeyPrefix) {
				out.Prerequisites = append(out.Prerequisites, restore(ob))
				out.PrereqKeys = append(out.PrereqKeys, k)
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s %s is cluster-scoped with a fixed name; moved to prerequisites (shared by all instances)", ob.Kind(), ob.Name()))

				continue
			}
		}

		if len(ob.Hook) > 0 {
			out.Warnings = append(out.Warnings, fmt.Sprintf("hook %s %s (%s) is kept as an ordinary resource: kro creates it once per instance and never re-runs it on upgrade", ob.Kind(), ob.Name(), strings.Join(ob.Hook, ",")))
		}

		if ob.Kind() == "Job" {
			if _, ok := manifest.Get(ob.Value, manifest.Path{"spec", "ttlSecondsAfterFinished"}); ok {
				out.Warnings = append(out.Warnings, fmt.Sprintf("Job %s sets ttlSecondsAfterFinished: kro recreates it after Kubernetes deletes it, so it runs again", ob.Name()))
			}
		}

		kept = append(kept, k)

		if !strings.HasPrefix(k, loopKeyPrefix) {
			idIn = append(idIn, idInput{key: k, kind: ob.Kind(), name: idName(ob.Name(), o.Release, o.ChartName)})
		}
	}

	ids := assignIDs(idIn)

	used := map[string]bool{}
	for _, id := range ids {
		used[id] = true
	}

	for i := range loops {
		loops[i].v = iteratorName(strings.Split(loops[i].path, "."), used)
	}

	for k, id := range collectionIDs(loops, objs, used) {
		ids[k] = id
	}

	var final, excluded []string

	matched := map[string]bool{}

	for _, k := range kept {
		if contains(o.Exclude, ids[k]) {
			matched[ids[k]] = true
			excluded = append(excluded, k)
			out.Dropped = append(out.Dropped, dropped(objs[k], "excluded"))

			continue
		}

		final = append(final, k)
	}

	for _, id := range out.excludedIDs {
		matched[id] = true
	}

	for _, x := range o.Exclude {
		if !matched[x] {
			out.Warnings = append(out.Warnings, fmt.Sprintf("--exclude %s matches no resource (see 'chart2kro inspect' for ids)", x))
		}
	}

	out.Warnings = append(out.Warnings, danglingNames(objs, excluded, final)...)

	// 2. Leaf edits per object.
	edits := map[string]map[string]*edit{}

	add := func(key string, p manifest.Path, e *edit) {
		if edits[key] == nil {
			edits[key] = map[string]*edit{}
		}

		pk := p.Key()
		e.path = p
		prev, ok := edits[key][pk]

		switch {
		case !ok:
			edits[key][pk] = e
		case prev.whole == "" && e.whole == "" && !prev.isTernary && !e.isTernary:
			prev.holes = append(prev.holes, e.holes...)
			if owner, bad := overlaps(append([]hole{}, prev.holes...)); bad && owner != "" {
				out.Demote[owner] = probe.ReasonConflict
			}
		default:
			owner := e.owner
			if owner == "" {
				owner = prev.owner
			}

			if owner != "" {
				out.Demote[owner] = probe.ReasonConflict
			}
		}
	}

	for _, p := range sortedKeys(exposed) {
		r := exposed[p]

		ref := refs[p]
		if ref == "" {
			continue
		}

		str := r.Candidate.Kind == probe.String

		for _, t := range r.Targets {
			if objs[t.Key] == nil {
				continue
			}

			switch t.Mode {
			case probe.Whole:
				if t.Stringify {
					add(t.Key, t.Path, &edit{whole: "${string(" + ref + ")}", owner: p})
				} else {
					add(t.Key, t.Path, &edit{whole: "${" + ref + "}", owner: p})
				}
			case probe.WholeTree:
				add(t.Key, t.Path, &edit{whole: "${" + ref + "}", owner: p})
			case probe.Span:
				var hs []hole
				for _, s := range t.Spans {
					hs = append(hs, hole{start: s[0], end: s[1], expr: ref, str: str, owner: p})
				}

				add(t.Key, t.Path, &edit{holes: hs, owner: p})
			case probe.Ternary:
				expr, err := ternary(ref, r.Candidate.Default.(bool), t.Default, t.Flipped, o)
				if err != nil {
					out.Demote[p] = probe.ReasonTernaryTypes
					continue
				}

				add(t.Key, t.Path, &edit{whole: expr, owner: p, isTernary: true})
			}
		}
	}

	for _, li := range loops {
		es, reason := loopEdits(li)
		if reason != "" {
			out.Demote[li.path] = reason
			continue
		}

		for k, list := range es {
			for _, e := range list {
				add(k, e.path, e)
			}
		}
	}

	var generated []Field

	// 3. Build objects.
	var resources []any

	for _, k := range final {
		ob := objs[k]
		id := ids[k]
		v := manifest.Normalize(ob.Value).(map[string]any)

		namespaced := true
		if o.Scope != nil {
			if ns, known := o.Scope(ob.APIVersion(), ob.Kind()); known {
				namespaced = ns
			}
		}

		if namespaced {
			if md, ok := v["metadata"].(map[string]any); ok && md["namespace"] == o.Namespace {
				delete(md, "namespace")
			}
		}

		manifest.Leaves(v, func(p manifest.Path, leaf any) {
			s, ok := leaf.(string)
			if !ok {
				return
			}

			pk := p.Key()
			if e := edits[k][pk]; e != nil && (e.whole != "" || e.isTernary) {
				return // whole-leaf edits win; identity inside handled by ternary builder
			}

			if probe.IsChecksumAnnotation(p) {
				add(k, p, &edit{whole: "${base64.encode(hash.sha256(json.marshal(schema.spec)))}"})
				return
			}

			if in.Noise.Has(k, p) {
				g, ok := generatedEdit(k, id, p, s, in.Base2, isSecretData(ob, p))
				if ok {
					add(k, p, g.edit)
					if g.field != nil {
						generated = append(generated, *g.field)
					}
				}

				return
			}

			var hs []hole
			hs = append(hs, occurrences(s, o.Release, refName)...)
			hs = append(hs, occurrences(s, o.Namespace, refNamespace)...)

			if len(hs) > 0 {
				add(k, p, &edit{holes: hs})
			}
		})

		rendered := map[string]bool{}

		for pk, e := range edits[k] {
			rendered[pk] = true

			leaf, ok := manifest.Get(v, e.path)
			if !ok {
				continue
			}

			val := render(leaf, e)

			if ob.B64[pk] {
				// kro cannot write computed bytes; the API server merges stringData into data.
				manifest.Delete(v, e.path)
				sd, _ := v["stringData"].(map[string]any)
				if sd == nil {
					sd = map[string]any{}
					v["stringData"] = sd
				}

				name := e.path[len(e.path)-1].(string)
				sd[name] = val
				rendered[manifest.Path{"stringData", name}.Key()] = true

				continue
			}

			manifest.Set(v, e.path, val)
		}

		if d, ok := v["data"].(map[string]any); ok && len(d) == 0 && v["stringData"] != nil {
			delete(v, "data")
		}

		// Remaining string leaves: escape literal ${ and re-encode Secret data.
		manifest.Leaves(v, func(p manifest.Path, leaf any) {
			s, ok := leaf.(string)
			if !ok || rendered[p.Key()] {
				return
			}

			if ob.B64[p.Key()] {
				manifest.Set(v, p, base64.StdEncoding.EncodeToString([]byte(s)))
				return
			}

			manifest.Set(v, p, escapeLiteral(s))
		})

		res := map[string]any{"id": id, "template": v}

		readyRef := id

		for _, li := range loops {
			for j, pos := range li.l.Positions {
				if k == loopKey(li.path, j) {
					res["forEach"] = []any{map[string]any{li.v: "${" + li.ref + "}"}}
					readyRef = "each"
					out.Collections[id] = Collection{List: li.path, Var: li.v, Position: pos}
				}
			}
		}

		if g := gates[k]; len(g) > 0 {
			sort.Strings(g)
			res["includeWhen"] = toAny(g)
		}

		if rw := readyWhen(ob.Kind(), readyRef); rw != "" {
			res["readyWhen"] = []any{rw}
		}

		resources = append(resources, res)
		out.IDs[k] = id
		out.Keys[id] = k
	}

	isolate(resources, out, o)
	stableNames(out.Dropped, in.Noise)

	// 4. Schema.
	spec := map[string]any{}

	for _, p := range sortedKeys(exposed) {
		r := exposed[p]
		if refs[p] == "" || out.Demote[p] != "" {
			continue
		}

		f := Field{Path: r.Candidate.Path, Type: schemaType(r.Candidate), Default: r.Candidate.Default, Origin: "value", Description: in.Descriptions[p]}

		if in.UserSecrets[p] {
			f.Default, f.Required = nil, true
		}

		for _, t := range r.Targets {
			if id := out.IDs[t.Key]; id != "" {
				f.Targets = append(f.Targets, id+"."+t.Path.String())
			}
		}

		if r.Loop != nil {
			li := loopInfo{path: p, ref: refs[p], l: r.Loop, r: r}
			for _, x := range loops {
				if x.path == p {
					li = x
				}
			}

			typ, markers, elem, reason := listSchema(li)
			if reason != "" {
				out.Demote[p] = reason
				continue
			}

			f.Type, f.Markers = typ, markers
			f.Targets = append(f.Targets, loopTargets(li, out.IDs)...)

			if elem != nil {
				if out.Types == nil {
					out.Types = map[string]any{}
				}

				out.Types[strings.TrimPrefix(typ, "[]")] = elem
			}
		}

		if len(f.Targets) == 0 && !gatesKept(r, out.IDs) {
			// Everything it controls was dropped (hooks, excluded objects, prerequisites).
			out.Demote[p] = probe.ReasonOnlyDropped
			continue
		}

		f.Immutable = immutableTarget(r.Targets, objs)

		setField(spec, f)
		out.Fields = append(out.Fields, f)
	}

	for _, f := range generated {
		setField(spec, f)
		out.Fields = append(out.Fields, f)
	}

	out.RGD = map[string]any{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata": map[string]any{
			"name":   o.Name,
			"labels": map[string]any{"app.kubernetes.io/managed-by": "chart2kro"},
			"annotations": map[string]any{
				"chart2kro.io/chart": o.ChartName + "-" + o.ChartVersion,
			},
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": o.APIVersion,
				"kind":       o.Kind,
				"group":      o.Group,
				"spec":       spec,
			},
			"resources": resources,
		},
	}

	if len(out.Types) > 0 {
		out.RGD["spec"].(map[string]any)["schema"].(map[string]any)["types"] = out.Types
	}

	return out, nil
}

func dropped(ob *probe.Obj, reason string) Dropped {
	return Dropped{Key: ob.Key, Kind: ob.Kind(), Name: ob.Name(), Source: ob.Source, Reason: reason}
}

// stableNames replaces generated (random) names of dropped objects, keeping reports deterministic.
func stableNames(ds []Dropped, noise probe.Noise) {
	for i := range ds {
		if noise.Has(ds[i].Key, manifest.Path{"metadata", "name"}) {
			ds[i].Name = "<generated>"
		}
	}
}

// restore returns an object with Secret data re-encoded.
func restore(ob *probe.Obj) map[string]any {
	v := manifest.Normalize(ob.Value).(map[string]any)

	if data, ok := v["data"].(map[string]any); ok {
		for k, x := range data {
			if s, ok := x.(string); ok && ob.B64[manifest.Path{"data", k}.Key()] {
				data[k] = base64.StdEncoding.EncodeToString([]byte(s))
			}
		}
	}

	return v
}

func getString(v any, p manifest.Path) (string, bool) {
	if p == nil {
		return "", false
	}

	x, ok := manifest.Get(v, p)
	if !ok {
		return "", false
	}

	s, ok := x.(string)

	return s, ok
}

func render(leaf any, e *edit) any {
	if e.whole != "" {
		return e.whole
	}

	s, _ := leaf.(string)

	return template(segments(s, e.holes))
}

func occurrences(s, token, expr string) []hole {
	if token == "" {
		return nil
	}

	var hs []hole

	for i := 0; ; {
		j := strings.Index(s[i:], token)
		if j < 0 {
			break
		}

		hs = append(hs, hole{start: i + j, end: i + j + len(token), expr: expr, str: true})
		i += j + len(token)
	}

	return hs
}

// ternary builds a boolean selection; string branches may contain the release name.
func ternary(ref string, def bool, atDefault, flipped any, o Options) (string, error) {
	branch := func(v any) (string, error) {
		if s, ok := v.(string); ok {
			hs := append(occurrences(s, o.Release, refName), occurrences(s, o.Namespace, refNamespace)...)
			return celConcat(segments(s, hs)), nil
		}

		return celLiteral(v)
	}

	a, err := branch(atDefault)
	if err != nil {
		return "", err
	}

	b, err := branch(flipped)
	if err != nil {
		return "", err
	}

	whenTrue, whenFalse := b, a
	if def {
		whenTrue, whenFalse = a, b
	}

	return "${" + ref + " ? " + whenTrue + " : " + whenFalse + "}", nil
}

type genEdit struct {
	edit  *edit
	field *Field
}

var alnum = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// generatedEdit replaces a value the chart generates at render time. In Secrets it
// always becomes a required input: kro's random.seededString is deterministic by
// design (seeded by the instance UID), which suits names but not secrets. Elsewhere
// (e.g. random name suffixes) an alphanumeric run becomes random.seededString.
func generatedEdit(key, id string, p manifest.Path, s1 string, base2 *probe.View, secret bool) (genEdit, bool) {
	var s2 string

	if o2, ok := base2.Objects[key]; ok {
		s2, _ = getString(o2.Value, p)
	}

	pre, suf := commonAffixes(s1, s2)
	mid := s1[pre : len(s1)-suf]

	if !secret && mid != "" && alnum.MatchString(mid) {
		h := hole{start: pre, end: len(s1) - suf, expr: fmt.Sprintf("random.seededString(%d, schema.metadata.uid)", len(mid)), str: true}

		return genEdit{edit: &edit{holes: []hole{h}}}, true
	}

	name := slug(p)
	if secret && len(p) == 2 {
		name = slug(p[1:]) // chart2kroGenerated.<secret id>.<key>
	}

	fp := []string{GeneratedRoot, id, name}
	ref, _ := SpecRef(fp)
	expr := "${" + ref + "}"

	return genEdit{
		edit:  &edit{whole: expr},
		field: &Field{Path: fp, Type: "string", Required: true, Origin: "generated", Targets: []string{id + "." + p.String()}},
	}, true
}

func isSecretData(ob *probe.Obj, p manifest.Path) bool {
	return ob.Kind() == "Secret" && len(p) > 0 && (p[0] == "data" || p[0] == "stringData")
}

func commonAffixes(a, b string) (int, int) {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}

	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}

	return pre, suf
}

func slug(p manifest.Path) string {
	var parts []string

	for _, s := range p {
		parts = append(parts, fmt.Sprint(s))
	}

	return camel(parts...)
}

func readyWhen(kind, id string) string {
	switch kind {
	case "Deployment":
		return fmt.Sprintf("${%s.status.?availableReplicas.orValue(0) == %s.spec.?replicas.orValue(1)}", id, id)
	case "StatefulSet":
		return fmt.Sprintf("${%s.status.?readyReplicas.orValue(0) == %s.spec.?replicas.orValue(1)}", id, id)
	case "DaemonSet":
		return fmt.Sprintf("${%s.status.?numberReady.orValue(0) == %s.status.?desiredNumberScheduled.orValue(0)}", id, id)
	}

	return ""
}

func schemaType(c probe.Candidate) string {
	switch c.Kind {
	case probe.Bool:
		return "boolean"
	case probe.Int:
		return "integer"
	case probe.Float:
		return "float"
	case probe.List:
		if c.Elem == "string" {
			return "[]string"
		}

		return "[]object"
	case probe.Map:
		// kro v0.9.4 map[string]T forbids keys in instances; object preserves them.
		return "object"
	default:
		return "string"
	}
}

// setField writes "type | default=…" (kro SimpleSchema) at f.Path in spec.
func setField(spec map[string]any, f Field) {
	cur := spec

	for _, s := range f.Path[:len(f.Path)-1] {
		next, ok := cur[s].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[s] = next
		}

		cur = next
	}

	cur[f.Path[len(f.Path)-1]] = marker(f)
}

// marker renders kro SimpleSchema: "type | marker marker …" (markers are space-separated).
func marker(f Field) string {
	var ms []string

	switch {
	case f.Required:
		ms = append(ms, "required=true")
	case f.Default != nil:
		ms = append(ms, defaultMarker(f))
	}

	if f.Immutable {
		ms = append(ms, "immutable=true")
	}

	ms = append(ms, f.Markers...)

	if f.Description != "" {
		ms = append(ms, "description="+markerString(f.Description))
	}

	if len(ms) == 0 {
		return f.Type
	}

	return f.Type + " | " + strings.Join(ms, " ")
}

// quoted renders a string marker value the way kro's marker parser reads it back.
func quoted(s string) string {
	b := jsonString(s)
	b = strings.ReplaceAll(b, `\`, `\\`)
	b = strings.ReplaceAll(b, `"`, `\"`)

	return `"` + b + `"`
}

// markerString quotes a plain-text marker value (description): kro unescapes \" and
// \\ once and does not JSON-decode it (unlike default).
func markerString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)

	return `"` + s + `"`
}

func defaultMarker(f Field) string {

	switch t := f.Default.(type) {
	case string:
		return "default=" + quoted(t)
	case bool:
		return fmt.Sprintf("default=%t", t)
	case float64:
		if f.Type == "integer" {
			return fmt.Sprintf("default=%d", int64(t))
		}

		return fmt.Sprintf("default=%v", t)
	default:
		raw, _ := marshalCompact(t)
		return "default=" + raw
	}
}

// gatesKept reports whether a boolean includes or excludes objects that made it into the RGD.
func gatesKept(r probe.Result, ids map[string]string) bool {
	for _, k := range append(append([]string{}, r.Added...), r.Removed...) {
		if ids[k] != "" {
			return true
		}
	}

	return false
}

// immutableTarget reports whether a value lands in a field Kubernetes forbids changing
// after creation, so the generated API must not let instances change it either.
func immutableTarget(ts []probe.Target, objs map[string]*probe.Obj) bool {
	for _, t := range ts {
		ob := objs[t.Key]
		if ob == nil || len(t.Path) < 2 || t.Path[0] != "spec" {
			continue
		}

		field, _ := t.Path[1].(string)

		switch ob.Kind() {
		case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
			if field == "selector" {
				return true
			}

			if ob.Kind() == "StatefulSet" && (field == "serviceName" || field == "volumeClaimTemplates" || field == "podManagementPolicy") {
				return true
			}
		case "Job":
			if field == "template" || field == "selector" || field == "completionMode" {
				return true
			}
		case "Service":
			if field == "clusterIP" || field == "clusterIPs" {
				return true
			}
		case "PersistentVolumeClaim":
			if field != "resources" {
				return true
			}
		}
	}

	return false
}

// jsonString JSON-escapes s without the surrounding quotes (kro wraps the marker value in quotes).
func jsonString(s string) string {
	b, _ := marshalCompact(s)
	return b[1 : len(b)-1]
}

func marshalCompact(v any) (string, error) {
	var sb strings.Builder

	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return "", err
	}

	return strings.TrimSuffix(sb.String(), "\n"), nil
}

// isolate adds the instance label to workloads whose selectors do not vary per instance
// and to Service/PDB/NetworkPolicy selectors that matched those pods.
func isolate(resources []any, out *Output, o Options) {
	type sel struct {
		res    map[string]any
		labels map[string]any
		path   manifest.Path
	}

	var workloads []sel

	var selectors []sel

	for _, r := range resources {
		res := r.(map[string]any)
		t := res["template"].(map[string]any)
		kind, _ := t["kind"].(string)

		switch kind {
		case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
			p := manifest.Path{"spec", "selector", "matchLabels"}
			if m, ok := mapAt(t, p); ok {
				workloads = append(workloads, sel{res, m, p})
			}
		case "Service":
			if m, ok := mapAt(t, manifest.Path{"spec", "selector"}); ok && len(m) > 0 {
				selectors = append(selectors, sel{res, m, manifest.Path{"spec", "selector"}})
			}
		case "PodDisruptionBudget":
			if m, ok := mapAt(t, manifest.Path{"spec", "selector", "matchLabels"}); ok && len(m) > 0 {
				selectors = append(selectors, sel{res, m, manifest.Path{"spec", "selector", "matchLabels"}})
			}
		case "NetworkPolicy":
			if m, ok := mapAt(t, manifest.Path{"spec", "podSelector", "matchLabels"}); ok && len(m) > 0 {
				selectors = append(selectors, sel{res, m, manifest.Path{"spec", "podSelector", "matchLabels"}})
			}
		}
	}

	varies := func(m map[string]any) bool {
		for _, v := range m {
			if s, ok := v.(string); ok && strings.Contains(s, refName) {
				return true
			}
		}

		return false
	}

	inject := func(res map[string]any, p manifest.Path) {
		t := res["template"].(map[string]any)
		m, _ := mapAt(t, p)
		m[InstanceLabel] = "${" + refName + "}"
		id := res["id"].(string)
		key := out.Keys[id]
		out.Injected[key] = append(out.Injected[key], p.Append(InstanceLabel))
	}

	for _, w := range workloads {
		if varies(w.labels) {
			continue
		}

		t := w.res["template"].(map[string]any)

		podLabels, ok := mapAt(t, manifest.Path{"spec", "template", "metadata", "labels"})
		if !ok {
			continue
		}

		before := copyMap(podLabels)

		inject(w.res, w.path)
		inject(w.res, manifest.Path{"spec", "template", "metadata", "labels"})
		out.Isolated = append(out.Isolated, w.res["id"].(string))

		for _, s := range selectors {
			if varies(s.labels) || !subset(s.labels, before) {
				continue
			}

			if _, done := s.labels[InstanceLabel]; done {
				continue
			}

			inject(s.res, s.path)
		}
	}

	sort.Strings(out.Isolated)
}

func mapAt(v any, p manifest.Path) (map[string]any, bool) {
	x, ok := manifest.Get(v, p)
	if !ok {
		return nil, false
	}

	m, ok := x.(map[string]any)

	return m, ok
}

func subset(a, b map[string]any) bool {
	for k, v := range a {
		if bv, ok := b[k]; !ok || !manifest.Equal(bv, v) {
			return false
		}
	}

	return true
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}

	return out
}

func idName(name, release, chartName string) string {
	n := strings.ReplaceAll(name, release, "")
	n = strings.ReplaceAll(n, chartName, "")

	return strings.Trim(n, "-.")
}

func inSubchart(source, chartName string, subs []string) bool {
	for _, s := range subs {
		if strings.HasPrefix(source, chartName+"/charts/"+s+"/") {
			return true
		}
	}

	return false
}

// danglingNames warns when a kept resource still mentions an excluded resource's name
// (e.g. a Deployment mounting an excluded ConfigMap); the user must provide it.
func danglingNames(objs map[string]*probe.Obj, excluded, kept []string) []string {
	var ws []string

	for _, x := range excluded {
		name := objs[x].Name()
		if name == "" {
			continue
		}

		for _, k := range kept {
			found := false

			manifest.Leaves(objs[k].Value, func(_ manifest.Path, v any) {
				if s, ok := v.(string); ok && s == name {
					found = true
				}
			})

			if found {
				ws = append(ws, fmt.Sprintf("excluded %s %s is still referenced by name from %s %s; it must exist when instances are created", objs[x].Kind(), name, objs[k].Kind(), objs[k].Name()))
			}
		}
	}

	return ws
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}

	return false
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}

	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
