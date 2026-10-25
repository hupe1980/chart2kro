package chart2kro

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/hupe1980/chart2kro/internal/chart"
	"github.com/hupe1980/chart2kro/internal/probe"
)

// loopHint tells users how to make an empty object list convertible.
const loopHint = "the default list is empty, so its entries' fields are unknown; give it an example entry with -f"

// detectLoops turns lists whose probes add or remove objects into kro collections.
// Lists with an empty default that loop over string entries get an example
// baseline: values unused at the defaults are probed again with one example entry.
// Lists are examined in parallel on the probe pool; results are applied afterwards.
func (c *conversion) detectLoops(ctx context.Context, cands []probe.Candidate, results []probe.Result, runs []probeRun,
	b1 *probe.View, noise probe.Noise,
) int {
	type decision struct {
		loop           *probe.Loop
		reason, detail string
		renders        int
	}

	decisions := make([]decision, len(cands))

	c.pool(len(cands), func(i int) {
		r, cand := results[i], cands[i]

		switch {
		case cand.Kind == probe.Map && r.Outcome == probe.Frozen && r.Reason == probe.ReasonObjects:
			decisions[i] = decision{reason: probe.ReasonRangeOverMap, detail: "kro iterates lists only; " + r.Detail}
			return
		case cand.Kind != probe.List || (r.Outcome != probe.Frozen && r.Outcome != probe.Failed):
			return
		case r.Reason != probe.ReasonObjects && !c.multiplies(ctx, cand, b1, r):
			return // not a loop: the pre-check render is not a witness
		}

		l, n, reason, detail := c.loop(ctx, cand, b1, noise)
		decisions[i] = decision{loop: l, reason: reason, detail: detail, renders: n}
	})

	witnesses := 0

	for i, d := range decisions {
		switch {
		case d.loop != nil:
			witnesses += d.renders // only renders of collections are compared with kro

			results[i] = probe.Result{Candidate: cands[i], Outcome: probe.Exposed, Loop: d.loop}
			runs[i] = probeRun{result: results[i]}
		case d.reason != "":
			results[i].Reason, results[i].Detail = d.reason, d.detail
		}
	}

	for i, d := range decisions {
		if d.loop != nil && d.loop.Example != nil {
			witnesses += c.reprobeWithExample(ctx, cands, results, runs, i, d.loop, noise)
		}
	}

	return witnesses
}

// pool runs fn(0..n-1) on the conversion's worker pool.
func (c *conversion) pool(n int, fn func(i int)) {
	workers := c.o.Parallelism
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}

	sem := make(chan struct{}, workers)

	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)

		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			fn(i)
		}()
	}

	wg.Wait()
}

// multiplies reports whether one more copy of the list's first entry adds objects
// (a cheap pre-check before the loop probes).
func (c *conversion) multiplies(ctx context.Context, cand probe.Candidate, b1 *probe.View, r probe.Result) bool {
	entries, _ := cand.Default.([]any)
	if len(entries) == 0 {
		return r.Reason == probe.ReasonObjects // without an entry, only an added object shows a loop
	}

	v, err := c.render(ctx, probe.With(c.values, cand.Path, append(append([]any{}, entries...), entries[0])), release1, namespace1)

	return err == nil && countObjects(v) != countObjects(b1)
}

func countObjects(v *probe.View) int {
	n := 0

	for _, o := range v.Objects {
		if !chart.IsTestHook(o.Hook) {
			n++
		}
	}

	return n
}

