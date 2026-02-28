package driver

import (
	"context"
	"errors"
	"fmt"

	"github.com/nvandessel/frond/internal/gh"
	"github.com/nvandessel/frond/internal/git"
)

// Native is the default driver using git + gh CLIs directly.
type Native struct{}

// NewNative validates that gh is installed and returns a Native driver.
func NewNative() (*Native, error) {
	if err := gh.Available(); err != nil {
		return nil, err
	}
	return &Native{}, nil
}

func (n *Native) Name() string { return "native" }

func (n *Native) CurrentBranch(ctx context.Context) (string, error) {
	return git.CurrentBranch(ctx)
}

func (n *Native) BranchExists(ctx context.Context, name string) (bool, error) {
	return git.BranchExists(ctx, name)
}

func (n *Native) Checkout(ctx context.Context, name string) error {
	return git.Checkout(ctx, name)
}

func (n *Native) CreateBranch(ctx context.Context, name, parent string) error {
	return git.CreateBranch(ctx, name, parent)
}

func (n *Native) Fetch(ctx context.Context) error {
	return git.Fetch(ctx)
}

func (n *Native) Push(ctx context.Context, opts PushOpts) (*PushResult, error) {
	// Push the branch to origin.
	if err := git.Push(ctx, opts.Branch); err != nil {
		return nil, fmt.Errorf("pushing %s: %w", opts.Branch, err)
	}

	if opts.ExistingPR != nil {
		// Existing PR — check if base needs retargeting.
		info, err := gh.PRView(ctx, *opts.ExistingPR)
		if err != nil {
			return nil, fmt.Errorf("viewing PR #%d: %w", *opts.ExistingPR, err)
		}
		if info.BaseRefName != opts.Base {
			if err := gh.PREdit(ctx, *opts.ExistingPR, opts.Base); err != nil {
				return nil, fmt.Errorf("retargeting PR #%d: %w", *opts.ExistingPR, err)
			}
		}
		return &PushResult{PRNumber: *opts.ExistingPR, Created: false}, nil
	}

	// New PR — create it.
	prNum, err := gh.PRCreate(ctx, gh.PRCreateOpts{
		Base:  opts.Base,
		Head:  opts.Branch,
		Title: opts.Title,
		Body:  opts.Body,
		Draft: opts.Draft,
	})
	if err != nil {
		return nil, fmt.Errorf("creating PR: %w", err)
	}
	return &PushResult{PRNumber: prNum, Created: true}, nil
}

func (n *Native) Rebase(ctx context.Context, onto, branch string) error {
	err := git.Rebase(ctx, onto, branch)
	if err != nil {
		var conflictErr *git.RebaseConflictError
		if errors.As(err, &conflictErr) {
			return &RebaseConflictError{
				Branch: conflictErr.Branch,
				Detail: conflictErr.Stderr,
			}
		}
		return err
	}
	return nil
}

func (n *Native) PRState(ctx context.Context, prNumber int) (string, error) {
	return gh.PRState(ctx, prNumber)
}

func (n *Native) RetargetPR(ctx context.Context, prNumber int, newBase string) error {
	return gh.PREdit(ctx, prNumber, newBase)
}

func (n *Native) SupportsStackComments() bool { return true }
