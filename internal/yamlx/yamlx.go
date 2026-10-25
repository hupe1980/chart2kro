// Package yamlx encodes JSON-like value trees as deterministic YAML. It only formats:
// empty maps, empty lists and nulls are kept exactly, strings are never edited.
package yamlx

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// priority keys come first in every mapping, in this order; the rest are sorted.
var priority = []string{"apiVersion", "kind", "metadata", "name", "namespace", "id"}

// Marshal encodes v as one YAML document.
func Marshal(v any) ([]byte, error) {
	n, err := node(v)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(n); err != nil {
		return nil, err
	}

	if err := enc.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// MarshalStream encodes several documents separated by "---".
func MarshalStream(docs ...any) ([]byte, error) {
	var out bytes.Buffer

	for i, d := range docs {
		b, err := Marshal(d)
		if err != nil {
			return nil, err
		}

		if i > 0 {
			out.WriteString("---\n")
		}

		out.Write(b)
	}

	return out.Bytes(), nil
}

func node(v any) (*yaml.Node, error) {
	switch t := v.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	case map[string]any:
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if len(t) == 0 {
			m.Style = yaml.FlowStyle
		}

		for _, k := range orderedKeys(t) {
			vn, err := node(t[k])
			if err != nil {
				return nil, err
			}

			m.Content = append(m.Content, str(k), vn)
		}

		return m, nil
	case []any:
		s := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if len(t) == 0 {
			s.Style = yaml.FlowStyle
		}

		for _, e := range t {
			en, err := node(e)
			if err != nil {
				return nil, err
			}

			s.Content = append(s.Content, en)
		}

		return s, nil
	case string:
		return str(t), nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(t)}, nil
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(t)}, nil
	case int64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(t, 10)}, nil
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(int64(t), 10)}, nil
		}

		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(t, 'g', -1, 64)}, nil
	default:
		return nil, fmt.Errorf("yamlx: unsupported type %T", v)
	}
}

func str(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}

	switch {
	case yaml11Bool[s]:
		// Kubernetes tooling (sigs.k8s.io/yaml) reads YAML 1.1, where these are booleans.
		n.Style = yaml.DoubleQuotedStyle
	case strings.Contains(s, "\n") && !strings.HasPrefix(s, " ") && !hasTrailingSpaceLines(s):
		n.Style = yaml.LiteralStyle
	}

	return n
}

var yaml11Bool = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range []string{"y", "yes", "n", "no", "true", "false", "on", "off"} {
		m[w], m[strings.ToUpper(w)], m[strings.ToUpper(w[:1])+w[1:]] = true, true, true
	}

	return m
}()

// Literal blocks cannot represent lines with trailing spaces faithfully in all parsers.
func hasTrailingSpaceLines(s string) bool {
	for l := range strings.SplitSeq(s, "\n") {
		if strings.HasSuffix(l, " ") || strings.HasSuffix(l, "\t") {
			return true
		}
	}

	return false
}

func orderedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	rank := func(k string) int {
		for i, p := range priority {
			if p == k {
				return i
			}
		}

		return len(priority)
	}

	sort.SliceStable(keys, func(i, j int) bool {
		ri, rj := rank(keys[i]), rank(keys[j])
		if ri != rj {
			return ri < rj
		}

		return keys[i] < keys[j]
	})

	return keys
}
