package output

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ValidationSeverity indicates the severity of a validation finding.
type ValidationSeverity int

const (
	// SeverityError means the RGD is invalid.
	SeverityError ValidationSeverity = iota
	// SeverityWarning means the RGD may be problematic.
	SeverityWarning
)

// String returns the severity name.
func (s ValidationSeverity) String() string {
	if s == SeverityError {
		return "error"
	}

	return "warning"
}

// ValidationFinding is a single validation issue.
type ValidationFinding struct {
	Severity ValidationSeverity
	Field    string
	Message  string
}

// Error implements the error interface.
func (f *ValidationFinding) Error() string {
	return fmt.Sprintf("[%s] %s: %s", f.Severity, f.Field, f.Message)
}

// ValidationResult holds all findings from a validation run.
type ValidationResult struct {
	Findings []ValidationFinding
}

// Errors returns only error-severity findings.
func (r *ValidationResult) Errors() []ValidationFinding {
	var result []ValidationFinding

	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			result = append(result, f)
		}
	}

	return result
}

// Warnings returns only warning-severity findings.
func (r *ValidationResult) Warnings() []ValidationFinding {
	var result []ValidationFinding

	for _, f := range r.Findings {
		if f.Severity == SeverityWarning {
			result = append(result, f)
		}
	}

	return result
}

// HasErrors returns true if any error-severity findings exist.
func (r *ValidationResult) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}

	return false
}

// HasWarnings returns true if any warning-severity findings exist.
func (r *ValidationResult) HasWarnings() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityWarning {
			return true
		}
	}

	return false
}

// ValidateRGD validates an RGD map for correctness.
func ValidateRGD(rgdMap map[string]interface{}) *ValidationResult {
	v := &validator{rgdMap: rgdMap}
	v.validate()

	return &v.result
}

type validator struct {
	rgdMap map[string]interface{}
	result ValidationResult
}

func (v *validator) addError(field, msg string) {
	v.result.Findings = append(v.result.Findings, ValidationFinding{
		Severity: SeverityError,
		Field:    field,
		Message:  msg,
	})
}

func (v *validator) addWarning(field, msg string) {
	v.result.Findings = append(v.result.Findings, ValidationFinding{
		Severity: SeverityWarning,
		Field:    field,
		Message:  msg,
	})
}

func (v *validator) validate() {
	v.validateRequiredFields()
	v.validateSchema()
	v.validateResources()
	v.validateCELReferences()
	v.validateDependencyGraph()
}

// validateRequiredFields checks for required top-level fields.
func (v *validator) validateRequiredFields() {
	apiVersion, _ := v.rgdMap["apiVersion"].(string)
	if apiVersion == "" {
		v.addError("apiVersion", "required field is missing")
	} else if apiVersion != "kro.run/v1alpha1" {
		v.addWarning("apiVersion", fmt.Sprintf("unexpected apiVersion: %s (expected kro.run/v1alpha1)", apiVersion))
	}

	kind, _ := v.rgdMap["kind"].(string)
	if kind == "" {
		v.addError("kind", "required field is missing")
	} else if kind != "ResourceGraphDefinition" {
		v.addError("kind", fmt.Sprintf("unexpected kind: %s (expected ResourceGraphDefinition)", kind))
	}

	meta, _ := v.rgdMap["metadata"].(map[string]interface{})
	if meta == nil {
		v.addError("metadata", "required field is missing")
	} else {
		name, _ := meta["name"].(string)
		if name == "" {
			v.addError("metadata.name", "required field is missing")
		}
	}

	spec, _ := v.rgdMap["spec"].(map[string]interface{})
	if spec == nil {
		v.addError("spec", "required field is missing")
	}
}

