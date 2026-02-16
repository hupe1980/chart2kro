// Package parser splits multi-document YAML manifests and parses them into
// k8s.Resource structs.
package parser

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/yamlutil"
)

// Parser parses raw rendered manifests into k8s Resources.
type Parser interface {
	Parse(ctx context.Context, manifests []byte) ([]*k8s.Resource, error)
}

// compile-time interface conformance check.
var _ Parser = (*DefaultParser)(nil)

// DefaultParser is the default implementation of the Parser interface.
type DefaultParser struct{}

// NewParser creates a new DefaultParser.
func NewParser() *DefaultParser {
	return &DefaultParser{}
}

// Parse splits the manifests into documents and parses each into a Resource.
// Documents without apiVersion or kind are skipped.
func (p *DefaultParser) Parse(_ context.Context, manifests []byte) ([]*k8s.Resource, error) {
	docs := SplitDocuments(manifests)

	var resources []*k8s.Resource

	for _, doc := range docs {
		r, err := parseDocument(doc)
		if err != nil {
			return nil, fmt.Errorf("parsing document: %w", err)
		}

		if r != nil {
			resources = append(resources, r)
		}
	}

	return resources, nil
}

// SplitDocuments splits a multi-document YAML byte slice into individual
// documents, filtering out empty ones. Delegates to the shared yamlutil package.
func SplitDocuments(data []byte) [][]byte {
	return yamlutil.SplitDocuments(data)
}

// parseDocument parses a single YAML document into a Resource.
// Returns nil (no error) if the document lacks apiVersion or kind.
func parseDocument(doc []byte) (*k8s.Resource, error) {
	var obj map[string]interface{}
	if err := sigsyaml.Unmarshal(doc, &obj); err != nil {
		return nil, fmt.Errorf("unmarshaling YAML: %w", err)
	}

	if obj == nil {
		return nil, nil
	}

	apiVersion, _ := obj["apiVersion"].(string)
	kind, _ := obj["kind"].(string)

	if apiVersion == "" || kind == "" {
		return nil, nil
	}

	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)

	u := &unstructured.Unstructured{Object: obj}

	return &k8s.Resource{
		GVK:         gvk,
		Name:        u.GetName(),
		Namespace:   u.GetNamespace(),
		Labels:      u.GetLabels(),
		Annotations: u.GetAnnotations(),
		Object:      u,
	}, nil
}
