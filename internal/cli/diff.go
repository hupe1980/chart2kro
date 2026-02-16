package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/output"
	"github.com/hupe1980/chart2kro/internal/plan"
)

type diffOptions struct {
	convertOptions

	// Existing RGD file to diff against.
	existing string

	// Output format: "unified" (default), "json".
	format string

	// Disable ANSI color output.
	noColor bool
}

func newDiffCommand() *cobra.Command {
	opts := &diffOptions{}

	cmd := &cobra.Command{
		Use:   "diff <chart-reference>",
		Short: "Compare generated RGD against a previous version",
		Long: `Diff compares a newly generated ResourceGraphDefinition against a
previous version to detect YAML-level and schema-level changes.

When --existing is specified, the RGD file on disk is used as the
baseline. Schema evolution analysis is included to highlight
breaking changes.

Exit codes:
  0  No differences
  1  Error
  2  Invalid arguments
  8  Breaking schema changes detected`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(cmd.Context(), cmd, args[0], opts)
		},
	}

	// Diff-specific flags.
	f := cmd.Flags()
	f.StringVar(&opts.existing, "existing", "", "path to existing RGD YAML file to diff against")
	f.StringVar(&opts.format, "format", "unified", "output format: unified, json")
	f.BoolVar(&opts.noColor, "no-color", false, "disable ANSI color output")

	// Shared pipeline flags (chart loading, rendering, values, transform, filtering).
	registerPipelineFlags(cmd, &opts.convertOptions)

	return cmd
}

func runDiff(ctx context.Context, cmd *cobra.Command, ref string, opts *diffOptions) error {
	if opts.existing == "" {
		return &ExitError{Code: 2, Err: fmt.Errorf("--existing flag is required: specify the path to the existing RGD file")}
	}

	// Load existing RGD.
	existingRGD, err := loadRGDFile(opts.existing, 7)
	if err != nil {
		return err
	}

	// Run the conversion pipeline to produce the proposed RGD.
	pResult, err := runPipeline(ctx, ref, &opts.convertOptions)
	if err != nil {
		return err
	}

	// Serialize both for YAML diff.
	serOpts := output.SerializeOptions{Indent: 2}

	existingYAML, err := output.Serialize(existingRGD, serOpts)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("serializing existing RGD: %w", err)}
	}

	proposedYAML, err := output.Serialize(pResult.RGDMap, serOpts)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("serializing proposed RGD: %w", err)}
	}

	// Run schema evolution analysis.
	evolution := plan.Analyze(existingRGD, pResult.RGDMap)

	w := cmd.OutOrStdout()

	switch opts.format {
	case "json":
		if err := plan.FormatJSON(w, evolution); err != nil {
			return &ExitError{Code: 1, Err: fmt.Errorf("formatting JSON: %w", err)}
		}
	default:
		// Unified diff.
		diffOpts := plan.DefaultDiffOptions()
		diffOpts.OldLabel = opts.existing
		diffOpts.NewLabel = "proposed"

		diffResult, diffErr := plan.ComputeDiff(string(existingYAML), string(proposedYAML), diffOpts)
		if diffErr != nil {
			return &ExitError{Code: 1, Err: fmt.Errorf("computing diff: %w", diffErr)}
		}

		plan.WriteDiff(w, diffResult, !opts.noColor)

		// Append evolution summary.
		if evolution.HasChanges() {
			_, _ = fmt.Fprintln(w)
			plan.FormatTable(w, evolution)
		}
	}

	// Exit code 8 for breaking changes.
	if evolution.HasBreakingChanges() {
		return &ExitError{
			Code: 8,
			Err:  fmt.Errorf("%d breaking change(s) detected", evolution.BreakingCount()),
		}
	}

	return nil
}
