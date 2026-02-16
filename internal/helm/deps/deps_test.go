package deps

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/chart"
)

func testLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestAnalyze_NoDependencies(t *testing.T) {
	logger, _ := testLogger()
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "simple",
			Version: "1.0.0",
		},
	}

	result := Analyze(ch, logger)
	assert.True(t, result.AllResolved)
	assert.Empty(t, result.Dependencies)
}

func TestAnalyze_AllVendored(t *testing.T) {
	logger, _ := testLogger()
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Dependencies: []*chart.Dependency{
				{Name: "postgresql", Version: "12.0.0", Repository: "https://charts.bitnami.com/bitnami"},
			},
		},
	}
	ch.AddDependency(&chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "postgresql",
			Version: "12.0.0",
		},
	})

	result := Analyze(ch, logger)
	assert.True(t, result.AllResolved)
	assert.Len(t, result.Dependencies, 1)
	assert.Equal(t, StatusOK, result.Dependencies[0].Status)
	assert.Equal(t, "12.0.0", result.Dependencies[0].Actual)
}

func TestAnalyze_MissingDependency(t *testing.T) {
	logger, logBuf := testLogger()
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Dependencies: []*chart.Dependency{
				{Name: "redis", Version: "17.0.0", Repository: "https://charts.bitnami.com/bitnami"},
			},
		},
	}

	result := Analyze(ch, logger)
	assert.False(t, result.AllResolved)
	assert.Len(t, result.Dependencies, 1)
	assert.Equal(t, StatusMissing, result.Dependencies[0].Status)
	assert.Contains(t, logBuf.String(), "subchart dependency not vendored")
}

func TestAnalyze_VersionMismatch(t *testing.T) {
	logger, logBuf := testLogger()
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Dependencies: []*chart.Dependency{
				{Name: "postgresql", Version: "12.0.0"},
			},
		},
	}
	ch.AddDependency(&chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "postgresql",
			Version: "11.9.0",
		},
	})

	result := Analyze(ch, logger)
	assert.True(t, result.AllResolved) // Still resolved, just mismatched.
	assert.Equal(t, StatusVersionMismatch, result.Dependencies[0].Status)
	assert.Equal(t, "11.9.0", result.Dependencies[0].Actual)
	assert.Contains(t, logBuf.String(), "subchart version mismatch")
}

func TestAnalyze_MissingLock(t *testing.T) {
	logger, logBuf := testLogger()
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Dependencies: []*chart.Dependency{
				{Name: "sub", Version: "1.0.0"},
			},
		},
	}
	ch.AddDependency(&chart.Chart{
		Metadata: &chart.Metadata{Name: "sub", Version: "1.0.0"},
	})

	result := Analyze(ch, logger)
	assert.False(t, result.HasLock)
	assert.Contains(t, logBuf.String(), "Chart.lock is missing")
}

func TestAnalyze_WithLock(t *testing.T) {
	logger, logBuf := testLogger()
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Dependencies: []*chart.Dependency{
				{Name: "sub", Version: "1.0.0"},
			},
		},
		Lock: &chart.Lock{
			Dependencies: []*chart.Dependency{
				{Name: "sub", Version: "1.0.0"},
			},
		},
	}
	ch.AddDependency(&chart.Chart{
		Metadata: &chart.Metadata{Name: "sub", Version: "1.0.0"},
	})

	result := Analyze(ch, logger)
	assert.True(t, result.HasLock)
	assert.NotContains(t, logBuf.String(), "Chart.lock is missing")
}

func TestAnalyze_MultipleDependencies(t *testing.T) {
	logger, _ := testLogger()
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Dependencies: []*chart.Dependency{
				{Name: "postgresql", Version: "12.0.0"},
				{Name: "redis", Version: "17.0.0"},
				{Name: "common", Version: "2.0.0"},
			},
		},
	}
	ch.AddDependency(&chart.Chart{
		Metadata: &chart.Metadata{Name: "postgresql", Version: "12.0.0"},
	})
	ch.AddDependency(&chart.Chart{
		Metadata: &chart.Metadata{Name: "common", Version: "2.0.0"},
	})

	result := Analyze(ch, logger)
	assert.False(t, result.AllResolved)
	assert.Len(t, result.Dependencies, 3)
	assert.Equal(t, StatusOK, result.Dependencies[0].Status)
	assert.Equal(t, StatusMissing, result.Dependencies[1].Status)
	assert.Equal(t, StatusOK, result.Dependencies[2].Status)
}

func TestAnalyze_DependencyWithConditionAndTags(t *testing.T) {
	logger, _ := testLogger()
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Dependencies: []*chart.Dependency{
				{Name: "pg", Version: "1.0.0", Condition: "postgresql.enabled", Tags: []string{"database"}},
			},
		},
	}
	ch.AddDependency(&chart.Chart{
		Metadata: &chart.Metadata{Name: "pg", Version: "1.0.0"},
	})

	result := Analyze(ch, logger)
	assert.Equal(t, "postgresql.enabled", result.Dependencies[0].Condition)
	assert.Equal(t, []string{"database"}, result.Dependencies[0].Tags)
}

func TestMissingDependencies(t *testing.T) {
	result := &Result{
		Dependencies: []DependencyInfo{
			{Name: "a", Status: StatusOK},
			{Name: "b", Status: StatusMissing},
			{Name: "c", Status: StatusOK},
			{Name: "d", Status: StatusMissing},
		},
	}
	missing := MissingDependencies(result)
	assert.Equal(t, []string{"b", "d"}, missing)
}

