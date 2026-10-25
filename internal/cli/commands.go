package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/kroval"
	"github.com/hupe1980/chart2kro/internal/version"
	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

func newInspectCmd(g *globals) *cobra.Command {
	var (
		o      chart2kro.Options
		value  string
		format string
	)

	cmd := &cobra.Command{
		Use:   "inspect CHART",
		Short: "Show what a conversion exposes, freezes and drops, and why",
		Example: `  chart2kro inspect ./mychart
  chart2kro inspect ./mychart --value service.port
  chart2kro inspect ./mychart --format json | jq '.values[] | select(.outcome=="frozen")'`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log, err := g.logger()
			if err != nil {
				return err
			}

			o.Chart, o.Logger = args[0], log

			rep, err := chart2kro.Inspect(cmd.Context(), o)
			if err != nil {
				return err
			}

			switch format {
			case "json":
				enc := json.NewEncoder(g.stdout)
				enc.SetIndent("", "  ")

				return enc.Encode(rep)
			case "text":
				if value != "" {
					return explain(g.stdout, rep, value)
				}

				printReport(g.stdout, rep)

				return nil
			default:
				return &usageError{fmt.Errorf("--format %q: want text or json", format)}
			}
		},
	}

	chartFlags(cmd.Flags(), &o)
	cmd.Flags().StringVar(&value, "value", "", "explain one value path")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")

	return cmd
}

func printReport(w io.Writer, r *chart2kro.Report) {
	printSummary(w, r)
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "VALUE\tOUTCOME\tTYPE\tDETAIL")

	for _, v := range r.Values {
		detail := strings.Join(v.Targets, ", ")
		if v.Outcome != "exposed" {
			detail = strings.TrimSpace(v.Reason + " " + v.Detail)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", v.Path, v.Outcome, v.Type, truncate(detail, 100))
	}

	_ = tw.Flush()
	fmt.Fprintln(w)

	tw = tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "RESOURCE\tKIND\tSOURCE\tCONDITION")

	for _, res := range r.Resources {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", res.ID, res.Kind, res.Source, strings.Join(res.IncludeWhen, " && "))
	}

	_ = tw.Flush()
}

func explain(w io.Writer, r *chart2kro.Report, path string) error {
	for _, v := range r.Values {
		if v.Path != path {
			continue
		}

		fmt.Fprintf(w, "%s: %s\n", v.Path, v.Outcome)

		if v.Description != "" {
			fmt.Fprintf(w, "  %s\n", v.Description)
		}

		if v.Outcome == "exposed" {
			fmt.Fprintf(w, "  spec field: spec.%s (%s, default %v)\n", v.Path, v.Type, v.Default)
			fmt.Fprintf(w, "  how:        %s\n", v.Kind)

			for _, t := range v.Targets {
				fmt.Fprintf(w, "  sets:       %s\n", t)
			}

			return nil
		}

		fmt.Fprintf(w, "  reason: %s\n", v.Reason)

		if v.Detail != "" {
			fmt.Fprintf(w, "  where:  %s\n", v.Detail)
		}

		fmt.Fprintln(w, "  the value stays at its default in every instance")

		return nil
	}

	return fmt.Errorf("%w: %q is not a chart value (see 'chart2kro inspect' for the list)", chart2kro.ErrUsage, path)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n-1] + "…"
}

