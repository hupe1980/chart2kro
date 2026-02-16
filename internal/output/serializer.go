package output

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"
)

// SerializeOptions configures the canonical YAML serializer.
type SerializeOptions struct {
	// Comments enables inline comments on CEL expressions.
	Comments bool
	// Indent is the number of spaces per indentation level (default: 2).
	Indent int
}

// DefaultSerializeOptions returns sensible defaults.
func DefaultSerializeOptions() SerializeOptions {
	return SerializeOptions{
		Comments: false,
		Indent:   2,
	}
}

// Serialize converts an RGD map to canonical YAML bytes.
// The output has deterministic key ordering, consistent formatting,
// and optional inline comments on CEL expressions.
func Serialize(rgdMap map[string]interface{}, opts SerializeOptions) ([]byte, error) {
	if opts.Indent == 0 {
		opts.Indent = 2
	}

	// Produce the canonical ordered map.
	ordered := canonicalizeRGD(rgdMap)

	// Serialize with sigs.k8s.io/yaml (which sorts keys alphabetically).
	yamlBytes, err := sigsyaml.Marshal(ordered)
	if err != nil {
		return nil, fmt.Errorf("serializing YAML: %w", err)
	}

	// Strip null values that might have leaked through.
	yamlBytes = stripNullFields(yamlBytes)

	// Add CEL expression comments if enabled.
	if opts.Comments {
		yamlBytes = addCELComments(yamlBytes)
	}

	// Ensure trailing newline.
	if len(yamlBytes) > 0 && yamlBytes[len(yamlBytes)-1] != '\n' {
		yamlBytes = append(yamlBytes, '\n')
	}

	return yamlBytes, nil
}

// SerializeJSON converts an RGD map to indented JSON bytes.
func SerializeJSON(rgdMap map[string]interface{}, indent string) ([]byte, error) {
	if indent == "" {
		indent = "  "
	}

	ordered := canonicalizeRGD(rgdMap)

	// Use sigs.k8s.io/yaml to produce JSON (it handles the conversion).
	jsonBytes, err := sigsyaml.Marshal(ordered)
	if err != nil {
		return nil, fmt.Errorf("serializing intermediate YAML: %w", err)
	}

	// Convert YAML to JSON.
	jsonOut, err := sigsyaml.YAMLToJSON(jsonBytes)
	if err != nil {
		return nil, fmt.Errorf("converting to JSON: %w", err)
	}

	// Pretty-print the JSON.
	var buf bytes.Buffer
	if err := prettyPrintJSON(&buf, jsonOut, indent); err != nil {
		return nil, fmt.Errorf("formatting JSON: %w", err)
	}

	b := buf.Bytes()

	// Ensure trailing newline.
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}

	return b, nil
}

// canonicalizeRGD produces a clean map ready for deterministic serialization.
// It removes nil values, empty maps, and ensures all nested structures are
// properly typed for consistent YAML output.
func canonicalizeRGD(rgdMap map[string]interface{}) map[string]interface{} {
	cleaned := deepCleanMap(rgdMap)

	// Ensure spec.resources order is preserved (slice ordering is stable).
	// No additional reordering needed — sigs.k8s.io/yaml sorts map keys
	// alphabetically, and the RGD top-level keys (apiVersion, kind,
	// metadata, spec) already sort correctly.
	return cleaned
}

// deepCleanMap recursively cleans a map by removing nil values and
// ensuring all nested structures are properly typed.
func deepCleanMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))

	for k, v := range m {
		cleaned := deepCleanValue(v)
		if cleaned != nil {
			result[k] = cleaned
		}
	}

	return result
}

// deepCleanValue cleans a single value recursively.
func deepCleanValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]interface{}:
		cleaned := deepCleanMap(val)
		if len(cleaned) == 0 {
			return nil
		}

		return cleaned
	case []interface{}:
		result := make([]interface{}, 0, len(val))
		for _, item := range val {
			cleaned := deepCleanValue(item)
			if cleaned != nil {
				result = append(result, cleaned)
			}
		}

		return result
	default:
		return v
	}
}

// stripNullRegex matches YAML lines containing `: null`.
var stripNullRegex = regexp.MustCompile(`(?m)^\s+\w[^:]*:\s+null\s*\n`)

