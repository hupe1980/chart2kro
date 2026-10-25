package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

// chartFlags are shared by every command that converts a chart.
func chartFlags(f *pflag.FlagSet, o *chart2kro.Options) {
	f.StringVar(&o.Version, "version", "", "chart version (OCI and repository charts)")
	f.StringVar(&o.RepoURL, "repo", "", "chart repository URL (for NAME without a repo/ prefix)")
	f.StringVar(&o.Username, "username", "", "registry or repository username")
	f.StringVar(&o.Password, "password", "", "registry or repository password")
	f.StringVar(&o.CAFile, "ca-file", "", "TLS CA bundle for the registry or repository")
	f.StringVar(&o.CertFile, "cert-file", "", "TLS client certificate")
	f.StringVar(&o.KeyFile, "key-file", "", "TLS client key")

	f.StringArrayVarP(&o.ValueFiles, "values", "f", nil, "values file (repeatable); merged values become the API's defaults")
	f.StringArrayVar(&o.Set, "set", nil, "set a value (key=value, repeatable)")
	f.StringArrayVar(&o.SetString, "set-string", nil, "set a string value (repeatable)")
	f.StringArrayVar(&o.SetFile, "set-file", nil, "set a value from a file (key=path, repeatable)")

	f.StringVar(&o.Kind, "kind", "", "kind of the generated API (default: PascalCase chart name)")
	f.StringVar(&o.Group, "group", "", "API group (default: kro.run)")
	f.StringVar(&o.APIVersion, "api-version", "", "API version (default: v1alpha1)")

	f.StringSliceVar(&o.Expose, "expose", nil, "expose only these value paths (globs); fail if one cannot be exposed")
	f.StringSliceVar(&o.Freeze, "freeze", nil, "never expose these value paths (globs)")
	f.StringSliceVar(&o.Exclude, "exclude", nil, "drop resources by id (see 'inspect')")
	f.StringSliceVar(&o.ExcludeSubcharts, "exclude-subchart", nil, "drop every resource of a subchart")
	f.BoolVar(&o.IncludeHooks, "include-hooks", false, "keep non-test hooks as ordinary resources")

	f.StringVar(&o.KubeVersion, "kube-version", "", "Kubernetes version for rendering and validation (default: newest supported)")
	f.StringArrayVar(&o.CRDFiles, "crd", nil, "CRD file for validating custom resources (repeatable)")
	f.DurationVar(&o.Timeout, "timeout", 30*time.Second, "timeout per render")
	f.IntVar(&o.Parallelism, "parallel", 0, "concurrent renders (default: number of CPUs)")
}

func newConvertCmd(g *globals) *cobra.Command {
	var (
		o      chart2kro.Options
		outDir string
		check  bool
		report string
	)

	cmd := &cobra.Command{
		Use:   "convert CHART",
		Short: "Convert a chart into a ResourceGraphDefinition",
		Long: `Convert a chart (directory, .tgz, oci://…, or repo/chart) into a kro ResourceGraphDefinition.

Without -o the RGD is written to stdout (prerequisites first, if any). With -o DIR
chart2kro writes rgd.yaml, prerequisites.yaml (if any), instance.yaml and report.json.`,
		Example: `  chart2kro convert oci://ghcr.io/stefanprodan/charts/podinfo
  chart2kro convert ./mychart -f platform-values.yaml --expose 'image.tag,replicaCount' -o out/
  chart2kro convert ./mychart -o out/ --check   # CI: fail if out/ is stale`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if check && outDir == "" {
				return &usageError{fmt.Errorf("--check needs -o DIR")}
			}

			log, err := g.logger()
			if err != nil {
				return err
			}

			o.Chart, o.Logger = args[0], log

			res, err := chart2kro.Convert(cmd.Context(), o)
			if err != nil {
				return err
			}

			if !g.quiet {
				printSummary(g.stderr, res.Report)
			}

			if report != "" {
				if err := writeJSON(report, res.Report); err != nil {
					return err
				}
			}

			switch {
			case check:
				return checkDir(outDir, files(res))
			case outDir != "":
				return writeDir(outDir, files(res))
			default:
				if len(res.Prerequisites) > 0 {
					_, _ = g.stdout.Write(res.Prerequisites)
					_, _ = io.WriteString(g.stdout, "---\n")
				}

				_, err = g.stdout.Write(res.RGD)

				return err
			}
		},
	}

	chartFlags(cmd.Flags(), &o)
	cmd.Flags().StringVarP(&outDir, "output-dir", "o", "", "write rgd.yaml, prerequisites.yaml, instance.yaml and report.json to DIR")
	cmd.Flags().BoolVar(&check, "check", false, "compare with the files in -o DIR instead of writing (exit 9 if stale)")
	cmd.Flags().StringVar(&report, "report", "", "also write the report JSON to this file")

	return cmd
}

