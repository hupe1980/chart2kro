package harden

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hupe1980/chart2kro/internal/version"
)

// ProvenanceConfig holds parameters for provenance annotation generation.
type ProvenanceConfig struct {
	// ChartRef is the chart reference (e.g., "oci://registry/chart:1.0.0").
	ChartRef string

	// ChartDigest is the sha256 digest of the chart archive (optional).
	ChartDigest string

	// Profile is the applied conversion profile name (optional).
	Profile string

	// HardeningLevel is the PSS level applied.
	HardeningLevel SecurityLevel

	// ExcludedSubcharts lists excluded subcharts (optional).
	ExcludedSubcharts []string

	// EmbedTimestamp controls whether chart2kro.io/generated-at is added.
	EmbedTimestamp bool
}

// SLSAPredicate represents a simplified SLSA v1.0 provenance predicate.
type SLSAPredicate struct {
	BuildType  string         `json:"buildType"`
	Builder    SLSABuilder    `json:"builder"`
	Invocation SLSAInvocation `json:"invocation"`
	Materials  []SLSAMaterial `json:"materials,omitempty"`
	Metadata   SLSAMetadata   `json:"metadata"`
}

// SLSABuilder identifies the build system.
type SLSABuilder struct {
	ID string `json:"id"`
}

// SLSAInvocation describes how the build was invoked.
type SLSAInvocation struct {
	ConfigSource SLSAConfigSource  `json:"configSource"`
	Parameters   map[string]string `json:"parameters,omitempty"`
}

// SLSAConfigSource identifies the source of the build configuration.
type SLSAConfigSource struct {
	URI    string            `json:"uri"`
	Digest map[string]string `json:"digest,omitempty"`
}

// SLSAMaterial represents an input material (e.g., the chart archive).
type SLSAMaterial struct {
	URI    string            `json:"uri"`
	Digest map[string]string `json:"digest,omitempty"`
}

// SLSAMetadata contains build metadata.
type SLSAMetadata struct {
	BuildStartedOn string `json:"buildStartedOn,omitempty"`
	Reproducible   bool   `json:"reproducible"`
}

// GenerateProvenanceAnnotations creates the standard provenance annotations for an RGD.
func GenerateProvenanceAnnotations(cfg ProvenanceConfig) (map[string]string, error) {
	info := version.GetInfo()

	annotations := map[string]string{
		"chart2kro.io/source":            cfg.ChartRef,
		"chart2kro.io/generator-version": info.Version,
	}

	if cfg.ChartDigest != "" {
		annotations["chart2kro.io/chart-digest"] = cfg.ChartDigest
	}

	if cfg.Profile != "" {
		annotations["chart2kro.io/profile"] = cfg.Profile
	}

	if cfg.HardeningLevel != "" && cfg.HardeningLevel != SecurityLevelNone {
		annotations["chart2kro.io/hardening-level"] = string(cfg.HardeningLevel)
	}

	if len(cfg.ExcludedSubcharts) > 0 {
		annotations["chart2kro.io/excluded-subcharts"] = strings.Join(cfg.ExcludedSubcharts, ",")
	}

	var buildTime string

	if cfg.EmbedTimestamp {
		buildTime = time.Now().UTC().Format(time.RFC3339)
		annotations["chart2kro.io/generated-at"] = buildTime
	}

	// Generate SLSA provenance JSON.
	predicate := buildSLSAPredicate(cfg, info, buildTime)

	provenanceJSON, err := json.Marshal(predicate)
	if err != nil {
		return nil, fmt.Errorf("marshaling provenance: %w", err)
	}

	annotations["chart2kro.io/provenance"] = string(provenanceJSON)

	return annotations, nil
}

// buildSLSAPredicate constructs a SLSA v1.0 provenance predicate.
func buildSLSAPredicate(cfg ProvenanceConfig, info version.Info, buildTime string) SLSAPredicate {
	predicate := SLSAPredicate{
		BuildType: "https://chart2kro.io/HelmConversion@v1",
		Builder: SLSABuilder{
			ID: fmt.Sprintf("https://chart2kro.io/builder@%s", info.Version),
		},
		Invocation: SLSAInvocation{
			ConfigSource: SLSAConfigSource{
				URI: cfg.ChartRef,
			},
			Parameters: map[string]string{
				"hardening-level": string(cfg.HardeningLevel),
			},
		},
		Metadata: SLSAMetadata{
			Reproducible: !cfg.EmbedTimestamp,
		},
	}

	if cfg.ChartDigest != "" {
		predicate.Invocation.ConfigSource.Digest = map[string]string{
			"sha256": cfg.ChartDigest,
		}

		predicate.Materials = []SLSAMaterial{
			{
				URI: cfg.ChartRef,
				Digest: map[string]string{
					"sha256": cfg.ChartDigest,
				},
			},
		}
	}

	if cfg.Profile != "" {
		predicate.Invocation.Parameters["profile"] = cfg.Profile
	}

	if buildTime != "" {
		predicate.Metadata.BuildStartedOn = buildTime
	}

	return predicate
}
