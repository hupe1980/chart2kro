package chart2kro

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/chart"
	"github.com/hupe1980/chart2kro/internal/kroval"
	"github.com/hupe1980/chart2kro/internal/manifest"
	"github.com/hupe1980/chart2kro/internal/oracle"
	"github.com/hupe1980/chart2kro/internal/probe"
	"github.com/hupe1980/chart2kro/internal/rgd"

	"github.com/kubernetes-sigs/kro/pkg/graph"
)

// Probe identities: valid DNS labels that never contain a chart name.
const (
	release1, release2     = "c2kra", "c2krb"
	namespace1, namespace2 = "c2kna", "c2knb"
)

// MaxRGDSize keeps generated RGDs well below the API server's request limit (~1.5 MiB).
const MaxRGDSize = 1 << 20

// Result of a conversion.
type Result struct {
	RGD           []byte // YAML
	Prerequisites []byte // YAML stream; empty when there are none
	Instance      []byte // example instance (required inputs are listed, not set)
	Report        *Report
}

// Convert converts a chart. See the package documentation for the guarantees.
func Convert(ctx context.Context, o Options) (*Result, error) {
	c, err := newConversion(ctx, o)
	if err != nil {
		return nil, err
	}

	return c.run(ctx)
}

// Inspect runs a conversion and returns only its report.
func Inspect(ctx context.Context, o Options) (*Report, error) {
	res, err := Convert(ctx, o)
	if res != nil {
		return res.Report, err
	}

	return nil, err
}

type conversion struct {
	o         Options
	loaded    *chart.Loaded
	r         *chart.Renderer
	values    map[string]any // merged: chart defaults ⊕ user values (the API's defaults)
	defaults  map[string]any // chart defaults only
	validator *kroval.Validator
	kind      string
	group     string
	version   string
	warnings  []string
}

func newConversion(ctx context.Context, o Options) (*conversion, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}

	loaded, err := chart.Load(ctx, o.Chart, chart.LoadOptions{
		Version: o.Version, RepoURL: o.RepoURL, Username: o.Username, Password: o.Password,
		CAFile: o.CAFile, CertFile: o.CertFile, KeyFile: o.KeyFile,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoad, err)
	}

	if loaded.Chart.Metadata != nil && loaded.Chart.Metadata.Type == "library" {
		return nil, fmt.Errorf("%w: %s is a library chart and renders no resources", ErrLoad, loaded.Chart.Name())
	}

	kube := o.KubeVersion
	if kube == "" {
		kube = kroval.DefaultKubeVersion()
	}

	r, err := chart.NewRenderer(loaded.Chart, chart.RenderOptions{KubeVersion: kube, Timeout: o.Timeout})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRender, err)
	}

	user, err := chart.UserValues(o.ValueFiles, o.Set, o.SetString, o.SetFile)
	if err != nil {
		return nil, fmt.Errorf("%w: values: %w", ErrUsage, err)
	}

	values, err := r.Values(user)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRender, err)
	}

	defaults, err := r.Values(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRender, err)
	}

	c := &conversion{o: o, loaded: loaded, r: r, values: values, defaults: defaults}

	c.kind = o.Kind
	if c.kind == "" {
		c.kind = pascal(loaded.Chart.Name())
	}

	c.group = firstNonEmpty(o.Group, "kro.run")
	c.version = firstNonEmpty(o.APIVersion, "v1alpha1")

	if !kindRe.MatchString(c.kind) {
		return nil, fmt.Errorf("%w: derived kind %q is not a valid kro kind; pass --kind", ErrUsage, c.kind)
	}

	var crds []map[string]any

	for _, f := range o.CRDFiles {
		docs, err := readYAMLDocs(f)
		if err != nil {
			return nil, fmt.Errorf("%w: --crd %s: %w", ErrUsage, f, err)
		}

		crds = append(crds, docs...)
	}

	base, err := r.Render(ctx, values, release1, namespace1)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRender, err)
	}

	for _, ob := range base {
		if ob.Obj["kind"] == "CustomResourceDefinition" {
			crds = append(crds, ob.Obj)
		}
	}

	c.validator, err = kroval.New(kube, crds)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUsage, err)
	}

	return c, nil
}