// loop examines one list; it returns the loop and the number of renders spent.
func (c *conversion) loop(ctx context.Context, cand probe.Candidate, b1 *probe.View, noise probe.Noise) (*probe.Loop, int, string, string) {
	entries, _ := cand.Default.([]any)

	var example any
	if len(entries) == 0 {
		example = fmt.Sprintf("c2kx%d", cand.Index)
		entries = []any{example}
	}

	renders := 0
	render := func(list []any) (*probe.View, error) {
		renders++
		return c.render(ctx, probe.With(c.values, cand.Path, list), release1, namespace1)
	}

	var lengths [probe.LoopLengths]*probe.View

	for n := range lengths {
		list := make([]any, n)
		for j := range list {
			list[j] = entries[0]
		}

		lengths[n], _ = render(list)
	}

	l, reason, detail := probe.DetectLoop(lengths, noise)

	switch {
	case reason != "" && example != nil:
		return nil, renders, probe.ReasonLoopShape, loopHint
	case reason != "":
		return nil, renders, reason, detail
	case l == nil && example != nil:
		// The list adds objects (its standard probe showed it), but not one per string entry.
		return nil, renders, probe.ReasonLoopShape, loopHint
	case l == nil:
		return nil, renders, "", "" // not a loop: keep the original classification
	}

	fields, reason, detail := probe.EntryFields(cand, entries)
	if reason != "" {
		return nil, renders, reason, detail
	}

	l.Template, l.Example, l.Elem = b1, example, "object"
	if example != nil {
		l.Template = lengths[1]
	}

	if _, ok := entries[0].(string); ok {
		l.Elem = "string"
	}

	// Each entry field of the first entry, at two probe values.
	for _, f := range fields {
		// A list field that multiplies objects is a nested loop.
		if sub, ok := f.Default.([]any); ok && len(sub) > 0 {
			v, err := render(withEntry0(entries, f, append(append([]any{}, sub...), sub[0])))
			if err == nil && loopObjects(v, l) != loopObjects(l.Template, l) {
				return nil, renders, probe.ReasonNestedLoop, "entry field " + f.Path[len(f.Path)-1] + " emits objects per element"
			}
		}

		var views [2]*probe.View

		var errs [2]error

		for point := 1; point <= 2; point++ {
			list := withEntry0(entries, f, f.Value(point))
			views[point-1], errs[point-1] = render(list)

			if errs[point-1] == nil {
				l.Witnesses = append(l.Witnesses, probe.LoopWitness{Value: list, View: views[point-1]})
			}
		}

		targets, reason, detail := probe.ClassifyLoopField(l, f, l.Template, noise, views[0], views[1], errs[0], errs[1])
		if reason != "" {
			return nil, renders, reason, detail
		}

		name := f.Path[len(f.Path)-1]
		if name == "[0]" {
			name = ""
		}

		l.Fields = append(l.Fields, probe.LoopField{Name: name, Type: probe.FieldType(f), Targets: targets})
	}

	// Verification witnesses: empty, one and three distinct entries.
	l.Witnesses = append(l.Witnesses, probe.LoopWitness{Value: []any{}, View: lengths[0]})

	for _, n := range []int{1, 3} {
		list := distinctEntries(entries[0], fields, cand.Index, n)

		v, err := render(list)
		if err != nil {
			return nil, renders, probe.ReasonRenderFailed, firstLine(err.Error())
		}

		l.Witnesses = append(l.Witnesses, probe.LoopWitness{Value: list, View: v})
	}

	return l, renders, "", ""
}

// loopObjects counts the objects of a loop's template files in a render.
func loopObjects(v *probe.View, l *probe.Loop) int {
	srcs := map[string]bool{}
	for _, p := range l.Positions {
		srcs[p.Source] = true
	}

	n := 0

	for _, o := range v.Objects {
		if srcs[o.Source] {
			n++
		}
	}

	return n
}

// withEntry0 returns the list with the first entry's field f set to v.
func withEntry0(entries []any, f probe.Candidate, v any) []any {
	out := append([]any{}, entries...)

	name := f.Path[len(f.Path)-1]
	if name == "[0]" {
		out[0] = v
		return out
	}

	m := map[string]any{}
	for k, x := range entries[0].(map[string]any) {
		m[k] = x
	}

	m[name] = v
	out[0] = m

	return out
}

// distinctEntries builds n entries from the first one with unique strings and
// numbers, so names and keys differ between entries.
func distinctEntries(first any, fields []probe.Candidate, idx, n int) []any {
	out := make([]any, n)

	for i := range out {
		if _, ok := first.(string); ok {
			out[i] = fmt.Sprintf("c2kw%de%d", idx, i)
			continue
		}

		m := map[string]any{}
		for k, x := range first.(map[string]any) {
			m[k] = x
		}

		for j, f := range fields {
			name := f.Path[len(f.Path)-1]

			switch f.Kind {
			case probe.String:
				m[name] = fmt.Sprintf("c2kw%de%df%d", idx, i, j)
			case probe.Int:
				m[name] = float64(300000 + 100*i + j)
			case probe.Float:
				m[name] = 0.5 + float64(i) + float64(j)/10
			default:
			}
		}

		out[i] = m
	}

	return out
}

// reprobeWithExample probes, against the example baseline, every value that had no
// effect at the defaults, so values used only inside the loop body can become inputs.
func (c *conversion) reprobeWithExample(ctx context.Context, cands []probe.Candidate, results []probe.Result, runs []probeRun,
	list int, l *probe.Loop, noise probe.Noise,
) int {
	over := map[string]any{cands[list].Dotted(): []any{l.Example}}
	vals := probe.With(c.values, cands[list].Path, []any{l.Example})

	var todo []int

	for j := range cands {
		if results[j].Outcome == probe.Unused && j != list {
			todo = append(todo, j)
		}
	}

	c.pool(len(todo), func(k int) {
		j := todo[k]

		pr := c.probeWith(ctx, vals, cands[j], l.Template, noise)
		if pr.result.Outcome != probe.Unused {
			pr.over = over
			runs[j], results[j] = pr, pr.result // each worker writes only its own index
		}
	})

	return 2 * len(todo)
}
