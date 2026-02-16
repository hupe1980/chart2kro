package harden

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestPSS_Restricted_InjectsAllFields(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "nginx:1.25"),
	})

	policy := NewPSSPolicy(SecurityLevelRestricted)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Verify container security context was injected.
	podSpec := getPodSpec(deploy)
	require.NotNil(t, podSpec)

	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	sc := container["securityContext"].(map[string]interface{})

	assert.Equal(t, true, sc["runAsNonRoot"])
	assert.Equal(t, true, sc["readOnlyRootFilesystem"])
	assert.Equal(t, false, sc["allowPrivilegeEscalation"])

	caps := sc["capabilities"].(map[string]interface{})
	assert.Equal(t, []interface{}{"ALL"}, caps["drop"])

	seccomp := sc["seccompProfile"].(map[string]interface{})
	assert.Equal(t, "RuntimeDefault", seccomp["type"])

	// Verify pod-level security context.
	podSC := podSpec["securityContext"].(map[string]interface{})
	assert.Equal(t, true, podSC["runAsNonRoot"])

	podSeccomp := podSC["seccompProfile"].(map[string]interface{})
	assert.Equal(t, "RuntimeDefault", podSeccomp["type"])

	// Verify automountServiceAccountToken is disabled.
	assert.Equal(t, false, podSpec["automountServiceAccountToken"])
}

func TestPSS_Restricted_InitContainersHardened(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "app",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "app"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								makeContainer("web", "nginx:1.25"),
							},
							"initContainers": []interface{}{
								makeContainer("init", "busybox:latest"),
							},
						},
					},
				},
			},
		},
	}

	policy := NewPSSPolicy(SecurityLevelRestricted)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	podSpec := getPodSpec(deploy)
	initContainers := podSpec["initContainers"].([]interface{})
	initContainer := initContainers[0].(map[string]interface{})
	sc := initContainer["securityContext"].(map[string]interface{})

	assert.Equal(t, true, sc["runAsNonRoot"])
	assert.Equal(t, false, sc["allowPrivilegeEscalation"])
}

func TestPSS_Restricted_AllWorkloadTypes(t *testing.T) {
	workloadTypes := []struct {
		group string
		kind  string
	}{
		{"apps", "Deployment"},
		{"apps", "StatefulSet"},
		{"apps", "DaemonSet"},
		{"apps", "ReplicaSet"},
		{"batch", "Job"},
	}

	for _, wt := range workloadTypes {
		t.Run(wt.kind, func(t *testing.T) {
			res := &k8s.Resource{
				GVK:  schema.GroupVersionKind{Group: wt.group, Version: "v1", Kind: wt.kind},
				Name: "test",
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": wt.group + "/v1",
						"kind":       wt.kind,
						"metadata":   map[string]interface{}{"name": "test"},
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"containers": []interface{}{
										makeContainer("app", "nginx:1.25"),
									},
								},
							},
						},
					},
				},
			}

			policy := NewPSSPolicy(SecurityLevelRestricted)
			result := &Result{Resources: []*k8s.Resource{res}}

			err := policy.Apply(context.Background(), result.Resources, result)
			require.NoError(t, err)
			assert.True(t, len(result.Changes) > 0, "expected changes for %s", wt.kind)
		})
	}
}

