// Package driver defines the interface for branch/PR/git operations.
// Frond delegates all external CLI interactions through a Driver so that
// different stacking tools (native git+gh, Graphite, etc.) can be used
// interchangeably while frond manages the DAG layer.
package driver

import (
	"context"
	"fmt"
)

// Driver abstracts branch creation, pushing, rebasing, and PR management.
type Driver interface {
	Name() string

	// Git queries
	CurrentBranch(ctx context.Context) (string, error)
	BranchExists(ctx context.Context, name string) (bool, error)
	Checkout(ctx context.Context, name string) error

	// Branch mutation
	CreateBranch(ctx context.Context, name, parent string) error

	// Remote + PR
	Fetch(ctx context.Context) error
	Push(ctx context.Context, opts PushOpts) (*PushResult, error)
	Rebase(ctx context.Context, onto, branch string) error
	PRState(ctx context.Context, prNumber int) (string, error)
	RetargetPR(ctx context.Context, prNumber int, newBase string) error
}

// PushOpts configures a push + PR create/update operation.
type PushOpts struct {
	Branch string // branch to push
	Base   string // desired PR base branch
	Title  string
	Body   string
	Draft  bool
	// ExistingPR is nil for new PRs; non-nil to push + retarget an existing PR.
	ExistingPR *int
}

// PushResult is returned after a successful push.
type PushResult struct {
	PRNumber int
	Created  bool
}

// RebaseConflictError is returned when a rebase fails due to merge conflicts.
type RebaseConflictError struct {
	Branch string
	Detail string
}

func (e *RebaseConflictError) Error() string {
	return fmt.Sprintf("rebase conflict on branch %s: %s", e.Branch, e.Detail)
}

// PR state constants returned by PRState.
const (
	PRStateOpen   = "OPEN"
	PRStateClosed = "CLOSED"
	PRStateMerged = "MERGED"
)

// Resolve returns the Driver for the given driver name.
// An empty name resolves to the native (git+gh) driver.
func Resolve(name string) (Driver, error) {
	switch name {
	case "", "native":
		return NewNative()
	case "graphite":
		return NewGraphite()
	default:
		return nil, fmt.Errorf("unknown driver %q (supported: native, graphite)", name)
	}
}
