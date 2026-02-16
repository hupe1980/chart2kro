package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/output"
)

type validateOptions struct {
	strict bool
}

func newValidateCommand() *cobra.Command {
	opts := &validateOptions{}

	cmd := &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a generated ResourceGraphDefinition",
		Long: `Validate a generated ResourceGraphDefinition against the KRO schema,
Kubernetes API conventions, and CEL expression syntax.

Reports all errors and warnings found in the RGD file. Returns exit code 7
on validation failure (or on warnings with --strict).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(cmd, args[0], opts)
		},
	}

	cmd.Flags().BoolVar(&opts.strict, "strict", false, "fail on warnings in addition to errors")

	return cmd
}

func runValidate(cmd *cobra.Command, filePath string, opts *validateOptions) error {
	// 1. Read and parse the file.
	rgdMap, err := loadRGDFile(filePath, 7)
	if err != nil {
		if exitErr, ok := err.(*ExitError); ok && exitErr.Code == 7 {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "YAML syntax error: %v\n", exitErr.Err)
		}

		return err
	}

	// 2. Run validation.
	result := output.ValidateRGD(rgdMap)

	// 3. Print results.
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), output.FormatValidationResult(result))

	// 4. Determine exit code.
	if result.HasErrors() {
		return &ExitError{Code: 7, Err: fmt.Errorf("validation failed with %d error(s)", len(result.Errors()))}
	}

	if opts.strict && result.HasWarnings() {
		return &ExitError{Code: 7, Err: fmt.Errorf("validation failed with %d warning(s) (strict mode)", len(result.Warnings()))}
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Validation passed.")

	return nil
}
