package plan

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
)

func TestBuildPlan_Basic(t *testing.T) {
	deployRes := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "my-deploy",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
			},
		},
	}

	result := &transform.Result{
		SchemaFields: []*transform.SchemaField{
			{
				Name:    "replicas",
				Type:    "integer",
				Default: "1",
				Path:    "replicas",
			},
		},
		Resources:   []*k8s.Resource{deployRes},
		ResourceIDs: map[*k8s.Resource]string{deployRes: "deployment"},
		StatusFields: []transform.StatusField{
			{Name: "ready", CELExpression: ".deployment.status.readyReplicas"},
		},
	}

	rgd := map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata": map[string]interface{}{
			"name": "test-rgd",
		},
	}

	plan := BuildPlan(result, rgd)
	assert.Equal(t, "test-rgd", plan.Name)
	require.Len(t, plan.SchemaFields, 1)
	assert.Equal(t, "replicas", plan.SchemaFields[0].Name)
	assert.Equal(t, "integer", plan.SchemaFields[0].Type)
	require.Len(t, plan.Resources, 1)
	assert.Equal(t, "deployment", plan.Resources[0].ID)
	assert.Equal(t, "Deployment", plan.Resources[0].Kind)
	require.Len(t, plan.StatusFields, 1)
	assert.Equal(t, "ready", plan.StatusFields[0].Name)
}

func TestBuildPlan_NestedSchemaFields(t *testing.T) {
	result := &transform.Result{
		SchemaFields: []*transform.SchemaField{
			{
				Name: "server",
				Type: "object",
				Children: []*transform.SchemaField{
					{Name: "port", Type: "integer", Default: "8080"},
					{Name: "host", Type: "string", Default: "localhost"},
				},
			},
		},
		ResourceIDs: map[*k8s.Resource]string{},
	}

	rgd := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "nested"},
	}

	plan := BuildPlan(result, rgd)
	require.Len(t, plan.SchemaFields, 3)
	assert.Equal(t, "server", plan.SchemaFields[0].Name)
	assert.Equal(t, "server.port", plan.SchemaFields[1].Name)
	assert.Equal(t, "server.host", plan.SchemaFields[2].Name)
}

func TestBuildPlan_FallbackToRGDMap(t *testing.T) {
	result := &transform.Result{
		ResourceIDs: map[*k8s.Resource]string{},
	}

	rgd := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "fallback"},
		"spec": map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "ingress",
					"template": map[string]interface{}{"kind": "Ingress"},
					"includeWhen": map[string]interface{}{
						"fieldPath": "spec.enableIngress",
					},
				},
			},
		},
	}

	plan := BuildPlan(result, rgd)
	require.Len(t, plan.Resources, 1)
	assert.Equal(t, "ingress", plan.Resources[0].ID)
	assert.Equal(t, "Ingress", plan.Resources[0].Kind)
	assert.True(t, plan.Resources[0].Conditional)
}

func TestApplyEvolution(t *testing.T) {
	p := &Result{
		SchemaFields: []SchemaField{
			{Name: "replicas", Type: "integer"},
		},
		Resources: []Resource{
			{ID: "deployment", Kind: "Deployment"},
		},
	}

	evolution := &EvolutionResult{
		SchemaChanges: []SchemaChange{
			{Type: ChangeAdded, Field: "version", Breaking: false},
			{Type: ChangeRemoved, Field: "port", Breaking: true},
		},
		ResourceChanges: []ResourceChange{
			{Type: ChangeAdded, ID: "service"},
		},
	}

	ApplyEvolution(p, evolution)
	assert.True(t, p.HasBreakingChanges)
	assert.Equal(t, 1, p.BreakingChangeCount)
	require.NotNil(t, p.Evolution)
	assert.Len(t, p.Evolution.SchemaChanges, 2)
}

func TestFormatPlan_Basic(t *testing.T) {
	p := &Result{
		Name: "test-rgd",
		SchemaFields: []SchemaField{
			{Name: "replicas", Type: "integer", Default: "1"},
		},
		Resources: []Resource{
			{ID: "deployment", Kind: "Deployment"},
		},
	}

	var buf bytes.Buffer
	FormatPlan(&buf, p)
	out := buf.String()
	assert.Contains(t, out, "test-rgd")
	assert.Contains(t, out, "replicas")
	assert.Contains(t, out, "deployment")
	assert.Contains(t, out, "Summary: 1 schema fields, 1 resources, 0 status projections")
}

func TestFormatPlanJSON(t *testing.T) {
	p := &Result{
		Name: "test",
		SchemaFields: []SchemaField{
			{Name: "n", Type: "string"},
		},
	}

	var buf bytes.Buffer
	err := FormatPlanJSON(&buf, p)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "test", parsed["name"])
}

