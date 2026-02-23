// Package git provides a thin wrapper around the git CLI.
// All functions shell out to the git binary via exec.CommandContext,
// leveraging the user's existing git config and authentication.
package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitError represents a failure from a git command invocation.
type GitError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), e.Stderr)
}

func (e *GitError) Unwrap() error {
	return e.Err
}

// RebaseConflictError is returned when a rebase fails due to merge conflicts.
type RebaseConflictError struct {
	Branch string
	Stderr string
}

func (e *RebaseConflictError) Error() string {
	return fmt.Sprintf("rebase conflict on branch %s: %s", e.Branch, e.Stderr)
}

// run executes a git command and returns trimmed stdout on success.
// On failure it returns a *GitError with the captured stderr.
func run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", &GitError{
			Args:   args,
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    err,
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// CommonDir returns the path to the git common directory (where tier.json lives).
// It runs: git rev-parse --git-common-dir
func CommonDir(ctx context.Context) (string, error) {
	return run(ctx, "rev-parse", "--git-common-dir")
}

// CurrentBranch returns the name of the currently checked-out branch.
// It runs: git rev-parse --abbrev-ref HEAD
func CurrentBranch(ctx context.Context) (string, error) {
	return run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// BranchExists checks whether a local branch with the given name exists.
// It runs: git rev-parse --verify refs/heads/<name>
func BranchExists(ctx context.Context, name string) (bool, error) {
	_, err := run(ctx, "rev-parse", "--verify", "refs/heads/"+name)
	if err != nil {
		// If git rev-parse --verify fails, the branch does not exist.
		if gitErr, ok := err.(*GitError); ok {
			if exitErr, ok2 := gitErr.Err.(*exec.ExitError); ok2 {
				if exitErr.ExitCode() == 128 || exitErr.ExitCode() == 1 {
					return false, nil
				}
			}
		}
		return false, fmt.Errorf("git branch-exists %s: %w", name, err)
	}
	return true, nil
}

// CreateBranch creates a new branch at startPoint and checks it out.
// It runs: git checkout -b <name> <startPoint>
func CreateBranch(ctx context.Context, name, startPoint string) error {
	_, err := run(ctx, "checkout", "-b", name, startPoint)
	if err != nil {
		return fmt.Errorf("git create-branch %s %s: %w", name, startPoint, err)
	}
	return nil
}

// Checkout switches to the named branch.
// It runs: git checkout <name>
func Checkout(ctx context.Context, name string) error {
	_, err := run(ctx, "checkout", name)
	if err != nil {
		return fmt.Errorf("git checkout %s: %w", name, err)
	}
	return nil
}

// Fetch fetches from the origin remote.
// It runs: git fetch origin
func Fetch(ctx context.Context) error {
	_, err := run(ctx, "fetch", "origin")
	if err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}
	return nil
}

// Rebase rebases branch onto the given base.
// It runs: git rebase <onto> <branch>
// If a conflict is detected, it returns a *RebaseConflictError.
func Rebase(ctx context.Context, onto, branch string) error {
	_, err := run(ctx, "rebase", onto, branch)
	if err != nil {
		if gitErr, ok := err.(*GitError); ok {
			if strings.Contains(gitErr.Stderr, "CONFLICT") ||
				strings.Contains(gitErr.Stderr, "could not apply") {
				// Abort the in-progress rebase so the repo is left clean.
				_, _ = run(ctx, "rebase", "--abort")
				return &RebaseConflictError{
					Branch: branch,
					Stderr: gitErr.Stderr,
				}
			}
		}
		return fmt.Errorf("git rebase %s %s: %w", onto, branch, err)
	}
	return nil
}

// Push pushes a branch to origin with upstream tracking.
// It runs: git push -u origin <branch>
func Push(ctx context.Context, branch string) error {
	_, err := run(ctx, "push", "-u", "origin", branch)
	if err != nil {
		return fmt.Errorf("git push %s: %w", branch, err)
	}
	return nil
}
