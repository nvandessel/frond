// Command fakegt is a test double for the Graphite CLI (gt).
// It returns canned responses for gt commands used by the driver.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]

	// Log invocation if FAKEGT_RECORD is set.
	if recordFile := os.Getenv("FAKEGT_RECORD"); recordFile != "" {
		f, err := os.OpenFile(recordFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, strings.Join(args, " "))
			f.Close()
		}
	}

	// Fail mode: if FAKEGT_FAIL is set, exit non-zero.
	if os.Getenv("FAKEGT_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "fatal: something went wrong")
		os.Exit(1)
	}

	if len(args) == 0 {
		os.Exit(0)
	}

	switch args[0] {
	case "--version":
		fmt.Println("gt version 1.0.0")
	case "create":
		// gt create <name> — no output on success
	case "submit":
		fmt.Println("Submitted PR")
	case "restack":
		fmt.Println("Restacked")
	}

	os.Exit(0)
}
