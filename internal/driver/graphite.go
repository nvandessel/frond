package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nvandessel/frond/internal/git"
)

// Graphite delegates stacking operations to the Graphite CLI (gt).
// It embeds Native and overrides CreateBranch, Push, and Rebase.
type Graphite struct {
	Native
}

// NewGraphite validates that gt is installed and returns a Graphite driver.
func NewGraphite() (*Graphite, error) {
	if _, err := exec.LookPath("gt"); err != nil {
		return nil, fmt.Errorf("graphite CLI (gt) not found. Install: https://graphite.dev/docs/installing-the-cli")
	}
	return &Graphite{}, nil
}

func (g *Graphite) Name() string { return "graphite" }

func (g *Graphite) CreateBranch(ctx context.Context, name, parent string) error {
	// Checkout parent first, then use gt create.
	if err := git.Checkout(ctx, parent); err != nil {
		return fmt.Errorf("checking out parent %s: %w", parent, err)
	}
	out, err := runGT(ctx, "create", name)
	if err != nil {
		return fmt.Errorf("gt create %s: %s: %w", name, out, err)
	}
	return nil
}

func (g *Graphite) Push(ctx context.Context, opts PushOpts) (*PushResult, error) {
	args := []string{"submit", "--no-interactive", "--no-edit"}
	if opts.Draft {
		args = append(args, "--draft")
	}
	if opts.Title != "" && opts.ExistingPR == nil {
		args = append(args, "--title", opts.Title)
	}

	out, err := runGT(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gt submit: %s: %w", out, err)
	}

	// gt submit doesn't return a PR number directly.
	// For existing PRs, return the existing number.
	if opts.ExistingPR != nil {
		return &PushResult{PRNumber: *opts.ExistingPR, Created: false}, nil
	}

	// For new PRs, look up the PR number via gh.
	prNum, err := lookupPRByBranch(ctx, opts.Branch)
	if err != nil {
		return nil, fmt.Errorf("looking up PR after gt submit: %w", err)
	}
	return &PushResult{PRNumber: prNum, Created: true}, nil
}

func (g *Graphite) Rebase(_ context.Context, _, _ string) error {
	// gt restack handles the entire stack; called per-branch in topo loop
	// but is idempotent so repeated calls are safe.
	ctx := context.Background()
	out, err := runGT(ctx, "restack")
	if err != nil {
		if strings.Contains(out, "CONFLICT") || strings.Contains(out, "could not apply") {
			return &RebaseConflictError{
				Branch: "stack",
				Detail: out,
			}
		}
		return fmt.Errorf("gt restack: %s: %w", out, err)
	}
	return nil
}

// runGT executes a gt command and returns combined stdout/stderr.
func runGT(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gt", args...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// lookupPRByBranch uses gh to find the PR number for a branch after gt submit.
func lookupPRByBranch(ctx context.Context, branch string) (int, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", branch, "--json", "number")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("gh pr view %s: %s: %w", branch, strings.TrimSpace(stderr.String()), err)
	}

	var result struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
		return 0, fmt.Errorf("parsing pr view output: %w", err)
	}
	return result.Number, nil
}
