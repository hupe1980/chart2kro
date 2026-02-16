package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/version"
)

func newVersionCommand() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Display the version, git commit, build date, Go version, and platform.",
		Args:  cobra.NoArgs,
		// Override parent PersistentPreRunE — version needs no config.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.GetInfo()

			if jsonOutput {
				j, err := info.JSON()
				if err != nil {
					return err
				}

				_, err = fmt.Fprintln(cmd.OutOrStdout(), j)

				return err
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), info.String())

			return err
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output version info as JSON")

	return cmd
}
