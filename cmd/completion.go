package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for frond.

To load completions:

  bash:
    source <(frond completion bash)

    # To install permanently (Linux):
    frond completion bash > /etc/bash_completion.d/frond

    # To install permanently (macOS with Homebrew):
    frond completion bash > $(brew --prefix)/etc/bash_completion.d/frond

  zsh:
    # If shell completion is not already enabled, add this to ~/.zshrc:
    autoload -U compinit; compinit

    source <(frond completion zsh)

    # To install permanently:
    frond completion zsh > "${fpath[1]}/_frond"

  fish:
    frond completion fish | source

    # To install permanently:
    frond completion fish > ~/.config/fish/completions/frond.fish
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
