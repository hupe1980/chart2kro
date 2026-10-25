package kroval

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"testing/fstest"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/controller/openapi/builder"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/spec3"
)

// Catalogs hold, per Kubernetes minor, the OpenAPI v3 group-version documents
// (components only, descriptions stripped, gzipped) and discovery lists dumped from
// a kind cluster of that version. Regenerate with `just catalog`.
//
//go:embed data/catalog data/discovery
var catalogFS embed.FS

// KubeVersions returns the embedded Kubernetes minors, newest first (e.g. "1.37").
func KubeVersions() []string {
	entries, _ := fs.ReadDir(catalogFS, "data/catalog")

	var out []string

	for _, e := range entries {
		if e.IsDir() {
			out = append(out, strings.TrimPrefix(e.Name(), "v"))
		}
	}

	sort.Slice(out, func(i, j int) bool { return minor(out[i]) > minor(out[j]) })

	return out
}

// DefaultKubeVersion is the newest embedded minor.
func DefaultKubeVersion() string { return KubeVersions()[0] }

// NormalizeKubeVersion maps "v1.34.2", "1.34" to "1.34".
func NormalizeKubeVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)

	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}

	return v
}

func minor(v string) int {
	var maj, mi int
	_, _ = fmt.Sscanf(v, "%d.%d", &maj, &mi)

	return maj*1000 + mi
}

func catalogFor(kube string) (fs.FS, fs.FS, error) {
	k := NormalizeKubeVersion(kube)

	oa, err := fs.Sub(catalogFS, "data/catalog/v"+k)
	if err != nil {
		return nil, nil, err
	}

	if _, err := fs.ReadDir(oa, "."); err != nil {
		return nil, nil, fmt.Errorf("no validation catalog for Kubernetes %s (have %s)", k, strings.Join(KubeVersions(), ", "))
	}

	disc, err := fs.Sub(catalogFS, "data/discovery/v"+k)
	if err != nil {
		return nil, nil, err
	}

	return oa, disc, nil
}

// openAPIClient serves "apis__apps__v1_openapi.json[.gz]" files as an openapi.Client.
type openAPIClient struct{ fsys fs.FS }

func (c *openAPIClient) Paths() (map[string]openapi.GroupVersion, error) {
	entries, err := fs.ReadDir(c.fsys, ".")
	if err != nil {
		return nil, err
	}

	out := map[string]openapi.GroupVersion{}

	for _, e := range entries {
		n := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".gz"), "_openapi.json")
		out[strings.ReplaceAll(n, "__", "/")] = &gvDoc{fsys: c.fsys, name: e.Name()}
	}

	return out, nil
}

type gvDoc struct {
	fsys fs.FS
	name string
	once sync.Once
	data []byte
	err  error
}

func (g *gvDoc) Schema(contentType string) ([]byte, error) {
	if contentType != "application/json" {
		return nil, fmt.Errorf("unsupported content type %q", contentType)
	}

	g.once.Do(func() {
		b, err := fs.ReadFile(g.fsys, g.name)
		if err != nil {
			g.err = err
			return
		}

		if strings.HasSuffix(g.name, ".gz") {
			zr, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				g.err = err
				return
			}

			b, err = io.ReadAll(zr)
			if err != nil {
				g.err = err
				return
			}
		}

		g.data = b
	})

	return g.data, g.err
}

func (g *gvDoc) ServerRelativeURL() string { return g.name }

// offlineDiscovery implements only OpenAPIV3, which is all ClientDiscoveryResolver uses.
type offlineDiscovery struct {
	discovery.DiscoveryInterface
	v3 openapi.Client
}

func (d offlineDiscovery) OpenAPIV3() openapi.Client { return d.v3 }

func schemaResolver(fsys fs.FS) resolver.SchemaResolver {
	return &resolver.ClientDiscoveryResolver{Discovery: offlineDiscovery{v3: &openAPIClient{fsys: fsys}}}
}

func restMapper(disc fs.FS) (*meta.DefaultRESTMapper, error) {
	entries, err := fs.ReadDir(disc, ".")
	if err != nil {
		return nil, err
	}

	m := meta.NewDefaultRESTMapper(nil)

	for _, e := range entries {
		b, err := fs.ReadFile(disc, e.Name())
		if err != nil {
			return nil, err
		}

		var list metav1.APIResourceList
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}

		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			return nil, err
		}

		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue // subresource
			}

			scope := meta.RESTScopeRoot
			if r.Namespaced {
				scope = meta.RESTScopeNamespace
			}

			m.AddSpecific(gv.WithKind(r.Kind), gv.WithResource(r.Name), gv.WithResource(r.SingularName), scope)
		}
	}

	return m, nil
}

// withCRDs overlays OpenAPI documents generated from CRDs the way kube-apiserver
// does, and registers their kinds in the mapper.
func withCRDs(base fs.FS, m *meta.DefaultRESTMapper, crds []*extv1.CustomResourceDefinition) (fs.FS, error) {
	if len(crds) == 0 {
		return base, nil
	}

	byGV := map[schema.GroupVersion][]*spec3.OpenAPI{}

	for _, crd := range crds {
		for _, v := range crd.Spec.Versions {
			if !v.Served {
				continue
			}

			doc, err := builder.BuildOpenAPIV3(crd, v.Name, builder.Options{V2: false})
			if err != nil {
				return nil, fmt.Errorf("CRD %s/%s: %w", crd.Name, v.Name, err)
			}

			gv := schema.GroupVersion{Group: crd.Spec.Group, Version: v.Name}
			byGV[gv] = append(byGV[gv], doc)

			scope := meta.RESTScopeNamespace
			if crd.Spec.Scope == extv1.ClusterScoped {
				scope = meta.RESTScopeRoot
			}

			m.AddSpecific(gv.WithKind(crd.Spec.Names.Kind), gv.WithResource(crd.Spec.Names.Plural), gv.WithResource(crd.Spec.Names.Singular), scope)
		}
	}

	overlay := fstest.MapFS{}

	entries, err := fs.ReadDir(base, ".")
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		b, err := fs.ReadFile(base, e.Name())
		if err != nil {
			return nil, err
		}

		overlay[e.Name()] = &fstest.MapFile{Data: b}
	}

	for gv, docs := range byGV {
		merged, err := builder.MergeSpecsV3(docs...)
		if err != nil {
			return nil, err
		}

		b, err := json.Marshal(merged)
		if err != nil {
			return nil, err
		}

		overlay[fmt.Sprintf("apis__%s__%s_openapi.json", gv.Group, gv.Version)] = &fstest.MapFile{Data: b}
	}

	return overlay, nil
}
