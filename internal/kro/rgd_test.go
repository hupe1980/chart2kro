package kro_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/kro"
	"github.com/hupe1980/chart2kro/internal/transform"
)

func makeResource(apiVersion, kind, name string, obj map[string]interface{}) *k8s.Resource {
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)
	r := &k8s.Resource{
		GVK:  gvk,
		Name: name,
	}

	if obj != nil {
		obj["apiVersion"] = apiVersion
		obj["kind"] = kind
		obj["metadata"] = map[string]interface{}{"name": name}
		r.Object = &unstructured.Unstructured{Object: obj}
	}

	return r
}

func TestGenerator_Generate_SingleResource(t *testing.T) {
	g := kro.NewGenerator(kro.GeneratorConfig{
		Name:      "my-app",
		ChartName: "my-chart",
	})

	depGraph := transform.NewDependencyGraph()
	cm := makeResource("v1", "ConfigMap", "my-config", map[string]interface{}{
		"data": map[string]interface{}{"key": "value"},
	})
	depGraph.AddNode("configmap", cm)

	rgd, err := g.Generate(depGraph)
	require.NoError(t, err)
	assert.Equal(t, kro.APIVersion, rgd.APIVersion)
	assert.Equal(t, kro.Kind, rgd.Kind)
	assert.Equal(t, "my-app", rgd.Metadata.Name)
	assert.Equal(t, "chart2kro", rgd.Metadata.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "my-chart", rgd.Metadata.Labels["app.kubernetes.io/name"])
	assert.Len(t, rgd.Spec.Resources, 1)
	assert.Equal(t, "configmap", rgd.Spec.Resources[0].ID)
}

func TestGenerator_Generate_WithDependencies(t *testing.T) {
	g := kro.NewGenerator(kro.GeneratorConfig{
		Name: "web-app",
	})

	depGraph := transform.NewDependencyGraph()
	cm := makeResource("v1", "ConfigMap", "config", map[string]interface{}{
		"data": map[string]interface{}{"key": "value"},
	})
	deploy := makeResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(3),
		},
	})

	depGraph.AddNode("configmap", cm)
	depGraph.AddNode("deployment", deploy)
	depGraph.AddEdge("deployment", "configmap")

	rgd, err := g.Generate(depGraph)
	require.NoError(t, err)
	assert.Len(t, rgd.Spec.Resources, 2)

	// ConfigMap should come first (topo order).
	assert.Equal(t, "configmap", rgd.Spec.Resources[0].ID)
	assert.Empty(t, rgd.Spec.Resources[0].DependsOn)

	// Deployment should come second with dependency.
	assert.Equal(t, "deployment", rgd.Spec.Resources[1].ID)
	assert.Equal(t, []string{"configmap"}, rgd.Spec.Resources[1].DependsOn)
}

func TestGenerator_Generate_WithReadyWhen(t *testing.T) {
	g := kro.NewGenerator(kro.GeneratorConfig{
		Name: "app",
	})

	depGraph := transform.NewDependencyGraph()
	deploy := makeResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(1),
		},
	})
	depGraph.AddNode("deployment", deploy)

	rgd, err := g.Generate(depGraph)
	require.NoError(t, err)
	assert.Len(t, rgd.Spec.Resources[0].ReadyWhen, 1)
	assert.Contains(t, rgd.Spec.Resources[0].ReadyWhen[0], "availableReplicas")
}

func TestGenerator_Generate_WithSchema(t *testing.T) {
	schemaFields := []*transform.SchemaField{
		{Name: "replicas", Path: "replicas", Type: "integer", Default: "3"},
		{Name: "image", Path: "image", Type: "string", Default: "nginx"},
	}

	g := kro.NewGenerator(kro.GeneratorConfig{
		Name:         "app",
		SchemaFields: schemaFields,
	})

	depGraph := transform.NewDependencyGraph()
	cm := makeResource("v1", "ConfigMap", "x", map[string]interface{}{
		"data": map[string]interface{}{"k": "v"},
	})
	depGraph.AddNode("configmap", cm)

	rgd, err := g.Generate(depGraph)
	require.NoError(t, err)
	require.NotNil(t, rgd.Spec.Schema)
	assert.Equal(t, "app.kro.run/v1alpha1", rgd.Spec.Schema.APIVersion)
	assert.Equal(t, "App", rgd.Spec.Schema.Kind)
	assert.NotNil(t, rgd.Spec.Schema.Spec)
}

