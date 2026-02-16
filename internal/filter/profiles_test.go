package filter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Profile resolution tests
// ---------------------------------------------------------------------------

func TestResolveProfile_BuiltIn(t *testing.T) {
	tests := []struct {
		name     string
		expected int // number of ExcludeSubcharts
	}{
		{"enterprise", 8},
		{"app-only", 0},
		{"minimal", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ResolveProfile(tt.name, nil)
			require.NoError(t, err)
			assert.Len(t, p.ExcludeSubcharts, tt.expected)
		})
	}
}

func TestResolveProfile_Unknown(t *testing.T) {
	_, err := ResolveProfile("nonexistent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown profile")
}

func TestResolveProfile_Custom(t *testing.T) {
	custom := map[string]ProfileConfig{
		"my-company": {
			ExcludeSubcharts: []string{"internal-db", "internal-cache"},
			ExcludeKinds:     []string{"PodDisruptionBudget"},
		},
	}

	p, err := ResolveProfile("my-company", custom)
	require.NoError(t, err)
	assert.Equal(t, []string{"internal-db", "internal-cache"}, p.ExcludeSubcharts)
	assert.Equal(t, []string{"PodDisruptionBudget"}, p.ExcludeKinds)
}

func TestResolveProfile_CustomExtendsBuiltIn(t *testing.T) {
	custom := map[string]ProfileConfig{
		"my-enterprise": {
			Extends:          "enterprise",
			ExcludeSubcharts: []string{"custom-db"},
			ExcludeKinds:     []string{"Job"},
		},
	}

	p, err := ResolveProfile("my-enterprise", custom)
	require.NoError(t, err)

	// Should contain enterprise subcharts + custom-db.
	assert.Len(t, p.ExcludeSubcharts, 9) // 8 enterprise + 1 custom
	assert.Contains(t, p.ExcludeSubcharts, "postgresql")
	assert.Contains(t, p.ExcludeSubcharts, "custom-db")
	assert.Equal(t, []string{"Job"}, p.ExcludeKinds)
}

func TestResolveProfile_CustomExtendsUnknown(t *testing.T) {
	custom := map[string]ProfileConfig{
		"broken": {Extends: "nonexistent"},
	}

	_, err := ResolveProfile("broken", custom)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "extends unknown profile")
}

// ---------------------------------------------------------------------------
// Profile filter building tests
// ---------------------------------------------------------------------------

func TestBuildFiltersFromProfile_Enterprise(t *testing.T) {
	p, err := ResolveProfile("enterprise", nil)
	require.NoError(t, err)

	filters, err := BuildFiltersFromProfile(p)
	require.NoError(t, err)
	require.Len(t, filters, 1)

	_, isSubchart := filters[0].(*SubchartFilter)
	assert.True(t, isSubchart)
}

func TestBuildFiltersFromProfile_AppOnly(t *testing.T) {
	p, err := ResolveProfile("app-only", nil)
	require.NoError(t, err)

	filters, err := BuildFiltersFromProfile(p)
	require.NoError(t, err)
	require.Len(t, filters, 1)

	_, isKind := filters[0].(*KindFilter)
	assert.True(t, isKind)
}

func TestBuildFiltersFromProfile_WithExternalResources(t *testing.T) {
	p := ProfileConfig{
		ExcludeSubcharts:  []string{"postgresql"},
		ExternalResources: []string{"Secret:my-db-secret=ext.secretName"},
	}

	filters, err := BuildFiltersFromProfile(p)
	require.NoError(t, err)
	assert.Len(t, filters, 2)
}

func TestBuildFiltersFromProfile_InvalidExternalResource(t *testing.T) {
	p := ProfileConfig{
		ExternalResources: []string{"no-colon-here"},
	}

	_, err := BuildFiltersFromProfile(p)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// MinimalFilter tests
// ---------------------------------------------------------------------------

func TestMinimalFilter_ExcludesAllSubcharts(t *testing.T) {
	resources := makeResources(
		makeResourceWithSource("Deployment", "app", "chart/templates/deploy.yaml"),
		makeResourceWithSource("StatefulSet", "pg", "chart/charts/postgresql/templates/sts.yaml"),
		makeResourceWithSource("Service", "redis-svc", "chart/charts/redis/templates/svc.yaml"),
		makeResourceWithSource("Service", "app-svc", "chart/templates/service.yaml"),
	)

	f := NewMinimalFilter()
	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)

	assert.Len(t, result.Included, 2)
	assert.Equal(t, "app", result.Included[0].Name)
	assert.Equal(t, "app-svc", result.Included[1].Name)
	assert.Len(t, result.Excluded, 2)
}

func TestMinimalFilter_NoSubcharts(t *testing.T) {
	resources := makeResources(
		makeResourceWithSource("Deployment", "app", "chart/templates/deploy.yaml"),
	)

	f := NewMinimalFilter()
	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)

	assert.Len(t, result.Included, 1)
	assert.Empty(t, result.Excluded)
}

// ---------------------------------------------------------------------------
// ParseCustomProfiles tests
// ---------------------------------------------------------------------------

func TestParseCustomProfiles(t *testing.T) {
	yamlData := []byte(`
profiles:
  my-team:
    excludeSubcharts:
      - internal-db
    excludeKinds:
      - PodDisruptionBudget
  production:
    extends: enterprise
    excludeSubcharts:
      - monitoring
`)

	profiles, err := ParseCustomProfiles(yamlData)
	require.NoError(t, err)
	require.Len(t, profiles, 2)

	assert.Equal(t, []string{"internal-db"}, profiles["my-team"].ExcludeSubcharts)
	assert.Equal(t, "enterprise", profiles["production"].Extends)
}

func TestParseCustomProfiles_NoProfilesKey(t *testing.T) {
	yamlData := []byte(`
log-level: debug
`)

	profiles, err := ParseCustomProfiles(yamlData)
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestParseCustomProfiles_InvalidYAML(t *testing.T) {
	_, err := ParseCustomProfiles([]byte(`{invalid`))
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// BuiltinProfileNames test
// ---------------------------------------------------------------------------

func TestBuiltinProfileNames(t *testing.T) {
	names := BuiltinProfileNames()
	assert.Len(t, names, 3)
	assert.Contains(t, names, "enterprise")
	assert.Contains(t, names, "minimal")
	assert.Contains(t, names, "app-only")
}

// ---------------------------------------------------------------------------
// IsMinimalProfile test
// ---------------------------------------------------------------------------

func TestIsMinimalProfile(t *testing.T) {
	assert.True(t, IsMinimalProfile("minimal"))
	assert.False(t, IsMinimalProfile("enterprise"))
	assert.False(t, IsMinimalProfile(""))
}
