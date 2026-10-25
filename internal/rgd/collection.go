package rgd

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hupe1980/chart2kro/internal/probe"
)

// Collection is a forEach resource: one object per entry of a list input.
type Collection struct {
	List     string // dotted value path of the list
	Var      string // forEach iterator
	Position probe.Position
}

// loopKeyPrefix marks the object keys of collection templates.
const loopKeyPrefix = "loop:"

type loopInfo struct {
	path string // dotted list path
	ref  string // schema.spec.<list>
	l    *probe.Loop
	r    probe.Result
	v    string // iterator name
}

func loopKey(path string, j int) string { return loopKeyPrefix + path + "#" + strconv.Itoa(j) }

// owner maps an object key produced by a loop (in any render) to its collection key.
func (li loopInfo) owner(key string) (string, bool) {
	if strings.HasPrefix(key, loopKeyPrefix) {
		return "", false
	}

	j, ok := li.l.PositionOf(key)
	if !ok {
		return "", false
	}

	return loopKey(li.path, j), true
}

func translate(loops []loopInfo, key string) string {
	for _, li := range loops {
		if vk, ok := li.owner(key); ok {
			return vk
		}
	}

	return key
}

// retarget rewrites targets and object lists of r onto collection keys, deduplicated.
func retarget(loops []loopInfo, r probe.Result) probe.Result {
	if len(loops) == 0 {
		return r
	}

	seen := map[string]bool{}

	var ts []probe.Target

	for _, t := range r.Targets {
		t.Key = translate(loops, t.Key)
		if k := t.Key + "|" + t.Path.Key(); !seen[k] {
			seen[k] = true
			ts = append(ts, t)
		}
	}

	r.Targets = ts
	r.Added = translateAll(loops, r.Added)
	r.Removed = translateAll(loops, r.Removed)

	return r
}

func translateAll(loops []loopInfo, keys []string) []string {
	seen := map[string]bool{}

	var out []string

	for _, k := range keys {
		if k = translate(loops, k); !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}

	return out
}

var identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// iteratorName is the singular of the list's last segment, or "<singular>Entry" when
// that is reserved by kro, taken by a resource id, or not an identifier.
func iteratorName(path []string, taken map[string]bool) string {
	last := camel(path[len(path)-1])
	if last == "" {
		last = "list"
	}

	single := last
	if len(single) > 3 && strings.HasSuffix(single, "s") && !strings.HasSuffix(single, "ss") {
		single = strings.TrimSuffix(single, "s")
	}

	if !identRe.MatchString(single) || reservedIDs[single] || taken[single] {
		single += "Entry"
	}

	for n := 2; taken[single]; n++ {
		single = strings.TrimRightFunc(single, unicode.IsDigit) + strconv.Itoa(n)
	}

	return single
}

// elementTypeName names the custom schema type of an object list's entries.
func elementTypeName(path []string) string {
	c := []rune(camel(path...))
	if len(c) > 0 {
		c[0] = unicode.ToUpper(c[0])
	}

	return string(c) + "Entry"
}

// fieldExpr returns "<var>.<field>" (or the iterator itself for string entries).
func fieldExpr(v, field string) (string, bool) {
	if field == "" {
		return v, true
	}

	if !identRe.MatchString(field) || reservedIDs[field] {
		return "", false
	}

	return v + "." + field, true
}

// loopEdits returns the leaf edits that turn entry fields and the index into
// expressions of the iterator, keyed by collection key and path.
func loopEdits(li loopInfo) (map[string][]*edit, string) {
	out := map[string][]*edit{}

	addTargets := func(expr string, str bool, ts []probe.Target) {
		for _, t := range ts {
			vk, ok := li.owner(t.Key)
			if !ok {
				continue
			}

			e := &edit{path: t.Path, owner: li.path}

			switch t.Mode {
			case probe.Whole:
				if t.Stringify && !str {
					e.whole = "${string(" + expr + ")}"
				} else {
					e.whole = "${" + expr + "}"
				}
			case probe.WholeTree:
				e.whole = "${" + expr + "}"
			case probe.Span:
				for _, s := range t.Spans {
					e.holes = append(e.holes, hole{start: s[0], end: s[1], expr: expr, str: str, owner: li.path})
				}
			default:
				continue
			}

			out[vk] = append(out[vk], e)
		}
	}

	for _, f := range li.l.Fields {
		expr, ok := fieldExpr(li.v, f.Name)
		if !ok {
			return nil, probe.ReasonKeyNotAddressable
		}

		addTargets(expr, f.Type == "string", f.Targets)
	}

	addTargets(li.ref+".indexOf("+li.v+")", false, li.l.Index)

	return out, ""
}

