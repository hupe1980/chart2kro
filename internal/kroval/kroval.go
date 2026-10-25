// Package kroval validates kro ResourceGraphDefinitions offline with kro's own code:
//  1. admission — kro's RGD CRD structural schema (defaults, unknown fields, OpenAPI, CEL
//     rules), which the API server enforces;
//  2. graph — kro's graph builder (pkg/graph) against an embedded OpenAPI catalog plus CRDs.
//
// It is pinned to the kro version in go.mod (see KroVersion).
package kroval

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	structuraldefaulting "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	structuralpruning "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	apiservervalidation "k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/kubernetes-sigs/kro/api/v1alpha1"
	"github.com/kubernetes-sigs/kro/pkg/graph"
)

//go:embed data/kro.run_resourcegraphdefinitions.yaml
var rgdCRDYAML []byte

// GraphConfig mirrors the kro controller's default limits.
var GraphConfig = graph.Config{MaxCollectionSize: 1000, MaxCollectionDimensionSize: 10}

// KroVersion is the linked kro module version (e.g. "v0.9.4").
func KroVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/kubernetes-sigs/kro" {
				return d.Version
			}
		}
	}

	return "v0.9.4"
}

// Error lists every reason kro would reject an RGD.
type Error struct{ Problems []string }

func (e *Error) Error() string {
	return "\n  - " + strings.Join(e.Problems, "\n  - ")
}

// Validator validates RGDs for one Kubernetes version plus a set of CRDs.
type Validator struct {
	builder *graph.Builder
	admit   *CRDValidator
	mapper  *meta.DefaultRESTMapper
	kube    string
}

// New builds a validator for the Kubernetes minor (empty = newest embedded) and CRDs.
// CRD objects are generic maps as parsed from YAML.
func New(kube string, crds []map[string]any) (*Validator, error) {
	if kube == "" {
		kube = DefaultKubeVersion()
	}

	oa, disc, err := catalogFor(kube)
	if err != nil {
		return nil, err
	}

	m, err := restMapper(disc)
	if err != nil {
		return nil, err
	}

	parsed, err := ParseCRDs(crds)
	if err != nil {
		return nil, err
	}

	oa, err = withCRDs(oa, m, parsed)
	if err != nil {
		return nil, err
	}

	b, err := graph.NewBuilder(&rest.Config{}, nil, graph.WithSchemaResolver(schemaResolver(oa)), graph.WithRESTMapper(m))
	if err != nil {
		return nil, err
	}

	admit, err := rgdAdmission()
	if err != nil {
		return nil, err
	}

	return &Validator{builder: b, admit: admit, mapper: m, kube: NormalizeKubeVersion(kube)}, nil
}

// KubeVersion is the catalog minor this validator uses.
func (v *Validator) KubeVersion() string { return v.kube }

// ParseCRDs converts CRD documents into typed CRDs, ignoring non-CRD objects.
func ParseCRDs(objs []map[string]any) ([]*extv1.CustomResourceDefinition, error) {
	var out []*extv1.CustomResourceDefinition

	for _, o := range objs {
		if o["kind"] != "CustomResourceDefinition" {
			continue
		}

		var crd extv1.CustomResourceDefinition
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(o, &crd); err != nil {
			return nil, fmt.Errorf("parsing CRD: %w", err)
		}

		out = append(out, &crd)
	}

	return out, nil
}

// Scope reports whether apiVersion/kind is namespaced; known is false for unresolvable kinds.
func (v *Validator) Scope(apiVersion, kind string) (namespaced, known bool) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return false, false
	}

	mapping, err := v.mapper.RESTMapping(gv.WithKind(kind).GroupKind(), gv.Version)
	if err != nil {
		return false, false
	}

	return mapping.Scope.Name() == meta.RESTScopeNameNamespace, true
}