func TestGenerator_Generate_CustomSchemaOverrides(t *testing.T) {
	schemaFields := []*transform.SchemaField{
		{Name: "port", Path: "port", Type: "integer", Default: "8080"},
	}

	g := kro.NewGenerator(kro.GeneratorConfig{
		Name:             "my-app",
		SchemaKind:       "WebService",
		SchemaAPIVersion: "v1beta1",
		SchemaGroup:      "platform.example.com",
		SchemaFields:     schemaFields,
	})

	depGraph := transform.NewDependencyGraph()
	cm := makeResource("v1", "ConfigMap", "x", map[string]interface{}{
		"data": map[string]interface{}{"k": "v"},
	})
	depGraph.AddNode("configmap", cm)

	rgd, err := g.Generate(depGraph)
	require.NoError(t, err)
	require.NotNil(t, rgd.Spec.Schema)
	assert.Equal(t, "my-app.platform.example.com/v1beta1", rgd.Spec.Schema.APIVersion)
	assert.Equal(t, "WebService", rgd.Spec.Schema.Kind)
}

func TestGenerator_Generate_WithStatusProjections(t *testing.T) {
	statusFields := []transform.StatusField{
		{Name: "deploymentReady", CELExpression: "${deployment.status.availableReplicas}"},
	}

	g := kro.NewGenerator(kro.GeneratorConfig{
		Name:         "app",
		StatusFields: statusFields,
	})

	depGraph := transform.NewDependencyGraph()
	cm := makeResource("v1", "ConfigMap", "x", map[string]interface{}{
		"data": map[string]interface{}{"k": "v"},
	})
	depGraph.AddNode("configmap", cm)

	rgd, err := g.Generate(depGraph)
	require.NoError(t, err)
	require.NotNil(t, rgd.Spec.Schema)
	assert.NotNil(t, rgd.Spec.Schema.Status)
	assert.Equal(t, "${deployment.status.availableReplicas}", rgd.Spec.Schema.Status["deploymentReady"])
}

func TestGenerator_Generate_CycleError(t *testing.T) {
	g := kro.NewGenerator(kro.GeneratorConfig{
		Name: "broken",
	})

	depGraph := transform.NewDependencyGraph()
	depGraph.AddNode("a", makeResource("v1", "ConfigMap", "a", map[string]interface{}{
		"data": map[string]interface{}{"k": "v"},
	}))
	depGraph.AddNode("b", makeResource("v1", "Secret", "b", map[string]interface{}{
		"data": map[string]interface{}{"k": "v"},
	}))
	depGraph.AddEdge("a", "b")
	depGraph.AddEdge("b", "a")

	_, err := g.Generate(depGraph)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestRGD_ToMap(t *testing.T) {
	rgd := &kro.RGD{
		APIVersion: kro.APIVersion,
		Kind:       kro.Kind,
		Metadata: kro.Metadata{
			Name:   "test",
			Labels: map[string]string{"app": "test"},
		},
		Spec: kro.Spec{
			Resources: []kro.Resource{
				{
					ID: "configmap",
					Template: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
					},
				},
			},
		},
	}

	m := rgd.ToMap()
	assert.Equal(t, kro.APIVersion, m["apiVersion"])
	assert.Equal(t, kro.Kind, m["kind"])

	meta, ok := m["metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test", meta["name"])
}