// pascalCaseRegex matches PascalCase identifiers.
var pascalCaseRegex = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`)

// validateSchema checks the spec.schema section.
func (v *validator) validateSchema() {
	spec, _ := v.rgdMap["spec"].(map[string]interface{})
	if spec == nil {
		return
	}

	schema, _ := spec["schema"].(map[string]interface{})
	if schema == nil {
		v.addWarning("spec.schema", "schema is missing (no custom resource will be generated)")
		return
	}

	// Check required schema fields.
	apiVersion, _ := schema["apiVersion"].(string)
	if apiVersion == "" {
		v.addError("spec.schema.apiVersion", "required field is missing")
	}

	kind, _ := schema["kind"].(string)
	if kind == "" {
		v.addError("spec.schema.kind", "required field is missing")
	} else if !pascalCaseRegex.MatchString(kind) {
		v.addError("spec.schema.kind", fmt.Sprintf("kind %q is not PascalCase", kind))
	}

	// Validate spec fields have valid types.
	if specFields, ok := schema["spec"].(map[string]interface{}); ok {
		v.validateSchemaFields("spec.schema.spec", specFields)
	}
}

// validSimpleSchemaTypes are the types allowed in a KRO SimpleSchema.
var validSimpleSchemaTypes = map[string]bool{
	"string":  true,
	"integer": true,
	"boolean": true,
	"number":  true,
	"object":  true,
	"array":   true,
}

// validateSchemaFields recursively validates schema field types.
func (v *validator) validateSchemaFields(prefix string, fields map[string]interface{}) {
	for name, val := range fields {
		fieldPath := prefix + "." + name

		switch t := val.(type) {
		case string:
			// Check if it's a valid type, handling the "| default" suffix.
			typeName := strings.Split(t, " ")[0]
			if !validSimpleSchemaTypes[typeName] {
				v.addError(fieldPath, fmt.Sprintf("invalid type %q", typeName))
			}
		case map[string]interface{}:
			v.validateSchemaFields(fieldPath, t)
		default:
			v.addError(fieldPath, fmt.Sprintf("unexpected value type: %T", val))
		}
	}
}

// validateResources checks the spec.resources list.
func (v *validator) validateResources() {
	spec, _ := v.rgdMap["spec"].(map[string]interface{})
	if spec == nil {
		return
	}

	resourcesRaw, ok := spec["resources"]
	if !ok {
		v.addError("spec.resources", "required field is missing")
		return
	}

	resources, ok := resourcesRaw.([]interface{})
	if !ok {
		v.addError("spec.resources", "must be a list")
		return
	}

	if len(resources) == 0 {
		v.addWarning("spec.resources", "resource list is empty")
		return
	}

	seenIDs := make(map[string]bool)

	for i, res := range resources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			v.addError(fmt.Sprintf("spec.resources[%d]", i), "resource must be a map")
			continue
		}

		id, _ := resMap["id"].(string)
		if id == "" {
			v.addError(fmt.Sprintf("spec.resources[%d].id", i), "required field is missing")
		} else if seenIDs[id] {
			v.addError(fmt.Sprintf("spec.resources[%d].id", i), fmt.Sprintf("duplicate resource ID: %s", id))
		} else {
			seenIDs[id] = true
		}

		// Validate template has required GVK fields.
		tmpl, _ := resMap["template"].(map[string]interface{})
		if tmpl == nil {
			v.addError(fmt.Sprintf("spec.resources[%d].template", i), "required field is missing")
		} else {
			if tmplAPIVersion, _ := tmpl["apiVersion"].(string); tmplAPIVersion == "" {
				v.addWarning(fmt.Sprintf("spec.resources[%d].template.apiVersion", i), "apiVersion is missing")
			}

			if tmplKind, _ := tmpl["kind"].(string); tmplKind == "" {
				v.addError(fmt.Sprintf("spec.resources[%d].template.kind", i), "required field is missing")
			}
		}

		// Warn about missing readyWhen.
		if _, hasReady := resMap["readyWhen"]; !hasReady {
			v.addWarning(fmt.Sprintf("spec.resources[%d]", i),
				fmt.Sprintf("resource %q has no readyWhen conditions", id))
		}
	}
}

// celRefRegex matches CEL expressions like ${schema.spec.foo} or ${resourceId.status.bar}.
var celRefRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

// validateCELReferences validates CEL expression references against the schema and resource IDs.
func (v *validator) validateCELReferences() {
	spec, _ := v.rgdMap["spec"].(map[string]interface{})
	if spec == nil {
		return
	}

	// Collect declared schema fields.
	schemaFields := collectSchemaFields(spec)

	// Collect declared resource IDs.
	resourceIDs := collectResourceIDs(spec)

	// Walk all resources and validate CEL references.
	resources, _ := spec["resources"].([]interface{})

	for i, res := range resources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := resMap["id"].(string)
		prefix := fmt.Sprintf("spec.resources[%d]", i)

		// Walk all string values to find CEL expressions.
		v.walkCELExpressions(prefix, resMap, schemaFields, resourceIDs, id)
	}

	// Also check schema status fields for CEL references.
	if schema, ok := spec["schema"].(map[string]interface{}); ok {
		if status, ok := schema["status"].(map[string]interface{}); ok {
			v.walkCELExpressions("spec.schema.status", status, schemaFields, resourceIDs, "")
		}
	}
}

// walkCELExpressions recursively finds and validates CEL expressions in a map.
func (v *validator) walkCELExpressions(
	prefix string,
	m map[string]interface{},
	schemaFields map[string]bool,
	resourceIDs map[string]bool,
	currentResourceID string,
) {
	for key, val := range m {
		fieldPath := prefix + "." + key

		switch t := val.(type) {
		case string:
			v.validateCELString(fieldPath, t, schemaFields, resourceIDs, currentResourceID)
		case map[string]interface{}:
			v.walkCELExpressions(fieldPath, t, schemaFields, resourceIDs, currentResourceID)
		case []interface{}:
			for i, item := range t {
				switch it := item.(type) {
				case string:
					v.validateCELString(fmt.Sprintf("%s[%d]", fieldPath, i), it, schemaFields, resourceIDs, currentResourceID)
				case map[string]interface{}:
					v.walkCELExpressions(fmt.Sprintf("%s[%d]", fieldPath, i), it, schemaFields, resourceIDs, currentResourceID)
				}
			}
		}
	}
}

// validateCELString checks a single string for CEL references.
func (v *validator) validateCELString(
	fieldPath, value string,
	schemaFields map[string]bool,
	resourceIDs map[string]bool,
	currentResourceID string,
) {
	matches := celRefRegex.FindAllStringSubmatch(value, -1)

	for _, match := range matches {
		expr := match[1]

		switch {
		case strings.HasPrefix(expr, "schema.spec."):
			// Validate against declared schema fields.
			field := strings.TrimPrefix(expr, "schema.spec.")
			root := strings.Split(field, ".")[0]
			if !schemaFields[root] {
				v.addError(fieldPath, fmt.Sprintf("unknown schema field: %s", root))
			}
		case strings.HasPrefix(expr, "self."):
			// self references are always valid in readyWhen.
			_ = currentResourceID
		default:
			// Should be a resource ID reference like resourceId.status.foo.
			parts := strings.SplitN(expr, ".", 2)
			if len(parts) >= 1 && !resourceIDs[parts[0]] && parts[0] != "schema" {
				v.addError(fieldPath, fmt.Sprintf("unknown resource ID: %s", parts[0]))
			}
		}
	}
}

// collectSchemaFields returns all top-level schema spec field names.
func collectSchemaFields(spec map[string]interface{}) map[string]bool {
	fields := make(map[string]bool)

	schema, _ := spec["schema"].(map[string]interface{})
	if schema == nil {
		return fields
	}

	specFields, _ := schema["spec"].(map[string]interface{})

	for name := range specFields {
		fields[name] = true
	}

	return fields
}

// collectResourceIDs returns all declared resource IDs.
func collectResourceIDs(spec map[string]interface{}) map[string]bool {
	ids := make(map[string]bool)

	resources, _ := spec["resources"].([]interface{})

	for _, res := range resources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		if id, ok := resMap["id"].(string); ok && id != "" {
			ids[id] = true
		}
	}

	return ids
}

// validateDependencyGraph checks for cycles and validates dependsOn references.
func (v *validator) validateDependencyGraph() {
	spec, _ := v.rgdMap["spec"].(map[string]interface{})
	if spec == nil {
		return
	}

	resources, _ := spec["resources"].([]interface{})
	if len(resources) == 0 {
		return
	}

	resourceIDs := collectResourceIDs(spec)

	// Build adjacency list.
	adj := make(map[string][]string)

	for i, res := range resources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := resMap["id"].(string)
		if id == "" {
			continue
		}

		deps, _ := resMap["dependsOn"].([]interface{})

		for _, dep := range deps {
			depStr, ok := dep.(string)
			if !ok {
				continue
			}

			if !resourceIDs[depStr] {
				v.addError(fmt.Sprintf("spec.resources[%d].dependsOn", i),
					fmt.Sprintf("depends on unknown resource ID: %s", depStr))
			}

			adj[id] = append(adj[id], depStr)
		}
	}

	// Detect cycles using DFS.
	if cycle := detectCycle(adj); len(cycle) > 0 {
		v.addError("spec.resources", fmt.Sprintf("dependency cycle detected: %s", strings.Join(cycle, " -> ")))
	}
}

// detectCycle finds a cycle in a directed graph using DFS.
func detectCycle(adj map[string][]string) []string {
	const (
		white = 0 // unvisited
		gray  = 1 // in progress
		black = 2 // done
	)

	color := make(map[string]int)
	parent := make(map[string]string)

	// Sort nodes for deterministic output.
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}

	sort.Strings(nodes)

	var dfs func(node string) []string

	dfs = func(node string) []string {
		color[node] = gray

		neighbors := adj[node]
		sort.Strings(neighbors)

		for _, neighbor := range neighbors {
			if color[neighbor] == gray {
				// Found a cycle — reconstruct path.
				cycle := []string{neighbor, node}

				for cur := node; cur != neighbor; {
					cur = parent[cur]
					if cur == neighbor {
						break
					}

					cycle = append(cycle, cur)
				}

				// Reverse to get the correct order.
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}

				cycle = append(cycle, neighbor) // close the cycle

				return cycle
			}

			if color[neighbor] == white {
				parent[neighbor] = node

				if cycle := dfs(neighbor); len(cycle) > 0 {
					return cycle
				}
			}
		}

		color[node] = black

		return nil
	}

	for _, n := range nodes {
		if color[n] == white {
			if cycle := dfs(n); len(cycle) > 0 {
				return cycle
			}
		}
	}

	return nil
}

// FormatValidationResult returns a human-readable string of all findings.
func FormatValidationResult(result *ValidationResult) string {
	if len(result.Findings) == 0 {
		return "Validation passed: no issues found."
	}

	var sb strings.Builder

	errors := result.Errors()
	warnings := result.Warnings()

	if len(errors) > 0 {
		_, _ = fmt.Fprintf(&sb, "Errors (%d):\n", len(errors))

		for _, f := range errors {
			_, _ = fmt.Fprintf(&sb, "  - %s: %s\n", f.Field, f.Message)
		}
	}

	if len(warnings) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}

		_, _ = fmt.Fprintf(&sb, "Warnings (%d):\n", len(warnings))

		for _, f := range warnings {
			_, _ = fmt.Fprintf(&sb, "  - %s: %s\n", f.Field, f.Message)
		}
	}

	return sb.String()
}