// Validate runs both layers. The input is not modified. On success it returns kro's
// compiled graph (used by the oracle).
func (v *Validator) Validate(ctx context.Context, rgd map[string]any) (*graph.Graph, error) {
	obj := runtime.DeepCopyJSON(rgd)

	unknown, errs := v.admit.Validate(ctx, obj)

	var problems []string
	for _, u := range unknown {
		problems = append(problems, "unknown field "+u)
	}

	for _, e := range errs {
		problems = append(problems, e.Error())
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, &Error{Problems: problems}
	}

	var typed v1alpha1.ResourceGraphDefinition
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &typed); err != nil {
		return nil, &Error{Problems: []string{err.Error()}}
	}

	g, err := v.builder.NewResourceGraphDefinition(&typed, GraphConfig)
	if err != nil {
		return nil, &Error{Problems: []string{err.Error()}}
	}

	return g, nil
}

// ValidateYAML parses and validates every ResourceGraphDefinition in a YAML stream.
func (v *Validator) ValidateYAML(ctx context.Context, docs [][]byte) error {
	var all []string

	for i, d := range docs {
		var obj map[string]any
		if err := yaml.Unmarshal(d, &obj); err != nil {
			return fmt.Errorf("document %d: %w", i, err)
		}

		if obj["kind"] != "ResourceGraphDefinition" {
			continue
		}

		if _, err := v.Validate(ctx, obj); err != nil {
			var ke *Error
			if errors.As(err, &ke) {
				name, _ := obj["metadata"].(map[string]any)["name"].(string)
				for _, p := range ke.Problems {
					all = append(all, name+": "+p)
				}

				continue
			}

			return err
		}
	}

	if len(all) > 0 {
		return &Error{Problems: all}
	}

	return nil
}

// CRDValidator validates objects against one version of a CRD like the API server does.
type CRDValidator struct {
	structural *structuralschema.Structural
	validator  apiservervalidation.SchemaValidator
	cel        *cel.Validator
}

// NewCRDValidator builds a validator for a CRD version.
func NewCRDValidator(crd *extv1.CustomResourceDefinition, version string) (*CRDValidator, error) {
	var props *extv1.JSONSchemaProps

	for _, ver := range crd.Spec.Versions {
		if ver.Name == version && ver.Schema != nil {
			props = ver.Schema.OpenAPIV3Schema
		}
	}

	if props == nil {
		return nil, fmt.Errorf("version %s not found in CRD %s", version, crd.Name)
	}

	internal := &apiextensions.JSONSchemaProps{}
	if err := extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(props, internal, nil); err != nil {
		return nil, err
	}

	s, err := structuralschema.NewStructural(internal)
	if err != nil {
		return nil, err
	}

	sv, _, err := apiservervalidation.NewSchemaValidator(internal)
	if err != nil {
		return nil, err
	}

	return &CRDValidator{structural: s, validator: sv, cel: cel.NewValidator(s, true, celconfig.PerCallLimit)}, nil
}

// Validate applies defaults in place and returns unknown (pruned) fields plus errors.
func (v *CRDValidator) Validate(ctx context.Context, obj map[string]any) (unknown []string, errs field.ErrorList) {
	structuraldefaulting.Default(obj, v.structural)
	unknown = structuralpruning.PruneWithOptions(obj, v.structural, true,
		structuralschema.UnknownFieldPathOptions{TrackUnknownFieldPaths: true})
	errs = apiservervalidation.ValidateCustomResource(nil, obj, v.validator)

	if v.cel != nil {
		celErrs, _ := v.cel.Validate(ctx, nil, v.structural, obj, nil, celconfig.RuntimeCELCostBudget)
		errs = append(errs, celErrs...)
	}

	return unknown, errs
}

func rgdAdmission() (*CRDValidator, error) {
	var crd extv1.CustomResourceDefinition
	if err := yaml.Unmarshal(rgdCRDYAML, &crd); err != nil {
		return nil, err
	}

	return NewCRDValidator(&crd, "v1alpha1")
}