func TestPSS_Restricted_CronJob(t *testing.T) {
	cronJob := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
		Name: "cron",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "batch/v1",
				"kind":       "CronJob",
				"metadata":   map[string]interface{}{"name": "cron"},
				"spec": map[string]interface{}{
					"jobTemplate": map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"containers": []interface{}{
										makeContainer("worker", "busybox:1.36"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	policy := NewPSSPolicy(SecurityLevelRestricted)
	result := &Result{Resources: []*k8s.Resource{cronJob}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	podSpec := getPodSpec(cronJob)
	require.NotNil(t, podSpec)

	containers := podSpec["containers"].([]interface{})
	sc := containers[0].(map[string]interface{})["securityContext"].(map[string]interface{})
	assert.Equal(t, true, sc["runAsNonRoot"])
}

func TestPSS_NonDestructiveMerge_PreservesExisting(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "app",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "app"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "web",
									"image": "nginx:1.25",
									"securityContext": map[string]interface{}{
										"runAsUser":  int64(1000),
										"runAsGroup": int64(1000),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	policy := NewPSSPolicy(SecurityLevelRestricted)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	containers := getPodSpec(deploy)["containers"].([]interface{})
	sc := containers[0].(map[string]interface{})["securityContext"].(map[string]interface{})

	// Existing fields must be preserved.
	assert.Equal(t, int64(1000), sc["runAsUser"])
	assert.Equal(t, int64(1000), sc["runAsGroup"])
	// Missing fields should be filled in.
	assert.Equal(t, true, sc["runAsNonRoot"])
	assert.Equal(t, true, sc["readOnlyRootFilesystem"])
}

func TestPSS_NonDestructiveMerge_ConflictWarning(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "app",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "app"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "web",
									"image": "nginx:1.25",
									"securityContext": map[string]interface{}{
										"allowPrivilegeEscalation": true,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	policy := NewPSSPolicy(SecurityLevelRestricted)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Existing conflicting value must NOT be overridden.
	containers := getPodSpec(deploy)["containers"].([]interface{})
	sc := containers[0].(map[string]interface{})["securityContext"].(map[string]interface{})
	assert.Equal(t, true, sc["allowPrivilegeEscalation"], "existing value must be preserved")

	// A warning must be emitted.
	assert.True(t, len(result.Warnings) > 0, "expected conflict warning")
	assert.Contains(t, result.Warnings[0], "allowPrivilegeEscalation")
}

func TestPSS_Baseline_NoPrivileged(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "app",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "app"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"hostNetwork": true,
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "web",
									"image": "nginx:1.25",
									"securityContext": map[string]interface{}{
										"privileged": true,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	policy := NewPSSPolicy(SecurityLevelBaseline)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Warnings for hostNetwork and privileged.
	assert.True(t, len(result.Warnings) >= 2)

	hasHostNetworkWarning := false
	hasPrivilegedWarning := false

	for _, w := range result.Warnings {
		if strings.Contains(w, "hostNetwork") {
			hasHostNetworkWarning = true
		}

		if strings.Contains(w, "privileged") {
			hasPrivilegedWarning = true
		}
	}

	assert.True(t, hasHostNetworkWarning, "expected hostNetwork warning")
	assert.True(t, hasPrivilegedWarning, "expected privileged warning")
}

func TestPSS_Baseline_DropsDangerousCaps(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "app",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "app"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "web",
									"image": "nginx:1.25",
									"securityContext": map[string]interface{}{
										"capabilities": map[string]interface{}{
											"add": []interface{}{"SYS_ADMIN", "NET_BIND_SERVICE"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	policy := NewPSSPolicy(SecurityLevelBaseline)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Should warn about SYS_ADMIN.
	assert.True(t, len(result.Warnings) > 0)
	assert.Contains(t, result.Warnings[0], "SYS_ADMIN")
}

func TestPSS_Baseline_DoesNotEnforceRestricted(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx:1.25")})

	policy := NewPSSPolicy(SecurityLevelBaseline)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Baseline should NOT inject runAsNonRoot or readOnlyRootFilesystem.
	podSpec := getPodSpec(deploy)
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})

	sc, hasSC := container["securityContext"].(map[string]interface{})
	if hasSC {
		_, hasRunAsNonRoot := sc["runAsNonRoot"]
		assert.False(t, hasRunAsNonRoot, "baseline should not set runAsNonRoot")
	}
}

func TestPSS_NoneLevel_NoChanges(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx:1.25")})

	policy := NewPSSPolicy(SecurityLevelNone)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	assert.Empty(t, result.Changes)
	assert.Empty(t, result.Warnings)
}

func TestPSS_Restricted_ExistingDropAll_NotDuplicated(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "app",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "app"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "web",
									"image": "nginx:1.25",
									"securityContext": map[string]interface{}{
										"capabilities": map[string]interface{}{
											"drop": []interface{}{"ALL"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	policy := NewPSSPolicy(SecurityLevelRestricted)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Should not add a duplicate capabilities.drop change.
	for _, c := range result.Changes {
		if strings.Contains(c.FieldPath, "capabilities.drop") {
			t.Error("should not add capabilities.drop when already present")
		}
	}
}

func TestGetPodSpec_NilObject(t *testing.T) {
	res := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "test",
	}

	assert.Nil(t, getPodSpec(res))
}

func TestGetPodSpec_MissingSpec(t *testing.T) {
	res := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "test",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "test"},
			},
		},
	}

	assert.Nil(t, getPodSpec(res))
}

func TestGetPodSpec_MissingTemplate(t *testing.T) {
	res := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "test",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "test"},
				"spec":       map[string]interface{}{},
			},
		},
	}

	assert.Nil(t, getPodSpec(res))
}

func TestGetPodSpec_CronJob_MissingJobTemplate(t *testing.T) {
	res := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
		Name: "test",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "batch/v1",
				"kind":       "CronJob",
				"metadata":   map[string]interface{}{"name": "test"},
				"spec":       map[string]interface{}{},
			},
		},
	}

	assert.Nil(t, getPodSpec(res))
}

