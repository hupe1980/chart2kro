// Package docs generates human-readable API documentation from a
// KRO ResourceGraphDefinition. It supports Markdown, HTML, and
// AsciiDoc output formats, with optional example YAML generation.
package docs

import (
	"fmt"
	"sort"
	"strings"
)

// FieldInfo describes a single API field.
type FieldInfo struct {
	// Name is the field name (e.g., "replicaCount").
	Name string
	// Path is the dot-separated path (e.g., "spec.image.repository").
	Path string
	// Type is the schema type (string, integer, number, boolean, object, array).
	Type string
	// Default is the default value, if any.
	Default string
	// Children are nested fields for object types.
	Children []FieldInfo
}

// StatusInfo describes a status field.
type StatusInfo struct {
	// Name is the status field name.
	Name string
	// Expression is the CEL expression that produces the value.
	Expression string
}

// ResourceInfo describes a resource within the RGD.
type ResourceInfo struct {
	// ID is the resource identifier (e.g., "deployment").
	ID string
	// Kind is the Kubernetes kind (e.g., "Deployment").
	Kind string
	// APIVersion is the Kubernetes apiVersion.
	APIVersion string
	// DependsOn lists upstream resource IDs.
	DependsOn []string
	// ReadyWhen lists readiness CEL conditions.
	ReadyWhen []string
}

// DocModel is the structured data model for documentation generation.
type DocModel struct {
	// Title overrides the document title.
	Title string
	// Name is the RGD metadata.name.
	Name string
	// APIVersion is the schema apiVersion (e.g., "simple.kro.run/v1alpha1").
	APIVersion string
	// Kind is the schema kind (e.g., "Simple").
	Kind string
	// SpecFields are the top-level spec fields.
	SpecFields []FieldInfo
	// StatusFields are the status projections.
	StatusFields []StatusInfo
	// Resources are the managed resources.
	Resources []ResourceInfo
	// IncludeExamples controls whether an example YAML section is appended.
	IncludeExamples bool
}

// ParseRGDMap extracts a DocModel from a parsed RGD map.
func ParseRGDMap(rgdMap map[string]interface{}) (*DocModel, error) {
	model := &DocModel{}

	// metadata.name
	meta, _ := rgdMap["metadata"].(map[string]interface{})
	model.Name, _ = meta["name"].(string)

	// spec.schema
	spec, _ := rgdMap["spec"].(map[string]interface{})

	schemaMap, _ := spec["schema"].(map[string]interface{})
	if schemaMap == nil {
		return nil, fmt.Errorf("RGD missing spec.schema")
	}

	model.APIVersion, _ = schemaMap["apiVersion"].(string)
	model.Kind, _ = schemaMap["kind"].(string)

	// Parse spec fields.
	specMap, _ := schemaMap["spec"].(map[string]interface{})
	model.SpecFields = parseFields(specMap, "spec")

	// Parse status fields.
	statusMap, _ := schemaMap["status"].(map[string]interface{})
	for name, expr := range statusMap {
		exprStr, _ := expr.(string)
		model.StatusFields = append(model.StatusFields, StatusInfo{
			Name:       name,
			Expression: exprStr,
		})
	}

	sort.Slice(model.StatusFields, func(i, j int) bool {
		return model.StatusFields[i].Name < model.StatusFields[j].Name
	})

	// Parse resources.
	resourcesList, _ := spec["resources"].([]interface{})

	for _, r := range resourcesList {
		rMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		ri := ResourceInfo{}
		ri.ID, _ = rMap["id"].(string)

		if tpl, ok := rMap["template"].(map[string]interface{}); ok {
			ri.Kind, _ = tpl["kind"].(string)
			ri.APIVersion, _ = tpl["apiVersion"].(string)
		}

		if deps, ok := rMap["dependsOn"].([]interface{}); ok {
			for _, d := range deps {
				if ds, ok := d.(string); ok {
					ri.DependsOn = append(ri.DependsOn, ds)
				}
			}
		}

		if ready, ok := rMap["readyWhen"].([]interface{}); ok {
			for _, rw := range ready {
				if rws, ok := rw.(string); ok {
					ri.ReadyWhen = append(ri.ReadyWhen, rws)
				}
			}
		}

		model.Resources = append(model.Resources, ri)
	}

	return model, nil
}

// parseFields recursively parses SimpleSchema fields.
func parseFields(m map[string]interface{}, parentPath string) []FieldInfo {
	var fields []FieldInfo

	// Sort keys for stable output.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, name := range keys {
		val := m[name]
		fi := FieldInfo{
			Name: name,
			Path: parentPath + "." + name,
		}

		switch v := val.(type) {
		case string:
			fi.Type, fi.Default = parseSimpleSchemaString(v)
		case map[string]interface{}:
			fi.Type = "object"
			fi.Children = parseFields(v, fi.Path)
		default:
			fi.Type = fmt.Sprintf("%T", val)
		}

		fields = append(fields, fi)
	}

	return fields
}

// parseSimpleSchemaString parses a SimpleSchema value into type and default.
func parseSimpleSchemaString(s string) (typ, def string) {
	parts := strings.SplitN(s, "|", 2)
	typ = strings.TrimSpace(parts[0])

	if len(parts) == 2 {
		rest := strings.TrimSpace(parts[1])
		if strings.HasPrefix(rest, "default=") {
			def = strings.TrimPrefix(rest, "default=")
		}
	}

	return typ, def
}

// GenerateExampleYAML creates an example custom resource YAML from the model.
func GenerateExampleYAML(model *DocModel) string {
	var b strings.Builder

	b.WriteString("apiVersion: ")
	b.WriteString(model.APIVersion)
	b.WriteString("\nkind: ")
	b.WriteString(model.Kind)
	b.WriteString("\nmetadata:\n  name: example\nspec:\n")

	writeExampleFields(&b, model.SpecFields, 2)

	return b.String()
}

func writeExampleFields(b *strings.Builder, fields []FieldInfo, indent int) {
	prefix := strings.Repeat(" ", indent)

	for _, f := range fields {
		if len(f.Children) > 0 {
			b.WriteString(prefix)
			b.WriteString(f.Name)
			b.WriteString(":\n")

			writeExampleFields(b, f.Children, indent+2)
		} else {
			b.WriteString(prefix)
			b.WriteString(f.Name)
			b.WriteString(": ")

			if f.Default != "" {
				b.WriteString(f.Default)
			} else {
				b.WriteString(exampleValue(f.Type))
			}

			b.WriteString("\n")
		}
	}
}

func exampleValue(typ string) string {
	switch typ {
	case "integer":
		return "1"
	case "number":
		return "1.0"
	case "boolean":
		return "true"
	case "array":
		return "[]"
	default:
		return `""`
	}
}
