package cmd

import (
	"encoding/json"
	"fmt"
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
func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}
