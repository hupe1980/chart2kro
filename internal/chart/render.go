package chart

import (
	"context"
	"fmt"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/chart/common/util"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"helm.sh/helm/v4/pkg/engine"
	"sigs.k8s.io/yaml"
)

// Object is one rendered Kubernetes document.
type Object struct {
	Source  string         // template path, e.g. "podinfo/templates/deployment.yaml"
	Ordinal int            // index of the document within Source
	Hook    []string       // helm.sh/hook events; empty for regular resources
	CRD     bool           // from the chart's crds/ directory
	Obj     map[string]any // parsed document (JSON-compatible types)
}

// Key identifies an object across renders independently of its name.
func (o Object) Key() string {
	apiVersion, _ := o.Obj["apiVersion"].(string)
	kind, _ := o.Obj["kind"].(string)

	return o.Source + "#" + strconv.Itoa(o.Ordinal) + "#" + apiVersion + "/" + kind
}

// Renderer renders one loaded chart repeatedly. It is safe for concurrent use:
// every render works on a private copy of the chart.
type Renderer struct {
	base    *chartv2.Chart
	caps    *common.Capabilities
	timeout time.Duration
}

// RenderOptions configures Capabilities and per-render limits.
type RenderOptions struct {
	KubeVersion string        // e.g. "1.34" or "v1.34.0"; empty = Helm default
	APIVersions []string      // extra .Capabilities.APIVersions
	Timeout     time.Duration // per render; default 30s
}

// NewRenderer prepares a renderer with fixed Capabilities.
func NewRenderer(ch *chartv2.Chart, o RenderOptions) (*Renderer, error) {
	caps := common.DefaultCapabilities.Copy()

	if o.KubeVersion != "" {
		v := o.KubeVersion
		if !strings.HasPrefix(v, "v") {
			v = "v" + v
		}

		if strings.Count(v, ".") == 1 {
			v += ".0"
		}

		kv, err := common.ParseKubeVersion(v)
		if err != nil {
			return nil, fmt.Errorf("invalid kube version %q: %w", o.KubeVersion, err)
		}

		caps.KubeVersion = *kv
	}

	caps.APIVersions = append(caps.APIVersions, o.APIVersions...)
	// Library builds report "v4.3"; templates may compare the Helm version.
	if v := helmVersion(); v != "" {
		caps.HelmVersion.Version = v
	}

	if ch.Metadata != nil && ch.Metadata.KubeVersion != "" &&
		!chartutil.IsCompatibleRange(ch.Metadata.KubeVersion, caps.KubeVersion.String()) {
		return nil, fmt.Errorf("chart requires kubeVersion %s, incompatible with Kubernetes %s",
			ch.Metadata.KubeVersion, caps.KubeVersion.Version)
	}

	t := o.Timeout
	if t <= 0 {
		t = 30 * time.Second
	}

	return &Renderer{base: ch, caps: caps, timeout: t}, nil
}

// Values coalesces user values with the chart's defaults (including subcharts),
// exactly as Helm does before rendering.
func (r *Renderer) Values(user map[string]any) (map[string]any, error) {
	c := clone(r.base)

	v, err := util.CoalesceValues(c, deepCopy(user))
	if err != nil {
		return nil, err
	}

	return map[string]any(v), nil
}

// Render renders the chart with the given (complete) values, release name and namespace.
// Hooks are returned with Hook set; test hooks are included and flagged so callers can drop them.
func (r *Renderer) Render(ctx context.Context, values map[string]any, release, namespace string) ([]Object, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	c := clone(r.base)
	vals := deepCopy(values)

	if err := chartutil.ProcessDependencies(c, vals); err != nil {
		return nil, fmt.Errorf("processing dependencies: %w", err)
	}

	top, err := util.ToRenderValuesWithSchemaValidation(c, vals, common.ReleaseOptions{
		Name: release, Namespace: namespace, Revision: 1, IsInstall: true,
	}, r.caps, false)
	if err != nil {
		return nil, err
	}

	var e engine.Engine // zero value: no cluster access, lookup returns empty

	type result struct {
		files map[string]string
		err   error
	}

	done := make(chan result, 1)

	go func() {
		f, err := e.RenderWithContext(ctx, c, top)
		done <- result{f, err}
	}()

	var files map[string]string

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rendering timed out after %s", r.timeout)
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}

		files = res.files
	}

	return split(files, c)
}

