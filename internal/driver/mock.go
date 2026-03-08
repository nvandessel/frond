package driver

import (
	"context"
	"fmt"
)

// Mock is a stateful in-memory driver for testing.
// It tracks branches and current branch so multi-step tests work without git.
type Mock struct {
	Branches          map[string]bool
	CurrentBranchName string
	StackComments     bool // whether SupportsStackComments() returns true

	// Override hooks — nil means use default behavior.
	FetchFn      func(ctx context.Context) error
	PushFn       func(ctx context.Context, opts PushOpts) (*PushResult, error)
	RebaseFn     func(ctx context.Context, onto, branch string) error
	PRStateFn    func(ctx context.Context, prNumber int) (string, error)
	RetargetPRFn func(ctx context.Context, prNumber int, newBase string) error
}

// NewMock returns a Mock with "main" as the only branch and current branch.
func NewMock() *Mock {
	return &Mock{
		Branches:          map[string]bool{"main": true},
		CurrentBranchName: "main",
	}
}

func (m *Mock) Name() string { return "mock" }

func (m *Mock) CurrentBranch(_ context.Context) (string, error) {
	return m.CurrentBranchName, nil
}

func (m *Mock) BranchExists(_ context.Context, name string) (bool, error) {
	return m.Branches[name], nil
}

func (m *Mock) CreateBranch(_ context.Context, name, _ string) error {
	m.Branches[name] = true
	m.CurrentBranchName = name
	return nil
}

func (m *Mock) Checkout(_ context.Context, name string) error {
	if !m.Branches[name] {
		return fmt.Errorf("branch %q does not exist", name)
	}
	m.CurrentBranchName = name
	return nil
}

func (m *Mock) Fetch(ctx context.Context) error {
	if m.FetchFn != nil {
		return m.FetchFn(ctx)
	}
	return nil
}

func (m *Mock) Push(ctx context.Context, opts PushOpts) (*PushResult, error) {
	if m.PushFn != nil {
		return m.PushFn(ctx, opts)
	}
	return &PushResult{PRNumber: 1, Created: opts.ExistingPR == nil}, nil
}

func (m *Mock) Rebase(ctx context.Context, onto, branch string) error {
	if m.RebaseFn != nil {
		return m.RebaseFn(ctx, onto, branch)
	}
	return nil
}

func (m *Mock) PRState(ctx context.Context, prNumber int) (string, error) {
	if m.PRStateFn != nil {
		return m.PRStateFn(ctx, prNumber)
	}
	return "OPEN", nil
}

func (m *Mock) RetargetPR(ctx context.Context, prNumber int, newBase string) error {
	if m.RetargetPRFn != nil {
		return m.RetargetPRFn(ctx, prNumber, newBase)
	}
	return nil
}

func (m *Mock) SupportsStackComments() bool { return m.StackComments }
