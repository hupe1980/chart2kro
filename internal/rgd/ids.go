// Package rgd assembles a kro ResourceGraphDefinition from a classified chart render,
// following the kro contract: lowerCamel ids, readiness by id, no inferred references,
// identity from the instance, faithful literal content.
package rgd

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// reservedIDs mirrors kro v0.9.4 pkg/graph/validation.go (unexported there); a test pins
// it against the builder.
var reservedIDs = map[string]bool{}

func init() {
	// kroReservedKeyWords ∪ celReservedSymbols at kro v0.9.4.
	for _, w := range strings.Fields(`apiVersion context dependency dependencies each externalRef
		externalReference externalRefs externalReferences graph instance item items kind kro metadata
		namespace object resource resourcegraphdefinition resourceGraphDefinition resources root runtime
		schema self serviceAccountName spec status this variables vars version
		true false null in as break const continue else for function if import let loop package
		return var void while`) {
		reservedIDs[w] = true
	}
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]+`)

// camel joins words as lowerCamelCase.
func camel(words ...string) string {
	var b strings.Builder

	for _, w := range words {
		for _, part := range nonAlnum.Split(w, -1) {
			if part == "" {
				continue
			}

			if b.Len() == 0 {
				b.WriteString(lowerFirst(part))
				continue
			}

			r := []rune(part)
			r[0] = unicode.ToUpper(r[0])
			b.WriteString(string(r))
		}
	}

	s := b.String()
	if s == "" || !unicode.IsLetter(rune(s[0])) {
		s = "r" + s
	}

	return s
}

func lowerFirst(s string) string {
	r := []rune(s)
	// Keep acronyms readable: "HTTPRoute" → "httpRoute".
	i := 0
	for i < len(r) && unicode.IsUpper(r[i]) {
		i++
	}

	switch {
	case i == 0:
	case i == len(r) || i == 1:
		for j := 0; j < i; j++ {
			r[j] = unicode.ToLower(r[j])
		}
	default:
		for j := 0; j < i-1; j++ {
			r[j] = unicode.ToLower(r[j])
		}
	}

	return string(r)
}

type idInput struct {
	key  string
	kind string
	name string // name with release token and chart name removed
}

// assignIDs gives every object a unique, kro-valid id: lowerCamel(kind) plus the
// shortest suffix from its name that distinguishes it among objects of the same kind.
func assignIDs(in []idInput) map[string]string {
	sort.Slice(in, func(i, j int) bool { return in[i].key < in[j].key })

	byKind := map[string][]idInput{}
	for _, x := range in {
		byKind[x.kind] = append(byKind[x.kind], x)
	}

	out := map[string]string{}
	used := map[string]bool{}

	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}

	sort.Strings(kinds)

	for _, kind := range kinds {
		group := byKind[kind]
		base := camel(kind)

		for _, x := range group {
			id := base
			if len(group) > 1 {
				id = camel(kind, x.name)
			}

			if reservedIDs[id] {
				id += "Res"
			}

			cand := id
			for n := 2; used[cand]; n++ {
				cand = id + strconv.Itoa(n)
			}

			used[cand] = true
			out[x.key] = cand
		}
	}

	return out
}
