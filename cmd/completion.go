package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for tier.

To load completions:

  bash:
    source <(tier completion bash)

    # To install permanently (Linux):
    tier completion bash > /etc/bash_completion.d/tier

    # To install permanently (macOS with Homebrew):
    tier completion bash > $(brew --prefix)/etc/bash_completion.d/tier

  zsh:
    # If shell completion is not already enabled, add this to ~/.zshrc:
    autoload -U compinit; compinit

    source <(tier completion zsh)

    # To install permanently:
    tier completion zsh > "${fpath[1]}/_tier"

  fish:
    tier completion fish | source

    # To install permanently:
    tier completion fish > ~/.config/fish/completions/tier.fish
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
