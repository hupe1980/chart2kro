package harden

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateProvenanceAnnotations_Basic(t *testing.T) {
	cfg := ProvenanceConfig{
		ChartRef:       "oci://registry.example.com/my-chart:1.0.0",
		ChartDigest:    "abc123def456",
		HardeningLevel: SecurityLevelRestricted,
		EmbedTimestamp: false,
	}

	annotations, err := GenerateProvenanceAnnotations(cfg)
	require.NoError(t, err)

	assert.Equal(t, "oci://registry.example.com/my-chart:1.0.0", annotations["chart2kro.io/source"])
	assert.NotEmpty(t, annotations["chart2kro.io/generator-version"])
	assert.Equal(t, "abc123def456", annotations["chart2kro.io/chart-digest"])
	assert.Equal(t, "restricted", annotations["chart2kro.io/hardening-level"])
	assert.NotEmpty(t, annotations["chart2kro.io/provenance"])

	// Verify no timestamp when EmbedTimestamp is false.
	_, hasTimestamp := annotations["chart2kro.io/generated-at"]
	assert.False(t, hasTimestamp, "should not have timestamp when EmbedTimestamp is false")
}

func TestGenerateProvenanceAnnotations_WithTimestamp(t *testing.T) {
	cfg := ProvenanceConfig{
		ChartRef:       "my-chart",
		HardeningLevel: SecurityLevelBaseline,
		EmbedTimestamp: true,
	}

	annotations, err := GenerateProvenanceAnnotations(cfg)
	require.NoError(t, err)

	assert.NotEmpty(t, annotations["chart2kro.io/generated-at"])
}

func TestGenerateProvenanceAnnotations_WithProfile(t *testing.T) {
	cfg := ProvenanceConfig{
		ChartRef: "my-chart",
		Profile:  "enterprise",
	}

	annotations, err := GenerateProvenanceAnnotations(cfg)
	require.NoError(t, err)

	assert.Equal(t, "enterprise", annotations["chart2kro.io/profile"])
}

func TestGenerateProvenanceAnnotations_ExcludedSubcharts(t *testing.T) {
	cfg := ProvenanceConfig{
		ChartRef:          "my-chart",
		ExcludedSubcharts: []string{"redis", "postgresql"},
	}

	annotations, err := GenerateProvenanceAnnotations(cfg)
	require.NoError(t, err)

	assert.Equal(t, "redis,postgresql", annotations["chart2kro.io/excluded-subcharts"])
}

func TestGenerateProvenanceAnnotations_NoHardeningLevel(t *testing.T) {
	cfg := ProvenanceConfig{
		ChartRef:       "my-chart",
		HardeningLevel: SecurityLevelNone,
	}

	annotations, err := GenerateProvenanceAnnotations(cfg)
	require.NoError(t, err)

	_, hasLevel := annotations["chart2kro.io/hardening-level"]
	assert.False(t, hasLevel, "should not include hardening-level when none")
}

func TestSLSAPredicate_ValidJSON(t *testing.T) {
	cfg := ProvenanceConfig{
		ChartRef:       "oci://registry.example.com/chart:1.0.0",
		ChartDigest:    "abc123",
		HardeningLevel: SecurityLevelRestricted,
		Profile:        "enterprise",
		EmbedTimestamp: true,
	}

	annotations, err := GenerateProvenanceAnnotations(cfg)
	require.NoError(t, err)

	provenanceJSON := annotations["chart2kro.io/provenance"]

	var predicate SLSAPredicate
	err = json.Unmarshal([]byte(provenanceJSON), &predicate)
	require.NoError(t, err)

	assert.Equal(t, "https://chart2kro.io/HelmConversion@v1", predicate.BuildType)
	assert.Contains(t, predicate.Builder.ID, "chart2kro.io/builder@")
	assert.Equal(t, cfg.ChartRef, predicate.Invocation.ConfigSource.URI)
	assert.Equal(t, "restricted", predicate.Invocation.Parameters["hardening-level"])
	assert.Equal(t, "enterprise", predicate.Invocation.Parameters["profile"])
	assert.NotEmpty(t, predicate.Metadata.BuildStartedOn)
	assert.False(t, predicate.Metadata.Reproducible)
}

func TestSLSAPredicate_Reproducible(t *testing.T) {
	cfg := ProvenanceConfig{
		ChartRef:       "my-chart",
		EmbedTimestamp: false,
	}

	annotations, err := GenerateProvenanceAnnotations(cfg)
	require.NoError(t, err)

	var predicate SLSAPredicate
	err = json.Unmarshal([]byte(annotations["chart2kro.io/provenance"]), &predicate)
	require.NoError(t, err)

	assert.True(t, predicate.Metadata.Reproducible)
	assert.Empty(t, predicate.Metadata.BuildStartedOn)
}

func TestSLSAPredicate_Materials(t *testing.T) {
	cfg := ProvenanceConfig{
		ChartRef:    "oci://registry/chart:1.0",
		ChartDigest: "sha256hash",
	}

	annotations, err := GenerateProvenanceAnnotations(cfg)
	require.NoError(t, err)

	var predicate SLSAPredicate
	err = json.Unmarshal([]byte(annotations["chart2kro.io/provenance"]), &predicate)
	require.NoError(t, err)

	assert.Len(t, predicate.Materials, 1)
	assert.Equal(t, cfg.ChartRef, predicate.Materials[0].URI)
	assert.Equal(t, "sha256hash", predicate.Materials[0].Digest["sha256"])
}
