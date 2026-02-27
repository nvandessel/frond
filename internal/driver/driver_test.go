package driver

import (
	"context"
	"testing"
)

func TestResolveNative(t *testing.T) {
	drv, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve empty: %v", err)
	}
	if drv.Name() != "native" {
		t.Errorf("Name() = %q, want %q", drv.Name(), "native")
	}

	drv, err = Resolve("native")
	if err != nil {
		t.Fatalf("Resolve native: %v", err)
	}
	if drv.Name() != "native" {
		t.Errorf("Name() = %q, want %q", drv.Name(), "native")
	}
}

func TestResolveUnknown(t *testing.T) {
	_, err := Resolve("bogus")
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

func TestMockBasicFlow(t *testing.T) {
	ctx := context.Background()
	m := NewMock()

	// Initial state.
	br, _ := m.CurrentBranch(ctx)
	if br != "main" {
		t.Errorf("initial branch = %q, want main", br)
	}

	// Create branch.
	if err := m.CreateBranch(ctx, "feature", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	br, _ = m.CurrentBranch(ctx)
	if br != "feature" {
		t.Errorf("after create, branch = %q, want feature", br)
	}
	exists, _ := m.BranchExists(ctx, "feature")
	if !exists {
		t.Error("feature should exist")
	}

	// Checkout.
	if err := m.Checkout(ctx, "main"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	br, _ = m.CurrentBranch(ctx)
	if br != "main" {
		t.Errorf("after checkout, branch = %q, want main", br)
	}

	// Push (default).
	result, err := m.Push(ctx, PushOpts{Branch: "feature", Base: "main"})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !result.Created {
		t.Error("expected Created=true for new PR")
	}

	// Push with ExistingPR.
	pr := 42
	result, err = m.Push(ctx, PushOpts{Branch: "feature", Base: "main", ExistingPR: &pr})
	if err != nil {
		t.Fatalf("Push existing: %v", err)
	}
	if result.Created {
		t.Error("expected Created=false for existing PR")
	}

	// Fetch, Rebase, PRState, RetargetPR — defaults are no-ops.
	if err := m.Fetch(ctx); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if err := m.Rebase(ctx, "main", "feature"); err != nil {
		t.Fatalf("Rebase: %v", err)
	}
	state, err := m.PRState(ctx, 42)
	if err != nil {
		t.Fatalf("PRState: %v", err)
	}
	if state != "OPEN" {
		t.Errorf("PRState = %q, want OPEN", state)
	}
	if err := m.RetargetPR(ctx, 42, "main"); err != nil {
		t.Fatalf("RetargetPR: %v", err)
	}
}

func TestMockOverrides(t *testing.T) {
	ctx := context.Background()
	m := NewMock()

	fetchCalled := false
	m.FetchFn = func(_ context.Context) error {
		fetchCalled = true
		return nil
	}

	if err := m.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	if !fetchCalled {
		t.Error("FetchFn not called")
	}
}

func TestRebaseConflictError(t *testing.T) {
	e := &RebaseConflictError{Branch: "feat", Detail: "CONFLICT in file.go"}
	got := e.Error()
	if got != "rebase conflict on branch feat: CONFLICT in file.go" {
		t.Errorf("Error() = %q", got)
	}
}
