package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/plan"
)

type planOptions struct {
	convertOptions

	// Existing RGD file for schema evolution analysis.
	existing string

	// Output format: "table" (default), "json", "compact".
	format string
}

func newPlanCommand() *cobra.Command {
	opts := &planOptions{}

	cmd := &cobra.Command{
		Use:   "plan <chart-reference>",
		Short: "Preview what a conversion would produce",
		Long: `Plan shows what a conversion would produce without writing any
output. Displays schema fields, resources, status projections,
and optional schema evolution analysis.

When --existing is specified, the plan includes change detection
against the existing RGD file.

Exit codes:
  0  Success (no breaking changes)
  1  Error
  2  Invalid arguments
  8  Breaking schema changes detected`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlan(cmd.Context(), cmd, args[0], opts)
		},
	}

	// Plan-specific flags.
	f := cmd.Flags()
	f.StringVar(&opts.existing, "existing", "", "path to existing RGD YAML file for evolution analysis")
	f.StringVar(&opts.format, "format", "table", "output format: table, json, compact")

	// Shared pipeline flags (chart loading, rendering, values, transform, filtering).
	registerPipelineFlags(cmd, &opts.convertOptions)

	return cmd
}

func runPlan(ctx context.Context, cmd *cobra.Command, ref string, opts *planOptions) error {
	// Run the conversion pipeline.
	pResult, err := runPipeline(ctx, ref, &opts.convertOptions)
	if err != nil {
		return err
	}

	// Build the plan.
	p := plan.BuildPlan(pResult.Result, pResult.RGDMap)

	// If existing file specified, run evolution analysis.
	if opts.existing != "" {
		existingRGD, loadErr := loadRGDFile(opts.existing, 7)
		if loadErr != nil {
			return loadErr
		}

		evolution := plan.Analyze(existingRGD, pResult.RGDMap)
		plan.ApplyEvolution(p, evolution)
	}

	// Format output.
	w := cmd.OutOrStdout()

	switch opts.format {
	case "json":
		if err := plan.FormatPlanJSON(w, p); err != nil {
			return &ExitError{Code: 1, Err: fmt.Errorf("formatting JSON: %w", err)}
		}
	case "compact":
		plan.FormatPlanCompact(w, p)
	default:
		plan.FormatPlan(w, p)
	}

	// Exit code 8 for breaking changes.
	if p.HasBreakingChanges {
		return &ExitError{
			Code: 8,
			Err:  fmt.Errorf("%d breaking change(s) detected", p.BreakingChangeCount),
		}
	}

	return nil
}
