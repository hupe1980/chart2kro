// Package oracle instantiates a compiled RGD offline with kro's own runtime
// (pkg/runtime): the same CEL programs, includeWhen and defaulting as the controller.
package oracle

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubernetes-sigs/kro/pkg/graph"
	krort "github.com/kubernetes-sigs/kro/pkg/runtime"

	"github.com/hupe1980/chart2kro/internal/kroval"
	"github.com/hupe1980/chart2kro/internal/manifest"
)

// Node is the evaluated state of one RGD resource.
type Node struct {
	ID      string
	Ignored bool             // includeWhen evaluated to false
	Pending string           // needs observed cluster state (not comparable offline)
	Objects []map[string]any // desired objects
}

// Instance builds the stored instance object: defaulted and validated with the
// CRD kro generates from the RGD's schema, as the API server would.
func Instance(ctx context.Context, g *graph.Graph, name, namespace string, spec map[string]any) (map[string]any, error) {
	crd := g.CRD
	version := crd.Spec.Versions[0].Name

	obj := map[string]any{
		"apiVersion": crd.Spec.Group + "/" + version,
		"kind":       crd.Spec.Names.Kind,
		"metadata": map[string]any{
			"name": name, "namespace": namespace,
			// kro exposes schema.metadata.uid; a stable value keeps evaluation deterministic.
			"uid": "00000000-0000-0000-0000-000000000000",
		},
		"spec": manifest.NormalizeMap(spec),
	}

	v, err := kroval.NewCRDValidator(crd, version)
	if err != nil {
		return nil, err
	}

	unknown, errs := v.Validate(ctx, obj)
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown instance fields: %v", unknown)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("instance invalid: %v", errs.ToAggregate())
	}

	return obj, nil
}

// Instantiate evaluates every node for an instance with the given spec.
func Instantiate(ctx context.Context, g *graph.Graph, name, namespace string, spec map[string]any) ([]Node, error) {
	inst, err := Instance(ctx, g, name, namespace, spec)
	if err != nil {
		return nil, err
	}

	rt, err := krort.FromGraph(g, graph.Config{MaxCollectionSize: 1000, MaxCollectionDimensionSize: 10},
		krort.WithInstance(&unstructured.Unstructured{Object: inst}))
	if err != nil {
		return nil, err
	}

	var out []Node

	for _, n := range rt.Nodes() {
		node := Node{ID: n.Spec.Meta.ID}

		ignored, err := n.IsIgnored()

		switch {
		case krort.IsDataPending(err):
			node.Pending = err.Error()
		case err != nil:
			return nil, fmt.Errorf("resource %s includeWhen: %w", node.ID, err)
		case ignored:
			node.Ignored = true
		default:
			objs, err := n.GetDesired()

			switch {
			case krort.IsDataPending(err):
				node.Pending = err.Error()
			case err != nil:
				return nil, fmt.Errorf("resource %s: %w", node.ID, err)
			default:
				for _, o := range objs {
					node.Objects = append(node.Objects, runtime.DeepCopyJSON(o.Object))
				}

				// Downstream nodes may read this node's spec; feed desired back as observed.
				n.SetObserved(objs)
			}
		}

		out = append(out, node)
	}

	return out, nil
}
