// Command fakegh is a test double for the gh CLI.
// It returns canned JSON responses and optionally logs invocations to a file.
package main

import (
	"fmt"
	"os"
	"strings"
)

// handleAPI handles "gh api" subcommands for comment operations.
func handleAPI(args []string) {
	// Detect HTTP method: default GET, -X PATCH means update.
	method := "GET"
	var endpoint string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-X":
			if i+1 < len(args) {
				method = args[i+1]
				i++
			}
		case "--paginate", "--jq":
			// skip flags
		case "-f":
			// skip -f key=value
			i++
		default:
			if !strings.HasPrefix(args[i], "-") && endpoint == "" {
				endpoint = args[i]
			}
		}
	}

	// Detect if this is a comment list, create, or update based on endpoint + method.
	if strings.Contains(endpoint, "/issues/comments/") && method == "PATCH" {
		// Update comment
		fmt.Println(`{}`)
		return
	}

	if strings.Contains(endpoint, "/comments") {
		// Has -f body=... → create; otherwise → list.
		hasBody := false
		for _, a := range args {
			if strings.HasPrefix(a, "body=") {
				hasBody = true
				break
			}
		}
		if hasBody {
			// Create comment
			fmt.Println(`{"id": 100, "body": "created"}`)
			return
		}

		// List comments
		if os.Getenv("FAKEGH_EXISTING_COMMENT") != "" {
			fmt.Println(`[{"id": 99, "body": "<!-- frond-stack -->\nold comment"}]`)
		} else {
			fmt.Println(`[]`)
		}
		return
	}

	// Default: empty response
	fmt.Println(`{}`)
}

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

	if len(args) >= 1 && args[0] == "api" {
		handleAPI(args[1:])
		os.Exit(0)
	}

	os.Exit(0)
}