func (c *conversion) render(ctx context.Context, vals map[string]any, rel, ns string) (*probe.View, error) {
	objs, err := c.r.Render(ctx, vals, rel, ns)
	if err != nil {
		return nil, err
	}

	return probe.NewView(objs), nil
}

// probeRun holds a candidate's classification and, to bound memory, only the
// objects its probes touched (everything else equals the verified baseline).
type probeRun struct {
	result     probe.Result
	p1, p2     *probe.View
	err1, err2 error
}

func prune(v *probe.View, keep map[string]bool) *probe.View {
	if v == nil {
		return nil
	}

	out := &probe.View{Objects: map[string]*probe.Obj{}}

	for _, k := range v.Order {
		if keep[k] {
			out.Objects[k] = v.Objects[k]
			out.Order = append(out.Order, k)
		}
	}

	return out
}

func touched(r probe.Result) map[string]bool {
	keep := map[string]bool{}
	for _, t := range r.Targets {
		keep[t.Key] = true
	}

	for _, k := range append(append([]string{}, r.Added...), r.Removed...) {
		keep[k] = true
	}

	return keep
}

func (c *conversion) run(ctx context.Context) (*Result, error) {
	log := c.o.logger()

	b1, err := c.render(ctx, c.values, release1, namespace1)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRender, err)
	}

	b2, err := c.render(ctx, c.values, release1, namespace1)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRender, err)
	}

	noise := probe.Noise{}
	probe.AddNoise(noise, b1, b2)

	if err := c.checkIdentity(ctx, b1, noise); err != nil {
		return nil, err
	}

	cands := probe.Collect(c.values, c.o.Expose, c.o.Freeze)
	if err := c.checkExposeGlobs(cands); err != nil {
		return nil, err
	}

	log.Info("probing values", "candidates", len(cands))

	runs := c.probeAll(ctx, cands, b1, noise)

	results := make([]probe.Result, len(cands))
	conditional := map[string]*probe.Obj{}

	for i, cand := range cands {
		pr := runs[i]
		results[i] = pr.result

		if results[i].Outcome == probe.Exposed && cand.Kind == probe.Bool {
			probe.AddNoise(noise, pr.p1, pr.p2)

			for _, k := range results[i].Added {
				conditional[k] = pr.p1.Objects[k]
			}
		}
	}

	condWitnesses := c.refineConditional(ctx, results, runs, cands)
	witnesses := len(runs)*2 + 2 + len(condWitnesses)

	var (
		out *rgd.Output
		g   *graph.Graph
	)

	// demoted maps value paths to "reason\x00detail".
	demoted := map[string]string{}

	for iter := 0; ; iter++ {
		if iter > len(cands)+2 {
			return nil, fmt.Errorf("%w: verification did not converge", ErrConstraint)
		}

		for i := range results {
			if d, ok := demoted[results[i].Candidate.Dotted()]; ok && results[i].Outcome == probe.Exposed {
				why, detail, _ := strings.Cut(d, "\x00")

				outcome := probe.Frozen
				if why == probe.ReasonOnlyDropped {
					outcome = probe.Unused // it has no effect on any instance
				}

				results[i].Outcome, results[i].Reason, results[i].Detail = outcome, why, detail
			}
		}

		out, err = rgd.Assemble(rgd.Input{
			Base: b1, Base2: b2, Noise: noise, Results: results,
			Conditional: conditional, UserSecrets: c.userSecrets(results, b1, conditional),
		}, rgd.Options{
			Name: dnsName(c.loaded.Chart.Name()), Kind: c.kind, Group: c.group, APIVersion: c.version,
			ChartName: c.loaded.Chart.Name(), ChartVersion: chartVersion(c.loaded),
			Release: release1, Namespace: namespace1,
			IncludeHooks: c.o.IncludeHooks, Exclude: c.o.Exclude, ExcludeSubcharts: c.o.ExcludeSubcharts,
			Scope: c.validator.Scope,
		})
		if err != nil {
			return nil, err
		}

		if len(out.Demote) > 0 {
			for p, why := range out.Demote {
				demoted[p] = why + "\x00" + reasonText[why]
			}

			continue
		}

		g, err = c.validator.Validate(ctx, out.RGD)
		if err != nil {
			blamed := blame(err.Error(), results)
			for p, why := range blameResources(err.Error(), out, results) {
				blamed[p] = why
			}

			if len(blamed) == 0 {
				return nil, fmt.Errorf("%w: %w%s%s", ErrInvalid, err, sources(err.Error(), out), crdHint(err.Error()))
			}

			for p, why := range blamed {
				if why == probe.ReasonKroType {
					if i := c.retryAsStrings(ctx, p, cands, results, runs, b1, noise); i >= 0 {
						continue
					}
				}

				demoted[p] = why + "\x00" + kroProblem(err.Error())
			}

			continue
		}

		bad, err := c.verify(ctx, g, out, b1, noise, results, runs, cands, condWitnesses)
		if err != nil {
			return nil, err
		}

		if len(bad) == 0 {
			break
		}

		for p, d := range bad {
			demoted[p] = probe.ReasonOracle + "\x00" + d
		}
	}

	if err := c.checkStrictExpose(results); err != nil {
		return nil, err
	}

	return c.result(out, results, witnesses)
}

