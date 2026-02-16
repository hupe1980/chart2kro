package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/output"
)

type exportOptions struct {
	format    string
	outputArg string
	outputDir string
	comments  bool
}

func newExportCommand() *cobra.Command {
	opts := &exportOptions{}

	cmd := &cobra.Command{
		Use:   "export <file>",
		Short: "Export a ResourceGraphDefinition in various formats",
		Long: `Export a generated ResourceGraphDefinition as YAML, JSON, or Kustomize format.

Supported formats:
  yaml       Re-serialized canonical YAML (normalizes formatting)
  json       JSON output with proper indentation
  kustomize  Generates a directory with the RGD YAML and a kustomization.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd, args[0], opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.format, "format", "yaml", "output format: yaml, json, kustomize")
	f.StringVarP(&opts.outputArg, "output", "o", "", "output file path (default: stdout)")
	f.StringVar(&opts.outputDir, "output-dir", "", "output directory (required for kustomize format)")
	f.BoolVar(&opts.comments, "comments", false, "add inline comments on CEL expressions")

	return cmd
}

func runExport(cmd *cobra.Command, filePath string, opts *exportOptions) error {
	// 1. Read and parse the input file.
	rgdMap, err := loadRGDFile(filePath, 1)
	if err != nil {
		return err
	}

	// 2. Resolve output format via the output registry.
	reg := output.DefaultRegistry()

	serOpts := output.SerializeOptions{
		Comments: opts.comments,
		Indent:   2,
	}

	// 3. Format and output.
	switch opts.format {
	case "yaml":
		return exportYAML(cmd, reg, rgdMap, serOpts, opts.outputArg)
	case "json":
		return exportJSON(cmd, reg, rgdMap, opts.outputArg)
	case "kustomize":
		return exportKustomize(cmd, rgdMap, serOpts, opts.outputDir)
	default:
		return &ExitError{Code: 1, Err: fmt.Errorf("unsupported format: %s (supported: yaml, json, kustomize; registered: %s)", opts.format, reg.AvailableFormats())}
	}
}

func exportYAML(cmd *cobra.Command, reg *output.Registry, rgdMap map[string]interface{}, opts output.SerializeOptions, outputPath string) error {
	yamlBytes, err := output.Serialize(rgdMap, opts)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("serializing YAML: %w", err)}
	}

	return writeWithRegistry(cmd, reg, "yaml", yamlBytes, outputPath)
}

func exportJSON(cmd *cobra.Command, reg *output.Registry, rgdMap map[string]interface{}, outputPath string) error {
	jsonBytes, err := output.SerializeJSON(rgdMap, "  ")
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("serializing JSON: %w", err)}
	}

	return writeWithRegistry(cmd, reg, "json", jsonBytes, outputPath)
}

func exportKustomize(_ *cobra.Command, rgdMap map[string]interface{}, opts output.SerializeOptions, outputDir string) error {
	if outputDir == "" {
		return &ExitError{Code: 2, Err: fmt.Errorf("--output-dir is required for kustomize format")}
	}

	if err := output.FormatKustomizeDir(outputDir, rgdMap, opts); err != nil {
		return &ExitError{Code: 6, Err: fmt.Errorf("writing kustomize output: %w", err)}
	}

	return nil
}

// writeWithRegistry creates an output.Writer via the registry's format
// factory and writes data through it. For stdout (empty outputPath) the
// cmd's output stream is used to preserve testability.
func writeWithRegistry(cmd *cobra.Command, reg *output.Registry, format string, data []byte, outputPath string) error {
	factory, err := reg.Writer(format)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}

	var w output.Writer
	if outputPath == "" {
		// Use cmd's output stream so tests can capture output.
		w = output.NewStdoutWriter(cmd.OutOrStdout())
	} else {
		w = factory(outputPath)
	}

	if err := w.Write(data); err != nil {
		return &ExitError{Code: 6, Err: fmt.Errorf("writing output: %w", err)}
	}

	if outputPath != "" {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Written to %s\n", outputPath)
	}

	return nil
}
