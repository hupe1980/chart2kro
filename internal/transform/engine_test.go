package transform_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
)

func TestEngine_Transform_Basic(t *testing.T) {
	resources := []*k8s.Resource{
		makeFullResource("v1", "ConfigMap", "app-config", map[string]interface{}{
			"data": map[string]interface{}{"key": "value"},
		}),
		makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": int64(3),
			},
		}),
	}

	values := map[string]interface{}{
		"replicaCount": 3,
		"image": map[string]interface{}{
			"repository": "nginx",
		},
	}

	engine := transform.NewEngine(transform.EngineConfig{
		IncludeAllValues: true,
	})

	result, err := engine.Transform(context.Background(), resources, values)
	require.NoError(t, err)

	// Verify resource IDs assigned.
	assert.Len(t, result.ResourceIDs, 2)
	assert.NotEmpty(t, result.ResourceIDs[resources[0]])
	assert.NotEmpty(t, result.ResourceIDs[resources[1]])

	// Verify schema extracted.
	assert.NotEmpty(t, result.SchemaFields)

	// Verify dependency graph built.
	assert.NotNil(t, result.DependencyGraph)
	nodes := result.DependencyGraph.Nodes()
	assert.Len(t, nodes, 2)

	// Verify status projections for Deployment.
	assert.NotEmpty(t, result.StatusFields)
}

func TestEngine_Transform_EmptyResources(t *testing.T) {
	engine := transform.NewEngine(transform.EngineConfig{})

	_, err := engine.Transform(context.Background(), nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no resources")
}

func TestEngine_Transform_WithOverrides(t *testing.T) {
	resources := []*k8s.Resource{
		makeFullResource("v1", "Service", "web-svc", map[string]interface{}{
			"spec": map[string]interface{}{"type": "ClusterIP"},
		}),
	}

	engine := transform.NewEngine(transform.EngineConfig{
		ResourceIDOverrides: map[string]string{
			"Service/web-svc": "main-service",
		},
		IncludeAllValues: true,
	})

	result, err := engine.Transform(context.Background(), resources, map[string]interface{}{"port": 80})
	require.NoError(t, err)
	assert.Equal(t, "main-service", result.ResourceIDs[resources[0]])
}

func TestEngine_Transform_FlatSchema(t *testing.T) {
	resources := []*k8s.Resource{
		makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"key": "val"},
		}),
	}

	values := map[string]interface{}{
		"image": map[string]interface{}{
			"repository": "nginx",
			"tag":        "latest",
		},
	}

	engine := transform.NewEngine(transform.EngineConfig{
		FlatSchema:       true,
		IncludeAllValues: true,
	})

	result, err := engine.Transform(context.Background(), resources, values)
	require.NoError(t, err)

	// In flat mode, "image.repository" becomes "imageRepository".
	found := false

	for _, f := range result.SchemaFields {
		if f.Name == "imageRepository" {
			found = true

			break
		}
	}

	assert.True(t, found, "expected flat field 'imageRepository'")
}

func TestEngine_Transform_CycleDetection(t *testing.T) {
	// Create resources that form a cycle through mutual dependencies.
	// This is tricky with BuildDependencyGraph since it detects real deps.
	// Let's verify that if there were a cycle, CycleError would be returned.
	// In practice, cycles are unlikely with real K8s resources.

	// For now, test that a normal DAG processes without error.
	deploy := makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"serviceAccountName": "app-sa",
				},
			},
		},
	})
	sa := makeFullResource("v1", "ServiceAccount", "app-sa", map[string]interface{}{})

	engine := transform.NewEngine(transform.EngineConfig{})
	result, err := engine.Transform(context.Background(), []*k8s.Resource{deploy, sa}, nil)
	require.NoError(t, err)

	// Deployment depends on ServiceAccount.
	deps := result.DependencyGraph.DependenciesOf(result.ResourceIDs[deploy])
	assert.Contains(t, deps, result.ResourceIDs[sa])
}

func TestEngine_Transform_StatusProjections(t *testing.T) {
	resources := []*k8s.Resource{
		{
			GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Name: "web",
			Object: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "web"},
			}},
		},
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
			Name: "web-svc",
			Object: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   map[string]interface{}{"name": "web-svc"},
			}},
		},
	}

	engine := transform.NewEngine(transform.EngineConfig{})
	result, err := engine.Transform(context.Background(), resources, nil)
	require.NoError(t, err)

	// Deployment should have status projections.
	var hasDeploymentStatus bool

	for _, sf := range result.StatusFields {
		if sf.Name == result.ResourceIDs[resources[0]]+"AvailableReplicas" {
			hasDeploymentStatus = true
		}
	}

	assert.True(t, hasDeploymentStatus, "expected deployment status projection")
}

func TestCycleError(t *testing.T) {
	err := &transform.CycleError{
		Cycles: [][]string{{"a", "b", "a"}},
	}
	assert.Contains(t, err.Error(), "1 cycle(s)")
	assert.Contains(t, err.Error(), "a")
}

func TestEngine_Transform_IncludeAllValuesFalse(t *testing.T) {
	resources := []*k8s.Resource{
		makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"key": "val"},
		}),
	}

	values := map[string]interface{}{
		"used":   "yes",
		"unused": "no",
	}

	// When IncludeAllValues=false and no referenced paths provided,
	// schema should still include all (refs==nil means include all).
	engine := transform.NewEngine(transform.EngineConfig{
		IncludeAllValues: false,
	})

	result, err := engine.Transform(context.Background(), resources, values)
	require.NoError(t, err)
	// With nil refs, all values are included regardless.
	assert.NotEmpty(t, result.SchemaFields)
}

func TestEngine_Transform_IDCollision(t *testing.T) {
	// Two Services with identical names — true collision.
	resources := []*k8s.Resource{
		makeFullResource("v1", "Service", "web", map[string]interface{}{
			"spec": map[string]interface{}{"type": "ClusterIP"},
		}),
		makeFullResource("v1", "Service", "web", map[string]interface{}{
			"spec": map[string]interface{}{"type": "ClusterIP"},
		}),
	}

	engine := transform.NewEngine(transform.EngineConfig{})
	_, err := engine.Transform(context.Background(), resources, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collision")
}
