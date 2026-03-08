// Command fakegt is a test double for the Graphite CLI (gt).
// Behavior is controlled via environment variables:
//
//   - FAKEGT_FAIL: if set, exit 1 with error message
//   - FAKEGT_CONFLICT: if set, exit 1 with CONFLICT output (for restack)
//   - FAKEGT_SUBMIT_OUTPUT: custom stdout for "submit" command
//   - FAKEGT_RECORD: if set to a file path, append each invocation's args
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]

	// Record invocations for test assertions.
	if recordFile := os.Getenv("FAKEGT_RECORD"); recordFile != "" {
		f, err := os.OpenFile(recordFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, strings.Join(args, " "))
			f.Close()
		}
	}

	// Unconditional failure mode.
	if os.Getenv("FAKEGT_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "fatal: something went wrong")
		os.Exit(1)
	}

	if len(args) == 0 {
		os.Exit(0)
	}

	switch args[0] {
	case "create":
		// gt create <name> — no output on success.
	case "submit":
		// Conflict mode for submit.
		if os.Getenv("FAKEGT_CONFLICT") != "" {
			fmt.Fprintln(os.Stderr, "CONFLICT (content): Merge conflict in file.go")
			os.Exit(1)
		}
		// Custom output or default.
		if out := os.Getenv("FAKEGT_SUBMIT_OUTPUT"); out != "" {
			fmt.Println(out)
		} else {
			fmt.Println("default-branch: https://app.graphite.com/github/pr/owner/repo/1 (created)")
		}
	case "restack":
		if os.Getenv("FAKEGT_CONFLICT") != "" {
			fmt.Println("CONFLICT (content): Merge conflict in file.go")
			fmt.Fprintln(os.Stderr, "could not apply abc1234... commit message")
			os.Exit(1)
		}
		fmt.Println("Restacked")
	default:
		// Unknown commands succeed silently.
	}
}
