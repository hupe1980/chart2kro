package docs_test

import (
	"testing"

	"github.com/hupe1980/chart2kro/internal/docs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleRGDMap() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata": map[string]interface{}{
			"name": "my-app",
		},
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"apiVersion": "myapp.kro.run/v1alpha1",
				"kind":       "MyApp",
				"spec": map[string]interface{}{
					"name":         "string",
					"replicaCount": `integer | default=3`,
					"image": map[string]interface{}{
						"repository": `string | default="nginx"`,
						"tag":        `string | default="latest"`,
					},
				},
				"status": map[string]interface{}{
					"ready":    "${deployment.status.availableReplicas}",
					"endpoint": "${service.status.loadBalancer.ingress[0].hostname}",
				},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id": "deployment",
					"template": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
					},
					"readyWhen": []interface{}{
						"${deployment.status.readyReplicas == deployment.status.replicas}",
					},
				},
				map[string]interface{}{
					"id": "service",
					"template": map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Service",
					},
					"dependsOn": []interface{}{"deployment"},
				},
			},
		},
	}
}

func TestParseRGDMap(t *testing.T) {
	model, err := docs.ParseRGDMap(sampleRGDMap())
	require.NoError(t, err)

	assert.Equal(t, "my-app", model.Name)
	assert.Equal(t, "myapp.kro.run/v1alpha1", model.APIVersion)
	assert.Equal(t, "MyApp", model.Kind)

	// Spec fields (sorted: image, name, replicaCount).
	require.Len(t, model.SpecFields, 3)
	assert.Equal(t, "image", model.SpecFields[0].Name)
	assert.Equal(t, "object", model.SpecFields[0].Type)
	assert.Len(t, model.SpecFields[0].Children, 2)
	assert.Equal(t, "name", model.SpecFields[1].Name)
	assert.Equal(t, "string", model.SpecFields[1].Type)
	assert.Equal(t, "replicaCount", model.SpecFields[2].Name)
	assert.Equal(t, "integer", model.SpecFields[2].Type)
	assert.Equal(t, "3", model.SpecFields[2].Default)

	// Nested image fields.
	assert.Equal(t, "repository", model.SpecFields[0].Children[0].Name)
	assert.Equal(t, `"nginx"`, model.SpecFields[0].Children[0].Default)
	assert.Equal(t, "spec.image.repository", model.SpecFields[0].Children[0].Path)

	// Status fields (sorted: endpoint, ready).
	require.Len(t, model.StatusFields, 2)
	assert.Equal(t, "endpoint", model.StatusFields[0].Name)
	assert.Equal(t, "ready", model.StatusFields[1].Name)
	assert.Contains(t, model.StatusFields[1].Expression, "availableReplicas")

	// Resources.
	require.Len(t, model.Resources, 2)
	assert.Equal(t, "deployment", model.Resources[0].ID)
	assert.Equal(t, "Deployment", model.Resources[0].Kind)
	assert.Len(t, model.Resources[0].ReadyWhen, 1)
	assert.Equal(t, "service", model.Resources[1].ID)
	assert.Equal(t, []string{"deployment"}, model.Resources[1].DependsOn)
}

func TestParseRGDMap_MissingSchema(t *testing.T) {
	rgd := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "x"},
		"spec":     map[string]interface{}{},
	}

	_, err := docs.ParseRGDMap(rgd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "spec.schema")
}

func TestParseRGDMap_EmptySpec(t *testing.T) {
	rgd := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "x"},
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"apiVersion": "v1alpha1",
				"kind":       "Empty",
			},
		},
	}

	model, err := docs.ParseRGDMap(rgd)
	require.NoError(t, err)
	assert.Empty(t, model.SpecFields)
	assert.Empty(t, model.StatusFields)
	assert.Empty(t, model.Resources)
}

func TestGenerateExampleYAML(t *testing.T) {
	model := &docs.DocModel{
		APIVersion: "myapp.kro.run/v1alpha1",
		Kind:       "MyApp",
		SpecFields: []docs.FieldInfo{
			{Name: "name", Type: "string"},
			{Name: "replicas", Type: "integer", Default: "3"},
			{
				Name: "image",
				Type: "object",
				Children: []docs.FieldInfo{
					{Name: "repo", Type: "string", Default: `"nginx"`},
				},
			},
		},
	}

	yaml := docs.GenerateExampleYAML(model)
	assert.Contains(t, yaml, "apiVersion: myapp.kro.run/v1alpha1")
	assert.Contains(t, yaml, "kind: MyApp")
	assert.Contains(t, yaml, "name: example")
	assert.Contains(t, yaml, `  name: ""`)
	assert.Contains(t, yaml, "  replicas: 3")
	assert.Contains(t, yaml, "  image:")
	assert.Contains(t, yaml, `    repo: "nginx"`)
}

func TestGenerateExampleYAML_EmptyFields(t *testing.T) {
	model := &docs.DocModel{
		APIVersion: "v1",
		Kind:       "Test",
	}

	yaml := docs.GenerateExampleYAML(model)
	assert.Contains(t, yaml, "apiVersion: v1")
	assert.Contains(t, yaml, "kind: Test")
	assert.Contains(t, yaml, "spec:")
}

func TestExampleValueTypes(t *testing.T) {
	model := &docs.DocModel{
		APIVersion: "v1",
		Kind:       "T",
		SpecFields: []docs.FieldInfo{
			{Name: "a", Type: "integer"},
			{Name: "b", Type: "number"},
			{Name: "c", Type: "boolean"},
			{Name: "d", Type: "array"},
			{Name: "e", Type: "string"},
		},
	}

	yaml := docs.GenerateExampleYAML(model)
	assert.Contains(t, yaml, "a: 1")
	assert.Contains(t, yaml, "b: 1.0")
	assert.Contains(t, yaml, "c: true")
	assert.Contains(t, yaml, "d: []")
	assert.Contains(t, yaml, `e: ""`)
}
