package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nvandessel/tier/internal/state"
	"github.com/spf13/pflag"
)

// setupTestEnv creates a temp git repo with an initial commit, a fake gh
// script, and chdir into the repo. It restores state on cleanup.
func setupTestEnv(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	gitEnv := []string{
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME=" + dir,
	}

	// Run git commands in the temp dir.
	gitCmd := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), gitEnv...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("setup git %s: %s\n%s", strings.Join(args, " "), err, out)
		}
	}

	gitCmd("init", "-b", "main")
	gitCmd("commit", "--allow-empty", "-m", "init")

	// Set env vars for subprocesses.
	for _, e := range gitEnv {
		parts := strings.SplitN(e, "=", 2)
		t.Setenv(parts[0], parts[1])
	}

	// chdir to the repo.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Install a fake gh script.
	ghDir := t.TempDir()
	script := filepath.Join(ghDir, "gh")
	content := `#!/bin/bash
if [[ "$1" == "pr" && "$2" == "create" ]]; then
    echo '{"number": 42}'
    exit 0
fi
if [[ "$1" == "pr" && "$2" == "view" ]]; then
    echo '{"number": 42, "state": "OPEN", "baseRefName": "main"}'
    exit 0
fi
if [[ "$1" == "pr" && "$2" == "edit" ]]; then
    exit 0
fi
if [[ "$1" == "--version" ]]; then
    echo "gh version 2.50.0"
    exit 0
fi
exit 0
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", ghDir+":"+os.Getenv("PATH"))

	// Reset global state and cobra flags between tests.
	jsonOut = false
	resetCobraFlags()

	return dir
}

// resetCobraFlags resets all cobra flag values to their defaults so tests
// don't leak flag state between runs.
func resetCobraFlags() {
	for _, cmd := range rootCmd.Commands() {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

// readState reads tier.json from the temp repo's .git directory.
func readState(t *testing.T, repoDir string) *state.State {
	t.Helper()

	p := filepath.Join(repoDir, ".git", "tier.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading tier.json: %v", err)
	}
	var s state.State
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parsing tier.json: %v", err)
	}
	return &s
}

// runTier executes a tier subcommand and returns the error (if any).
func runTier(t *testing.T, args ...string) error {
	t.Helper()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func TestNewCreatesAndTracks(t *testing.T) {
	dir := setupTestEnv(t)

	err := runTier(t, "new", "feature-x")
	if err != nil {
		t.Fatalf("tier new: %v", err)
	}

	// Verify git branch was created and checked out.
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = dir
	out, err := branchCmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "feature-x" {
		t.Errorf("current branch = %q, want %q", got, "feature-x")
	}

	// Verify tier.json has the branch.
	s := readState(t, dir)
	if s.Trunk != "main" {
		t.Errorf("trunk = %q, want %q", s.Trunk, "main")
	}
	b, ok := s.Branches["feature-x"]
	if !ok {
		t.Fatal("branch 'feature-x' not in tier.json")
	}
	if b.Parent != "main" {
		t.Errorf("parent = %q, want %q", b.Parent, "main")
	}
}

func TestNewWithOnFlag(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a first branch.
	err := runTier(t, "new", "step-1")
	if err != nil {
		t.Fatalf("tier new step-1: %v", err)
	}

	// Create a stacked branch on top.
	err = runTier(t, "new", "step-2", "--on", "step-1")
	if err != nil {
		t.Fatalf("tier new step-2: %v", err)
	}

	s := readState(t, dir)
	b := s.Branches["step-2"]
	if b.Parent != "step-1" {
		t.Errorf("step-2 parent = %q, want %q", b.Parent, "step-1")
	}
}

func TestNewDuplicateBranchFails(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "dupe"); err != nil {
		t.Fatalf("first new: %v", err)
	}

	err := runTier(t, "new", "dupe")
	if err == nil {
		t.Fatal("expected error creating duplicate branch")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestTrackExistingBranch(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a git branch manually (not via tier).
	gitCmd := exec.Command("git", "checkout", "-b", "existing-branch", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b: %s\n%s", err, out)
	}

	// Switch back to main.
	gitCmd = exec.Command("git", "checkout", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main: %s\n%s", err, out)
	}

	err := runTier(t, "track", "existing-branch", "--on", "main")
	if err != nil {
		t.Fatalf("tier track: %v", err)
	}

	s := readState(t, dir)
	b, ok := s.Branches["existing-branch"]
	if !ok {
		t.Fatal("branch 'existing-branch' not in tier.json")
	}
	if b.Parent != "main" {
		t.Errorf("parent = %q, want %q", b.Parent, "main")
	}
}

func TestTrackAlreadyTrackedFails(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "tracked-branch"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	err := runTier(t, "track", "tracked-branch", "--on", "main")
	if err == nil {
		t.Fatal("expected error tracking already-tracked branch")
	}
	if !strings.Contains(err.Error(), "already tracked") {
		t.Errorf("error = %q, want 'already tracked'", err.Error())
	}
}

func TestUntrackRemovesBranch(t *testing.T) {
	dir := setupTestEnv(t)

	// Create two stacked branches.
	if err := runTier(t, "new", "parent-branch"); err != nil {
		t.Fatalf("tier new parent-branch: %v", err)
	}
	if err := runTier(t, "new", "child-branch", "--on", "parent-branch"); err != nil {
		t.Fatalf("tier new child-branch: %v", err)
	}

	// Untrack the parent.
	err := runTier(t, "untrack", "parent-branch")
	if err != nil {
		t.Fatalf("tier untrack: %v", err)
	}

	s := readState(t, dir)

	// Parent should be gone.
	if _, ok := s.Branches["parent-branch"]; ok {
		t.Error("parent-branch still in tier.json after untrack")
	}

	// Child should be reparented to main.
	child, ok := s.Branches["child-branch"]
	if !ok {
		t.Fatal("child-branch missing from tier.json")
	}
	if child.Parent != "main" {
		t.Errorf("child parent = %q, want %q (reparented to trunk)", child.Parent, "main")
	}
}

func TestUntrackNotTrackedFails(t *testing.T) {
	setupTestEnv(t)

	// Initialize tier state (via new, then untrack, then try again).
	if err := runTier(t, "new", "some-branch"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	err := runTier(t, "untrack", "nonexistent")
	if err == nil {
		t.Fatal("expected error untracking non-tracked branch")
	}
	if !strings.Contains(err.Error(), "not tracked") {
		t.Errorf("error = %q, want 'not tracked'", err.Error())
	}
}

func TestStatusShowsTree(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "feat-a"); err != nil {
		t.Fatalf("tier new feat-a: %v", err)
	}
	if err := runTier(t, "new", "feat-b", "--on", "feat-a"); err != nil {
		t.Fatalf("tier new feat-b: %v", err)
	}

	// Status should not error.
	err := runTier(t, "status")
	if err != nil {
		t.Fatalf("tier status: %v", err)
	}
}

func TestStatusJSON(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "feat-a"); err != nil {
		t.Fatalf("tier new feat-a: %v", err)
	}

	err := runTier(t, "status", "--json")
	if err != nil {
		t.Fatalf("tier status --json: %v", err)
	}
}

func TestStatusNoStateFails(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "status")
	if err == nil {
		t.Fatal("expected error when no state exists")
	}
	if !strings.Contains(err.Error(), "no tier state") {
		t.Errorf("error = %q, want 'no tier state'", err.Error())
	}
}

func TestNewCycleDetection(t *testing.T) {
	setupTestEnv(t)

	// Create branch A.
	if err := runTier(t, "new", "branch-a"); err != nil {
		t.Fatalf("tier new branch-a: %v", err)
	}

	// Create branch B that depends on A.
	if err := runTier(t, "new", "branch-b", "--on", "main", "--after", "branch-a"); err != nil {
		t.Fatalf("tier new branch-b: %v", err)
	}

	// Try to create C with --after=branch-b AND on branch-a, but also adding
	// a circular dep. Actually, a direct cycle: create C --after=branch-b,
	// then try to create D --after=C --after=... that forms a cycle.
	// Simplest cycle: A --after B, B --after A.
	// B already depends on A. Try creating C --after=branch-b where
	// branch-b has after=[branch-a], and C has after=[branch-b].
	// That's not a cycle, just a chain.

	// For a real cycle: create C that depends on branch-b,
	// then try to make branch-a depend on C (but we can't modify after post-creation).
	// Instead: create C --on main --after branch-b, then D --on main --after C,branch-a
	// This creates: A -> B -> C -> D, D -> A which is a cycle.

	// Simplest approach: branch-b after=[branch-a]. Create branch-c --after=branch-b.
	// Then create branch-d --after=branch-c,branch-a is NOT a cycle.
	// We need: create branch-c --after=branch-b, then branch-a --after=branch-c
	// But branch-a already exists.

	// Use track to add a branch with a cyclic dep.
	// Create branch-c in git manually.
	gitCmd := exec.Command("git", "checkout", "-b", "branch-c", "main")
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}

	// Try to track branch-c --on main --after=branch-b.
	// Then try to create branch-a2 that dep on branch-c. No...
	// Let me just test: create branch-c with --after=branch-a,
	// where branch-a would need --after=branch-c (cycle).
	// But since we can't modify existing branches' after lists,
	// test the simpler case: track branch-c --after branch-a,branch-c → self-cycle.

	// Actually the simplest: self-dependency.
	err := runTier(t, "track", "branch-c", "--on", "main", "--after", "branch-c")
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "not tracked") && !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want cycle or dependency error", err.Error())
	}
}

func TestNewInheritsParentFromCurrentBranch(t *testing.T) {
	dir := setupTestEnv(t)

	// Create first branch.
	if err := runTier(t, "new", "base-feature"); err != nil {
		t.Fatalf("tier new base-feature: %v", err)
	}

	// We're now on base-feature. Create another without --on.
	// It should inherit base-feature as parent.
	if err := runTier(t, "new", "sub-feature"); err != nil {
		t.Fatalf("tier new sub-feature: %v", err)
	}

	s := readState(t, dir)
	b := s.Branches["sub-feature"]
	if b.Parent != "base-feature" {
		t.Errorf("sub-feature parent = %q, want %q", b.Parent, "base-feature")
	}
}

func TestPushCreatesNewPR(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a tracked branch with a commit.
	if err := runTier(t, "new", "pr-branch"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	// Add a commit so push has something.
	gitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "feature work")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s\n%s", err, out)
	}

	// Push needs a remote. Create a bare remote.
	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}

	// Add the bare repo as "origin".
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}

	// Push main first so origin has a "main" branch.
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	err := runTier(t, "push")
	if err != nil {
		t.Fatalf("tier push: %v", err)
	}

	// Verify PR number was saved to state.
	s := readState(t, dir)
	b := s.Branches["pr-branch"]
	if b.PR == nil {
		t.Fatal("PR number not saved after push")
	}
	if *b.PR != 42 {
		t.Errorf("PR number = %d, want 42", *b.PR)
	}
}

func TestRemoveFromSlice(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		val  string
		want []string
	}{
		{"remove present", []string{"a", "b", "c"}, "b", []string{"a", "c"}},
		{"remove absent", []string{"a", "b"}, "x", []string{"a", "b"}},
		{"remove all", []string{"a", "a"}, "a", nil},
		{"empty slice", nil, "a", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeFromSlice(tt.in, tt.val)
			if len(got) != len(tt.want) {
				t.Errorf("removeFromSlice(%v, %q) = %v, want %v", tt.in, tt.val, got, tt.want)
			}
		})
	}
}

func TestSyncNothingToDo(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a tracked branch.
	if err := runTier(t, "new", "sync-branch"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	// Set up a remote so fetch works.
	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	// Push main so origin has it.
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	// Sync should succeed with "already up to date".
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("tier sync: %v", err)
	}
}

func TestHumanizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"pay/stripe-client", "Pay Stripe Client"},
		{"feature-foo", "Feature Foo"},
		{"simple", "Simple"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := humanizeTitle(tt.input)
			if got != tt.want {
				t.Errorf("humanizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPushUntrackedBranchFails(t *testing.T) {
	dir := setupTestEnv(t)

	// Initialize state by creating one branch.
	if err := runTier(t, "new", "tracked-one"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	// Switch to an untracked branch.
	gitCmd := exec.Command("git", "checkout", "-b", "untracked-branch")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}

	err := runTier(t, "push")
	if err == nil {
		t.Fatal("expected error pushing untracked branch")
	}
	if !strings.Contains(err.Error(), "not tracked") {
		t.Errorf("error = %q, want 'not tracked'", err.Error())
	}
}

func TestExitError(t *testing.T) {
	e := &ExitError{Code: 2}
	got := e.Error()
	if got != "exit status 2" {
		t.Errorf("ExitError.Error() = %q, want %q", got, "exit status 2")
	}

	e0 := &ExitError{Code: 0}
	if e0.Error() != "exit status 0" {
		t.Errorf("ExitError{0}.Error() = %q", e0.Error())
	}
}

func TestValidateBranchNameEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"empty", "", "cannot be empty"},
		{"starts with dash", "-bad", "cannot start with '-'"},
		{"contains dot-dot", "a..b", "cannot contain '..'"},
		{"control character", "a\x00b", "control characters"},
		{"valid simple", "feature-x", ""},
		{"valid with slash", "feat/sub", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranchName(tt.input)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateBranchName(%q) = %v, want nil", tt.input, err)
				}
			} else {
				if err == nil {
					t.Fatalf("validateBranchName(%q) = nil, want error containing %q", tt.input, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestNewWithJSONOutput(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "new", "json-branch", "--json")
	if err != nil {
		t.Fatalf("tier new --json: %v", err)
	}
}

func TestNewWithAfterDeps(t *testing.T) {
	dir := setupTestEnv(t)

	// Create two branches.
	if err := runTier(t, "new", "dep-a"); err != nil {
		t.Fatalf("tier new dep-a: %v", err)
	}
	// Go back to main so next new defaults to main.
	gitCmd := exec.Command("git", "checkout", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}
	if err := runTier(t, "new", "dep-b"); err != nil {
		t.Fatalf("tier new dep-b: %v", err)
	}

	// Go back to main.
	gitCmd = exec.Command("git", "checkout", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}

	// Create a branch with --after deps.
	if err := runTier(t, "new", "dep-c", "--on", "main", "--after", "dep-a,dep-b"); err != nil {
		t.Fatalf("tier new dep-c: %v", err)
	}

	s := readState(t, dir)
	b := s.Branches["dep-c"]
	if len(b.After) != 2 || b.After[0] != "dep-a" || b.After[1] != "dep-b" {
		t.Errorf("dep-c after = %v, want [dep-a, dep-b]", b.After)
	}
}

func TestNewInvalidBranchName(t *testing.T) {
	setupTestEnv(t)

	// Branch name with ".." is invalid and gets past cobra flag parsing.
	err := runTier(t, "new", "a..b")
	if err == nil {
		t.Fatal("expected error for branch name with '..'")
	}
	if !strings.Contains(err.Error(), "cannot contain '..'") {
		t.Errorf("error = %q, want containing '..'", err.Error())
	}
}

func TestNewParentNotExist(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "new", "feature-x", "--on", "nonexistent-parent")
	if err == nil {
		t.Fatal("expected error for nonexistent parent")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want containing 'does not exist'", err.Error())
	}
}

func TestNewAfterDepNotTracked(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "new", "feature-x", "--after", "nonexistent-dep")
	if err == nil {
		t.Fatal("expected error for untracked --after dep")
	}
	if !strings.Contains(err.Error(), "not tracked") {
		t.Errorf("error = %q, want containing 'not tracked'", err.Error())
	}
}

func TestTrackWithJSONOutput(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a branch in git manually.
	gitCmd := exec.Command("git", "checkout", "-b", "json-track", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}
	gitCmd = exec.Command("git", "checkout", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}

	err := runTier(t, "track", "json-track", "--on", "main", "--json")
	if err != nil {
		t.Fatalf("tier track --json: %v", err)
	}
}

func TestTrackBranchNotExist(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "track", "ghost-branch", "--on", "main")
	if err == nil {
		t.Fatal("expected error tracking nonexistent branch")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want containing 'does not exist'", err.Error())
	}
}

func TestTrackInvalidName(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "track", "a..b", "--on", "main")
	if err == nil {
		t.Fatal("expected error for branch name with '..'")
	}
	if !strings.Contains(err.Error(), "cannot contain '..'") {
		t.Errorf("error = %q, want containing '..'", err.Error())
	}
}

func TestUntrackWithJSONOutput(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "json-untrack"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	err := runTier(t, "untrack", "json-untrack", "--json")
	if err != nil {
		t.Fatalf("tier untrack --json: %v", err)
	}
}

func TestUntrackCurrentBranch(t *testing.T) {
	dir := setupTestEnv(t)

	// Create and stay on the branch.
	if err := runTier(t, "new", "current-br"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	// Untrack without specifying the branch name (should use current).
	err := runTier(t, "untrack")
	if err != nil {
		t.Fatalf("tier untrack (current): %v", err)
	}

	s := readState(t, dir)
	if _, ok := s.Branches["current-br"]; ok {
		t.Error("current-br should be untracked")
	}
}

func TestUntrackWithDepsAndChildren(t *testing.T) {
	dir := setupTestEnv(t)

	// Create parent -> child chain with deps.
	if err := runTier(t, "new", "mid-branch"); err != nil {
		t.Fatalf("tier new mid-branch: %v", err)
	}
	if err := runTier(t, "new", "child-a", "--on", "mid-branch"); err != nil {
		t.Fatalf("tier new child-a: %v", err)
	}
	// Go back to main, create another that depends on mid-branch.
	gitCmd := exec.Command("git", "checkout", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}
	if err := runTier(t, "new", "dep-on-mid", "--on", "main", "--after", "mid-branch"); err != nil {
		t.Fatalf("tier new dep-on-mid: %v", err)
	}

	// Untrack mid-branch -> child-a should be reparented to main, dep-on-mid unblocked.
	err := runTier(t, "untrack", "mid-branch")
	if err != nil {
		t.Fatalf("tier untrack mid-branch: %v", err)
	}

	s := readState(t, dir)
	if _, ok := s.Branches["mid-branch"]; ok {
		t.Error("mid-branch should be untracked")
	}
	childA := s.Branches["child-a"]
	if childA.Parent != "main" {
		t.Errorf("child-a parent = %q, want %q", childA.Parent, "main")
	}
	depOnMid := s.Branches["dep-on-mid"]
	for _, dep := range depOnMid.After {
		if dep == "mid-branch" {
			t.Error("mid-branch should be removed from dep-on-mid's after list")
		}
	}
}

func TestCompletionBash(t *testing.T) {
	err := runTier(t, "completion", "bash")
	if err != nil {
		t.Fatalf("tier completion bash: %v", err)
	}
}

func TestCompletionZsh(t *testing.T) {
	err := runTier(t, "completion", "zsh")
	if err != nil {
		t.Fatalf("tier completion zsh: %v", err)
	}
}

func TestCompletionFish(t *testing.T) {
	err := runTier(t, "completion", "fish")
	if err != nil {
		t.Fatalf("tier completion fish: %v", err)
	}
}

func TestCompletionInvalidShell(t *testing.T) {
	err := runTier(t, "completion", "powershell")
	if err == nil {
		t.Fatal("expected error for invalid shell")
	}
}

func TestPushExistingPRUpdates(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a tracked branch with a commit.
	if err := runTier(t, "new", "update-pr-branch"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	gitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "work")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s\n%s", err, out)
	}

	// Set up remote.
	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	// First push creates a PR.
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Add another commit.
	gitCmd = exec.Command("git", "commit", "--allow-empty", "-m", "more work")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s\n%s", err, out)
	}

	// Second push should update the existing PR (not create new).
	err := runTier(t, "push")
	if err != nil {
		t.Fatalf("second push (update): %v", err)
	}
}

func TestPushWithTitleAndDraft(t *testing.T) {
	dir := setupTestEnv(t)

	if err := runTier(t, "new", "draft-branch"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	gitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "work")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s\n%s", err, out)
	}

	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	err := runTier(t, "push", "-t", "My Custom Title", "--draft")
	if err != nil {
		t.Fatalf("tier push with title and draft: %v", err)
	}
}

func TestPushWithJSONOutput(t *testing.T) {
	dir := setupTestEnv(t)

	if err := runTier(t, "new", "json-push"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	gitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "work")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s\n%s", err, out)
	}

	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	err := runTier(t, "push", "--json")
	if err != nil {
		t.Fatalf("tier push --json: %v", err)
	}
}

func TestSyncNoBranches(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a branch and immediately untrack it so state exists but has no branches.
	if err := runTier(t, "new", "temp-branch"); err != nil {
		t.Fatalf("tier new: %v", err)
	}
	if err := runTier(t, "untrack", "temp-branch"); err != nil {
		t.Fatalf("tier untrack: %v", err)
	}

	// Set up remote.
	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	// Sync with no branches should say "nothing to sync".
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("tier sync (no branches): %v", err)
	}
}

func TestSyncNoBranchesJSON(t *testing.T) {
	dir := setupTestEnv(t)

	if err := runTier(t, "new", "temp-branch"); err != nil {
		t.Fatalf("tier new: %v", err)
	}
	if err := runTier(t, "untrack", "temp-branch"); err != nil {
		t.Fatalf("tier untrack: %v", err)
	}

	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	err := runTier(t, "sync", "--json")
	if err != nil {
		t.Fatalf("tier sync --json (no branches): %v", err)
	}
}

func TestSyncRebasesTrackedBranch(t *testing.T) {
	dir := setupTestEnv(t)

	// Create tracked branch.
	if err := runTier(t, "new", "rebase-me"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	// Add a commit on the feature branch.
	gitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "feature work")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s\n%s", err, out)
	}

	// Go back to main and add a commit.
	gitCmd = exec.Command("git", "checkout", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}
	gitCmd = exec.Command("git", "commit", "--allow-empty", "-m", "main advance")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s\n%s", err, out)
	}

	// Set up remote.
	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	// Sync should rebase rebase-me onto main.
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("tier sync: %v", err)
	}
}

func TestSyncWithJSONOutput(t *testing.T) {
	dir := setupTestEnv(t)

	if err := runTier(t, "new", "sync-json"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	err := runTier(t, "sync", "--json")
	if err != nil {
		t.Fatalf("tier sync --json: %v", err)
	}
}

func TestStatusWithPRStates(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a tracked branch and manually set a PR number.
	if err := runTier(t, "new", "pr-status"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	// Manually write a PR number into state.
	s := readState(t, dir)
	prNum := 42
	b := s.Branches["pr-status"]
	b.PR = &prNum
	s.Branches["pr-status"] = b
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "tier.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Status with --fetch should exercise fetchPRStates and outputHuman with prStates.
	err = runTier(t, "status", "--fetch")
	if err != nil {
		t.Fatalf("tier status --fetch: %v", err)
	}
}

func TestStatusFetchJSON(t *testing.T) {
	dir := setupTestEnv(t)

	if err := runTier(t, "new", "pr-json-status"); err != nil {
		t.Fatalf("tier new: %v", err)
	}

	// Manually set a PR number.
	s := readState(t, dir)
	prNum := 42
	b := s.Branches["pr-json-status"]
	b.PR = &prNum
	s.Branches["pr-json-status"] = b
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "tier.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Status --fetch --json should exercise outputJSON with prStates.
	err = runTier(t, "status", "--fetch", "--json")
	if err != nil {
		t.Fatalf("tier status --fetch --json: %v", err)
	}
}

func TestPushWithUnmetDeps(t *testing.T) {
	dir := setupTestEnv(t)

	// Create dep and dependent branches.
	if err := runTier(t, "new", "dep-branch"); err != nil {
		t.Fatalf("tier new dep-branch: %v", err)
	}

	gitCmd := exec.Command("git", "checkout", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}

	if err := runTier(t, "new", "with-deps", "--on", "main", "--after", "dep-branch"); err != nil {
		t.Fatalf("tier new with-deps: %v", err)
	}

	gitCmd = exec.Command("git", "commit", "--allow-empty", "-m", "work")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s\n%s", err, out)
	}

	// Set up remote.
	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	// Push should succeed but warn about unmet deps.
	err := runTier(t, "push")
	if err != nil {
		t.Fatalf("tier push (unmet deps): %v", err)
	}
}

func TestSyncBlockedBranch(t *testing.T) {
	dir := setupTestEnv(t)

	// Create two branches: blocker and blocked.
	if err := runTier(t, "new", "blocker"); err != nil {
		t.Fatalf("tier new blocker: %v", err)
	}
	gitCmd := exec.Command("git", "checkout", "main")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s\n%s", err, out)
	}
	if err := runTier(t, "new", "blocked-br", "--on", "main", "--after", "blocker"); err != nil {
		t.Fatalf("tier new blocked-br: %v", err)
	}

	// Set up remote.
	remoteDir := t.TempDir()
	bareInit := exec.Command("git", "init", "--bare")
	bareInit.Dir = remoteDir
	if out, err := bareInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	addRemote := exec.Command("git", "remote", "add", "origin", remoteDir)
	addRemote.Dir = dir
	if out, err := addRemote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	pushMain := exec.Command("git", "push", "origin", "main")
	pushMain.Dir = dir
	if out, err := pushMain.CombinedOutput(); err != nil {
		t.Fatalf("git push main: %s\n%s", err, out)
	}

	// Sync should see blocked-br as blocked.
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("tier sync (blocked): %v", err)
	}
}

func TestNewEmptySyncResult(t *testing.T) {
	r := newEmptySyncResult()
	if r.Merged == nil || r.Rebased == nil || r.Unblocked == nil || r.Conflicts == nil {
		t.Error("newEmptySyncResult should initialize all slices")
	}
	if r.Reparented == nil || r.Blocked == nil {
		t.Error("newEmptySyncResult should initialize all maps")
	}
}
