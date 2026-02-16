package audit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/audit"
	"github.com/hupe1980/chart2kro/internal/harden"
	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestSeverity_String(t *testing.T) {
	tests := []struct {
		sev  audit.Severity
		want string
	}{
		{audit.SeverityInfo, "info"},
		{audit.SeverityLow, "low"},
		{audit.SeverityMedium, "medium"},
		{audit.SeverityHigh, "high"},
		{audit.SeverityCritical, "critical"},
		{audit.Severity(99), "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.sev.String())
		})
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		input   string
		want    audit.Severity
		wantErr bool
	}{
		{"critical", audit.SeverityCritical, false},
		{"CRITICAL", audit.SeverityCritical, false},
		{"  High  ", audit.SeverityHigh, false},
		{"medium", audit.SeverityMedium, false},
		{"low", audit.SeverityLow, false},
		{"info", audit.SeverityInfo, false},
		{"", audit.SeverityInfo, true},
		{"unknown", audit.SeverityInfo, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := audit.ParseSeverity(tt.input)
			assert.Equal(t, tt.want, got)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResult_Passed(t *testing.T) {
	t.Run("no findings passes any threshold", func(t *testing.T) {
		r := &audit.Result{Summary: map[string]int{}}
		assert.True(t, r.Passed(audit.SeverityCritical))
		assert.True(t, r.Passed(audit.SeverityInfo))
	})

	t.Run("findings below threshold passes", func(t *testing.T) {
		r := &audit.Result{
			Findings: []audit.Finding{
				{Severity: audit.SeverityLow, RuleID: "TEST-001"},
				{Severity: audit.SeverityInfo, RuleID: "TEST-002"},
			},
			Summary: map[string]int{"low": 1, "info": 1},
		}
		assert.True(t, r.Passed(audit.SeverityMedium))
		assert.True(t, r.Passed(audit.SeverityHigh))
	})

	t.Run("findings at threshold fails", func(t *testing.T) {
		r := &audit.Result{
			Findings: []audit.Finding{
				{Severity: audit.SeverityHigh, RuleID: "TEST-001"},
			},
			Summary: map[string]int{"high": 1},
		}
		assert.False(t, r.Passed(audit.SeverityHigh))
	})

	t.Run("findings above threshold fails", func(t *testing.T) {
		r := &audit.Result{
			Findings: []audit.Finding{
				{Severity: audit.SeverityCritical, RuleID: "TEST-001"},
			},
			Summary: map[string]int{"critical": 1},
		}
		assert.False(t, r.Passed(audit.SeverityHigh))
	})
}

type fakeCheck struct {
	id       string
	findings []audit.Finding
}

func (f *fakeCheck) ID() string { return f.id }

func (f *fakeCheck) Run(_ context.Context, _ []*k8s.Resource) []audit.Finding {
	return f.findings
}

func TestAuditor_Run(t *testing.T) {
	t.Run("no checks produces empty result", func(t *testing.T) {
		a := audit.New()
		r := a.Run(context.Background(), nil)
		assert.Empty(t, r.Findings)
		assert.Empty(t, r.Summary)
	})

	t.Run("combines findings from multiple checks", func(t *testing.T) {
		c1 := &fakeCheck{id: "A", findings: []audit.Finding{
			{RuleID: "A", Severity: audit.SeverityHigh, Message: "a"},
		}}
		c2 := &fakeCheck{id: "B", findings: []audit.Finding{
			{RuleID: "B", Severity: audit.SeverityLow, Message: "b"},
		}}
		a := audit.New(c1, c2)
		r := a.Run(context.Background(), nil)
		require.Len(t, r.Findings, 2)
		assert.Equal(t, "A", r.Findings[0].RuleID)
		assert.Equal(t, "B", r.Findings[1].RuleID)
	})

	t.Run("sorts by severity desc then ruleID asc", func(t *testing.T) {
		c := &fakeCheck{id: "X", findings: []audit.Finding{
			{RuleID: "C", Severity: audit.SeverityMedium},
			{RuleID: "A", Severity: audit.SeverityCritical},
			{RuleID: "B", Severity: audit.SeverityCritical},
			{RuleID: "D", Severity: audit.SeverityInfo},
		}}
		a := audit.New(c)
		r := a.Run(context.Background(), nil)
		require.Len(t, r.Findings, 4)
		assert.Equal(t, "A", r.Findings[0].RuleID)
		assert.Equal(t, "B", r.Findings[1].RuleID)
		assert.Equal(t, "C", r.Findings[2].RuleID)
		assert.Equal(t, "D", r.Findings[3].RuleID)
	})

	t.Run("summary counts correctly", func(t *testing.T) {
		c := &fakeCheck{id: "X", findings: []audit.Finding{
			{RuleID: "A", Severity: audit.SeverityHigh},
			{RuleID: "B", Severity: audit.SeverityHigh},
			{RuleID: "C", Severity: audit.SeverityLow},
		}}
		a := audit.New(c)
		r := a.Run(context.Background(), nil)
		assert.Equal(t, 2, r.Summary["high"])
		assert.Equal(t, 1, r.Summary["low"])
	})
}

func TestDefaultChecks(t *testing.T) {
	t.Run("restricted returns all 12 checks", func(t *testing.T) {
		checks := audit.DefaultChecks(harden.SecurityLevelRestricted)
		assert.Len(t, checks, 12)
		ids := make(map[string]bool)
		for _, c := range checks {
			ids[c.ID()] = true
		}
		for _, id := range []string{
			"SEC-001", "SEC-002", "SEC-003", "SEC-004", "SEC-005", "SEC-006",
			"SEC-007", "SEC-008", "SEC-009", "SEC-010", "SEC-011", "SEC-012",
		} {
			assert.True(t, ids[id], "missing check %s", id)
		}
	})

	t.Run("baseline excludes restricted-only checks", func(t *testing.T) {
		checks := audit.DefaultChecks(harden.SecurityLevelBaseline)
		ids := make(map[string]bool)
		for _, c := range checks {
			ids[c.ID()] = true
		}
		// Baseline should include PSS baseline + best-practice checks.
		assert.True(t, ids["SEC-001"], "should include RunAsRoot (baseline)")
		assert.True(t, ids["SEC-002"], "should include Privileged (baseline)")
		assert.True(t, ids["SEC-003"], "should include ResourceLimits (best-practice)")
		assert.True(t, ids["SEC-005"], "should include HostNamespace (baseline)")
		assert.True(t, ids["SEC-008"], "should include DangerousCaps (baseline)")
		// Restricted-only checks should be excluded.
		assert.False(t, ids["SEC-006"], "should exclude ReadOnlyRootFS (restricted)")
		assert.False(t, ids["SEC-012"], "should exclude SeccompProfile (restricted)")
		assert.Len(t, checks, 10)
	})

	t.Run("none returns only best-practice checks", func(t *testing.T) {
		checks := audit.DefaultChecks(harden.SecurityLevelNone)
		ids := make(map[string]bool)
		for _, c := range checks {
			ids[c.ID()] = true
		}
		// Only best-practice (non-PSS) checks.
		assert.True(t, ids["SEC-003"], "should include ResourceLimits")
		assert.True(t, ids["SEC-004"], "should include LatestTag")
		assert.True(t, ids["SEC-007"], "should include NetworkPolicy")
		assert.True(t, ids["SEC-009"], "should include BroadSelector")
		assert.True(t, ids["SEC-010"], "should include Probes")
		assert.True(t, ids["SEC-011"], "should include IngressTLS")
		// PSS checks should be excluded.
		assert.False(t, ids["SEC-001"], "should exclude RunAsRoot")
		assert.False(t, ids["SEC-002"], "should exclude Privileged")
		assert.False(t, ids["SEC-005"], "should exclude HostNamespace")
		assert.Len(t, checks, 6)
	})
}

// --- Test helpers shared across test files ---

func makeDeployment(name string, containers []map[string]interface{}) *k8s.Resource {
	return makeWorkload("Deployment", name, containers, nil)
}

func makeWorkload(kind, name string, containers, initContainers []map[string]interface{}) *k8s.Resource {
	podSpec := map[string]interface{}{
		"containers": toSlice(containers),
	}
	if len(initContainers) > 0 {
		podSpec["initContainers"] = toSlice(initContainers)
	}
	obj := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{"spec": podSpec},
		},
	}
	return &k8s.Resource{
		GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind},
		Name:   name,
		Object: &unstructured.Unstructured{Object: obj},
	}
}

