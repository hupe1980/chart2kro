package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validRGD() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata":   map[string]interface{}{"name": "test"},
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"apiVersion": "v1alpha1",
				"kind":       "TestApp",
				"spec":       map[string]interface{}{"replicaCount": "integer"},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":        "deployment",
					"readyWhen": []interface{}{"${self.status.availableReplicas == self.status.replicas}"},
					"template": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"spec":       map[string]interface{}{"replicas": "${schema.spec.replicaCount}"},
					},
				},
			},
		},
	}
}

func TestValidateRGD_ValidInput(t *testing.T) {
	result := ValidateRGD(validRGD())
	assert.False(t, result.HasErrors(), "expected no errors: %v", result.Errors())
}

func TestValidateRGD_MissingAPIVersion(t *testing.T) {
	rgd := validRGD()
	delete(rgd, "apiVersion")

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())

	found := false
	for _, f := range result.Errors() {
		if f.Field == "apiVersion" {
			found = true
		}
	}

	assert.True(t, found, "expected error for apiVersion")
}

func TestValidateRGD_MissingKind(t *testing.T) {
	rgd := validRGD()
	delete(rgd, "kind")

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_WrongKind(t *testing.T) {
	rgd := validRGD()
	rgd["kind"] = "SomethingElse"

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_MissingMetadataName(t *testing.T) {
	rgd := validRGD()
	rgd["metadata"] = map[string]interface{}{}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_MissingSpec(t *testing.T) {
	rgd := validRGD()
	delete(rgd, "spec")

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_MissingResources(t *testing.T) {
	rgd := validRGD()
	spec := rgd["spec"].(map[string]interface{})
	delete(spec, "resources")

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_SchemaKindNotPascalCase(t *testing.T) {
	rgd := validRGD()
	schema := rgd["spec"].(map[string]interface{})["schema"].(map[string]interface{})
	schema["kind"] = "test-app"

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_InvalidSchemaFieldType(t *testing.T) {
	rgd := validRGD()
	schema := rgd["spec"].(map[string]interface{})["schema"].(map[string]interface{})
	schema["spec"] = map[string]interface{}{"count": "uint64"}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_ValidSchemaFieldTypes(t *testing.T) {
	rgd := validRGD()
	schema := rgd["spec"].(map[string]interface{})["schema"].(map[string]interface{})
	schema["spec"] = map[string]interface{}{
		"name":         "string",
		"count":        "integer",
		"enabled":      "boolean",
		"ratio":        "number",
		"config":       "object",
		"items":        "array",
		"replicaCount": "integer | default=1",
	}

	result := ValidateRGD(rgd)
	assert.False(t, result.HasErrors(), "expected no errors: %v", result.Errors())
}

func TestValidateRGD_DuplicateResourceIDs(t *testing.T) {
	rgd := validRGD()
	spec := rgd["spec"].(map[string]interface{})
	spec["resources"] = []interface{}{
		map[string]interface{}{"id": "svc", "template": map[string]interface{}{"kind": "Service"}},
		map[string]interface{}{"id": "svc", "template": map[string]interface{}{"kind": "Service"}},
	}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())

	found := false
	for _, f := range result.Errors() {
		if f.Message == "duplicate resource ID: svc" {
			found = true
		}
	}

	assert.True(t, found, "expected duplicate ID error")
}

func TestValidateRGD_MissingResourceTemplate(t *testing.T) {
	rgd := validRGD()
	spec := rgd["spec"].(map[string]interface{})
	spec["resources"] = []interface{}{
		map[string]interface{}{"id": "svc"},
	}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_MissingResourceID(t *testing.T) {
	rgd := validRGD()
	spec := rgd["spec"].(map[string]interface{})
	spec["resources"] = []interface{}{
		map[string]interface{}{"template": map[string]interface{}{"kind": "Service"}},
	}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_UnknownCELSchemaRef(t *testing.T) {
	rgd := validRGD()
	res := rgd["spec"].(map[string]interface{})["resources"].([]interface{})[0].(map[string]interface{})
	tmpl := res["template"].(map[string]interface{})
	tmpl["spec"] = map[string]interface{}{
		"replicas": "${schema.spec.nonexistent}",
	}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())

	found := false
	for _, f := range result.Errors() {
		if f.Message == "unknown schema field: nonexistent" {
			found = true
		}
	}

	assert.True(t, found, "expected unknown schema field error")
}

func TestValidateRGD_UnknownResourceIDRef(t *testing.T) {
	rgd := validRGD()
	schema := rgd["spec"].(map[string]interface{})["schema"].(map[string]interface{})
	schema["status"] = map[string]interface{}{
		"replicas": "${nonexistent.status.replicas}",
	}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())

	found := false
	for _, f := range result.Errors() {
		if f.Message == "unknown resource ID: nonexistent" {
			found = true
		}
	}

	assert.True(t, found, "expected unknown resource ID error")
}

func TestValidateRGD_SelfRefAlwaysValid(t *testing.T) {
	rgd := validRGD()
	res := rgd["spec"].(map[string]interface{})["resources"].([]interface{})[0].(map[string]interface{})
	res["readyWhen"] = []interface{}{"${self.status.ready}"}

	result := ValidateRGD(rgd)
	// Self refs should not produce errors.
	for _, f := range result.Errors() {
		assert.NotContains(t, f.Message, "self")
	}
}

func TestValidateRGD_CycleDetection(t *testing.T) {
	rgd := validRGD()
	spec := rgd["spec"].(map[string]interface{})
	spec["resources"] = []interface{}{
		map[string]interface{}{
			"id":        "a",
			"dependsOn": []interface{}{"b"},
			"template":  map[string]interface{}{"kind": "Deployment"},
		},
		map[string]interface{}{
			"id":        "b",
			"dependsOn": []interface{}{"a"},
			"template":  map[string]interface{}{"kind": "Service"},
		},
	}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())

	found := false
	for _, f := range result.Errors() {
		if f.Field == "spec.resources" && len(f.Message) > 0 {
			found = true
		}
	}

	assert.True(t, found, "expected cycle error")
}

func TestValidateRGD_DependsOnUnknownResource(t *testing.T) {
	rgd := validRGD()
	res := rgd["spec"].(map[string]interface{})["resources"].([]interface{})[0].(map[string]interface{})
	res["dependsOn"] = []interface{}{"ghost"}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_WarningMissingReadyWhen(t *testing.T) {
	rgd := validRGD()
	res := rgd["spec"].(map[string]interface{})["resources"].([]interface{})[0].(map[string]interface{})
	delete(res, "readyWhen")

	result := ValidateRGD(rgd)
	assert.True(t, result.HasWarnings())
}

func TestValidateRGD_EmptyResources(t *testing.T) {
	rgd := validRGD()
	rgd["spec"].(map[string]interface{})["resources"] = []interface{}{}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasWarnings())
}

func TestValidateRGD_ReportsAllErrors(t *testing.T) {
	// An RGD with multiple errors should report all of them.
	rgd := map[string]interface{}{
		"kind": "Wrong",
		"spec": map[string]interface{}{},
	}

	result := ValidateRGD(rgd)
	// Should have errors for: apiVersion, kind, metadata, resources.
	assert.True(t, len(result.Errors()) >= 3, "expected at least 3 errors, got %d", len(result.Errors()))
}

func TestFormatValidationResult_NoIssues(t *testing.T) {
	result := &ValidationResult{}
	output := FormatValidationResult(result)
	assert.Contains(t, output, "no issues found")
}

func TestFormatValidationResult_WithErrors(t *testing.T) {
	result := &ValidationResult{
		Findings: []ValidationFinding{
			{Severity: SeverityError, Field: "apiVersion", Message: "missing"},
			{Severity: SeverityWarning, Field: "spec.resources", Message: "empty"},
		},
	}

	output := FormatValidationResult(result)
	assert.Contains(t, output, "Errors (1)")
	assert.Contains(t, output, "Warnings (1)")
	assert.Contains(t, output, "apiVersion: missing")
}

func TestDetectCycle_NoCycle(t *testing.T) {
	adj := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {},
	}

	cycle := detectCycle(adj)
	assert.Empty(t, cycle)
}

func TestDetectCycle_WithCycle(t *testing.T) {
	adj := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}

	cycle := detectCycle(adj)
	require.NotEmpty(t, cycle)
	assert.True(t, len(cycle) >= 3, "cycle should have at least 3 elements")
}

func TestValidationSeverity_String(t *testing.T) {
	assert.Equal(t, "error", SeverityError.String())
	assert.Equal(t, "warning", SeverityWarning.String())
}

func TestValidationFinding_Error(t *testing.T) {
	f := &ValidationFinding{Severity: SeverityError, Field: "foo", Message: "bar"}
	assert.Equal(t, "[error] foo: bar", f.Error())
}

func TestValidateRGD_MissingTemplateAPIVersion(t *testing.T) {
	rgd := validRGD()
	res := rgd["spec"].(map[string]interface{})["resources"].([]interface{})[0].(map[string]interface{})
	tmpl := res["template"].(map[string]interface{})
	delete(tmpl, "apiVersion")

	result := ValidateRGD(rgd)
	assert.True(t, result.HasWarnings())

	found := false
	for _, f := range result.Warnings() {
		if f.Message == "apiVersion is missing" {
			found = true
		}
	}

	assert.True(t, found, "expected apiVersion warning")
}

func TestValidateRGD_NestedSchemaFields(t *testing.T) {
	rgd := validRGD()
	schema := rgd["spec"].(map[string]interface{})["schema"].(map[string]interface{})
	schema["spec"] = map[string]interface{}{
		"replicaCount": "integer",
		"config": map[string]interface{}{
			"debug": "boolean",
			"level": "string",
		},
	}

	result := ValidateRGD(rgd)
	assert.False(t, result.HasErrors(), "nested schema fields should be valid: %v", result.Errors())
}

func TestValidateRGD_NestedSchemaInvalidType(t *testing.T) {
	rgd := validRGD()
	schema := rgd["spec"].(map[string]interface{})["schema"].(map[string]interface{})
	schema["spec"] = map[string]interface{}{
		"config": map[string]interface{}{
			"count": "uint32",
		},
	}

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}

func TestValidateRGD_UnexpectedAPIVersion(t *testing.T) {
	rgd := validRGD()
	rgd["apiVersion"] = "custom.io/v1"

	result := ValidateRGD(rgd)
	// Should produce a warning, not an error.
	assert.False(t, result.HasErrors(), "unexpected apiVersion should be warning, not error")
	assert.True(t, result.HasWarnings())
}

func TestValidateRGD_MultipleResources(t *testing.T) {
	rgd := validRGD()
	spec := rgd["spec"].(map[string]interface{})
	spec["resources"] = []interface{}{
		map[string]interface{}{
			"id":        "deployment",
			"readyWhen": []interface{}{"${self.status.ready}"},
			"template": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"spec":       map[string]interface{}{"replicas": "${schema.spec.replicaCount}"},
			},
		},
		map[string]interface{}{
			"id":        "service",
			"readyWhen": []interface{}{"${self.status.ready}"},
			"dependsOn": []interface{}{"deployment"},
			"template": map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
			},
		},
	}

	result := ValidateRGD(rgd)
	assert.False(t, result.HasErrors(), "multi-resource RGD should be valid: %v", result.Errors())
}

func TestValidateRGD_MissingMetadata(t *testing.T) {
	rgd := validRGD()
	delete(rgd, "metadata")

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())

	found := false
	for _, f := range result.Errors() {
		if f.Field == "metadata" {
			found = true
		}
	}

	assert.True(t, found, "expected error for missing metadata")
}

func TestValidateRGD_StatusCELReferences(t *testing.T) {
	rgd := validRGD()
	schema := rgd["spec"].(map[string]interface{})["schema"].(map[string]interface{})
	schema["status"] = map[string]interface{}{
		"availableReplicas": "${deployment.status.availableReplicas}",
	}

	result := ValidateRGD(rgd)
	assert.False(t, result.HasErrors(), "valid status CEL reference should pass: %v", result.Errors())
}

func TestValidateRGD_ResourcesNotAList(t *testing.T) {
	rgd := validRGD()
	rgd["spec"].(map[string]interface{})["resources"] = "not-a-list"

	result := ValidateRGD(rgd)
	assert.True(t, result.HasErrors())
}