func newValidateCmd(g *globals) *cobra.Command {
	var (
		o      chart2kro.ValidateOptions
		format string
	)

	cmd := &cobra.Command{
		Use:   "validate FILE...",
		Short: "Validate ResourceGraphDefinitions offline with kro's own checks",
		Long: `Validate every ResourceGraphDefinition in the given files ("-" for stdin) with kro's
admission schema and graph builder, offline. Works for any RGD, not only chart2kro's.`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return &usageError{fmt.Errorf("at least one file is required")}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var all []byte

			for _, f := range args {
				var (
					b   []byte
					err error
				)

				if f == "-" {
					b, err = io.ReadAll(os.Stdin)
				} else {
					b, err = os.ReadFile(f) //nolint:gosec // user-selected file
				}

				if err != nil {
					return &usageError{err}
				}

				all = append(append(all, b...), []byte("\n---\n")...)
			}

			if format != "text" && format != "json" {
				return &usageError{fmt.Errorf("--format %q: want text or json", format)}
			}

			err := chart2kro.Validate(cmd.Context(), all, o)

			if format == "json" {
				res := validateResult{
					SchemaVersion: "chart2kro.io/validate/v1",
					Valid:         err == nil,
					KroVersion:    kroval.KroVersion(),
					KubeVersion:   orDefault(o.KubeVersion, kroval.DefaultKubeVersion()),
					Problems:      []string{},
				}

				var ke *kroval.Error
				if errors.As(err, &ke) {
					res.Problems = ke.Problems
				} else if err != nil {
					return err
				}

				enc := json.NewEncoder(g.stdout)
				enc.SetIndent("", "  ")

				if eerr := enc.Encode(res); eerr != nil {
					return eerr
				}

				return err
			}

			if err != nil {
				return err
			}

			if !g.quiet {
				fmt.Fprintf(g.stderr, "valid (kro %s, Kubernetes %s)\n", kroval.KroVersion(), orDefault(o.KubeVersion, kroval.DefaultKubeVersion()))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")

	cmd.Flags().StringVar(&o.KubeVersion, "kube-version", "", "Kubernetes version of the validation catalog (default: newest)")
	cmd.Flags().StringArrayVar(&o.CRDFiles, "crd", nil, "CRD file for custom resources (repeatable)")

	return cmd
}

func newDiffCmd(g *globals) *cobra.Command {
	var (
		o       chart2kro.Options
		against string
		format  string
	)

	cmd := &cobra.Command{
		Use:     "diff CHART --against RGD",
		Short:   "Classify API changes against an existing RGD (exit 8 if breaking)",
		Example: `  chart2kro diff ./mychart --version 2.0.0 --against out/rgd.yaml`,
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if against == "" {
				return &usageError{fmt.Errorf("--against is required")}
			}

			existing, err := os.ReadFile(against) //nolint:gosec // user-selected file
			if err != nil {
				return &usageError{err}
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

			d, err := chart2kro.Diff(existing, res.RGD)
			if err != nil {
				return err
			}

			switch format {
			case "json":
				enc := json.NewEncoder(g.stdout)
				enc.SetIndent("", "  ")

				if err := enc.Encode(d); err != nil {
					return err
				}
			case "text":
				if len(d.Changes) == 0 {
					fmt.Fprintln(g.stdout, "no changes")
				}

				for _, c := range d.Changes {
					fmt.Fprintf(g.stdout, "%-11s %s: %s\n", c.Severity, c.Path, c.Message)
				}
			default:
				return &usageError{fmt.Errorf("--format %q: want text or json", format)}
			}

			if d.Breaking() {
				return fmt.Errorf("%w: existing instances are affected", chart2kro.ErrBreaking)
			}

			return nil
		},
	}

	chartFlags(cmd.Flags(), &o)
	cmd.Flags().StringVar(&against, "against", "", "existing RGD file")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")

	return cmd
}

func newInstanceCmd(g *globals) *cobra.Command {
	var (
		o  chart2kro.Options
		io chart2kro.InstanceOptions
	)

	cmd := &cobra.Command{
		Use:   "instance CHART --from VALUES --name NAME",
		Short: "Turn Helm values (e.g. of an existing release) into an instance",
		Long: `Convert the chart, then write an instance of the generated kind whose spec reproduces
the given Helm values. -f/--set keep their convert meaning (the API's defaults);
--from names the values the instance should have. Values the API does not expose
fail (exit 5) unless --allow-unexposed.`,
		Example: `  helm get values my-release -o yaml > release.yaml
  chart2kro instance ./mychart --from release.yaml --name my-release --namespace prod`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(io.ValueFiles) == 0 && len(io.Set) == 0 {
				return &usageError{fmt.Errorf("--from or --from-set is required")}
			}

			log, err := g.logger()
			if err != nil {
				return err
			}

			o.Chart, o.Logger = args[0], log

			b, err := chart2kro.Instance(cmd.Context(), o, io)
			if err != nil {
				return err
			}

			_, err = g.stdout.Write(b)

			return err
		},
	}

	chartFlags(cmd.Flags(), &o)
	cmd.Flags().StringArrayVar(&io.ValueFiles, "from", nil, "values the instance should reproduce (repeatable)")
	cmd.Flags().StringArrayVar(&io.Set, "from-set", nil, "a value the instance should reproduce (key=value, repeatable)")
	cmd.Flags().StringVar(&io.Name, "name", "", "instance name (required)")
	cmd.Flags().StringVar(&io.Namespace, "namespace", "", "instance namespace")
	cmd.Flags().BoolVar(&io.AllowUnexposed, "allow-unexposed", false, "drop values the API does not expose instead of failing")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newVersionCmd(g *globals) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			info := map[string]string{
				"version":      version.Version(),
				"commit":       version.Commit(),
				"buildDate":    version.BuildDate(),
				"go":           runtime.Version(),
				"kro":          kroval.KroVersion(),
				"minKro":       "v0.9.1",
				"kubeVersions": strings.Join(kroval.KubeVersions(), ","),
			}

			if format == "json" {
				enc := json.NewEncoder(g.stdout)
				enc.SetIndent("", "  ")

				return enc.Encode(info)
			}

			fmt.Fprintf(g.stdout, "chart2kro %s (commit %s, built %s, %s)\n", info["version"], info["commit"], info["buildDate"], info["go"])
			fmt.Fprintf(g.stdout, "validates with kro %s; output needs kro >= %s; Kubernetes catalogs: %s\n", info["kro"], info["minKro"], info["kubeVersions"])

			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")

	return cmd
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}

	return s
}

// validateResult is the `validate --format json` output (a public contract).
type validateResult struct {
	SchemaVersion string   `json:"schemaVersion"`
	Valid         bool     `json:"valid"`
	KroVersion    string   `json:"kroVersion"`
	KubeVersion   string   `json:"kubeVersion"`
	Problems      []string `json:"problems"`
}
