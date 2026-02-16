package audit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/audit"
	"github.com/hupe1980/chart2kro/internal/k8s"
)

var bgCtx = context.Background()

// --- SEC-001 ---

func TestRunAsRootCheck(t *testing.T) {
	check := &audit.RunAsRootCheck{}
	assert.Equal(t, "SEC-001", check.ID())

	t.Run("flags container without runAsNonRoot", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "image": "nginx:1.25"},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, audit.SeverityCritical, findings[0].Severity)
		assert.Contains(t, findings[0].Message, "app")
	})

	t.Run("passes with runAsNonRoot on container", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "securityContext": map[string]interface{}{"runAsNonRoot": true}},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("passes with runAsNonRoot on pod level", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app"},
		})
		setPodSpecField(res, "securityContext", map[string]interface{}{"runAsNonRoot": true})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("skips non-workload", func(t *testing.T) {
		res := makeService("svc", map[string]interface{}{"app": "web"})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("checks init containers too", func(t *testing.T) {
		res := makeWorkload("Deployment", "web",
			[]map[string]interface{}{
				{"name": "app", "securityContext": map[string]interface{}{"runAsNonRoot": true}},
			},
			[]map[string]interface{}{
				{"name": "init", "image": "busybox"},
			},
		)
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Contains(t, findings[0].Message, "init")
	})
}

// --- SEC-002 ---

func TestPrivilegedCheck(t *testing.T) {
	check := &audit.PrivilegedCheck{}
	assert.Equal(t, "SEC-002", check.ID())

	t.Run("flags privileged", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "securityContext": map[string]interface{}{"privileged": true}},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, audit.SeverityCritical, findings[0].Severity)
	})

	t.Run("passes non-privileged", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "securityContext": map[string]interface{}{"privileged": false}},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("passes no securityContext", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-003 ---

func TestResourceLimitsCheck(t *testing.T) {
	check := &audit.ResourceLimitsCheck{}
	assert.Equal(t, "SEC-003", check.ID())

	t.Run("flags no limits", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, audit.SeverityHigh, findings[0].Severity)
	})

	t.Run("passes with limits", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "resources": map[string]interface{}{
				"limits": map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
			}},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-004 ---

func TestLatestTagCheck(t *testing.T) {
	check := &audit.LatestTagCheck{}
	assert.Equal(t, "SEC-004", check.ID())

	t.Run("flags latest", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "image": "nginx:latest"},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, audit.SeverityHigh, findings[0].Severity)
	})

	t.Run("flags no tag", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "image": "nginx"},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
	})

	t.Run("passes specific version", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "image": "nginx:1.25.3"},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("passes digest", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "image": "nginx@sha256:abc123"},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("skips empty image", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("registry with port", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "image": "registry.example.com:5000/nginx:1.25"},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-005 ---

func TestHostNamespaceCheck(t *testing.T) {
	check := &audit.HostNamespaceCheck{}
	assert.Equal(t, "SEC-005", check.ID())

	t.Run("flags hostNetwork", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		setPodSpecField(res, "hostNetwork", true)
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Contains(t, findings[0].Message, "host networking")
	})

	t.Run("flags all three", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		setPodSpecField(res, "hostNetwork", true)
		setPodSpecField(res, "hostPID", true)
		setPodSpecField(res, "hostIPC", true)
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 3)
	})

	t.Run("passes when not set", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-006 ---

func TestReadOnlyRootFSCheck(t *testing.T) {
	check := &audit.ReadOnlyRootFSCheck{}
	assert.Equal(t, "SEC-006", check.ID())

	t.Run("flags without readOnlyRootFilesystem", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, audit.SeverityMedium, findings[0].Severity)
	})

	t.Run("passes with true", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "securityContext": map[string]interface{}{"readOnlyRootFilesystem": true}},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-007 ---

func TestNetworkPolicyCheck(t *testing.T) {
	check := &audit.NetworkPolicyCheck{}
	assert.Equal(t, "SEC-007", check.ID())

	t.Run("flags workloads without NetworkPolicy", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, "(global)", findings[0].ResourceID)
	})

	t.Run("passes with NetworkPolicy", func(t *testing.T) {
		deploy := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		netpol := makeNetworkPolicy("web-policy")
		findings := check.Run(bgCtx, []*k8s.Resource{deploy, netpol})
		assert.Empty(t, findings)
	})

	t.Run("passes with no workloads", func(t *testing.T) {
		svc := makeService("svc", map[string]interface{}{"app": "web"})
		findings := check.Run(bgCtx, []*k8s.Resource{svc})
		assert.Empty(t, findings)
	})
}

// --- SEC-008 ---