func TestMissingDependencies_NoneAlissing(t *testing.T) {
	result := &Result{
		Dependencies: []DependencyInfo{
			{Name: "a", Status: StatusOK},
		},
	}
	missing := MissingDependencies(result)
	assert.Empty(t, missing)
}

func TestIsEnabled_ConditionTrue(t *testing.T) {
	dep := &chart.Dependency{
		Name:      "postgresql",
		Condition: "postgresql.enabled",
	}
	vals := map[string]interface{}{
		"postgresql": map[string]interface{}{
			"enabled": true,
		},
	}
	assert.True(t, IsEnabled(dep, vals))
}

func TestIsEnabled_ConditionFalse(t *testing.T) {
	dep := &chart.Dependency{
		Name:      "postgresql",
		Condition: "postgresql.enabled",
	}
	vals := map[string]interface{}{
		"postgresql": map[string]interface{}{
			"enabled": false,
		},
	}
	assert.False(t, IsEnabled(dep, vals))
}

func TestIsEnabled_NoCondition(t *testing.T) {
	dep := &chart.Dependency{Name: "sub"}
	vals := map[string]interface{}{}
	assert.True(t, IsEnabled(dep, vals))
}

func TestIsEnabled_ConditionNotFound(t *testing.T) {
	dep := &chart.Dependency{
		Name:      "sub",
		Condition: "sub.enabled",
	}
	vals := map[string]interface{}{}
	// Condition key missing defaults to enabled.
	assert.True(t, IsEnabled(dep, vals))
}

func TestIsEnabled_TagDisabled(t *testing.T) {
	dep := &chart.Dependency{
		Name: "sub",
		Tags: []string{"backend"},
	}
	vals := map[string]interface{}{
		"tags": map[string]interface{}{
			"backend": false,
		},
	}
	assert.False(t, IsEnabled(dep, vals))
}

func TestIsEnabled_TagEnabled(t *testing.T) {
	dep := &chart.Dependency{
		Name: "sub",
		Tags: []string{"backend"},
	}
	vals := map[string]interface{}{
		"tags": map[string]interface{}{
			"backend": true,
		},
	}
	assert.True(t, IsEnabled(dep, vals))
}

func TestVersionSatisfied(t *testing.T) {
	tests := []struct {
		constraint string
		actual     string
		expected   bool
	}{
		// Exact match.
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		// Wildcard / x-range.
		{"1.x", "1.2.3", true},
		{"1.x", "2.0.0", false},
		{"1.2.x", "1.2.5", true},
		{"1.2.x", "1.3.0", false},
		// Tilde range (~): patch-level changes allowed.
		{"~1.2.0", "1.2.0", true},
		{"~1.2.0", "1.2.9", true},
		{"~1.2.0", "1.3.0", false},
		// Caret range (^): minor+patch changes allowed.
		{"^1.0.0", "1.0.0", true},
		{"^1.0.0", "1.9.9", true},
		{"^1.0.0", "2.0.0", false},
		// Comparison operators.
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "2.0.0", true},
		{">=1.0.0", "0.9.0", false},
		{">1.0.0", "1.0.1", true},
		{">1.0.0", "1.0.0", false},
		{"<2.0.0", "1.9.9", true},
		{"<2.0.0", "2.0.0", false},
		// Range with space (AND).
		{">=1.0.0 <2.0.0", "1.5.0", true},
		{">=1.0.0 <2.0.0", "2.0.0", false},
		// OR ranges.
		{"1.0.0 || 2.0.0", "1.0.0", true},
		{"1.0.0 || 2.0.0", "2.0.0", true},
		{"1.0.0 || 2.0.0", "3.0.0", false},
		// Not a valid semver.
		{"2.0.0", "not-semver", false},
		{"not-constraint", "1.0.0", false},
	}
	for _, tc := range tests {
		t.Run(tc.constraint+"_"+tc.actual, func(t *testing.T) {
			assert.Equal(t, tc.expected, versionSatisfied(tc.constraint, tc.actual))
		})
	}
}

func TestLookupValue(t *testing.T) {
	vals := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": true,
			},
		},
		"top": "val",
	}

	v, ok := lookupValue(vals, "a.b.c")
	assert.True(t, ok)
	assert.Equal(t, true, v)

	v, ok = lookupValue(vals, "top")
	assert.True(t, ok)
	assert.Equal(t, "val", v)

	_, ok = lookupValue(vals, "missing")
	assert.False(t, ok)

	_, ok = lookupValue(vals, "a.b.missing")
	assert.False(t, ok)
}

func TestPrintSummary(t *testing.T) {
	logger, logBuf := testLogger()
	result := &Result{
		HasLock: false,
		Dependencies: []DependencyInfo{
			{Name: "pg", Status: StatusOK, Actual: "12.0.0"},
			{Name: "redis", Status: StatusMissing, Version: "17.0.0", Repository: "https://example.com"},
			{Name: "common", Status: StatusVersionMismatch, Version: "2.0.0", Actual: "1.9.0"},
		},
	}

	PrintSummary(result, logger)
	output := logBuf.String()
	assert.Contains(t, output, "dependency resolved")
	assert.Contains(t, output, "dependency missing")
	assert.Contains(t, output, "dependency version mismatch")
	assert.Contains(t, output, "no Chart.lock found")
}

func TestStatus_String(t *testing.T) {
	assert.Equal(t, "ok", string(StatusOK))
	assert.Equal(t, "missing", string(StatusMissing))
	assert.Equal(t, "version-mismatch", string(StatusVersionMismatch))
}