// reasonText explains assembly-time demotions.
var reasonText = map[string]string{
	probe.ReasonKeyNotAddressable: "a key starts with a digit or has characters CEL cannot address",
	probe.ReasonConflict:          "another value or the release name controls part of the same field",
	probe.ReasonOnlyDropped:       "every field it controls is in objects that are not part of the RGD (hooks, excluded, prerequisites)",
	probe.ReasonTernaryTypes:      "the two branches have different types",
}

// retryAsStrings re-probes an empty list as a list of strings once, when kro's type
// check rejects it as a list of objects (e.g. container args). Returns the index or -1.
func (c *conversion) retryAsStrings(ctx context.Context, p string, cands []probe.Candidate, results []probe.Result,
	runs []probeRun, base *probe.View, noise probe.Noise,
) int {
	for i := range cands {
		cand := &cands[i]
		if cand.Dotted() != p || cand.Kind != probe.List || cand.Elem != "object" {
			continue
		}

		if l, _ := cand.Default.([]any); len(l) > 0 {
			return -1
		}

		cand.Elem = "string"
		runs[i] = c.probeOne(ctx, *cand, base, noise)
		results[i] = runs[i].result

		return i
	}

	return -1
}

// kroProblem extracts the first problem line of a kro validation error.
func kroProblem(msg string) string {
	for _, l := range strings.Split(msg, "\n") {
		if l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "- ")); l != "" && !strings.HasPrefix(l, "kro rejects") {
			return l
		}
	}

	return msg
}

// probeAll renders both probe points of every candidate on a worker pool.
func (c *conversion) probeAll(ctx context.Context, cands []probe.Candidate, base *probe.View, noise probe.Noise) []probeRun {
	runs := make([]probeRun, len(cands))

	n := c.o.Parallelism
	if n <= 0 {
		n = runtime.GOMAXPROCS(0)
	}

	sem := make(chan struct{}, n)

	var progress atomic.Int64

	var wg sync.WaitGroup

	for i, cand := range cands {
		wg.Add(1)

		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			runs[i] = c.probeOne(ctx, cand, base, noise)

			if done := progress.Add(1); done%200 == 0 {
				c.o.logger().Info("probing", "done", done, "of", len(cands))
			}
		}()
	}

	wg.Wait()

	return runs
}

// probeOne renders both probe points of one candidate and classifies it.
func (c *conversion) probeOne(ctx context.Context, cand probe.Candidate, base *probe.View, noise probe.Noise) probeRun {
	var pr probeRun

	pr.p1, pr.err1 = c.render(ctx, probe.With(c.values, cand.Path, cand.Value(1)), release1, namespace1)
	pr.p2, pr.err2 = c.render(ctx, probe.With(c.values, cand.Path, cand.Value(2)), release1, namespace1)
	pr.result = probe.Classify(cand, base, noise, pr.p1, pr.p2, pr.err1, pr.err2)

	keep := touched(pr.result)
	pr.p1, pr.p2 = prune(pr.p1, keep), prune(pr.p2, keep)

	return pr
}

