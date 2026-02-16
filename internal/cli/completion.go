package cli

import (
	"github.com/spf13/cobra"
)

func newCompletionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion <shell>",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for chart2kro.

To load completions:

Bash:
  $ source <(chart2kro completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ chart2kro completion bash > /etc/bash_completion.d/chart2kro

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. Execute once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ chart2kro completion zsh > "${fpath[1]}/_chart2kro"

Fish:
  $ chart2kro completion fish > ~/.config/fish/completions/chart2kro.fish

PowerShell:
  PS> chart2kro completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> chart2kro completion powershell > chart2kro.ps1
  # and source this file from your PowerShell profile.
`,
		// Override parent PersistentPreRunE — completion needs no config.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		Args:              cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:         []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()

			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(w, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(w)
			case "fish":
				return cmd.Root().GenFishCompletion(w, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(w)
			}

			return nil
		},
	}

	return cmd
}
