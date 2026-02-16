package audit_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/audit"
)

func sampleResult() *audit.Result {
	return &audit.Result{
		Findings: []audit.Finding{
			{RuleID: "SEC-001", Severity: audit.SeverityCritical,
				ResourceID: "Deployment/web", ResourceKind: "Deployment",
				Message: "container app does not set runAsNonRoot: true", Remediation: "Set runAsNonRoot"},
			{RuleID: "SEC-004", Severity: audit.SeverityHigh,
				ResourceID: "Deployment/web", ResourceKind: "Deployment",
				Message: "container app uses :latest tag (nginx:latest)", Remediation: "Pin version"},
		},
		Summary: map[string]int{"critical": 1, "high": 1},
	}
}

func TestNewFormatter(t *testing.T) {
	for _, format := range []string{"", "table", "TABLE", "json", "JSON", "sarif", "SARIF"} {
		f, err := audit.NewFormatter(format)
		assert.NoError(t, err, format)
		assert.NotNil(t, f, format)
	}
	f, err := audit.NewFormatter("xml")
	assert.Error(t, err)
	assert.Nil(t, f)
}

func TestTableFormatter(t *testing.T) {
	var buf bytes.Buffer
	f := &audit.TableFormatter{}
	require.NoError(t, f.Format(&buf, sampleResult()))
	out := buf.String()
	assert.Contains(t, out, "SEVERITY")
	assert.Contains(t, out, "CRITICAL")
	assert.Contains(t, out, "SEC-001")
	assert.Contains(t, out, "Deployment/web")
	assert.Contains(t, out, "Findings: 2 total")
	assert.Contains(t, out, "1 critical")
	assert.Contains(t, out, "1 high")
}

func TestTableFormatter_Empty(t *testing.T) {
	var buf bytes.Buffer
	f := &audit.TableFormatter{}
	require.NoError(t, f.Format(&buf, &audit.Result{Summary: map[string]int{}}))
	assert.Contains(t, buf.String(), "Findings: 0 total")
}

func TestJSONFormatter(t *testing.T) {
	var buf bytes.Buffer
	f := &audit.JSONFormatter{}
	require.NoError(t, f.Format(&buf, sampleResult()))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	findings := result["findings"].([]interface{})
	assert.Len(t, findings, 2)
	assert.Equal(t, float64(2), result["total"])
	first := findings[0].(map[string]interface{})
	assert.Equal(t, "critical", first["severity"])
}

func TestJSONFormatter_Empty(t *testing.T) {
	var buf bytes.Buffer
	f := &audit.JSONFormatter{}
	require.NoError(t, f.Format(&buf, &audit.Result{Summary: nil}))
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, float64(0), result["total"])
}

func TestSARIFFormatter(t *testing.T) {
	var buf bytes.Buffer
	f := &audit.SARIFFormatter{}
	require.NoError(t, f.Format(&buf, sampleResult()))

	var sarif map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &sarif))

	assert.Equal(t, "2.1.0", sarif["version"])
	assert.True(t, strings.Contains(sarif["$schema"].(string), "sarif"))

	runs := sarif["runs"].([]interface{})
	require.Len(t, runs, 1)
	run := runs[0].(map[string]interface{})
	tool := run["tool"].(map[string]interface{})
	driver := tool["driver"].(map[string]interface{})
	assert.Equal(t, "chart2kro-audit", driver["name"])

	rules := driver["rules"].([]interface{})
	assert.Len(t, rules, 2)

	results := run["results"].([]interface{})
	assert.Len(t, results, 2)
	first := results[0].(map[string]interface{})
	assert.Equal(t, "error", first["level"])
	assert.Equal(t, "SEC-001", first["ruleId"])
}

func TestSARIFFormatter_Empty(t *testing.T) {
	var buf bytes.Buffer
	f := &audit.SARIFFormatter{}
	require.NoError(t, f.Format(&buf, &audit.Result{Summary: map[string]int{}}))
	var sarif map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &sarif))
	assert.Equal(t, "2.1.0", sarif["version"])
}

func TestSARIFFormatter_SeverityMapping(t *testing.T) {
	result := &audit.Result{
		Findings: []audit.Finding{
			{RuleID: "C-001", Severity: audit.SeverityMedium, ResourceID: "Deploy/a", ResourceKind: "Deployment", Message: "medium issue"},
			{RuleID: "C-002", Severity: audit.SeverityLow, ResourceID: "Deploy/b", ResourceKind: "Deployment", Message: "low issue"},
			{RuleID: "C-003", Severity: audit.SeverityInfo, ResourceID: "Deploy/c", ResourceKind: "Deployment", Message: "info issue"},
		},
		Summary: map[string]int{"medium": 1, "low": 1, "info": 1},
	}

	var buf bytes.Buffer
	f := &audit.SARIFFormatter{}
	require.NoError(t, f.Format(&buf, result))

	var sarif map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &sarif))

	runs := sarif["runs"].([]interface{})
	run := runs[0].(map[string]interface{})
	results := run["results"].([]interface{})
	require.Len(t, results, 3)

	assert.Equal(t, "warning", results[0].(map[string]interface{})["level"])
	assert.Equal(t, "note", results[1].(map[string]interface{})["level"])
	assert.Equal(t, "note", results[2].(map[string]interface{})["level"])
}

func TestTableFormatter_SummaryOrder(t *testing.T) {
	result := &audit.Result{
		Findings: []audit.Finding{
			{RuleID: "SEC-010", Severity: audit.SeverityLow, ResourceID: "Deploy/x", ResourceKind: "Deployment", Message: "low"},
			{RuleID: "SEC-003", Severity: audit.SeverityHigh, ResourceID: "Deploy/x", ResourceKind: "Deployment", Message: "high"},
			{RuleID: "SEC-006", Severity: audit.SeverityMedium, ResourceID: "Deploy/x", ResourceKind: "Deployment", Message: "medium"},
		},
		Summary: map[string]int{"high": 1, "medium": 1, "low": 1},
	}

	var buf bytes.Buffer
	f := &audit.TableFormatter{}
	require.NoError(t, f.Format(&buf, result))

	out := buf.String()
	assert.Contains(t, out, "Findings: 3 total")
	assert.Contains(t, out, "HIGH")
	assert.Contains(t, out, "MEDIUM")
	assert.Contains(t, out, "LOW")
}
