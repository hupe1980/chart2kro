package chart

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
)

// MaxDescription bounds a value description (runes); longer text is cut with "…".
const MaxDescription = 300

var (
	bitnamiParam = regexp.MustCompile(`^@param\s+(\S+)\s+(.+)$`)
	yamlLike     = regexp.MustCompile(`^\s*(-\s|[\w.\-"']+:(\s|$))`)
)

// Descriptions reads value descriptions from the raw values.yaml of a chart and its
// subcharts, keyed by dotted value path. Conventions, in order of precedence:
// helm-docs ("# -- text"), bitnami readme-generator ("## @param path text"), and a
// plain comment directly above a key unless it looks like commented-out YAML.
func Descriptions(ch *chartv2.Chart) map[string]string {
	out := map[string]string{}
	collect(ch, "", out)

	return out
}

func collect(ch *chartv2.Chart, prefix string, out map[string]string) {
	for _, f := range ch.Raw {
		if f.Name == "values.yaml" {
			parseValues(f.Data, prefix, out)
		}
	}

	aliases := map[string]string{}

	if ch.Metadata != nil {
		for _, d := range ch.Metadata.Dependencies {
			if d.Alias != "" {
				aliases[d.Name] = d.Alias
			}
		}
	}

	for _, dep := range ch.Dependencies() {
		name := dep.Name()
		if a := aliases[name]; a != "" {
			name = a
		}

		collect(dep, prefix+name+".", out)
	}
}

func parseValues(data []byte, prefix string, out map[string]string) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return
	}

	helmDocs, plain := map[string]string{}, map[string]string{}
	bitnami := map[string]string{}

	var walk func(n *yaml.Node, path []string)

	walk = func(n *yaml.Node, path []string) {
		for _, c := range []string{n.HeadComment, n.LineComment, n.FootComment} {
			for _, l := range commentLines(c) {
				if m := bitnamiParam.FindStringSubmatch(l); m != nil {
					bitnami[prefix+m[1]] = m[2]
				}
			}
		}

		if n.Kind != yaml.MappingNode {
			for _, c := range n.Content {
				walk(c, path)
			}

			return
		}

		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			p := append(append([]string{}, path...), k.Value)
			key := prefix + strings.Join(p, ".")

			walk(k, p)

			if d, ok := helmDocsText(k.HeadComment); ok {
				helmDocs[key] = d
			} else if d, ok := plainText(k.HeadComment); ok {
				plain[key] = d
			}

			walk(v, p)
		}
	}

	walk(doc.Content[0], nil)

	for _, m := range []map[string]string{plain, bitnami, helmDocs} { // later wins
		for k, d := range m {
			if d = clean(d); d != "" {
				out[k] = d
			}
		}
	}
}

// commentLines strips "#" markers; blank comment lines become "".
func commentLines(c string) []string {
	if c == "" {
		return nil
	}

	var out []string

	for _, l := range strings.Split(c, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimSpace(strings.TrimLeft(l, "#"))
		out = append(out, l)
	}

	return out
}

// helmDocsText returns the text after "-- " and its continuation lines, without
// "@" annotations (e.g. "@default -- …").
func helmDocsText(c string) (string, bool) {
	lines := commentLines(c)

	for i, l := range lines {
		if !strings.HasPrefix(l, "--") {
			continue
		}

		parts := []string{strings.TrimSpace(strings.TrimPrefix(l, "--"))}

		for _, next := range lines[i+1:] {
			if next == "" || strings.HasPrefix(next, "@") || strings.HasPrefix(next, "--") {
				break
			}

			parts = append(parts, next)
		}

		return strings.Join(parts, " "), true
	}

	return "", false
}

// plainText returns the last paragraph of a head comment, unless it looks like
// commented-out YAML or is a bitnami annotation.
func plainText(c string) (string, bool) {
	lines := commentLines(c)

	var para []string

	for _, l := range lines {
		if l == "" {
			para = nil
			continue
		}

		para = append(para, l)
	}

	if len(para) == 0 {
		return "", false
	}

	for _, l := range para {
		if strings.HasPrefix(l, "@") || yamlLike.MatchString(l) {
			return "", false
		}
	}

	return strings.Join(para, " "), true
}

func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")

	if utf8.RuneCountInString(s) > MaxDescription {
		r := []rune(s)
		s = strings.TrimSpace(string(r[:MaxDescription-1])) + "…"
	}

	return s
}
