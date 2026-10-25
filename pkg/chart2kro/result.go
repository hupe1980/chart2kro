package chart2kro

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hupe1980/chart2kro/internal/kroval"
	"github.com/hupe1980/chart2kro/internal/probe"
	"github.com/hupe1980/chart2kro/internal/rgd"
	"github.com/hupe1980/chart2kro/internal/yamlx"
)

func (c *conversion) result(out *rgd.Output, results []probe.Result, witnesses int) (*Result, error) {
	rgdYAML, err := yamlx.Marshal(out.RGD)
	if err != nil {
		return nil, err
	}

	if len(rgdYAML) > MaxRGDSize {
		return nil, fmt.Errorf("%w: the RGD is %d bytes (limit %d); exclude resources (--exclude ID) or subcharts (--exclude-subchart)\n  largest: %s",
			ErrConstraint, len(rgdYAML), MaxRGDSize, strings.Join(largest(out.RGD, 5), ", "))
	}

	var prereq []byte

	if len(out.Prerequisites) > 0 {
		docs := make([]any, len(out.Prerequisites))
		for i, p := range out.Prerequisites {
			docs[i] = p
		}

		prereq, err = yamlx.MarshalStream(docs...)
		if err != nil {
			return nil, err
		}
	}

	rep := c.report(out, results, witnesses)

	inst, err := exampleInstance(c.group, c.version, c.kind, dnsName(c.loaded.Chart.Name()), rep.Inputs)
	if err != nil {
		return nil, err
	}

	return &Result{RGD: rgdYAML, Prerequisites: prereq, Instance: inst, Report: rep}, nil
}

func exampleInstance(group, version, kind, name string, inputs []InputReport) ([]byte, error) {
	body, err := yamlx.Marshal(map[string]any{
		"apiVersion": group + "/" + version,
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{},
	})
	if err != nil {
		return nil, err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "# Example %s instance. Every field defaults to the chart's values;\n", kind)

	if len(inputs) == 0 {
		b.WriteString("# set only what you want to change.\n")
	} else {
		b.WriteString("# these fields have no default and must be set:\n")
		for _, in := range inputs {
			fmt.Fprintf(&b, "#   spec.%s (%s)\n", in.Path, in.Reason)
		}
	}

	b.Write(body)

	return []byte(b.String()), nil
}

func (c *conversion) report(out *rgd.Output, results []probe.Result, witnesses int) *Report {
	rep := &Report{
		SchemaVersion: ReportSchema,
		Chart: ChartRef{
			Name: c.loaded.Chart.Name(), Version: chartVersion(c.loaded),
			Source: c.loaded.Ref, Digest: c.loaded.Digest,
		},
		Kind: c.kind, Group: c.group, APIVersion: c.version,
		KubeVersion: c.validator.KubeVersion(), KroVersion: kroval.KroVersion(),
		Witnesses: witnesses,
		Warnings:  dedupe(c.warnings),
	}

	fields := map[string]rgd.Field{}
	for _, f := range out.Fields {
		fields[strings.Join(f.Path, ".")] = f
	}

	for _, r := range results {
		vr := ValueReport{Path: r.Candidate.Dotted(), Outcome: string(r.Outcome), Reason: r.Reason, Detail: r.Detail}

		switch r.Outcome {
		case probe.Exposed:
			f := fields[vr.Path]
			vr.Type, vr.Default, vr.Targets, vr.Kind = f.Type, f.Default, f.Targets, kindOf(r)
			rep.Summary.Exposed++

			if f.Required {
				rep.Inputs = append(rep.Inputs, InputReport{Path: vr.Path, Reason: probe.ReasonSecretUser})
			}
		case probe.Frozen:
			rep.Summary.Frozen++
		case probe.Failed:
			rep.Summary.Failed++
		case probe.Unused:
			rep.Summary.Unused++
		}

		rep.Values = append(rep.Values, vr)
	}

	for _, f := range out.Fields {
		if f.Origin == "generated" {
			rep.Inputs = append(rep.Inputs, InputReport{Path: strings.Join(f.Path, "."), Reason: "generated"})
		}
	}

	isolated := map[string]bool{}
	for _, id := range out.Isolated {
		isolated[id] = true
	}

	spec, _ := out.RGD["spec"].(map[string]any)
	resources, _ := spec["resources"].([]any)

	for _, x := range resources {
		res := x.(map[string]any)
		id := res["id"].(string)
		t := res["template"].(map[string]any)

		rr := ResourceReport{ID: id, Kind: fmt.Sprint(t["kind"]), Source: sourceOf(out.Keys[id]), Isolated: isolated[id]}

		if iw, ok := res["includeWhen"].([]any); ok {
			for _, e := range iw {
				rr.IncludeWhen = append(rr.IncludeWhen, fmt.Sprint(e))
			}
		}

		rep.Resources = append(rep.Resources, rr)
	}

	for _, p := range out.Prerequisites {
		md, _ := p["metadata"].(map[string]any)
		rep.Prerequisites = append(rep.Prerequisites, ObjectRef{Kind: fmt.Sprint(p["kind"]), Name: placeholders(fmt.Sprint(md["name"]))})
	}

	for _, d := range out.Dropped {
		rep.Dropped = append(rep.Dropped, DroppedObject{Kind: d.Kind, Name: placeholders(d.Name), Source: d.Source, Reason: d.Reason})
	}

	rep.Warnings = append(rep.Warnings, dedupe(out.Warnings)...)
	for i, w := range rep.Warnings {
		rep.Warnings[i] = placeholders(w)
	}

	for i := range rep.Values {
		rep.Values[i].Detail = placeholders(rep.Values[i].Detail)
	}

	sort.Slice(rep.Inputs, func(i, j int) bool { return rep.Inputs[i].Path < rep.Inputs[j].Path })

	return rep
}

func kindOf(r probe.Result) string {
	switch r.Candidate.Kind {
	case probe.Bool:
		return "conditional"
	case probe.List, probe.Map:
		return "whole"
	default:
	}

	for _, t := range r.Targets {
		if t.Mode == probe.Span {
			return "interpolation"
		}
	}

	return "reference"
}

func sourceOf(key string) string {
	if i := strings.Index(key, "#"); i >= 0 {
		return key[:i]
	}

	return key
}

func dedupe(s []string) []string {
	seen := map[string]bool{}

	var out []string

	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}

	return out
}

// placeholders replaces the probe identity in user-facing text.
func placeholders(s string) string {
	s = objectKey.ReplaceAllString(s, "$4 ($1)")

	return strings.NewReplacer(release1, "<name>", namespace1, "<namespace>").Replace(s)
}

// objectKey matches internal object identities "source#ordinal#apiVersion/Kind".
var objectKey = regexp.MustCompile(`([^\s#]+)#(\d+)#(\S+)/(\w+)`)

// largest names the biggest resources of an RGD with their sizes.
func largest(rgdObj map[string]any, n int) []string {
	spec, _ := rgdObj["spec"].(map[string]any)
	resources, _ := spec["resources"].([]any)

	type sized struct {
		id   string
		size int
	}

	var all []sized

	for _, x := range resources {
		res, _ := x.(map[string]any)
		b, _ := yamlx.Marshal(res)
		all = append(all, sized{fmt.Sprint(res["id"]), len(b)})
	}

	sort.Slice(all, func(i, j int) bool { return all[i].size > all[j].size })

	var out []string

	for i := 0; i < len(all) && i < n; i++ {
		out = append(out, fmt.Sprintf("%s (%d KiB)", all[i].id, all[i].size/1024))
	}

	return out
}
