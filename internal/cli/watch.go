package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/output"
	"github.com/hupe1980/chart2kro/internal/transform"
	"github.com/hupe1980/chart2kro/internal/watch"
)

type watchOptions struct {
	convertOptions

	// Watch-specific options.
	debounce time.Duration
	validate bool
	apply    bool
}

func newWatchCommand() *cobra.Command {
	opts := &watchOptions{}

	cmd := &cobra.Command{
		Use:   "watch <chart-reference>",
		Short: "Watch a chart for changes and auto-convert",
		Long: `Watch monitors a Helm chart directory for file changes and
automatically re-runs the conversion when source files are modified.

File changes are debounced to avoid rapid re-runs. Each regeneration
reports resource count, schema field count, and any schema changes
(fields added, removed, or defaults changed).

Use --validate (enabled by default) to auto-validate after each
generation, and --apply to automatically apply the output to your
cluster via kubectl.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWatch(cmd.Context(), cmd, args[0], opts)
		},
	}

	// Register all shared pipeline flags (chart loading, rendering, values, etc.).
	registerPipelineFlags(cmd, &opts.convertOptions)

	// Output flags.
	f := cmd.Flags()
	f.StringVarP(&opts.output, "output", "o", "", "output file path (required)")
	f.BoolVar(&opts.dryRun, "dry-run", false, "preview output without writing")
	f.BoolVar(&opts.comments, "comments", false, "add inline comments on CEL expressions")
	f.BoolVar(&opts.embedTimestamp, "embed-timestamp", false, "add generated-at timestamp annotation")

	// Externalize flags (shared with convert).
	f.StringArrayVar(&opts.externalizeSecret, "externalize-secret", nil, "externalize a Secret (name=schemaField)")
	f.StringArrayVar(&opts.externalizeService, "externalize-service", nil, "externalize a Service (name=schemaField)")
	f.StringSliceVar(&opts.useExternalPattern, "use-external-pattern", nil, "auto-detect external pattern for subchart")

	// Watch-specific flags.
	f.DurationVar(&opts.debounce, "debounce", 500*time.Millisecond, "debounce interval for file changes")
	f.BoolVar(&opts.validate, "validate", true, "auto-validate after each generation")
	f.BoolVar(&opts.apply, "apply", false, "auto-apply to cluster via kubectl")

	return cmd
}

func runWatch(ctx context.Context, cmd *cobra.Command, ref string, opts *watchOptions) error {
	if opts.output == "" {
		return &ExitError{Code: 2, Err: fmt.Errorf("--output (-o) is required for watch mode")}
	}

	// Track previous schema fields for change detection across regenerations.
	var prevFields []*transform.SchemaField

	// Build the run function that executes the full pipeline.
	runFn := func(fnCtx context.Context) (*watch.RunResult, error) {
		pipeResult, err := runPipeline(fnCtx, ref, &opts.convertOptions)
		if err != nil {
			return nil, err
		}

		// Write output.
		serOpts := output.SerializeOptions{
			Comments: opts.comments,
			Indent:   2,
		}

		yamlBytes, serErr := output.Serialize(pipeResult.RGDMap, serOpts)
		if serErr != nil {
			return nil, fmt.Errorf("serializing RGD: %w", serErr)
		}

		if !opts.dryRun {
			w := output.NewFileWriter(opts.output)
			if writeErr := w.Write(yamlBytes); writeErr != nil {
				return nil, fmt.Errorf("writing output: %w", writeErr)
			}
		}

		// Detect schema changes.
		var schemaChanges []watch.SchemaChange
		if prevFields != nil {
			schemaChanges = watch.SchemaDiff(prevFields, pipeResult.Result.SchemaFields)
		}

		prevFields = pipeResult.Result.SchemaFields

		return &watch.RunResult{
			ResourceCount: len(pipeResult.Result.Resources),
			SchemaFields:  len(pipeResult.Result.SchemaFields),
			SchemaChanges: schemaChanges,
			OutputPath:    opts.output,
		}, nil
	}

	// Build optional validate function.
	var validateFn watch.ValidateFunc
	if opts.validate {
		validateFn = func(valCtx context.Context, outputPath string) error {
			return runValidateFile(valCtx, outputPath, false)
		}
	}

	watchOpts := watch.Options{
		ChartDir:   ref,
		ExtraFiles: opts.valueFiles,
		Debounce:   opts.debounce,
		Validate:   opts.validate,
		Apply:      opts.apply,
		ValidateFn: validateFn,
		Out:        cmd.ErrOrStderr(),
	}

	return watch.Run(ctx, watchOpts, runFn)
}

// runValidateFile validates an RGD file. This is a lightweight wrapper
// around the validate command's core logic.
func runValidateFile(_ context.Context, filePath string, _ bool) error {
	rgdMap, err := loadRGDFile(filePath, 7)
	if err != nil {
		return err
	}

	result := output.ValidateRGD(rgdMap)

	if result.HasErrors() {
		return fmt.Errorf("validation failed with %d error(s)", len(result.Errors()))
	}

	return nil
}
