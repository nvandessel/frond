// Command fakegh is a test double for the gh CLI.
// It returns canned JSON responses and optionally logs invocations to a file.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// nextPRNumber returns an incrementing PR number when FAKEGH_PR_COUNTER is
// set to a file path, otherwise defaults to 42 for backward compatibility.
func nextPRNumber() int {
	counterFile := os.Getenv("FAKEGH_PR_COUNTER")
	if counterFile == "" {
		return 42
	}
	n := 42
	data, err := os.ReadFile(counterFile)
	if err == nil {
		if parsed, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			n = parsed
		}
	}
	os.WriteFile(counterFile, []byte(strconv.Itoa(n+1)+"\n"), 0o644) //nolint:errcheck
	return n
}

// handleAPI handles "gh api" subcommands for comment operations.
func handleAPI(args []string) {
	// Fail mode for API-only: if FAKEGH_FAIL_API is set, exit non-zero.
	if os.Getenv("FAKEGH_FAIL_API") != "" {
		fmt.Fprintln(os.Stderr, "fatal: API request failed")
		os.Exit(1)
	}

	// Parse flags: detect HTTP method and endpoint.
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

	// Update comment: PATCH to /issues/comments/{id}.
	if strings.Contains(endpoint, "/issues/comments/") && method == "PATCH" {
		fmt.Println(`{}`)
		return
	}

	// Comment operations on /issues/{n}/comments.
	if strings.Contains(endpoint, "/comments") {
		// Presence of -f body=... distinguishes create from list.
		hasBody := false
		for _, a := range args {
			if strings.HasPrefix(a, "body=") {
				hasBody = true
				break
			}
		}
		if hasBody {
			fmt.Println(`{"id": 100, "body": "created"}`)
			return
		}

		// List comments.
		if os.Getenv("FAKEGH_EXISTING_COMMENT") != "" {
			fmt.Println(`[{"id": 99, "body": "<!-- frond-stack -->\nold comment"}]`)
		} else {
			fmt.Println(`[]`)
		}
		return
	}

	// Default: empty response.
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
			n := nextPRNumber()
			fmt.Printf("https://github.com/test/repo/pull/%d\n", n)
		case "view":
			// Parse the requested PR number from args.
			prNum := "42"
			if len(args) > 2 && !strings.HasPrefix(args[2], "-") {
				prNum = args[2]
			}
			prState := "OPEN"
			if s := os.Getenv("FAKEGH_PR_STATE"); s != "" {
				prState = s
			}
			fmt.Printf("{\"number\": %s, \"state\": \"%s\", \"baseRefName\": \"main\"}\n", prNum, prState)
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
