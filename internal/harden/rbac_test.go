package harden

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRBACGenerator_GeneratesResources(t *testing.T) {
	deploy := makeDeployment("web", []interface{}{makeContainer("web", "nginx:1.25")})

	gen := NewRBACGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Should generate SA + Role + RoleBinding.
	var saCount, roleCount, bindingCount int

	for _, res := range result.Resources {
		switch res.Kind() {
		case "ServiceAccount":
			saCount++
			assert.Equal(t, "web-sa", res.Name)
		case "Role":
			roleCount++
			assert.Equal(t, "web-role", res.Name)
		case "RoleBinding":
			bindingCount++
			assert.Equal(t, "web-rolebinding", res.Name)
		}
	}

	assert.Equal(t, 1, saCount)
	assert.Equal(t, 1, roleCount)
	assert.Equal(t, 1, bindingCount)
}

func TestRBACGenerator_SetsServiceAccountName(t *testing.T) {
	deploy := makeDeployment("web", []interface{}{makeContainer("web", "nginx:1.25")})

	gen := NewRBACGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	podSpec := getPodSpec(deploy)
	assert.Equal(t, "web-sa", podSpec["serviceAccountName"])
}

func TestRBACGenerator_PreservesExistingServiceAccount(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "web",
		Object: makeUnstructuredWorkload("Deployment", "web", map[string]interface{}{
			"serviceAccountName": "existing-sa",
			"containers": []interface{}{
				makeContainer("web", "nginx:1.25"),
			},
		}),
	}

	gen := NewRBACGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	podSpec := getPodSpec(deploy)
	assert.Equal(t, "existing-sa", podSpec["serviceAccountName"])
}

func TestRBACGenerator_RolePermissions(t *testing.T) {
	// Deployment that references a Secret via envFrom and a ConfigMap via volume.
	deploy := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "web",
		Object: makeUnstructuredWorkload("Deployment", "web", map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "web",
					"image": "nginx:1.25",
					"envFrom": []interface{}{
						map[string]interface{}{
							"secretRef": map[string]interface{}{
								"name": "secret",
							},
						},
					},
				},
			},
			"volumes": []interface{}{
				map[string]interface{}{
					"name": "config-vol",
					"configMap": map[string]interface{}{
						"name": "config",
					},
				},
			},
		}),
	}
	configMap := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		Name: "config",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "config"},
			},
		},
	}
	secret := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
		Name: "secret",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]interface{}{"name": "secret"},
			},
		},
	}

	gen := NewRBACGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy, configMap, secret}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Find the Role.
	var role *k8s.Resource

	for _, res := range result.Resources {
		if res.Kind() == "Role" {
			role = res
			break
		}
	}

	require.NotNil(t, role)

	rules, ok := role.Object.Object["rules"].([]interface{})
	require.True(t, ok)
	assert.True(t, len(rules) > 0, "expected RBAC rules")

	// Verify rules have get, list, watch verbs.
	for _, r := range rules {
		rule := r.(map[string]interface{})
		verbs := rule["verbs"].([]interface{})
		assert.Contains(t, verbs, "get")
		assert.Contains(t, verbs, "list")
		assert.Contains(t, verbs, "watch")
	}

	// Verify least-privilege: role should only contain Secret and ConfigMap
	// (the kinds the workload actually references).
	var allResources []string
	for _, r := range rules {
		rule := r.(map[string]interface{})
		resources := rule["resources"].([]interface{})
		for _, res := range resources {
			allResources = append(allResources, res.(string))
		}
	}
	assert.Contains(t, allResources, "secrets")
	assert.Contains(t, allResources, "configmaps")
}

func TestRBACGenerator_LeastPrivilegeIsolation(t *testing.T) {
	// Deployment A references only a Secret. Deployment B references only a ConfigMap.
	// Verify that A gets only Secret permissions and B gets only ConfigMap permissions.
	deployA := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "app-a",
		Object: makeUnstructuredWorkload("Deployment", "app-a", map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "a",
					"image": "nginx:1.25",
					"envFrom": []interface{}{
						map[string]interface{}{
							"secretRef": map[string]interface{}{
								"name": "my-secret",
							},
						},
					},
				},
			},
		}),
	}
	deployB := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "app-b",
		Object: makeUnstructuredWorkload("Deployment", "app-b", map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "b",
					"image": "nginx:1.25",
					"env": []interface{}{
						map[string]interface{}{
							"name": "CFG",
							"valueFrom": map[string]interface{}{
								"configMapKeyRef": map[string]interface{}{
									"name": "my-config",
									"key":  "val",
								},
							},
						},
					},
				},
			},
		}),
	}

	gen := NewRBACGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deployA, deployB}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Collect roles by name.
	rolesByName := make(map[string]*k8s.Resource)
	for _, res := range result.Resources {
		if res.Kind() == "Role" {
			rolesByName[res.Name] = res
		}
	}

	// App-A role should have secrets, not configmaps.
	roleA := rolesByName["app-a-role"]
	require.NotNil(t, roleA)
	resourcesA := collectRoleResources(roleA)
	assert.Contains(t, resourcesA, "secrets")
	assert.NotContains(t, resourcesA, "configmaps")

	// App-B role should have configmaps, not secrets.
	roleB := rolesByName["app-b-role"]
	require.NotNil(t, roleB)
	resourcesB := collectRoleResources(roleB)
	assert.Contains(t, resourcesB, "configmaps")
	assert.NotContains(t, resourcesB, "secrets")
}

// collectRoleResources extracts all resource strings from a Role's rules.
func collectRoleResources(role *k8s.Resource) []string {
	var result []string
	rules, _ := role.Object.Object["rules"].([]interface{})
	for _, r := range rules {
		rule, _ := r.(map[string]interface{})
		resources, _ := rule["resources"].([]interface{})
		for _, res := range resources {
			result = append(result, res.(string))
		}
	}
	return result
}

func TestRBACGenerator_RoleBindingStructure(t *testing.T) {
	deploy := makeDeployment("web", []interface{}{makeContainer("web", "nginx:1.25")})

	gen := NewRBACGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	var binding *k8s.Resource

	for _, res := range result.Resources {
		if res.Kind() == "RoleBinding" {
			binding = res
			break
		}
	}

	require.NotNil(t, binding)

	roleRef := binding.Object.Object["roleRef"].(map[string]interface{})
	assert.Equal(t, "Role", roleRef["kind"])
	assert.Equal(t, "web-role", roleRef["name"])
	assert.Equal(t, rbacAPIGroup, roleRef["apiGroup"])

	subjects := binding.Object.Object["subjects"].([]interface{})
	assert.Len(t, subjects, 1)

	subject := subjects[0].(map[string]interface{})
	assert.Equal(t, "ServiceAccount", subject["kind"])
	assert.Equal(t, "web-sa", subject["name"])
}

func TestRBACGenerator_Name(t *testing.T) {
	gen := NewRBACGenerator(nil)
	assert.Equal(t, "rbac-generator", gen.Name())
}

func TestInferAPIGroup(t *testing.T) {
	assert.Equal(t, "apps", inferAPIGroup("Deployment"))
	assert.Equal(t, "", inferAPIGroup("ConfigMap"))
	assert.Equal(t, "batch", inferAPIGroup("Job"))
	assert.Equal(t, "", inferAPIGroup("UnknownKind"))
}

func TestInferResourceName(t *testing.T) {
	assert.Equal(t, "deployments", inferResourceName("Deployment"))
	assert.Equal(t, "configmaps", inferResourceName("ConfigMap"))
	assert.Equal(t, "unknownkinds", inferResourceName("UnknownKind"))
}