// checkIdentity warns where the release name is transformed (e.g. truncated) in output.
func (c *conversion) checkIdentity(ctx context.Context, b1 *probe.View, noise probe.Noise) error {
	i2, err := c.render(ctx, c.values, release2, namespace2)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRender, err)
	}

	for _, k := range b1.Order {
		o2, ok := i2.Objects[k]
		if !ok {
			continue
		}

		expect := substitute(b1.Objects[k].Value, map[string]string{release1: release2, namespace1: namespace2})
		for _, ch := range manifest.Diff(expect, o2.Value) {
			if noise.Has(k, ch.Path) || probe.IsChecksumAnnotation(ch.Path) {
				continue
			}

			c.warnings = append(c.warnings, fmt.Sprintf("%s %s: the release name or namespace is transformed by the template; templated anyway", b1.Objects[k].Kind(), ch.Path))
		}
	}

	return nil
}

func substitute(v any, repl map[string]string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = substitute(x, repl)
		}

		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = substitute(x, repl)
		}

		return out
	case string:
		for a, b := range repl {
			t = strings.ReplaceAll(t, a, b)
		}

		return t
	default:
		return v
	}
}

func (c *conversion) checkExposeGlobs(cands []probe.Candidate) error {
	var unmatched []string

	for _, g := range c.o.Expose {
		ok := false

		for _, cand := range cands {
			if probe.MatchAny([]string{g}, cand.Dotted()) {
				ok = true
				break
			}
		}

		if !ok {
			unmatched = append(unmatched, g)
		}
	}

	if len(unmatched) > 0 {
		return fmt.Errorf("%w: --expose %s matches no chart value", ErrConstraint, strings.Join(unmatched, ", "))
	}

	return nil
}

// checkStrictExpose fails when an explicitly exposed path could not be exposed.
func (c *conversion) checkStrictExpose(results []probe.Result) error {
	if len(c.o.Expose) == 0 {
		return nil
	}

	var bad []string

	for _, r := range results {
		if r.Outcome != probe.Exposed && r.Outcome != probe.Unused {
			bad = append(bad, fmt.Sprintf("%s (%s: %s)", r.Candidate.Dotted(), r.Reason, placeholders(r.Detail)))
		}
	}

	if len(bad) > 0 {
		return fmt.Errorf("%w: cannot expose:\n  %s", ErrConstraint, strings.Join(bad, "\n  "))
	}

	return nil
}

// userSecrets returns exposed values supplied by the user (different from the chart's
// default) that land in Secret data: they must not become schema defaults.
func (c *conversion) userSecrets(results []probe.Result, b1 *probe.View, conditional map[string]*probe.Obj) map[string]bool {
	out := map[string]bool{}

	for _, r := range results {
		if r.Outcome != probe.Exposed {
			continue
		}

		def, ok := lookup(c.defaults, r.Candidate.Path)
		if ok && manifest.Equal(def, r.Candidate.Default) {
			continue
		}

		for _, t := range r.Targets {
			ob := b1.Objects[t.Key]
			if ob == nil {
				ob = conditional[t.Key] // Secrets that exist only behind a flag count too
			}

			if ob != nil && ob.Kind() == "Secret" && len(t.Path) > 0 &&
				(t.Path[0] == "data" || t.Path[0] == "stringData") {
				out[r.Candidate.Dotted()] = true
			}
		}
	}

	return out
}

// blame finds exposed values whose schema reference appears in a kro error.
func blame(msg string, results []probe.Result) map[string]string {
	out := map[string]string{}

	for _, r := range results {
		if r.Outcome != probe.Exposed {
			continue
		}

		ref, ok := rgd.SpecRef(r.Candidate.Path)
		if !ok {
			continue
		}

		for _, end := range []string{"\"", ")", " ", "}", ","} {
			if strings.Contains(msg, ref+end) {
				out[r.Candidate.Dotted()] = probe.ReasonKroType
				break
			}
		}
	}

	return out
}

var resourceInMsg = regexp.MustCompile(`resource "([a-zA-Z0-9]+)"`)

// blameResources freezes booleans whose conditional objects kro cannot validate
// (typically custom resources whose CRDs are not available offline).
func blameResources(msg string, out *rgd.Output, results []probe.Result) map[string]string {
	blamed := map[string]string{}

	for _, m := range resourceInMsg.FindAllStringSubmatch(msg, -1) {
		key := out.Keys[m[1]]
		if key == "" {
			continue
		}

		for _, r := range results {
			if r.Outcome != probe.Exposed {
				continue
			}

			for _, k := range append(append([]string{}, r.Added...), r.Removed...) {
				if k == key {
					why := probe.ReasonKroType
					if strings.Contains(msg, "cannot resolve") {
						why = probe.ReasonUnknownKind
					}

					blamed[r.Candidate.Dotted()] = why
				}
			}
		}
	}

	return blamed
}

