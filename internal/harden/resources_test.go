package harden

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestResourceRequirements_InjectsDefaults(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "nginx:1.25"),
	})

	policy := NewResourceRequirementsPolicy(DefaultResourceDefaults)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	podSpec := getPodSpec(deploy)
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	resources := container["resources"].(map[string]interface{})

	requests := resources["requests"].(map[string]interface{})
	assert.Equal(t, "100m", requests["cpu"])
	assert.Equal(t, "128Mi", requests["memory"])

	limits := resources["limits"].(map[string]interface{})
	assert.Equal(t, "500m", limits["cpu"])
	assert.Equal(t, "512Mi", limits["memory"])

	// Should produce 4 changes (cpu+memory for both requests and limits).
	assert.Len(t, result.Changes, 4)
}

func TestResourceRequirements_PreservesExisting(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "app",
		Object: makeUnstructuredWorkload("Deployment", "app", map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "web",
					"image": "nginx:1.25",
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu":    "200m",
							"memory": "256Mi",
						},
						"limits": map[string]interface{}{
							"cpu":    "1",
							"memory": "1Gi",
						},
					},
				},
			},
		}),
	}

	policy := NewResourceRequirementsPolicy(DefaultResourceDefaults)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// No changes because all fields already set.
	assert.Len(t, result.Changes, 0)

	podSpec := getPodSpec(deploy)
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	resources := container["resources"].(map[string]interface{})
	requests := resources["requests"].(map[string]interface{})

	assert.Equal(t, "200m", requests["cpu"])
	assert.Equal(t, "256Mi", requests["memory"])
}

func TestResourceRequirements_PartiallySet(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "app",
		Object: makeUnstructuredWorkload("Deployment", "app", map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "web",
					"image": "nginx:1.25",
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu": "200m",
						},
					},
				},
			},
		}),
	}

	policy := NewResourceRequirementsPolicy(DefaultResourceDefaults)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Should add missing: requests.memory, limits.cpu, limits.memory.
	assert.Len(t, result.Changes, 3)

	podSpec := getPodSpec(deploy)
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	resources := container["resources"].(map[string]interface{})

	requests := resources["requests"].(map[string]interface{})
	assert.Equal(t, "200m", requests["cpu"])     // preserved
	assert.Equal(t, "128Mi", requests["memory"]) // injected
}

func TestResourceRequirements_EmptyDefaults(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx:1.25")})

	// Empty config means no defaults to inject.
	policy := NewResourceRequirementsPolicy(&ResourceDefaultsConfig{})
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	assert.Len(t, result.Changes, 0)
}

func TestResourceRequirements_Name(t *testing.T) {
	policy := NewResourceRequirementsPolicy(DefaultResourceDefaults)
	assert.Equal(t, "resource-requirements", policy.Name())
}

func TestResourceRequirements_InitContainers(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "app",
		Object: makeUnstructuredWorkload("Deployment", "app", map[string]interface{}{
			"containers": []interface{}{
				makeContainer("web", "nginx:1.25"),
			},
			"initContainers": []interface{}{
				makeContainer("init", "busybox:1.36"),
			},
		}),
	}

	policy := NewResourceRequirementsPolicy(DefaultResourceDefaults)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// 4 changes for main container + 4 for init container = 8.
	assert.Len(t, result.Changes, 8)

	// Verify init container got resources.
	podSpec := getPodSpec(deploy)
	initContainers := podSpec["initContainers"].([]interface{})
	initContainer := initContainers[0].(map[string]interface{})
	resources := initContainer["resources"].(map[string]interface{})

	requests := resources["requests"].(map[string]interface{})
	assert.Equal(t, "100m", requests["cpu"])
	assert.Equal(t, "128Mi", requests["memory"])

	limits := resources["limits"].(map[string]interface{})
	assert.Equal(t, "500m", limits["cpu"])
	assert.Equal(t, "512Mi", limits["memory"])
}

func TestResourceRequirements_RequireLimits_ErrorWhenMissing(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "nginx:1.25"),
	})

	// RequireLimits with empty defaults → error.
	policy := NewResourceRequirementsPolicy(&ResourceDefaultsConfig{
		RequireLimits: true,
	})
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing cpu limit")
	assert.Contains(t, err.Error(), "requireLimits=true")
}

func TestResourceRequirements_RequireLimits_OKWithDefaults(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "nginx:1.25"),
	})

	// RequireLimits with defaults configured → OK (defaults fill the gap).
	policy := NewResourceRequirementsPolicy(&ResourceDefaultsConfig{
		CPULimit:      "500m",
		MemoryLimit:   "512Mi",
		RequireLimits: true,
	})
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	assert.Len(t, result.Changes, 2) // cpu limit + memory limit injected
}

func TestResourceRequirements_RequireLimits_OKWhenAlreadySet(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "app",
		Object: makeUnstructuredWorkload("Deployment", "app", map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "web",
					"image": "nginx:1.25",
					"resources": map[string]interface{}{
						"limits": map[string]interface{}{
							"cpu":    "1",
							"memory": "1Gi",
						},
					},
				},
			},
		}),
	}

	// RequireLimits TRUE, NO defaults, but container already has limits → OK.
	policy := NewResourceRequirementsPolicy(&ResourceDefaultsConfig{
		RequireLimits: true,
	})
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
}

func TestResourceRequirements_RequireLimits_MemoryMissing(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "app",
		Object: makeUnstructuredWorkload("Deployment", "app", map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "web",
					"image": "nginx:1.25",
					"resources": map[string]interface{}{
						"limits": map[string]interface{}{
							"cpu": "1",
						},
					},
				},
			},
		}),
	}

	// CPU limit set but memory missing, no defaults → error on memory.
	policy := NewResourceRequirementsPolicy(&ResourceDefaultsConfig{
		RequireLimits: true,
	})
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing memory limit")
}

func TestResourceRequirements_CronJob(t *testing.T) {
	cronJob := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
		Name: "cron",
		Object: makeUnstructuredWorkload("CronJob", "cron", map[string]interface{}{
			"containers": []interface{}{
				makeContainer("worker", "busybox:1.36"),
			},
		}),
	}

	policy := NewResourceRequirementsPolicy(DefaultResourceDefaults)
	result := &Result{Resources: []*k8s.Resource{cronJob}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// 4 changes: cpu+memory for both requests and limits.
	assert.Len(t, result.Changes, 4)

	podSpec := getPodSpec(cronJob)
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	resources := container["resources"].(map[string]interface{})

	requests := resources["requests"].(map[string]interface{})
	assert.Equal(t, "100m", requests["cpu"])
}