func makeCronJob(name string, containers []map[string]interface{}) *k8s.Resource {
	podSpec := map[string]interface{}{
		"containers": toSlice(containers),
	}
	obj := map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata":   map[string]interface{}{"name": name},
		"spec": map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{"spec": podSpec},
				},
			},
		},
	}
	return &k8s.Resource{
		GVK:    schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
		Name:   name,
		Object: &unstructured.Unstructured{Object: obj},
	}
}

func makeService(name string, selector map[string]interface{}) *k8s.Resource {
	obj := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]interface{}{"name": name},
		"spec":       map[string]interface{}{"selector": selector},
	}
	return &k8s.Resource{
		GVK:    schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Name:   name,
		Object: &unstructured.Unstructured{Object: obj},
	}
}

func makeIngress(name string, hasTLS bool) *k8s.Resource {
	spec := map[string]interface{}{}
	if hasTLS {
		spec["tls"] = []interface{}{map[string]interface{}{"secretName": "tls-secret"}}
	}
	obj := map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "Ingress",
		"metadata":   map[string]interface{}{"name": name},
		"spec":       spec,
	}
	return &k8s.Resource{
		GVK:    schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
		Name:   name,
		Object: &unstructured.Unstructured{Object: obj},
	}
}

func makeNetworkPolicy(name string) *k8s.Resource {
	return &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
		Name: name,
		Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata":   map[string]interface{}{"name": name},
		}},
	}
}

func toSlice(m []map[string]interface{}) []interface{} {
	result := make([]interface{}, len(m))
	for i, v := range m {
		result[i] = v
	}
	return result
}

func setPodSpecField(res *k8s.Resource, key string, value interface{}) {
	spec := res.Object.Object["spec"].(map[string]interface{})
	tpl := spec["template"].(map[string]interface{})
	ps := tpl["spec"].(map[string]interface{})
	ps[key] = value
}
