package chart2kro

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/chart2kro/internal/manifest"
	"github.com/hupe1980/chart2kro/internal/probe"
)

// condWitness checks values inside objects that exist only when a boolean is flipped.
type condWitness struct {
	flag   string         // the boolean's path
	view   *probe.View    // Helm render (only the conditional objects)
	over   map[string]any // instance overrides: the flag flipped, values at a probe
	values []string       // values refined inside the conditional objects
}

// refineConditional attributes fields of conditional objects to values. For each exposed
// boolean that adds objects it renders the flag flipped with every scalar candidate at
// its probe (tokens are unique, so attribution is joint) and records the mappings; the
// returned witnesses verify them at both probe points.
func (c *conversion) refineConditional(ctx context.Context, results []probe.Result, runs []probeRun, cands []probe.Candidate) []condWitness {
	var out []condWitness

	for i := range results {
		flag := results[i]
		if flag.Outcome != probe.Exposed || flag.Candidate.Kind != probe.Bool || len(flag.Added) == 0 {
			continue
		}

		var joint []int

		for j, cand := range cands {
			k := cand.Kind
			if j != i && (k == probe.String || k == probe.Int || k == probe.Float) &&
				(results[j].Outcome == probe.Exposed || results[j].Outcome == probe.Unused) {
				joint = append(joint, j)
			}
		}

		views := [2]*probe.View{}

		for point := 1; point <= 2; point++ {
			vals := probe.With(c.values, flag.Candidate.Path, flag.Candidate.Value(1))
			for _, j := range joint {
				vals = probe.With(vals, cands[j].Path, cands[j].Value(point))
			}

			v, err := c.render(ctx, vals, release1, namespace1)
			if err != nil {
				c.warnings = append(c.warnings, fmt.Sprintf("values inside objects enabled by %s were not probed: %s", flag.Candidate.Dotted(), firstLine(err.Error())))
				views[0] = nil

				break
			}

			views[point-1] = v
		}

		if views[0] == nil {
			continue
		}

		refined := map[int]bool{}

		for _, key := range flag.Added {
			base := runs[i].p1.Objects[key]
			if base == nil || base.CRD || (len(base.Hook) > 0 && !c.o.IncludeHooks) {
				continue // dropped or moved to prerequisites: nothing to parameterize
			}

			q1, q2 := views[0].Objects[key], views[1].Objects[key]

			if base == nil || q1 == nil || q2 == nil {
				continue
			}

			for _, ch := range manifest.Diff(base.Value, q1.Value) {
				if ch.Kind != manifest.Changed {
					continue
				}

				v2, _ := manifest.Get(q2.Value, ch.Path)

				for j, t := range attribute(cands, joint, key, ch, v2) {
					if !hasTarget(results[j].Targets, t) {
						results[j].Targets = append(results[j].Targets, t)
					}

					refined[j] = true
				}
			}
		}

		if len(refined) == 0 {
			continue
		}

		var names []string

		for j := range refined {
			if results[j].Outcome == probe.Unused {
				results[j].Outcome = probe.Exposed
			}

			names = append(names, cands[j].Dotted())
		}

		// Verify with only the attributed values at their probes: other values may change
		// structure inside the conditional objects and would not be comparable.
		keep := map[string]bool{}
		for _, k := range flag.Added {
			keep[k] = true
		}

		for point := 1; point <= 2; point++ {
			over := map[string]any{flag.Candidate.Dotted(): flag.Candidate.Value(1)}
			vals := probe.With(c.values, flag.Candidate.Path, flag.Candidate.Value(1))

			for j := range refined {
				over[cands[j].Dotted()] = cands[j].Value(point)
				vals = probe.With(vals, cands[j].Path, cands[j].Value(point))
			}

			v, err := c.render(ctx, vals, release1, namespace1)
			if err != nil {
				continue
			}

			out = append(out, condWitness{flag: flag.Candidate.Dotted(), view: prune(v, keep), over: over, values: names})
		}
	}

	return out
}

// attribute maps one changed leaf (baseline ch.A, joint probe ch.B, second probe v2)
// to the candidates whose tokens it carries. Every token is located in the probed
// string, the baseline is rebuilt by substituting defaults (it must equal ch.A) and
// the second probe by substituting second tokens (it must equal v2).
func attribute(cands []probe.Candidate, joint []int, key string, ch manifest.Change, v2 any) map[int]probe.Target {
	out := map[int]probe.Target{}

	// Whole non-string fields: the value equals one candidate's token.
	if _, isStr := ch.B.(string); !isStr {
		for _, j := range joint {
			c := cands[j]
			if manifest.Equal(ch.B, c.Value(1)) && manifest.Equal(v2, c.Value(2)) && manifest.Equal(ch.A, c.Default) {
				out[j] = probe.Target{Key: key, Path: ch.Path, Mode: probe.Whole}
				return out
			}
		}

		return nil
	}

	b, _ := ch.B.(string)
	a, okA := ch.A.(string)
	x2, ok2 := v2.(string)

	if !okA || !ok2 {
		return nil
	}

	type occ struct{ j, start, end int }

	var found []occ

	for pos := 0; pos < len(b); {
		best := -1

		for _, j := range joint {
			if t := probe.Format(cands[j].Value(1)); strings.HasPrefix(b[pos:], t) && (best < 0 || len(t) > len(probe.Format(cands[best].Value(1)))) {
				best = j
			}
		}

		if best < 0 {
			pos++
			continue
		}

		t := probe.Format(cands[best].Value(1))
		found = append(found, occ{best, pos, pos + len(t)})
		pos += len(t)
	}

	if len(found) == 0 {
		return nil
	}

	var rebuilt, second strings.Builder

	spans := map[int][][2]int{}
	prev := 0

	for _, o := range found {
		rebuilt.WriteString(b[prev:o.start])
		second.WriteString(b[prev:o.start])

		start := rebuilt.Len()
		def := probe.Format(cands[o.j].Default)
		rebuilt.WriteString(def)
		second.WriteString(probe.Format(cands[o.j].Value(2)))
		spans[o.j] = append(spans[o.j], [2]int{start, start + len(def)})
		prev = o.end
	}

	rebuilt.WriteString(b[prev:])
	second.WriteString(b[prev:])

	if rebuilt.String() != a || second.String() != x2 {
		return nil
	}

	for j, sp := range spans {
		c := cands[j]
		t := probe.Target{Key: key, Path: ch.Path, Mode: probe.Span, Spans: sp, Stringify: c.Kind != probe.String}

		if len(found) == 1 && b == probe.Format(c.Value(1)) {
			t = probe.Target{Key: key, Path: ch.Path, Mode: probe.Whole, Stringify: c.Kind != probe.String}
		}

		out[j] = t
	}

	return out
}

func hasTarget(ts []probe.Target, t probe.Target) bool {
	for _, x := range ts {
		if x.Key == t.Key && x.Path.Key() == t.Path.Key() {
			return true
		}
	}

	return false
}
