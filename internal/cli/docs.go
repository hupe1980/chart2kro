package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/docs"
)

type docsOptions struct {
	format          string
	title           string
	includeExamples bool
	outputFile      string
}

func newDocsCommand() *cobra.Command {
	opts := &docsOptions{}

	cmd := &cobra.Command{
		Use:   "docs <rgd-file>",
		Short: "Generate API documentation from a ResourceGraphDefinition",
		Long: `Generate human-readable API reference documentation from a
ResourceGraphDefinition YAML file.

Outputs documentation describing the custom resource API: spec fields with
types and defaults, status fields, managed resources, and optionally an
example YAML instance.

Supports markdown, HTML, and ASCIIDoc output formats.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocs(cmd, args[0], opts)
		},
	}

	cmd.Flags().StringVarP(&opts.format, "format", "f", "markdown", "output format (markdown, html, asciidoc)")
	cmd.Flags().StringVar(&opts.title, "title", "", "override document title")
	cmd.Flags().BoolVar(&opts.includeExamples, "include-examples", true, "include example YAML in output")
	cmd.Flags().StringVarP(&opts.outputFile, "output", "o", "", "write to file instead of stdout")

	return cmd
}

func runDocs(cmd *cobra.Command, filePath string, opts *docsOptions) error {
	// 1. Build the formatter.
	formatter, err := docs.NewFormatter(opts.format)
	if err != nil {
		return &ExitError{Code: 2, Err: err}
	}

	// 2. Parse the RGD file.
	rgdMap, err := loadRGDFile(filePath, 7)
	if err != nil {
		return err
	}

	// 3. Extract the doc model.
	model, err := docs.ParseRGDMap(rgdMap)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("parsing RGD: %w", err)}
	}

	if opts.title != "" {
		model.Title = opts.title
	}

	model.IncludeExamples = opts.includeExamples

	// 4. Determine output destination.
	w := cmd.OutOrStdout()

	if opts.outputFile != "" {
		f, ferr := os.Create(opts.outputFile) //nolint:gosec // User-specified output file
		if ferr != nil {
			return &ExitError{Code: 1, Err: fmt.Errorf("creating output file: %w", ferr)}
		}

		defer f.Close() //nolint:errcheck

		w = f
	}

	// 5. Render documentation.
	if err := formatter.Format(w, model); err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("formatting docs: %w", err)}
	}

	return nil
}