func files(res *chart2kro.Result) map[string][]byte {
	rep, _ := json.MarshalIndent(res.Report, "", "  ")

	m := map[string][]byte{
		"rgd.yaml":      res.RGD,
		"instance.yaml": res.Instance,
		"report.json":   append(rep, '\n'),
	}

	if len(res.Prerequisites) > 0 {
		m["prerequisites.yaml"] = res.Prerequisites
	}

	return m
}

func writeDir(dir string, fs map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", chart2kro.ErrWrite, err)
	}

	if _, ok := fs["prerequisites.yaml"]; !ok {
		_ = os.Remove(filepath.Join(dir, "prerequisites.yaml"))
	}

	for name, b := range fs {
		if err := writeAtomic(filepath.Join(dir, name), b); err != nil {
			return fmt.Errorf("%w: %w", chart2kro.ErrWrite, err)
		}
	}

	return nil
}

// writeAtomic writes via a temporary file and rename, so readers never see half a file.
func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())

		return err
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}

	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // generated manifests are not secret
		_ = os.Remove(tmp.Name())
		return err
	}

	return os.Rename(tmp.Name(), path)
}

func checkDir(dir string, fs map[string][]byte) error {
	var stale []string

	for name, want := range fs {
		got, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // user-selected output directory
		if err != nil || !bytes.Equal(got, want) {
			stale = append(stale, name)
		}
	}

	if _, ok := fs["prerequisites.yaml"]; !ok {
		if _, err := os.Stat(filepath.Join(dir, "prerequisites.yaml")); err == nil {
			stale = append(stale, "prerequisites.yaml (should not exist)")
		}
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		return fmt.Errorf("%w: %s in %s; run without --check to regenerate", chart2kro.ErrStale, strings.Join(stale, ", "), dir)
	}

	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	if err := writeAtomic(path, append(b, '\n')); err != nil {
		return fmt.Errorf("%w: %w", chart2kro.ErrWrite, err)
	}

	return nil
}

func printSummary(w io.Writer, r *chart2kro.Report) {
	fmt.Fprintf(w, "%s %s → %s.%s/%s: %d resources, %d exposed, %d frozen, %d failed, %d unused (%d witnesses, kro %s, Kubernetes %s)\n",
		r.Chart.Name, r.Chart.Version, r.Kind, r.Group, r.APIVersion, len(r.Resources),
		r.Summary.Exposed, r.Summary.Frozen, r.Summary.Failed, r.Summary.Unused, r.Witnesses, r.KroVersion, r.KubeVersion)

	reasons := map[string]int{}

	for _, v := range r.Values {
		if v.Reason != "" {
			reasons[v.Outcome+": "+v.Reason]++
		}
	}

	if len(reasons) > 0 {
		keys := make([]string, 0, len(reasons))
		for k := range reasons {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		tw := tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)
		for _, k := range keys {
			fmt.Fprintf(tw, "  %s\t%d\n", k, reasons[k])
		}

		_ = tw.Flush()
		fmt.Fprintln(w, "  run 'chart2kro inspect' for details")
	}

	for _, in := range r.Inputs {
		fmt.Fprintf(w, "  required input: spec.%s (%s)\n", in.Path, in.Reason)
	}

	for _, p := range r.Prerequisites {
		fmt.Fprintf(w, "  prerequisite (install once): %s %s\n", p.Kind, p.Name)
	}

	for _, d := range r.Dropped {
		fmt.Fprintf(w, "  dropped: %s %s (%s)\n", d.Kind, d.Name, d.Reason)
	}

	for _, x := range r.Warnings {
		fmt.Fprintf(w, "  warning: %s\n", x)
	}
}
