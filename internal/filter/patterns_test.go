package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/helm/chartmeta"
)

func TestDetectExternalPatterns_PostgreSQL(t *testing.T) {
	meta := &chartmeta.ChartMeta{
		Dependencies: []chartmeta.DependencyMeta{
			{Name: "postgresql", Condition: "postgresql.enabled"},
		},
	}

	values := map[string]interface{}{
		"externalDatabase": map[string]interface{}{
			"host":           "my-db.example.com",
			"port":           5432,
			"existingSecret": "my-db-secret",
		},
	}

	patterns := DetectExternalPatterns(meta, values)
	require.Len(t, patterns, 1)

	p := patterns[0]
	assert.Equal(t, "postgresql", p.SubchartName)
	assert.Equal(t, "postgresql.enabled", p.Condition)
	assert.Equal(t, "externalDatabase", p.ExternalValuesKey)
	assert.Contains(t, p.DetectedFields, "host")
	assert.Contains(t, p.DetectedFields, "port")
	assert.Contains(t, p.DetectedFields, "existingSecret")
}

func TestDetectExternalPatterns_Redis(t *testing.T) {
	meta := &chartmeta.ChartMeta{
		Dependencies: []chartmeta.DependencyMeta{
			{Name: "redis", Condition: "redis.enabled"},
		},
	}

	values := map[string]interface{}{
		"externalRedis": map[string]interface{}{
			"host": "redis.example.com",
		},
	}

	patterns := DetectExternalPatterns(meta, values)
	require.Len(t, patterns, 1)
	assert.Equal(t, "redis", patterns[0].SubchartName)
	assert.Equal(t, "externalRedis", patterns[0].ExternalValuesKey)
}

func TestDetectExternalPatterns_NoCondition(t *testing.T) {
	meta := &chartmeta.ChartMeta{
		Dependencies: []chartmeta.DependencyMeta{
			{Name: "common", Condition: ""},
		},
	}

	values := map[string]interface{}{
		"externalCommon": map[string]interface{}{"foo": "bar"},
	}

	patterns := DetectExternalPatterns(meta, values)
	assert.Empty(t, patterns)
}

func TestDetectExternalPatterns_NoExternalValues(t *testing.T) {
	meta := &chartmeta.ChartMeta{
		Dependencies: []chartmeta.DependencyMeta{
			{Name: "postgresql", Condition: "postgresql.enabled"},
		},
	}

	values := map[string]interface{}{
		"postgresql": map[string]interface{}{"enabled": true},
	}

	patterns := DetectExternalPatterns(meta, values)
	assert.Empty(t, patterns)
}

func TestDetectExternalPatterns_UnknownSubchart(t *testing.T) {
	meta := &chartmeta.ChartMeta{
		Dependencies: []chartmeta.DependencyMeta{
			{Name: "myservice", Condition: "myservice.enabled"},
		},
	}

	values := map[string]interface{}{
		"externalMyservice": map[string]interface{}{
			"host": "example.com",
		},
	}

	patterns := DetectExternalPatterns(meta, values)
	require.Len(t, patterns, 1)
	assert.Equal(t, "externalMyservice", patterns[0].ExternalValuesKey)
}

func TestDetectExternalPatternsForSubchart(t *testing.T) {
	meta := &chartmeta.ChartMeta{
		Dependencies: []chartmeta.DependencyMeta{
			{Name: "postgresql", Condition: "postgresql.enabled"},
			{Name: "redis", Condition: "redis.enabled"},
		},
	}

	values := map[string]interface{}{
		"externalDatabase": map[string]interface{}{"host": "db.example.com"},
		"externalRedis":    map[string]interface{}{"host": "redis.example.com"},
	}

	p, found := DetectExternalPatternsForSubchart(meta, values, "postgresql")
	assert.True(t, found)
	assert.Equal(t, "postgresql", p.SubchartName)

	_, found = DetectExternalPatternsForSubchart(meta, values, "nonexistent")
	assert.False(t, found)
}

func TestBuildFiltersFromPattern(t *testing.T) {
	pattern := DetectedPattern{
		SubchartName:      "postgresql",
		Condition:         "postgresql.enabled",
		ExternalValuesKey: "externalDatabase",
		DetectedFields:    []string{"host", "port", "existingSecret", "database"},
	}

	filters := BuildFiltersFromPattern(pattern)

	// Should have a SubchartFilter and optionally an ExternalRefFilter.
	assert.GreaterOrEqual(t, len(filters), 1)

	// First filter should be the SubchartFilter.
	_, isSubchart := filters[0].(*SubchartFilter)
	assert.True(t, isSubchart, "first filter should be SubchartFilter")
}

func TestBuildFiltersFromPattern_WithExistingSecret(t *testing.T) {
	pattern := DetectedPattern{
		SubchartName:      "postgresql",
		Condition:         "postgresql.enabled",
		ExternalValuesKey: "externalDatabase",
		DetectedFields:    []string{"host", "port", "existingSecret"},
	}

	filters := BuildFiltersFromPattern(pattern)
	assert.Len(t, filters, 2, "should have SubchartFilter + ExternalRefFilter")

	_, isExtRef := filters[1].(*ExternalRefFilter)
	assert.True(t, isExtRef, "second filter should be ExternalRefFilter")
}

func TestPascalCase(t *testing.T) {
	assert.Equal(t, "Postgresql", pascalCase("postgresql"))
	assert.Equal(t, "Redis", pascalCase("redis"))
	assert.Equal(t, "", pascalCase(""))
	assert.Equal(t, "A", pascalCase("a"))
}
