// Package cli implements the cobra command tree for chart2kro.
package cli

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/config"
	"github.com/hupe1980/chart2kro/internal/logging"
)

// ExitError wraps an error with a specific process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}

	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// Execute builds the command tree, runs it, and returns the exit code.
func Execute() int {
	cmd := NewRootCommand()

	if err := cmd.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}

		return 1
	}

	return 0
}

// NewRootCommand constructs the top-level cobra.Command with all
// subcommands attached.
func NewRootCommand() *cobra.Command {
	var cfgFile string

	cmd := &cobra.Command{
		Use:   "chart2kro",
		Short: "Transform Helm charts into KRO ResourceGraphDefinitions",
		Long: `chart2kro is a CLI tool that transforms Helm charts into KRO
(Kubernetes Resource Orchestrator) ResourceGraphDefinition resources.

It reads a Helm chart, renders its templates, and produces a fully
functional KRO ResourceGraphDefinition that encapsulates the chart's
Kubernetes resources as a reusable, composable platform abstraction.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cmd, cfgFile)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}

			logger := logging.Setup(cfg)

			ctx := cmd.Context()
			ctx = config.NewContext(ctx, cfg)
			ctx = logging.NewContext(ctx, logger)
			cmd.SetContext(ctx)

			logger.Debug("configuration loaded",
				slog.String("logLevel", cfg.LogLevel),
				slog.String("logFormat", cfg.LogFormat),
			)

			return nil
		},
	}

	// Global persistent flags.
	pf := cmd.PersistentFlags()
	pf.StringVar(&cfgFile, "config", "", "config file (default: .chart2kro.yaml)")
	pf.String("log-level", "info", "log level: debug, info, warn, error")
	pf.String("log-format", "text", "log format: text, json")
	pf.Bool("no-color", false, "disable colored output")
	pf.BoolP("quiet", "q", false, "suppress non-essential output")

	// Flag parsing errors return exit code 2.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &ExitError{Code: 2, Err: err}
	})

	// Register subcommands.
	cmd.AddCommand(
		newVersionCommand(),
		newConvertCommand(),
		newInspectCommand(),
		newValidateCommand(),
		newExportCommand(),
		newDiffCommand(),
		newAuditCommand(),
		newDocsCommand(),
		newPlanCommand(),
		newWatchCommand(),
		newCompletionCommand(),
	)

	return cmd
}
