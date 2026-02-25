// Command fakegh is a test double for the gh CLI.
// It returns canned JSON responses and optionally logs invocations to a file.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]

	// Log invocation if FAKEGH_RECORD is set.
	if recordFile := os.Getenv("FAKEGH_RECORD"); recordFile != "" {
		f, err := os.OpenFile(recordFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, strings.Join(args, " "))
			f.Close()
		}
	}

	// Fail mode: if FAKEGH_FAIL is set, exit non-zero.
	if os.Getenv("FAKEGH_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "fatal: something went wrong")
		os.Exit(1)
	}

	if len(args) >= 1 && args[0] == "--version" {
		fmt.Println("gh version 2.50.0 (2024-05-01)")
		os.Exit(0)
	}

	if len(args) >= 2 && args[0] == "pr" {
		switch args[1] {
		case "create":
			fmt.Println("https://github.com/test/repo/pull/42")
		case "view":
			fmt.Println(`{"number": 42, "state": "OPEN", "baseRefName": "main"}`)
		case "edit":
			// no output
		}
		os.Exit(0)
	}

	os.Exit(0)
}