// stripNullFields removes lines containing `: null` from YAML output.
func stripNullFields(yamlBytes []byte) []byte {
	return stripNullRegex.ReplaceAll(yamlBytes, nil)
}

// celExpressionRegex matches CEL expression values in YAML.
var celExpressionRegex = regexp.MustCompile(`^(\s*)(\S+):\s+(\$\{.+\})\s*$`)

// addCELComments adds explanatory comments above CEL expression lines.
func addCELComments(yamlBytes []byte) []byte {
	lines := strings.Split(string(yamlBytes), "\n")

	var result []string

	for _, line := range lines {
		matches := celExpressionRegex.FindStringSubmatch(line)
		if matches != nil {
			indent := matches[1]
			expr := matches[3]
			comment := describeCELExpression(expr)

			if comment != "" {
				result = append(result, indent+"# "+comment)
			}
		}

		result = append(result, line)
	}

	return []byte(strings.Join(result, "\n"))
}

// describeCELExpression produces a human-readable comment for a CEL expression.
func describeCELExpression(expr string) string {
	// Strip ${...} wrapper.
	inner := strings.TrimPrefix(expr, "${")
	inner = strings.TrimSuffix(inner, "}")

	switch {
	case strings.HasPrefix(inner, "schema.spec."):
		field := strings.TrimPrefix(inner, "schema.spec.")
		return fmt.Sprintf("From Helm values: .Values.%s", field)
	case strings.HasPrefix(inner, "self."):
		return "Readiness/status self-reference"
	case strings.Contains(inner, ".status."):
		parts := strings.SplitN(inner, ".", 2)
		return fmt.Sprintf("Status from resource: %s", parts[0])
	case strings.Contains(inner, ".metadata."):
		parts := strings.SplitN(inner, ".", 2)
		return fmt.Sprintf("Reference to resource: %s", parts[0])
	default:
		return ""
	}
}

// prettyPrintJSON reformats compact JSON with indentation.
func prettyPrintJSON(buf *bytes.Buffer, jsonBytes []byte, indent string) error {
	var raw interface{}

	if err := sigsyaml.Unmarshal(jsonBytes, &raw); err != nil {
		return err
	}

	return jsonMarshalIndent(buf, raw, indent)
}

// jsonMarshalIndent writes indented JSON to a buffer.
func jsonMarshalIndent(buf *bytes.Buffer, v interface{}, indent string) error {
	return jsonWriteValue(buf, v, indent, 0)
}

// jsonWriteValue recursively writes a JSON value with indentation.
func jsonWriteValue(buf *bytes.Buffer, v interface{}, indent string, level int) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case float64:
		if val == float64(int64(val)) {
			fmt.Fprintf(buf, "%d", int64(val))
		} else {
			fmt.Fprintf(buf, "%g", val)
		}
	case string:
		buf.WriteString(jsonQuote(val))
	case map[string]interface{}:
		if len(val) == 0 {
			buf.WriteString("{}")

			return nil
		}

		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}

		// Sort keys for determinism.
		sort.Strings(keys)

		buf.WriteString("{\n")

		for i, k := range keys {
			writeIndent(buf, indent, level+1)
			buf.WriteString(jsonQuote(k))
			buf.WriteString(": ")

			if err := jsonWriteValue(buf, val[k], indent, level+1); err != nil {
				return err
			}

			if i < len(keys)-1 {
				buf.WriteByte(',')
			}

			buf.WriteByte('\n')
		}

		writeIndent(buf, indent, level)
		buf.WriteByte('}')
	case []interface{}:
		if len(val) == 0 {
			buf.WriteString("[]")

			return nil
		}

		buf.WriteString("[\n")

		for i, item := range val {
			writeIndent(buf, indent, level+1)

			if err := jsonWriteValue(buf, item, indent, level+1); err != nil {
				return err
			}

			if i < len(val)-1 {
				buf.WriteByte(',')
			}

			buf.WriteByte('\n')
		}

		writeIndent(buf, indent, level)
		buf.WriteByte(']')
	default:
		fmt.Fprintf(buf, "%v", val)
	}

	return nil
}

func writeIndent(buf *bytes.Buffer, indent string, level int) {
	for range level {
		buf.WriteString(indent)
	}
}

// jsonQuote performs JSON string quoting with proper escaping.
func jsonQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\b", `\b`)
	s = strings.ReplaceAll(s, "\f", `\f`)

	return `"` + s + `"`
}
