package cmd

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	jsonOut bool
)

var rootCmd = &cobra.Command{
	Use:           "tier",
	Short:         "Agent-first CLI for managing stacked PRs with DAG dependencies",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output in JSON format")
}

func Execute() error {
	return rootCmd.Execute()
}

// printJSON marshals v to JSON and writes it to stdout.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