func TestIsWorkload(t *testing.T) {
	for _, kind := range []string{"Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "ReplicaSet"} {
		res := &k8s.Resource{GVK: schema.GroupVersionKind{Kind: kind}}
		assert.True(t, isWorkload(res), "expected %s to be a workload", kind)
	}

	for _, kind := range []string{"ConfigMap", "Secret", "Service", "Ingress"} {
		res := &k8s.Resource{GVK: schema.GroupVersionKind{Kind: kind}}
		assert.False(t, isWorkload(res), "expected %s to not be a workload", kind)
	}
}

func TestIsDangerousCapability(t *testing.T) {
	assert.True(t, isDangerousCapability("SYS_ADMIN"))
	assert.True(t, isDangerousCapability("NET_ADMIN"))
	assert.True(t, isDangerousCapability("ALL"), "ALL should be treated as dangerous")
	assert.False(t, isDangerousCapability("NET_BIND_SERVICE"))
	assert.False(t, isDangerousCapability("CHOWN"))
}

func TestPSS_Restricted_AutomountServiceAccountToken(t *testing.T) {
	t.Run("injects when missing", func(t *testing.T) {
		deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx:1.25")})
		policy := NewPSSPolicy(SecurityLevelRestricted)
		result := &Result{Resources: []*k8s.Resource{deploy}}

		err := policy.Apply(context.Background(), result.Resources, result)
		require.NoError(t, err)

		podSpec := getPodSpec(deploy)
		assert.Equal(t, false, podSpec["automountServiceAccountToken"])
	})

	t.Run("preserves explicit true with warning", func(t *testing.T) {
		deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx:1.25")})
		podSpec := getPodSpec(deploy)
		podSpec["automountServiceAccountToken"] = true

		policy := NewPSSPolicy(SecurityLevelRestricted)
		result := &Result{Resources: []*k8s.Resource{deploy}}

		err := policy.Apply(context.Background(), result.Resources, result)
		require.NoError(t, err)

		// Should not override, but warn.
		assert.Equal(t, true, podSpec["automountServiceAccountToken"])
		assert.NotEmpty(t, result.Warnings)
	})

	t.Run("baseline does not inject", func(t *testing.T) {
		deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx:1.25")})
		policy := NewPSSPolicy(SecurityLevelBaseline)
		result := &Result{Resources: []*k8s.Resource{deploy}}

		err := policy.Apply(context.Background(), result.Resources, result)
		require.NoError(t, err)

		podSpec := getPodSpec(deploy)
		_, exists := podSpec["automountServiceAccountToken"]
		assert.False(t, exists, "baseline should not inject automountServiceAccountToken")
	})
}
