// Package chartmeta provides a convenient wrapper around Helm chart metadata.
package chartmeta

import (
	"helm.sh/helm/v3/pkg/chart"
)

// DependencyMeta describes a chart dependency.
type DependencyMeta struct {
	Name       string
	Version    string
	Repository string
	Condition  string
	Tags       []string
}

// ChartMeta wraps key metadata extracted from a loaded Helm chart.
type ChartMeta struct {
	Name         string
	Version      string
	AppVersion   string
	Description  string
	Type         string
	Dependencies []DependencyMeta
	Values       map[string]interface{}
	Schema       []byte
}

// FromChart extracts metadata from a loaded Helm chart.
func FromChart(ch *chart.Chart) *ChartMeta {
	if ch == nil || ch.Metadata == nil {
		return &ChartMeta{}
	}

	meta := &ChartMeta{
		Name:        ch.Metadata.Name,
		Version:     ch.Metadata.Version,
		AppVersion:  ch.Metadata.AppVersion,
		Description: ch.Metadata.Description,
		Type:        ch.Metadata.Type,
		Values:      ch.Values,
		Schema:      ch.Schema,
	}

	for _, dep := range ch.Metadata.Dependencies {
		meta.Dependencies = append(meta.Dependencies, DependencyMeta{
			Name:       dep.Name,
			Version:    dep.Version,
			Repository: dep.Repository,
			Condition:  dep.Condition,
			Tags:       dep.Tags,
		})
	}

	return meta
}

// IsLibrary returns true if the chart is of type "library".
// Library charts cannot be installed and chart2kro rejects them.
func (m *ChartMeta) IsLibrary() bool {
	return m.Type == "library"
}

// HasSchema returns true if the chart has a values.schema.json.
func (m *ChartMeta) HasSchema() bool {
	return len(m.Schema) > 0
}

// HasDependencies returns true if the chart declares any dependencies.
func (m *ChartMeta) HasDependencies() bool {
	return len(m.Dependencies) > 0
}

// DependencyNames returns the names of all declared dependencies.
func (m *ChartMeta) DependencyNames() []string {
	names := make([]string, 0, len(m.Dependencies))

	for _, dep := range m.Dependencies {
		names = append(names, dep.Name)
	}

	return names
}
