// Package cli is the chart2kro command line: a thin client of pkg/chart2kro.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

type globals struct {
	logLevel  string
	logFormat string
	quiet     bool
	stdout    io.Writer
	stderr    io.Writer
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

// Run executes the CLI with explicit arguments and streams (used by tests).
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	g := &globals{stdout: stdout, stderr: stderr}
	root := newRoot(g)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}

	var usage *usageError
	if strings.HasPrefix(err.Error(), "unknown command") || strings.HasPrefix(err.Error(), "unknown flag") {
		err = &usageError{err}
	}

	if errors.As(err, &usage) {
		err = fmt.Errorf("%w: %w", chart2kro.ErrUsage, usage.err)
	}

	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "interrupted")
		return 130
	}

	fmt.Fprintln(stderr, "Error:", err)

	return chart2kro.ExitCode(err)
}

type usageError struct{ err error }

func (u *usageError) Error() string { return u.err.Error() }

func newRoot(g *globals) *cobra.Command {
	root := &cobra.Command{
		Use:   "chart2kro",
		Short: "Convert Helm charts into kro ResourceGraphDefinitions",
		Long: `chart2kro turns a Helm chart into a kro ResourceGraphDefinition.

kro accepts every RGD it writes (checked offline with kro's own validator). An
instance created with the chart's defaults deploys what 'helm template' deploys,
and every input it exposes was seen to behave like the chart value it replaces.
Every value that is not exposed is reported with the reason.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &usageError{err} })

	pf := root.PersistentFlags()
	pf.StringVar(&g.logLevel, "log-level", "warn", "log level: debug, info, warn, error")
	pf.StringVar(&g.logFormat, "log-format", "text", "log format: text, json")
	pf.BoolVarP(&g.quiet, "quiet", "q", false, "print only results and errors")

	root.AddCommand(
		newConvertCmd(g),
		newInspectCmd(g),
		newValidateCmd(g),
		newDiffCmd(g),
		newInstanceCmd(g),
		newVersionCmd(g),
	)

	return root
}

func (g *globals) logger() (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(g.logLevel)); err != nil {
		return nil, &usageError{fmt.Errorf("--log-level %q: %w", g.logLevel, err)}
	}

	if g.quiet && lvl < slog.LevelError {
		lvl = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: lvl}

	switch g.logFormat {
	case "text":
		return slog.New(slog.NewTextHandler(g.stderr, opts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(g.stderr, opts)), nil
	default:
		return nil, &usageError{fmt.Errorf("--log-format %q: want text or json", g.logFormat)}
	}
}

func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(n)(cmd, args); err != nil {
			return &usageError{err}
		}

		return nil
	}
}
