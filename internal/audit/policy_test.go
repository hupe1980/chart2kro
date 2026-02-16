package audit_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/audit"
	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestLoadPolicyFile(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		content := `rules:
  - id: CUSTOM-001
    severity: high
    match:
      kind: Deployment
    condition: "no liveness probe"
    message: "Deployment missing liveness probe"
    remediation: "Add livenessProbe"
`
		path := writeTempFile(t, "policy.yaml", content)
		pf, err := audit.LoadPolicyFile(path)
		require.NoError(t, err)
		require.Len(t, pf.Rules, 1)
		assert.Equal(t, "CUSTOM-001", pf.Rules[0].ID)
		assert.Equal(t, "Deployment", pf.Rules[0].Match.Kind)
	})

	t.Run("rejects missing id", func(t *testing.T) {
		path := writeTempFile(t, "bad.yaml", "rules:\n  - severity: high\n    message: test\n")
		_, err := audit.LoadPolicyFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required 'id'")
	})

	t.Run("rejects missing message", func(t *testing.T) {
		path := writeTempFile(t, "bad2.yaml", "rules:\n  - id: X\n    severity: high\n")
		_, err := audit.LoadPolicyFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required 'message'")
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := audit.LoadPolicyFile("/nonexistent/policy.yaml")
		assert.Error(t, err)
	})

	t.Run("rejects invalid severity", func(t *testing.T) {
		path := writeTempFile(t, "badsev.yaml", "rules:\n  - id: X\n    severity: banana\n    message: test\n")
		_, err := audit.LoadPolicyFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown severity")
	})

	t.Run("rejects unknown condition", func(t *testing.T) {
		content := "rules:\n  - id: X\n    severity: high\n    message: test\n    condition: \"no-resource-limit\"\n"
		path := writeTempFile(t, "badcond.yaml", content)
		_, err := audit.LoadPolicyFile(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown condition")
		assert.Contains(t, err.Error(), "no-resource-limit")
	})

	t.Run("accepts all known conditions", func(t *testing.T) {
		conditions := []string{
			"no liveness probe", "no readiness probe", "no resource limits",
			"uses latest tag", "privileged", "host networking", "no seccomp profile",
		}
		for _, cond := range conditions {
			content := fmt.Sprintf("rules:\n  - id: X\n    severity: high\n    message: test\n    condition: %q\n", cond)
			path := writeTempFile(t, "valid.yaml", content)
			_, err := audit.LoadPolicyFile(path)
			assert.NoError(t, err, "condition %q should be accepted", cond)
		}
	})
}

func TestPolicyFileToChecks(t *testing.T) {
	pf := &audit.PolicyFile{
		Rules: []audit.PolicyRule{
			{ID: "C-001", SeverityStr: "high", Message: "test1"},
			{ID: "C-002", SeverityStr: "low", Message: "test2"},
		},
	}
	checks := pf.ToChecks()
	require.Len(t, checks, 2)
	assert.Equal(t, "C-001", checks[0].ID())
	assert.Equal(t, "C-002", checks[1].ID())
}

func TestCustomRuleConditions(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		container map[string]interface{}
		wantMatch bool
	}{
		{"no liveness probe triggers", "no liveness probe",
			map[string]interface{}{"name": "app"}, true},
		{"no liveness probe passes", "no liveness probe",
			map[string]interface{}{"name": "app", "livenessProbe": map[string]interface{}{"httpGet": map[string]interface{}{"path": "/"}}}, false},
		{"no readiness probe triggers", "no readiness probe",
			map[string]interface{}{"name": "app"}, true},
		{"no resource limits triggers", "no resource limits",
			map[string]interface{}{"name": "app"}, true},
		{"no resource limits passes", "no resource limits",
			map[string]interface{}{"name": "app", "resources": map[string]interface{}{"limits": map[string]interface{}{"cpu": "100m"}}}, false},
		{"uses latest tag triggers", "uses latest tag",
			map[string]interface{}{"name": "app", "image": "nginx:latest"}, true},
		{"uses latest tag passes", "uses latest tag",
			map[string]interface{}{"name": "app", "image": "nginx:1.25"}, false},
		{"privileged triggers", "privileged",
			map[string]interface{}{"name": "app", "securityContext": map[string]interface{}{"privileged": true}}, true},
		{"privileged passes", "privileged",
			map[string]interface{}{"name": "app", "securityContext": map[string]interface{}{"privileged": false}}, false},
		{"no seccomp triggers", "no seccomp profile",
			map[string]interface{}{"name": "app"}, true},
		{"no seccomp passes", "no seccomp profile",
			map[string]interface{}{"name": "app", "securityContext": map[string]interface{}{
				"seccompProfile": map[string]interface{}{"type": "RuntimeDefault"},
			}}, false},
		{"unknown condition", "something unknown",
			map[string]interface{}{"name": "app"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := &audit.PolicyFile{
				Rules: []audit.PolicyRule{{
					ID: "T-001", SeverityStr: "medium",
					Condition: tt.condition, Message: "test",
				}},
			}
			checks := pf.ToChecks()
			res := makeDeployment("test", []map[string]interface{}{tt.container})
			findings := checks[0].Run(bgCtx, []*k8s.Resource{res})
			if tt.wantMatch {
				assert.NotEmpty(t, findings)
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestCustomRuleKindFilter(t *testing.T) {
	pf := &audit.PolicyFile{
		Rules: []audit.PolicyRule{{
			ID: "C-001", SeverityStr: "medium",
			Match:     audit.PolicyMatch{Kind: "StatefulSet"},
			Condition: "no liveness probe", Message: "test",
		}},
	}
	checks := pf.ToChecks()

	deploy := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
	findings := checks[0].Run(bgCtx, []*k8s.Resource{deploy})
	assert.Empty(t, findings, "should not match Deployment")

	sts := makeWorkload("StatefulSet", "db", []map[string]interface{}{{"name": "app"}}, nil)
	findings = checks[0].Run(bgCtx, []*k8s.Resource{sts})
	assert.NotEmpty(t, findings, "should match StatefulSet")
}

func TestCustomRuleHostNetworking(t *testing.T) {
	pf := &audit.PolicyFile{
		Rules: []audit.PolicyRule{{
			ID: "C-002", SeverityStr: "high",
			Condition: "host networking", Message: "bad",
		}},
	}
	checks := pf.ToChecks()

	t.Run("matches", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		setPodSpecField(res, "hostNetwork", true)
		findings := checks[0].Run(bgCtx, []*k8s.Resource{res})
		assert.NotEmpty(t, findings)
	})

	t.Run("no match", func(t *testing.T) {
		res := makeDeployment("web", []map[string]interface{}{{"name": "app"}})
		findings := checks[0].Run(bgCtx, []*k8s.Resource{res})
		assert.Empty(t, findings)
	})
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