func TestDangerousCapabilitiesCheck(t *testing.T) {
	check := &audit.DangerousCapabilitiesCheck{}
	assert.Equal(t, "SEC-008", check.ID())

	t.Run("flags SYS_ADMIN", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "securityContext": map[string]interface{}{
				"capabilities": map[string]interface{}{"add": []interface{}{"SYS_ADMIN"}},
			}},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Contains(t, findings[0].Message, "SYS_ADMIN")
	})

	t.Run("passes safe caps", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "securityContext": map[string]interface{}{
				"capabilities": map[string]interface{}{"add": []interface{}{"NET_BIND_SERVICE"}},
			}},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("passes no caps", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-009 ---

func TestBroadSelectorCheck(t *testing.T) {
	check := &audit.BroadSelectorCheck{}
	assert.Equal(t, "SEC-009", check.ID())

	t.Run("flags 1 label", func(t *testing.T) {
		res := makeService("svc", map[string]interface{}{"app": "web"})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, audit.SeverityLow, findings[0].Severity)
	})

	t.Run("passes 2+ labels", func(t *testing.T) {
		res := makeService("svc", map[string]interface{}{"app": "web", "component": "fe"})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("skips ExternalName services", func(t *testing.T) {
		obj := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]interface{}{"name": "ext"},
			"spec": map[string]interface{}{
				"type":         "ExternalName",
				"externalName": "db.example.com",
			},
		}
		res := &k8s.Resource{
			GVK:    schema.GroupVersionKind{Version: "v1", Kind: "Service"},
			Name:   "ext",
			Object: &unstructured.Unstructured{Object: obj},
		}
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-010 ---

func TestProbeCheck(t *testing.T) {
	check := &audit.ProbeCheck{}
	assert.Equal(t, "SEC-010", check.ID())

	t.Run("flags missing probes", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 2)
	})

	t.Run("passes with probes", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app",
				"livenessProbe":  map[string]interface{}{"httpGet": map[string]interface{}{"path": "/health"}},
				"readinessProbe": map[string]interface{}{"httpGet": map[string]interface{}{"path": "/ready"}},
			},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("only checks main containers", func(t *testing.T) {
		res := makeWorkload("Deployment", "web",
			[]map[string]interface{}{
				{"name": "app",
					"livenessProbe":  map[string]interface{}{"httpGet": map[string]interface{}{"path": "/health"}},
					"readinessProbe": map[string]interface{}{"httpGet": map[string]interface{}{"path": "/ready"}},
				},
			},
			[]map[string]interface{}{{"name": "init"}},
		)
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-011 ---

func TestIngressTLSCheck(t *testing.T) {
	check := &audit.IngressTLSCheck{}
	assert.Equal(t, "SEC-011", check.ID())

	t.Run("flags no TLS", func(t *testing.T) {
		res := makeIngress("web", false)
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, audit.SeverityInfo, findings[0].Severity)
	})

	t.Run("passes with TLS", func(t *testing.T) {
		res := makeIngress("web", true)
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- SEC-012 ---

func TestSeccompProfileCheck(t *testing.T) {
	check := &audit.SeccompProfileCheck{}
	assert.Equal(t, "SEC-012", check.ID())

	t.Run("flags no seccomp", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		require.Len(t, findings, 1)
		assert.Equal(t, audit.SeverityInfo, findings[0].Severity)
	})

	t.Run("passes pod-level seccomp", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		setPodSpecField(res, "securityContext", map[string]interface{}{
			"seccompProfile": map[string]interface{}{"type": "RuntimeDefault"},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})

	t.Run("passes container-level seccomp", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{
			{"name": "app", "securityContext": map[string]interface{}{
				"seccompProfile": map[string]interface{}{"type": "RuntimeDefault"},
			}},
		})
		findings := check.Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

// --- CronJob ---

func TestCronJobPodSpec(t *testing.T) {
	check := &audit.PrivilegedCheck{}

	res := makeCronJob("cleanup", []map[string]interface{}{
		{"name": "job", "securityContext": map[string]interface{}{"privileged": true}},
	})
	findings := check.Run(bgCtx, []*k8s.Resource{res})
	require.Len(t, findings, 1)
}

// --- Nil Object ---

func TestChecks_NilObject(t *testing.T) {
	resNil := &k8s.Resource{
		GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name:   "nil-obj",
		Object: nil,
	}
	checks := []audit.Check{
		&audit.RunAsRootCheck{},
		&audit.PrivilegedCheck{},
		&audit.ResourceLimitsCheck{},
		&audit.LatestTagCheck{},
		&audit.HostNamespaceCheck{},
		&audit.ReadOnlyRootFSCheck{},
		&audit.DangerousCapabilitiesCheck{},
		&audit.SeccompProfileCheck{},
	}
	for _, c := range checks {
		t.Run(c.ID(), func(t *testing.T) {
			findings := c.Run(bgCtx, []*k8s.Resource{resNil})
			assert.Empty(t, findings)
		})
	}
}

func TestCronJob_MalformedSpec(t *testing.T) {
	// CronJob with missing jobTemplate.spec.template.spec sub-paths.
	check := &audit.PrivilegedCheck{}

	// CronJob with empty jobTemplate.
	res := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
		Name: "bad-cron",
		Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"jobTemplate": map[string]interface{}{},
			},
		}},
	}
	findings := check.Run(bgCtx, []*k8s.Resource{res})
	assert.Empty(t, findings, "should safely skip malformed CronJob")

	// CronJob with jobTemplate.spec but no template.
	res2 := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
		Name: "bad-cron-2",
		Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"jobTemplate": map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		}},
	}
	findings = check.Run(bgCtx, []*k8s.Resource{res2})
	assert.Empty(t, findings, "should safely skip CronJob without template")

	// CronJob with template but no pod spec.
	res3 := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
		Name: "bad-cron-3",
		Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"jobTemplate": map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{},
					},
				},
			},
		}},
	}
	findings = check.Run(bgCtx, []*k8s.Resource{res3})
	assert.Empty(t, findings, "should safely skip CronJob without pod spec")
}

func TestDeployment_NoTemplate(t *testing.T) {
	// Deployment with spec but no template.
	check := &audit.RunAsRootCheck{}
	res := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "no-tpl",
		Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"spec": map[string]interface{}{},
		}},
	}
	findings := check.Run(bgCtx, []*k8s.Resource{res})
	assert.Empty(t, findings, "should safely skip Deployment with no template")
}

func TestContainerNameFallback(t *testing.T) {
	// Container without a name should produce a finding with index-based name.
	check := &audit.RunAsRootCheck{}
	res := makeDeployment("test", []map[string]interface{}{
		{"image": "nginx:latest"}, // no "name" field
	})
	findings := check.Run(bgCtx, []*k8s.Resource{res})
	require.NotEmpty(t, findings)
	assert.Contains(t, findings[0].Message, "[0]")
}