func TestFormatPlanCompact(t *testing.T) {
	p := &Result{
		Name: "test",
		SchemaFields: []SchemaField{
			{Name: "a", Type: "string"},
			{Name: "b", Type: "integer"},
		},
		Resources: []Resource{
			{ID: "r1", Kind: "Deployment"},
		},
		StatusFields: []StatusField{
			{Name: "ready"},
		},
	}

	var buf bytes.Buffer
	FormatPlanCompact(&buf, p)
	out := buf.String()
	assert.Contains(t, out, "test")
	assert.Contains(t, out, "2 schema fields")
	assert.Contains(t, out, "1 resources")
}

func TestFormatPlanCompact_WithEvolution(t *testing.T) {
	p := &Result{
		Name:                "test",
		HasBreakingChanges:  true,
		BreakingChangeCount: 1,
		Evolution: &EvolutionResult{
			SchemaChanges: []SchemaChange{
				{Type: ChangeRemoved, Breaking: true},
			},
		},
	}

	var buf bytes.Buffer
	FormatPlanCompact(&buf, p)
	out := buf.String()
	assert.Contains(t, out, "WARNING")
}

func TestBuildPlan_ResourcesFromMapWithDependsOn(t *testing.T) {
	result := &transform.Result{
		ResourceIDs: map[*k8s.Resource]string{},
	}

	rgd := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "deps-test"},
		"spec": map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "service",
					"template": map[string]interface{}{"kind": "Service"},
					"dependsOn": []interface{}{
						"deployment",
						"configMap",
					},
				},
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"kind": "Deployment"},
				},
			},
		},
	}

	p := BuildPlan(result, rgd)
	require.Len(t, p.Resources, 2)

	// Find the service resource.
	var svc Resource
	for _, r := range p.Resources {
		if r.ID == "service" {
			svc = r
			break
		}
	}

	assert.Equal(t, "Service", svc.Kind)
	assert.Equal(t, []string{"deployment", "configMap"}, svc.DependsOn)
}

func TestBuildPlan_EmptyResult(t *testing.T) {
	result := &transform.Result{
		ResourceIDs: map[*k8s.Resource]string{},
	}

	rgd := map[string]interface{}{}

	p := BuildPlan(result, rgd)
	assert.Equal(t, "", p.Name)
	assert.Empty(t, p.SchemaFields)
	assert.Empty(t, p.Resources)
	assert.Empty(t, p.StatusFields)
}

func TestFormatPlanJSON_Structure(t *testing.T) {
	p := &Result{
		Name: "json-test",
		SchemaFields: []SchemaField{
			{Name: "replicas", Type: "integer", Default: "3", Required: false, Path: "replicas"},
		},
		Resources: []Resource{
			{ID: "deployment", Kind: "Deployment", APIVersion: "apps/v1"},
		},
		StatusFields: []StatusField{
			{Name: "ready", Expression: ".deployment.status.readyReplicas"},
		},
		HasBreakingChanges:  true,
		BreakingChangeCount: 2,
	}

	var buf bytes.Buffer
	err := FormatPlanJSON(&buf, p)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "json-test", parsed["name"])
	assert.True(t, parsed["hasBreakingChanges"].(bool))
	assert.Equal(t, float64(2), parsed["breakingChangeCount"])

	fields := parsed["schemaFields"].([]interface{})
	require.Len(t, fields, 1)
	field := fields[0].(map[string]interface{})
	assert.Equal(t, "replicas", field["name"])
	assert.Equal(t, "integer", field["type"])

	resources := parsed["resources"].([]interface{})
	require.Len(t, resources, 1)
	res := resources[0].(map[string]interface{})
	assert.Equal(t, "deployment", res["id"])

	statuses := parsed["statusFields"].([]interface{})
	require.Len(t, statuses, 1)
}

func TestFormatPlan_WithEvolution(t *testing.T) {
	p := &Result{
		Name: "evo-test",
		SchemaFields: []SchemaField{
			{Name: "port", Type: "integer", Default: "80"},
		},
		Resources: []Resource{
			{ID: "svc", Kind: "Service"},
		},
		HasBreakingChanges:  true,
		BreakingChangeCount: 1,
		Evolution: &EvolutionResult{
			SchemaChanges: []SchemaChange{
				{Type: ChangeRemoved, Field: "old-field", Details: "removed", Impact: "breaking", Breaking: true},
			},
		},
	}

	var buf bytes.Buffer
	FormatPlan(&buf, p)
	out := buf.String()
	assert.Contains(t, out, "evo-test")
	assert.Contains(t, out, "Schema Changes:")
	assert.Contains(t, out, "Breaking changes: 1")
	assert.Contains(t, out, "WARNING")
}
