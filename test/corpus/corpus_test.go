//go:build corpus

// Package corpus converts real-world charts end to end. Run with `just corpus`.
package corpus

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

type entry struct {
	Name    string   `json:"name"`
	Chart   string   `json:"chart"`
	Repo    string   `json:"repo"`
	Version string   `json:"version"`
	Expose  []string `json:"expose"`
}

const planted = "Planted-Op3rator-Secret"

func TestCorpus(t *testing.T) {
	b, err := os.ReadFile("../../testdata/corpus/charts.yaml")
	require.NoError(t, err)

	var entries []entry
	require.NoError(t, yaml.Unmarshal(b, &entries))

	for _, e := range entries {
		t.Run(e.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			o := chart2kro.Options{Chart: e.Chart, RepoURL: e.Repo, Version: e.Version, Expose: e.Expose}

			start := time.Now()
			res, err := chart2kro.Convert(ctx, o)
			require.NoError(t, err)
			t.Logf("%s: %+v in %s", e.Name, res.Report.Summary, time.Since(start).Round(time.Millisecond))

			// Validated by kro's own checks (also done inside Convert; re-checked from bytes).
			stream := append(append([]byte{}, res.Prerequisites...), append([]byte("\n---\n"), res.RGD...)...)
			require.NoError(t, chart2kro.Validate(ctx, stream, chart2kro.ValidateOptions{}))

			// Every value has an outcome.
			for _, v := range res.Report.Values {
				require.Contains(t, []string{"exposed", "frozen", "failed", "unused"}, v.Outcome, v.Path)
				if v.Outcome == "frozen" || v.Outcome == "failed" {
					require.NotEmpty(t, v.Reason, v.Path)
				}
			}

			// Deterministic.
			again, err := chart2kro.Convert(ctx, o)
			require.NoError(t, err)
			require.Equal(t, string(res.RGD), string(again.RGD))

			// Operator secrets never reach the output: plant one in every exposed Secret input.
			var sets []string
			for _, v := range res.Report.Values {
				if v.Outcome == "exposed" && v.Type == "string" && targetsSecret(v.Targets, res.Report) {
					sets = append(sets, v.Path+"="+planted)
				}
			}

			if len(sets) > 0 {
				o.SetString = sets
				withSecret, err := chart2kro.Convert(ctx, o)
				require.NoError(t, err)
				rep, _ := json.Marshal(withSecret.Report)
				for _, out := range [][]byte{withSecret.RGD, withSecret.Prerequisites, withSecret.Instance, rep} {
					require.NotContains(t, string(out), planted)
				}
			}
		})
	}
}

func targetsSecret(targets []string, r *chart2kro.Report) bool {
	for _, t := range targets {
		id, _, _ := strings.Cut(t, ".")
		for _, res := range r.Resources {
			if res.ID == id && res.Kind == "Secret" {
				return true
			}
		}
	}

	return false
}