var docSep = regexp.MustCompile(`(?m)^---[ \t]*$`)

func split(files map[string]string, c *chartv2.Chart) ([]Object, error) {
	paths := make([]string, 0, len(files))

	for p := range files {
		if strings.HasSuffix(p, "NOTES.txt") || !isManifestFile(p) {
			continue
		}

		paths = append(paths, p)
	}

	sort.Strings(paths)

	var out []Object

	for _, p := range paths {
		ord := 0

		for _, doc := range docSep.Split(files[p], -1) {
			if strings.TrimSpace(doc) == "" {
				continue
			}

			var obj map[string]any
			if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
				return nil, fmt.Errorf("%s: invalid YAML: %w", p, err)
			}

			if len(obj) == 0 { // comment-only documents
				continue
			}

			if _, ok := obj["kind"]; !ok {
				return nil, fmt.Errorf("%s: document %d has no kind", p, ord)
			}

			out = append(out, Object{Source: p, Ordinal: ord, Hook: hookEvents(obj), Obj: obj})
			ord++
		}
	}

	for _, crd := range c.CRDObjects() {
		var obj map[string]any
		if err := yaml.Unmarshal(crd.File.Data, &obj); err != nil {
			return nil, fmt.Errorf("%s: invalid CRD YAML: %w", crd.Filename, err)
		}

		if len(obj) > 0 {
			out = append(out, Object{Source: crd.Filename, CRD: true, Obj: obj})
		}
	}

	return out, nil
}

func hookEvents(obj map[string]any) []string {
	md, _ := obj["metadata"].(map[string]any)
	ann, _ := md["annotations"].(map[string]any)

	h, _ := ann["helm.sh/hook"].(string)
	if h == "" {
		return nil
	}

	var ev []string
	for e := range strings.SplitSeq(h, ",") {
		ev = append(ev, strings.TrimSpace(e))
	}

	return ev
}

// IsTestHook reports whether events mark a test hook.
func IsTestHook(events []string) bool {
	for _, e := range events {
		if e == "test" || e == "test-success" || e == "test-failure" {
			return true
		}
	}

	return false
}

// clone returns a copy of ch that ProcessDependencies may mutate freely.
func clone(ch *chartv2.Chart) *chartv2.Chart {
	c := *ch
	c.Values = deepCopy(ch.Values)

	if ch.Metadata != nil {
		md := *ch.Metadata

		md.Dependencies = make([]*chartv2.Dependency, len(ch.Metadata.Dependencies))
		for i, d := range ch.Metadata.Dependencies {
			dd := *d
			md.Dependencies[i] = &dd
		}

		c.Metadata = &md
	}

	if ch.Lock != nil {
		l := *ch.Lock
		c.Lock = &l
	}

	deps := ch.Dependencies()
	cloned := make([]*chartv2.Chart, len(deps))

	for i, d := range deps {
		cloned[i] = clone(d)
	}

	c.SetDependencies(cloned...)

	return &c
}

func deepCopy(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}

	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}

	return out
}

func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopy(t)
	case common.Values:
		return deepCopy(map[string]any(t))
	case []any:
		s := make([]any, len(t))
		for i, x := range t {
			s[i] = deepCopyValue(x)
		}

		return s
	default:
		return v
	}
}

// DeepCopy copies a JSON-like value tree.
func DeepCopy(m map[string]any) map[string]any { return deepCopy(m) }

func helmVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	for _, d := range bi.Deps {
		if d.Path == "helm.sh/helm/v4" {
			return d.Version
		}
	}

	return ""
}

func isManifestFile(p string) bool {
	return strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".tpl")
}