// listSchema returns the list's field type, its list-type markers and, for object
// entries, the element type, or a demotion reason.
func listSchema(li loopInfo) (typ string, markers []string, elem map[string]any, reason string) {
	nameFields := map[string]bool{}

	for _, f := range li.l.Fields {
		for _, t := range f.Targets {
			if len(t.Path) == 2 && t.Path[0] == "metadata" && t.Path[1] == "name" {
				nameFields[f.Name] = true
			}
		}
	}

	indexNamed := false

	for _, t := range li.l.Index {
		if len(t.Path) == 2 && t.Path[0] == "metadata" && t.Path[1] == "name" {
			indexNamed = true
		}
	}

	if len(nameFields) == 0 && !indexNamed {
		return "", nil, nil, probe.ReasonLoopElement
	}

	if li.l.Elem == "string" {
		return "[]string", []string{"listType=set"}, nil, ""
	}

	name := elementTypeName(strings.Split(li.path, "."))
	elem = map[string]any{}

	for _, f := range li.l.Fields {
		elem[f.Name] = f.Type + " | required=true"
	}

	if len(nameFields) == 1 && !indexNamed {
		for k := range nameFields {
			return "[]" + name, []string{"listType=map", "listMapKey=" + k}, elem, ""
		}
	}

	// Without a single naming field (e.g. index-based names), entries must still be
	// unique: key the list on all scalar fields. A set of objects would need atomic
	// items, which kro's markers cannot express and the API server rejects.
	var keys []string

	for _, f := range li.l.Fields {
		switch f.Type {
		case "string", "integer", "boolean":
			keys = append(keys, "listMapKey="+f.Name)
		}
	}

	if len(keys) == 0 {
		return "", nil, nil, probe.ReasonLoopElement
	}

	sort.Strings(keys)

	return "[]" + name, append([]string{"listType=map"}, keys...), elem, ""
}

// loopTargets lists "<collection id>.<path>" for the report.
func loopTargets(li loopInfo, ids map[string]string) []string {
	var out []string

	seen := map[string]bool{}

	add := func(ts []probe.Target) {
		for _, t := range ts {
			vk, ok := li.owner(t.Key)
			if !ok || ids[vk] == "" {
				continue
			}

			s := ids[vk] + "." + t.Path.String()
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}

	for _, f := range li.l.Fields {
		add(f.Targets)
	}

	add(li.l.Index)
	sort.Strings(out)

	return out
}

// collectionIDs names collection resources "<var><Kind>", unique among all ids.
func collectionIDs(loops []loopInfo, objs map[string]*probe.Obj, used map[string]bool) map[string]string {
	out := map[string]string{}

	for _, li := range loops {
		for j := range li.l.Positions {
			vk := loopKey(li.path, j)

			ob := objs[vk]
			if ob == nil {
				continue
			}

			id := camel(li.v, ob.Kind())
			if reservedIDs[id] {
				id += "Res"
			}

			cand := id
			for n := 2; used[cand]; n++ {
				cand = id + strconv.Itoa(n)
			}

			used[cand] = true
			out[vk] = cand
		}
	}

	return out
}

// kindOf returns the kind of an "apiVersion/kind" string.
func kindOf(gvk string) string {
	if i := strings.LastIndex(gvk, "/"); i >= 0 {
		return gvk[i+1:]
	}

	return gvk
}
