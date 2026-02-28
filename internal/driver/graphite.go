package driver

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/nvandessel/frond/internal/git"
)

// submitLineRe matches gt submit output lines: "<branch>: <url> (created|updated)"
var submitLineRe = regexp.MustCompile(`^(\S+):\s+(https://\S+)\s+\((created|updated)\)$`)

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
	if opts.Body != "" && opts.ExistingPR == nil {
		args = append(args, "--description", opts.Body)
	}

	out, err := runGT(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gt submit: %s: %w", out, err)
	}

	// For existing PRs, return the existing number.
	if opts.ExistingPR != nil {
		return &PushResult{PRNumber: *opts.ExistingPR, Created: false}, nil
	}

	// Parse PR number and created/updated status from gt submit output.
	prNum, created, err := parseSubmitResult(out, opts.Branch)
	if err != nil {
		return nil, fmt.Errorf("parsing PR number from gt submit output: %w", err)
	}
	return &PushResult{PRNumber: prNum, Created: created}, nil
}

func (g *Graphite) Rebase(ctx context.Context, _, _ string) error {
	// gt restack handles the entire stack; called per-branch in topo loop
	// but is idempotent so repeated calls are safe.
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

// parseSubmitResult extracts the PR number and created/updated status for
// branch from gt submit output.
// gt submit prints one line per branch: "<branch>: <url> (created|updated)"
func parseSubmitResult(output, branch string) (prNumber int, created bool, err error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		matches := submitLineRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		if matches[1] != branch {
			continue
		}
		url := matches[2]
		idx := strings.LastIndex(url, "/")
		if idx == -1 || idx == len(url)-1 {
			return 0, false, fmt.Errorf("malformed PR URL %q: no trailing number", url)
		}
		num, parseErr := strconv.Atoi(url[idx+1:])
		if parseErr != nil {
			return 0, false, fmt.Errorf("malformed PR URL %q: %w", url, parseErr)
		}
		return num, matches[3] == "created", nil
	}
	return 0, false, fmt.Errorf("branch %q not found in gt submit output:\n%s", branch, output)
}
