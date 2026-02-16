// Package renderer executes Helm template rendering in-memory using the
// Helm SDK engine and provides values merging from multiple sources.
package renderer

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

// Renderer renders Helm chart templates into raw Kubernetes YAML manifests.
type Renderer interface {
	Render(ctx context.Context, ch *chart.Chart, vals map[string]interface{}) ([]byte, error)
}

// RenderOptions configures rendering behaviour.
type RenderOptions struct {
	ReleaseName string
	Namespace   string
	Strict      bool
}

// DefaultRenderOptions returns sensible defaults.
func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		ReleaseName: "release",
		Namespace:   "default",
		Strict:      false,
	}
}

// HelmRenderer implements Renderer using the Helm SDK engine.
type HelmRenderer struct {
	opts RenderOptions
}

// New creates a HelmRenderer with the given options.
func New(opts RenderOptions) *HelmRenderer {
	if opts.ReleaseName == "" {
		opts.ReleaseName = "release"
	}

	if opts.Namespace == "" {
		opts.Namespace = "default"
	}

	return &HelmRenderer{opts: opts}
}

// Render executes the chart templates and returns the combined YAML output.
func (r *HelmRenderer) Render(ctx context.Context, ch *chart.Chart, vals map[string]interface{}) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rendering cancelled: %w", ctx.Err())
	default:
	}

	options := chartutil.ReleaseOptions{
		Name:      r.opts.ReleaseName,
		Namespace: r.opts.Namespace,
		Revision:  1,
		IsInstall: true,
	}

	valuesToRender, err := chartutil.ToRenderValues(ch, vals, options, nil)
	if err != nil {
		return nil, fmt.Errorf("preparing render values: %w", err)
	}

	eng := engine.Engine{Strict: r.opts.Strict, LintMode: false}

	rendered, err := eng.Render(ch, valuesToRender)
	if err != nil {
		return nil, fmt.Errorf("rendering templates: %w", err)
	}

	return combineManifests(rendered), nil
}

// SourcedManifest pairs raw YAML content with the template path that produced it.
type SourcedManifest struct {
	// TemplatePath is the Helm template path (e.g., "my-chart/charts/pg/templates/statefulset.yaml").
	TemplatePath string
	// Content is the raw YAML content for this template.
	Content string
}

// RenderWithSources executes chart templates and returns manifests annotated
// with their source template paths. This is used by the filter pipeline to
// support subchart-based exclusion.
func (r *HelmRenderer) RenderWithSources(ctx context.Context, ch *chart.Chart, vals map[string]interface{}) ([]SourcedManifest, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rendering cancelled: %w", ctx.Err())
	default:
	}

	options := chartutil.ReleaseOptions{
		Name:      r.opts.ReleaseName,
		Namespace: r.opts.Namespace,
		Revision:  1,
		IsInstall: true,
	}

	valuesToRender, err := chartutil.ToRenderValues(ch, vals, options, nil)
	if err != nil {
		return nil, fmt.Errorf("preparing render values: %w", err)
	}

	eng := engine.Engine{Strict: r.opts.Strict, LintMode: false}

	rendered, err := eng.Render(ch, valuesToRender)
	if err != nil {
		return nil, fmt.Errorf("rendering templates: %w", err)
	}

	return buildSourcedManifests(rendered), nil
}

// buildSourcedManifests converts the Helm engine output map into sorted SourcedManifests.
func buildSourcedManifests(rendered map[string]string) []SourcedManifest {
	keys := make([]string, 0, len(rendered))
	for k := range rendered {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var manifests []SourcedManifest

	for _, k := range keys {
		content := rendered[k]

		if strings.HasSuffix(k, "NOTES.txt") {
			continue
		}

		trimmed := strings.TrimSpace(content)
		if trimmed == "" {
			continue
		}

		manifests = append(manifests, SourcedManifest{
			TemplatePath: k,
			Content:      trimmed,
		})
	}

	return manifests
}

// CombineSourcedManifests merges sourced manifests into a single multi-document YAML.
func CombineSourcedManifests(manifests []SourcedManifest) []byte {
	var buf bytes.Buffer

	for _, m := range manifests {
		if buf.Len() > 0 {
			buf.WriteString("---\n")
		}

		buf.WriteString(m.Content)
		buf.WriteByte('\n')
	}

	return buf.Bytes()
}

// combineManifests merges rendered templates into a single multi-document YAML.
func combineManifests(rendered map[string]string) []byte {
	keys := make([]string, 0, len(rendered))
	for k := range rendered {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var buf bytes.Buffer

	for _, k := range keys {
		content := rendered[k]

		if strings.HasSuffix(k, "NOTES.txt") {
			continue
		}

		trimmed := strings.TrimSpace(content)
		if trimmed == "" {
			continue
		}

		if buf.Len() > 0 {
			buf.WriteString("---\n")
		}

		buf.WriteString(trimmed)
		buf.WriteByte('\n')
	}

	return buf.Bytes()
}