// sources names the chart templates of the resources a kro error mentions.
func sources(msg string, out *rgd.Output) string {
	var lines []string

	seen := map[string]bool{}

	for _, m := range resourceInMsg.FindAllStringSubmatch(msg, -1) {
		if key := out.Keys[m[1]]; key != "" && !seen[m[1]] {
			seen[m[1]] = true
			lines = append(lines, fmt.Sprintf("%s comes from %s", m[1], placeholders(key)))
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return "\n  " + strings.Join(lines, "\n  ")
}

func crdHint(msg string) string {
	if strings.Contains(msg, "cannot resolve") {
		return "\n  hint: pass the CRDs of custom resources with --crd FILE"
	}

	return ""
}

// verify compares kro's instantiation with Helm at every witness; it returns values to demote.
func (c *conversion) verify(ctx context.Context, g *graph.Graph, out *rgd.Output, b1 *probe.View, noise probe.Noise,
	results []probe.Result, runs []probeRun, cands []probe.Candidate, conds []condWitness,
) (map[string]string, error) {
	exposed := map[string]probe.Candidate{}

	for _, r := range results {
		if r.Outcome == probe.Exposed {
			exposed[r.Candidate.Dotted()] = r.Candidate
		}
	}

	base := c.instanceSpec(out, exposed, nil)

	diffs, err := c.compare(ctx, g, out, b1, base, noise)
	if err != nil {
		return nil, err
	}

	if len(diffs) > 0 {
		return nil, fmt.Errorf("%w: the default instance differs from Helm (please report this): %s", ErrConstraint, strings.Join(diffs, "; "))
	}

	bad := map[string]string{}

	for i, cand := range cands {
		if _, ok := exposed[cand.Dotted()]; !ok {
			continue
		}

		for point, view := range []*probe.View{runs[i].p1, runs[i].p2} {
			spec := c.instanceSpec(out, exposed, map[string]any{cand.Dotted(): cand.Value(point + 1)})

			diffs, err := c.compareOnly(ctx, g, out, view, spec, noise, view.Objects)
			if err != nil {
				bad[cand.Dotted()] = "kro could not evaluate it: " + firstLine(err.Error())
				break
			}

			if len(diffs) > 0 {
				bad[cand.Dotted()] = fmt.Sprintf("at probe %d: %s", point+1, diffs[0])
				break
			}
		}
	}

	if len(bad) > 0 {
		return bad, nil
	}

	for _, w := range conds {
		if _, ok := exposed[w.flag]; !ok {
			continue
		}

		over := map[string]any{}

		var live []string

		for p, v := range w.over {
			if _, ok := exposed[p]; ok {
				over[p] = v
			}
		}

		for _, p := range w.values {
			if _, ok := exposed[p]; ok {
				live = append(live, p)
			}
		}

		diffs, err := c.compareOnly(ctx, g, out, w.view, c.instanceSpec(out, exposed, over), noise, w.view.Objects)
		if err == nil && len(diffs) == 0 {
			continue
		}

		detail := "inside objects enabled by " + w.flag
		if len(diffs) > 0 {
			detail += ": " + diffs[0]
		}

		if len(live) == 0 {
			bad[w.flag] = detail
		}

		for _, p := range live {
			bad[p] = detail
		}

		return bad, nil
	}

	// Joint witness: every exposed non-boolean value at its second probe at once.
	var joint []probe.Candidate

	for _, cand := range cands {
		if _, ok := exposed[cand.Dotted()]; ok && cand.Kind != probe.Bool {
			joint = append(joint, cand)
		}
	}

	return c.bisect(ctx, g, out, noise, exposed, joint)
}

func (c *conversion) bisect(ctx context.Context, g *graph.Graph, out *rgd.Output, noise probe.Noise,
	exposed map[string]probe.Candidate, set []probe.Candidate,
) (map[string]string, error) {
	if len(set) < 2 {
		return nil, nil
	}

	switch c.jointOK(ctx, g, out, noise, exposed, set) {
	case jointPass:
		return nil, nil
	case jointUnrenderable:
		// Some combination breaks the chart: verify the halves instead of passing (no
		// fail-open). Single values are already verified on their own.
		if len(set) == 2 {
			c.warnings = append(c.warnings, fmt.Sprintf("combination of %s and %s could not be rendered, so it was not verified", set[0].Dotted(), set[1].Dotted()))
			return nil, nil
		}
	case jointMismatch:
		if len(set) == 2 {
			return map[string]string{set[1].Dotted(): "wrong when combined with " + set[0].Dotted()}, nil
		}
	}

	mid := len(set) / 2

	for _, half := range [][]probe.Candidate{set[:mid], set[mid:]} {
		bad, err := c.bisect(ctx, g, out, noise, exposed, half)
		if err != nil || len(bad) > 0 {
			return bad, err
		}
	}

	if c.jointOK(ctx, g, out, noise, exposed, set) == jointMismatch {
		// Each half passes alone; blame the interaction's last value.
		return map[string]string{set[len(set)-1].Dotted(): "wrong when combined with other values"}, nil
	}

	return nil, nil
}

type jointStatus int

const (
	jointPass jointStatus = iota
	jointMismatch
	jointUnrenderable
)

func (c *conversion) jointOK(ctx context.Context, g *graph.Graph, out *rgd.Output, noise probe.Noise,
	exposed map[string]probe.Candidate, set []probe.Candidate,
) jointStatus {
	vals := c.values
	over := map[string]any{}

	for _, cand := range set {
		vals = probe.With(vals, cand.Path, cand.Value(2))
		over[cand.Dotted()] = cand.Value(2)
	}

	view, err := c.render(ctx, vals, release1, namespace1)
	if err != nil {
		return jointUnrenderable
	}

	diffs, err := c.compare(ctx, g, out, view, c.instanceSpec(out, exposed, over), noise)
	if err != nil || len(diffs) > 0 {
		return jointMismatch
	}

	return jointPass
}

// instanceSpec builds an instance spec: every exposed value at its default, with overrides.
func (c *conversion) instanceSpec(out *rgd.Output, exposed map[string]probe.Candidate, over map[string]any) map[string]any {
	spec := map[string]any{}

	for p, cand := range exposed {
		v := cand.Default
		if x, ok := over[p]; ok {
			v = x
		}

		setPath(spec, cand.Path, v)
	}

	for _, f := range out.Fields {
		if f.Origin == "generated" {
			setPath(spec, f.Path, "generated")
		}
	}

	return manifest.NormalizeMap(spec)
}

// compare instantiates the RGD and diffs every object against a Helm view.
func (c *conversion) compare(ctx context.Context, g *graph.Graph, out *rgd.Output, helm *probe.View, spec map[string]any, noise probe.Noise) ([]string, error) {
	return c.compareOnly(ctx, g, out, helm, spec, noise, nil)
}

// compareOnly diffs only the given objects (nil = all). A probe witness needs only the
// objects its value touched: the rest are identical to the verified default instance.
func (c *conversion) compareOnly(ctx context.Context, g *graph.Graph, out *rgd.Output, helm *probe.View, spec map[string]any, noise probe.Noise, only map[string]*probe.Obj) ([]string, error) {
	nodes, err := oracle.Instantiate(ctx, g, release1, namespace1, spec)
	if err != nil {
		return nil, err
	}

	var diffs []string

	got := map[string]map[string]any{}

	for _, n := range nodes {
		if n.Pending != "" {
			diffs = append(diffs, n.ID+": pending: "+n.Pending)
			continue
		}

		if n.Ignored || len(n.Objects) == 0 {
			continue
		}

		got[out.Keys[n.ID]] = n.Objects[0]
	}

	for key := range out.IDs {
		if only != nil {
			if _, ok := only[key]; !ok {
				continue
			}
		}

		h, inHelm := helm.Objects[key]
		k, inKro := got[key]

		switch {
		case !inHelm && !inKro:
			continue
		case inHelm != inKro:
			diffs = append(diffs, fmt.Sprintf("%s present in helm=%t kro=%t", out.IDs[key], inHelm, inKro))
			continue
		}

		hv := normalizeForCompare(decodeSecretView(h.Value), key, noise, nil)
		kv := normalizeForCompare(decodeSecret(k), key, noise, out.Injected[key])

		for _, d := range manifest.Diff(hv, kv) {
			diffs = append(diffs, fmt.Sprintf("%s %s: helm=%v kro=%v", out.IDs[key], d.Path, d.A, d.B))
		}
	}

	sort.Strings(diffs)

	return diffs, nil
}

// normalizeForCompare applies the declared transforms: namespace, checksum annotations,
// generated values (noise) and injected isolation labels are not compared.
func normalizeForCompare(v map[string]any, key string, noise probe.Noise, injected []manifest.Path) map[string]any {
	out := manifest.NormalizeMap(v)

	if md, ok := out["metadata"].(map[string]any); ok {
		delete(md, "namespace")
	}

	var drop []manifest.Path

	manifest.Leaves(out, func(p manifest.Path, _ any) {
		if noise.Has(key, p) || probe.IsChecksumAnnotation(p) {
			drop = append(drop, p)
		}
	})

	for _, p := range append(drop, injected...) {
		manifest.Delete(out, p)
	}

	return out
}

// decodeSecret returns a Secret's data as decoded strings, with stringData merged in
// (as the API server does): a declared transform, since computed entries are written
// to stringData.
func decodeSecret(v map[string]any) map[string]any {
	out := manifest.NormalizeMap(v)
	if out["kind"] != "Secret" {
		return out
	}

	data, _ := out["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}

	for k, raw := range data {
		if s, ok := raw.(string); ok {
			if dec, err := base64.StdEncoding.DecodeString(s); err == nil {
				data[k] = string(dec)
			}
		}
	}

	if sd, ok := out["stringData"].(map[string]any); ok {
		for k, x := range sd {
			data[k] = x
		}

		delete(out, "stringData")
	}

	if len(data) > 0 {
		out["data"] = data
	}

	return out
}

func setPath(m map[string]any, p []string, v any) {
	cur := m

	for _, s := range p[:len(p)-1] {
		next, ok := cur[s].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[s] = next
		}

		cur = next
	}

	cur[p[len(p)-1]] = v
}

func lookup(m map[string]any, p []string) (any, bool) {
	var cur any = m

	for _, s := range p {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}

		cur, ok = mm[s]
		if !ok {
			return nil, false
		}
	}

	return cur, true
}

func readYAMLDocs(path string) ([]map[string]any, error) {
	b, err := os.ReadFile(path) //nolint:gosec // user-provided path
	if err != nil {
		return nil, err
	}

	var out []map[string]any

	for _, d := range splitDocs(b) {
		var m map[string]any
		if err := yaml.Unmarshal(d, &m); err != nil {
			return nil, err
		}

		if len(m) > 0 {
			out = append(out, m)
		}
	}

	return out, nil
}

func splitDocs(b []byte) [][]byte {
	var out [][]byte

	for _, d := range strings.Split("\n"+string(b), "\n---") {
		if strings.TrimSpace(d) != "" {
			out = append(out, []byte(d))
		}
	}

	return out
}

func pascal(s string) string {
	var b strings.Builder

	up := true

	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			up = true
			continue
		}

		if up {
			b.WriteRune(unicode.ToUpper(r))
			up = false
		} else {
			b.WriteRune(r)
		}
	}

	return b.String()
}

func dnsName(s string) string {
	s = strings.ToLower(s)

	var b strings.Builder

	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}

	return strings.Trim(b.String(), "-.")
}

func chartVersion(l *chart.Loaded) string {
	if l.Chart.Metadata != nil {
		return l.Chart.Metadata.Version
	}

	return ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}

	return b
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}

	return s
}

// decodeSecretView merges a Helm Secret's stringData into its (already decoded) data.
func decodeSecretView(v map[string]any) map[string]any {
	out := manifest.NormalizeMap(v)
	if out["kind"] != "Secret" {
		return out
	}

	if sd, ok := out["stringData"].(map[string]any); ok {
		data, _ := out["data"].(map[string]any)
		if data == nil {
			data = map[string]any{}
			out["data"] = data
		}

		for k, x := range sd {
			data[k] = x
		}

		delete(out, "stringData")
	}

	return out
}
