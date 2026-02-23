// Package gh is a thin wrapper around the gh CLI (GitHub CLI).
// All functions shell out to gh — no GitHub API client library.
// This leverages the user's existing gh authentication.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PRInfo holds metadata about a pull request.
type PRInfo struct {
	Number      int    `json:"number"`
	State       string `json:"state"`
	BaseRefName string `json:"baseRefName"`
}

// GHError is returned when the gh CLI exits with a non-zero status.
type GHError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *GHError) Error() string {
	return fmt.Sprintf("gh %s: %s", strings.Join(e.Args, " "), strings.TrimSpace(e.Stderr))
}

func (e *GHError) Unwrap() error {
	return e.Err
}

// run executes gh with the given arguments and returns trimmed stdout.
// On failure it returns a *GHError containing stderr.
func run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", &GHError{
			Args:   args,
			Stderr: stderr.String(),
			Err:    err,
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Available checks whether the gh CLI is installed and accessible.
// It returns a descriptive error if not found.
func Available() error {
	_, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("gh CLI is required. Install: https://cli.github.com")
	}
	return nil
}

// PRCreate creates a pull request and returns the new PR number.
// When draft is true, the --draft flag is included.
func PRCreate(ctx context.Context, base, head, title, body string, draft bool) (int, error) {
	args := []string{
		"pr", "create",
		"--base", base,
		"--head", head,
		"-t", title,
		"-b", body,
	}
	if draft {
		args = append(args, "--draft")
	}
	args = append(args, "--json", "number")

	out, err := run(ctx, args...)
	if err != nil {
		return 0, err
	}

	var result struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return 0, fmt.Errorf("parsing pr create output: %w", err)
	}
	return result.Number, nil
}

// PRView retrieves metadata about a pull request by number.
func PRView(ctx context.Context, prNumber int) (*PRInfo, error) {
	out, err := run(ctx, "pr", "view", strconv.Itoa(prNumber), "--json", "number,state,baseRefName")
	if err != nil {
		return nil, err
	}

	var info PRInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return nil, fmt.Errorf("parsing pr view output: %w", err)
	}
	return &info, nil
}

// PREdit updates the base branch of a pull request.
func PREdit(ctx context.Context, prNumber int, newBase string) error {
	_, err := run(ctx, "pr", "edit", strconv.Itoa(prNumber), "--base", newBase)
	return err
}

// PRState returns the state of a pull request ("OPEN", "CLOSED", or "MERGED").
func PRState(ctx context.Context, prNumber int) (string, error) {
	info, err := PRView(ctx, prNumber)
	if err != nil {
		return "", err
	}
	return info.State, nil
}
