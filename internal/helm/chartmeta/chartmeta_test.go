package chartmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/chart"
)

func TestFromChart_Basic(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:        "myapp",
			Version:     "1.2.3",
			AppVersion:  "4.5.6",
			Description: "A test chart",
			Type:        "application",
		},
		Values: map[string]interface{}{
			"replicas": 3,
		},
	}

	meta := FromChart(ch)
	assert.Equal(t, "myapp", meta.Name)
	assert.Equal(t, "1.2.3", meta.Version)
	assert.Equal(t, "4.5.6", meta.AppVersion)
	assert.Equal(t, "A test chart", meta.Description)
	assert.Equal(t, "application", meta.Type)
	assert.False(t, meta.IsLibrary())
	assert.Equal(t, 3, meta.Values["replicas"])
}

func TestFromChart_NilChart(t *testing.T) {
	meta := FromChart(nil)
	assert.Equal(t, "", meta.Name)
	assert.Empty(t, meta.Dependencies)
	assert.False(t, meta.IsLibrary())
}

func TestFromChart_NilMetadata(t *testing.T) {
	ch := &chart.Chart{}
	meta := FromChart(ch)
	assert.Equal(t, "", meta.Name)
}

func TestFromChart_WithDependencies(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Dependencies: []*chart.Dependency{
				{
					Name:       "postgresql",
					Version:    "12.0.0",
					Repository: "https://charts.bitnami.com/bitnami",
					Condition:  "postgresql.enabled",
					Tags:       []string{"database"},
				},
				{
					Name:       "redis",
					Version:    "17.0.0",
					Repository: "https://charts.bitnami.com/bitnami",
				},
			},
		},
	}

	meta := FromChart(ch)
	assert.True(t, meta.HasDependencies())
	assert.Len(t, meta.Dependencies, 2)
	assert.Equal(t, "postgresql", meta.Dependencies[0].Name)
	assert.Equal(t, "12.0.0", meta.Dependencies[0].Version)
	assert.Equal(t, "https://charts.bitnami.com/bitnami", meta.Dependencies[0].Repository)
	assert.Equal(t, "postgresql.enabled", meta.Dependencies[0].Condition)
	assert.Equal(t, []string{"database"}, meta.Dependencies[0].Tags)
	assert.Equal(t, "redis", meta.Dependencies[1].Name)
	assert.Equal(t, []string{"postgresql", "redis"}, meta.DependencyNames())
}

func TestFromChart_NoDependencies(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "simple",
			Version: "1.0.0",
		},
	}

	meta := FromChart(ch)
	assert.False(t, meta.HasDependencies())
	assert.Empty(t, meta.DependencyNames())
}

func TestIsLibrary(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "mylib",
			Version: "1.0.0",
			Type:    "library",
		},
	}

	meta := FromChart(ch)
	assert.True(t, meta.IsLibrary())
}

func TestIsLibrary_ApplicationType(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
			Type:    "application",
		},
	}

	meta := FromChart(ch)
	assert.False(t, meta.IsLibrary())
}

func TestIsLibrary_EmptyType(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
		},
	}

	meta := FromChart(ch)
	assert.False(t, meta.IsLibrary())
}

func TestHasSchema(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
		},
		Schema: []byte(`{"type": "object"}`),
	}

	meta := FromChart(ch)
	assert.True(t, meta.HasSchema())
}

func TestHasSchema_NoSchema(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "myapp",
			Version: "1.0.0",
		},
	}

	meta := FromChart(ch)
	assert.False(t, meta.HasSchema())
}

func TestDependencyNames_Empty(t *testing.T) {
	meta := &ChartMeta{}
	assert.Empty(t, meta.DependencyNames())
}
