// Package gh is a thin wrapper around the gh CLI (GitHub CLI).
// All functions shell out to gh — no GitHub API client library.
// This leverages the user's existing gh authentication.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// PRCreateOpts configures the gh pr create command.
type PRCreateOpts struct {
	Base  string // Target branch (--base)
	Head  string // Source branch (--head)
	Title string // PR title (-t)
	Body  string // PR body (-b)
	Draft bool   // Create as draft PR (--draft)
}

// PRCreate creates a pull request and returns the new PR number.
// gh pr create outputs a URL like https://github.com/owner/repo/pull/123.
func PRCreate(ctx context.Context, opts PRCreateOpts) (int, error) {
	args := []string{
		"pr", "create",
		"--base", opts.Base,
		"--head", opts.Head,
		"-t", opts.Title,
		"-b", opts.Body,
	}
	if opts.Draft {
		args = append(args, "--draft")
	}

	out, err := run(ctx, args...)
	if err != nil {
		return 0, err
	}

	// Parse PR number from the URL in the last line of output.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	lastLine := lines[len(lines)-1]
	idx := strings.LastIndex(lastLine, "/")
	if idx < 0 {
		return 0, fmt.Errorf("unexpected pr create output: %s", out)
	}
	num, err := strconv.Atoi(lastLine[idx+1:])
	if err != nil {
		return 0, fmt.Errorf("parsing PR number from %q: %w", lastLine, err)
	}
	return num, nil
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

// Comment holds metadata about a PR/issue comment.
type Comment struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

// PRCommentList returns all comments on a pull request.
// Uses --paginate to handle PRs with many comments. The gh CLI outputs each
// page as a separate JSON array when paginating, so we decode them one at a
// time and merge into a single slice.
func PRCommentList(ctx context.Context, prNumber int) ([]Comment, error) {
	out, err := run(ctx, "api", "--paginate",
		fmt.Sprintf("repos/{owner}/{repo}/issues/%d/comments", prNumber))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	var comments []Comment
	dec := json.NewDecoder(strings.NewReader(out))
	for {
		var page []Comment
		if err := dec.Decode(&page); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parsing comment list: %w", err)
		}
		comments = append(comments, page...)
	}
	return comments, nil
}

// PRCommentCreate creates a new comment on a pull request.
func PRCommentCreate(ctx context.Context, prNumber int, body string) error {
	_, err := run(ctx, "api",
		fmt.Sprintf("repos/{owner}/{repo}/issues/%d/comments", prNumber),
		"-f", "body="+body)
	return err
}

// PRCommentUpdate updates an existing comment by ID.
func PRCommentUpdate(ctx context.Context, commentID int, body string) error {
	_, err := run(ctx, "api", "-X", "PATCH",
		fmt.Sprintf("repos/{owner}/{repo}/issues/comments/%d", commentID),
		"-f", "body="+body)
	return err
}

// PR state constants returned by the GitHub API.
const (
	PRStateOpen   = "OPEN"
	PRStateClosed = "CLOSED"
	PRStateMerged = "MERGED"
)

// PRState returns the state of a pull request ("OPEN", "CLOSED", or "MERGED").
func PRState(ctx context.Context, prNumber int) (string, error) {
	info, err := PRView(ctx, prNumber)
	if err != nil {
		return "", err
	}
	return info.State, nil
}
