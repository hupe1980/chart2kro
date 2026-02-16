// Package hooks detects and handles Helm lifecycle hooks in rendered manifests.
package hooks

import (
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hupe1980/chart2kro/internal/yamlutil"
)

// HookAnnotation is the Helm hook annotation key.
const HookAnnotation = "helm.sh/hook"

// HookType represents a Helm lifecycle hook type.
type HookType string

// HookType values enumerate the supported Helm lifecycle hooks.
const (
	HookPreInstall   HookType = "pre-install"
	HookPostInstall  HookType = "post-install"
	HookPreUpgrade   HookType = "pre-upgrade"
	HookPostUpgrade  HookType = "post-upgrade"
	HookPreDelete    HookType = "pre-delete"
	HookPostDelete   HookType = "post-delete"
	HookPreRollback  HookType = "pre-rollback"
	HookPostRollback HookType = "post-rollback"
	HookTest         HookType = "test"
	HookTestSuccess  HookType = "test-success"
)

// IsTestHook returns true if the hook type is a test hook.
func (h HookType) IsTestHook() bool {
	return h == HookTest || h == HookTestSuccess
}

// Resource represents a parsed Kubernetes resource with hook metadata.
type Resource struct {
	Name       string
	Kind       string
	APIVersion string
	RawYAML    string
	HookTypes  []HookType
	IsHook     bool
}

// FilterResult contains the outcome of hook filtering.
type FilterResult struct {
	Resources     []Resource
	DroppedHooks  []Resource
	IncludedHooks []Resource
	HookCount     int
}

// Filter processes rendered YAML documents and separates hooks from regular
// resources. When includeHooks is true, hook resources are included as regular
// resources with the helm.sh/hook annotation stripped.
func Filter(yamlDocs []byte, includeHooks bool, logger *slog.Logger) (*FilterResult, error) {
	docs := splitYAMLDocuments(yamlDocs)
	result := &FilterResult{}

	for _, doc := range docs {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" {
			continue
		}

		res := parseResource(trimmed)

		if !res.IsHook {
			result.Resources = append(result.Resources, res)
			continue
		}

		result.HookCount++

		if includeHooks {
			res.RawYAML = stripHookAnnotations(res.RawYAML)
			res.IsHook = false
			result.IncludedHooks = append(result.IncludedHooks, res)
			result.Resources = append(result.Resources, res)
		} else {
			result.DroppedHooks = append(result.DroppedHooks, res)

			for _, ht := range res.HookTypes {
				if ht.IsTestHook() {
					continue
				}

				logger.Warn("dropping Helm hook resource",
					slog.String("hook", string(ht)),
					slog.String("resource", fmt.Sprintf("%s/%s", res.Kind, res.Name)),
				)
			}
		}
	}

	return result, nil
}

// CombineResources produces a multi-document YAML from a FilterResult.
func CombineResources(result *FilterResult) []byte {
	var sb strings.Builder

	for i, res := range result.Resources {
		if i > 0 {
			sb.WriteString("---\n")
		}

		sb.WriteString(strings.TrimSpace(res.RawYAML))
		sb.WriteByte('\n')
	}

	return []byte(sb.String())
}

// PrintHookSummary writes a summary of hook handling to w.
func PrintHookSummary(w io.Writer, result *FilterResult) {
	if result.HookCount == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "Hooks detected: %d\n", result.HookCount)

	if len(result.DroppedHooks) > 0 {
		_, _ = fmt.Fprintf(w, "  Dropped: %d\n", len(result.DroppedHooks))

		for _, h := range result.DroppedHooks {
			types := make([]string, 0, len(h.HookTypes))
			for _, ht := range h.HookTypes {
				types = append(types, string(ht))
			}

			_, _ = fmt.Fprintf(w, "    - %s/%s (%s)\n", h.Kind, h.Name, strings.Join(types, ", "))
		}
	}

	if len(result.IncludedHooks) > 0 {
		_, _ = fmt.Fprintf(w, "  Included as regular resources: %d\n", len(result.IncludedHooks))
	}
}

func splitYAMLDocuments(data []byte) []string {
	return yamlutil.SplitDocumentsString(data)
}

// resourceMeta is a minimal Kubernetes resource structure used for YAML
// unmarshalling. Only the fields needed for hook detection are included.
type resourceMeta struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name        string            `yaml:"name"`
		Annotations map[string]string `yaml:"annotations"`
	} `yaml:"metadata"`
}

func parseResource(doc string) Resource {
	res := Resource{RawYAML: doc}

	var meta resourceMeta
	if err := yaml.Unmarshal([]byte(doc), &meta); err != nil {
		// If YAML is unparseable, return what we have — the resource
		// will simply be treated as a non-hook regular resource.
		return res
	}

	res.Kind = meta.Kind
	res.APIVersion = meta.APIVersion
	res.Name = meta.Metadata.Name

	// Check for hook annotation using an exact key lookup — no prefix matching
	// needed since we parse from a proper map.
	hookValue, hasHook := meta.Metadata.Annotations[HookAnnotation]
	if hasHook && hookValue != "" {
		res.IsHook = true
		for _, h := range strings.Split(hookValue, ",") {
			res.HookTypes = append(res.HookTypes, HookType(strings.TrimSpace(h)))
		}
	}

	return res
}

// hookAnnotationPrefix matches any annotation key starting with "helm.sh/hook".
var hookAnnotationRe = regexp.MustCompile(`(?m)^\s+helm\.sh/hook[^:]*:.*$`)

// emptyAnnotationsRe matches an annotations key left with no children after
// hook annotation stripping. It handles both block-style (annotations:\n)
// with only blank lines following until the next key or end-of-document, and
// inline null (annotations: null).
var emptyAnnotationsRe = regexp.MustCompile(`(?m)^\s+annotations:\s*(?:null)?\s*\n?`)

// blankLineCollapseRe collapses runs of 3+ newlines into a single blank line.
var blankLineCollapseRe = regexp.MustCompile(`\n{3,}`)

func stripHookAnnotations(yamlDoc string) string {
	result := hookAnnotationRe.ReplaceAllString(yamlDoc, "")

	// Check whether any non-hook annotations remain by re-parsing.
	var meta resourceMeta
	if err := yaml.Unmarshal([]byte(result), &meta); err == nil {
		if len(meta.Metadata.Annotations) == 0 {
			// All annotations were hook annotations — remove the empty key.
			result = emptyAnnotationsRe.ReplaceAllString(result, "")
		}
	}

	// Collapse multiple consecutive blank lines into one.
	result = blankLineCollapseRe.ReplaceAllString(result, "\n")

	return result
}