func TestGenerator_Generate_NilObject(t *testing.T) {
	g := kro.NewGenerator(kro.GeneratorConfig{
		Name: "test",
	})

	depGraph := transform.NewDependencyGraph()
	r := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
		Name: "orphan",
	}
	depGraph.AddNode("configmap", r)

	rgd, err := g.Generate(depGraph)
	require.NoError(t, err)
	assert.Len(t, rgd.Spec.Resources, 1)
	assert.Equal(t, "ConfigMap", rgd.Spec.Resources[0].Template["kind"])
}

func TestGenerator_Generate_ChartVersion(t *testing.T) {
	g := kro.NewGenerator(kro.GeneratorConfig{
		Name:         "app",
		ChartVersion: "1.2.3",
	})

	depGraph := transform.NewDependencyGraph()
	depGraph.AddNode("cm", makeResource("v1", "ConfigMap", "x", map[string]interface{}{
		"data": map[string]interface{}{"k": "v"},
	}))

	rgd, err := g.Generate(depGraph)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", rgd.Metadata.Labels["app.kubernetes.io/version"])
}

func TestGenerator_Generate_FullStack(t *testing.T) {
	schemaFields := []*transform.SchemaField{
		{Name: "replicas", Path: "replicas", Type: "integer", Default: "3"},
	}
	statusFields := []transform.StatusField{
		{Name: "ready", CELExpression: "${deployment.status.availableReplicas}"},
	}

	g := kro.NewGenerator(kro.GeneratorConfig{
		Name:         "nginx-app",
		ChartName:    "nginx",
		ChartVersion: "1.0.0",
		SchemaFields: schemaFields,
		StatusFields: statusFields,
	})

	sa := makeResource("v1", "ServiceAccount", "nginx-sa", map[string]interface{}{})
	cm := makeResource("v1", "ConfigMap", "nginx-config", map[string]interface{}{
		"data": map[string]interface{}{"nginx.conf": "server {}"},
	})
	deploy := makeResource("apps/v1", "Deployment", "nginx", map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "nginx"},
				},
				"spec": map[string]interface{}{
					"serviceAccountName": "nginx-sa",
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:latest",
						},
					},
					"volumes": []interface{}{
						map[string]interface{}{
							"name":      "config",
							"configMap": map[string]interface{}{"name": "nginx-config"},
						},
					},
				},
			},
		},
	})
	svc := makeResource("v1", "Service", "nginx-svc", map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{"app": "nginx"},
		},
	})

	resources := map[*k8s.Resource]string{
		sa:     "serviceaccount",
		cm:     "configmap",
		deploy: "deployment",
		svc:    "service",
	}

	builtGraph := transform.BuildDependencyGraph(resources)

	rgd, err := g.Generate(builtGraph)
	require.NoError(t, err)

	// Verify structure.
	assert.Equal(t, kro.APIVersion, rgd.APIVersion)
	assert.Equal(t, kro.Kind, rgd.Kind)
	assert.Equal(t, "nginx-app", rgd.Metadata.Name)
	assert.Equal(t, "nginx", rgd.Metadata.Labels["app.kubernetes.io/name"])
	assert.Equal(t, "1.0.0", rgd.Metadata.Labels["app.kubernetes.io/version"])

	// Schema should be set.
	require.NotNil(t, rgd.Spec.Schema)
	assert.Equal(t, "NginxApp", rgd.Spec.Schema.Kind)
	assert.NotNil(t, rgd.Spec.Schema.Spec)
	assert.NotNil(t, rgd.Spec.Schema.Status)

	// Resources should be in topological order.
	assert.Len(t, rgd.Spec.Resources, 4)

	// Find deployment and verify readyWhen.
	var deployRes *kro.Resource
	for i := range rgd.Spec.Resources {
		if rgd.Spec.Resources[i].ID == "deployment" {
			deployRes = &rgd.Spec.Resources[i]
			break
		}
	}

	require.NotNil(t, deployRes)
	assert.NotEmpty(t, deployRes.ReadyWhen)
	assert.NotEmpty(t, deployRes.DependsOn)

	// ToMap should work.
	m := rgd.ToMap()
	assert.NotNil(t, m["spec"])
}
