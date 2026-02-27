package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nvandessel/frond/internal/driver"
	"github.com/nvandessel/frond/internal/state"
	"github.com/spf13/pflag"
)

// setupTestEnv creates a temp directory, overrides GitCommonDir and
// injects a mock driver. No real git or gh commands are needed.
func setupTestEnv(t *testing.T) (*driver.Mock, string) {
	t.Helper()

	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	orig := state.GitCommonDir
	state.GitCommonDir = func(_ context.Context) (string, error) { return gitDir, nil }
	t.Cleanup(func() { state.GitCommonDir = orig })

	mock := driver.NewMock()
	mock.PushFn = func(_ context.Context, opts driver.PushOpts) (*driver.PushResult, error) {
		return &driver.PushResult{PRNumber: 42, Created: opts.ExistingPR == nil}, nil
	}
	mock.PRStateFn = func(_ context.Context, _ int) (string, error) {
		return "OPEN", nil
	}

	driverOverride = mock
	t.Cleanup(func() { driverOverride = nil })

	resetCobraFlags()
	jsonOut = false

	return mock, dir
}

// moduleRoot caches the repo root path, found before any test does os.Chdir.
// fakeGHBin is the path to the pre-built fake gh binary.
var (
	moduleRoot string
	fakeGHBin  string
)

func TestMain(m *testing.M) {
	// Find module root before any test changes cwd.
	dir, err := os.Getwd()
	if err != nil {
		panic("cmd_test: " + err.Error())
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			moduleRoot = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("cmd_test: could not find go.mod")
		}
		dir = parent
	}

	// Build the fake gh binary once for all tests.
	tmpDir, err := os.MkdirTemp("", "fakegh-*")
	if err != nil {
		panic("cmd_test: " + err.Error())
	}
	binName := "gh"
	if runtime.GOOS == "windows" {
		binName = "gh.exe"
	}
	fakeGHBin = filepath.Join(tmpDir, binName)

	cmd := exec.Command("go", "build", "-o", fakeGHBin, "./internal/gh/testdata/fakegh")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("cmd_test: building fakegh: %s\n%s", err, out))
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// withFakeGH installs the pre-built fakegh binary on PATH for tests that
// need the gh comment API (e.g., stack comment tests). Call after setupTestEnv.
func withFakeGH(t *testing.T) {
	t.Helper()

	ghDir := t.TempDir()
	binName := "gh"
	if runtime.GOOS == "windows" {
		binName = "gh.exe"
	}
	dst := filepath.Join(ghDir, binName)

	// Hard-link (fast) or copy the pre-built binary.
	if err := os.Link(fakeGHBin, dst); err != nil {
		// Fallback to copy if hard link fails (cross-device).
		data, err := os.ReadFile(fakeGHBin)
		if err != nil {
			t.Fatalf("reading fakegh binary: %v", err)
		}
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			t.Fatalf("writing fakegh binary: %v", err)
		}
	}

	t.Setenv("PATH", ghDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKEGH_FAIL", "")
	t.Setenv("FAKEGH_FAIL_API", "")
	t.Setenv("FAKEGH_PR_COUNTER", "")
	t.Setenv("FAKEGH_PR_STATE", "")
	t.Setenv("FAKEGH_EXISTING_COMMENT", "")
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

// readState reads frond.json from the temp repo's .git directory.
func readState(t *testing.T, repoDir string) *state.State {
	t.Helper()

	p := filepath.Join(repoDir, ".git", "frond.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading frond.json: %v", err)
	}
	var s state.State
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parsing frond.json: %v", err)
	}
	return &s
}

// runTier executes a frond subcommand and returns the error (if any).
func runTier(t *testing.T, args ...string) error {
	t.Helper()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func TestNewCreatesAndTracks(t *testing.T) {
	mock, dir := setupTestEnv(t)

	err := runTier(t, "new", "feature-x")
	if err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Verify mock branch was created and checked out.
	if mock.CurrentBranchName != "feature-x" {
		t.Errorf("current branch = %q, want %q", mock.CurrentBranchName, "feature-x")
	}
	if !mock.Branches["feature-x"] {
		t.Error("branch 'feature-x' not created in mock")
	}

	// Verify frond.json has the branch.
	s := readState(t, dir)
	if s.Trunk != "main" {
		t.Errorf("trunk = %q, want %q", s.Trunk, "main")
	}
	b, ok := s.Branches["feature-x"]
	if !ok {
		t.Fatal("branch 'feature-x' not in frond.json")
	}
	if b.Parent != "main" {
		t.Errorf("parent = %q, want %q", b.Parent, "main")
	}
}

func TestNewWithOnFlag(t *testing.T) {
	_, dir := setupTestEnv(t)

	// Create a first branch.
	if err := runTier(t, "new", "step-1"); err != nil {
		t.Fatalf("frond new step-1: %v", err)
	}

	// Create a stacked branch on top.
	if err := runTier(t, "new", "step-2", "--on", "step-1"); err != nil {
		t.Fatalf("frond new step-2: %v", err)
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
	mock, dir := setupTestEnv(t)

	// Pre-create a branch in the mock (simulates existing git branch).
	mock.Branches["existing-branch"] = true

	err := runTier(t, "track", "existing-branch", "--on", "main")
	if err != nil {
		t.Fatalf("frond track: %v", err)
	}

	s := readState(t, dir)
	b, ok := s.Branches["existing-branch"]
	if !ok {
		t.Fatal("branch 'existing-branch' not in frond.json")
	}
	if b.Parent != "main" {
		t.Errorf("parent = %q, want %q", b.Parent, "main")
	}
}

func TestTrackAlreadyTrackedFails(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "tracked-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
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
	_, dir := setupTestEnv(t)

	// Create two stacked branches.
	if err := runTier(t, "new", "parent-branch"); err != nil {
		t.Fatalf("frond new parent-branch: %v", err)
	}
	if err := runTier(t, "new", "child-branch", "--on", "parent-branch"); err != nil {
		t.Fatalf("frond new child-branch: %v", err)
	}

	// Untrack the parent.
	err := runTier(t, "untrack", "parent-branch")
	if err != nil {
		t.Fatalf("frond untrack: %v", err)
	}

	s := readState(t, dir)

	// Parent should be gone.
	if _, ok := s.Branches["parent-branch"]; ok {
		t.Error("parent-branch still in frond.json after untrack")
	}

	// Child should be reparented to main.
	child, ok := s.Branches["child-branch"]
	if !ok {
		t.Fatal("child-branch missing from frond.json")
	}
	if child.Parent != "main" {
		t.Errorf("child parent = %q, want %q (reparented to trunk)", child.Parent, "main")
	}
}

func TestUntrackNotTrackedFails(t *testing.T) {
	setupTestEnv(t)

	// Initialize frond state (via new, then untrack, then try again).
	if err := runTier(t, "new", "some-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
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
		t.Fatalf("frond new feat-a: %v", err)
	}
	if err := runTier(t, "new", "feat-b", "--on", "feat-a"); err != nil {
		t.Fatalf("frond new feat-b: %v", err)
	}

	// Status should not error.
	err := runTier(t, "status")
	if err != nil {
		t.Fatalf("frond status: %v", err)
	}
}

func TestStatusJSON(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "feat-a"); err != nil {
		t.Fatalf("frond new feat-a: %v", err)
	}

	err := runTier(t, "status", "--json")
	if err != nil {
		t.Fatalf("frond status --json: %v", err)
	}
}

func TestStatusNoStateFails(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "status")
	if err == nil {
		t.Fatal("expected error when no state exists")
	}
	if !strings.Contains(err.Error(), "no frond state") {
		t.Errorf("error = %q, want 'no frond state'", err.Error())
	}
}

func TestNewCycleDetection(t *testing.T) {
	mock, _ := setupTestEnv(t)

	// Create branch A.
	if err := runTier(t, "new", "branch-a"); err != nil {
		t.Fatalf("frond new branch-a: %v", err)
	}

	// Create branch B that depends on A.
	mock.CurrentBranchName = "main"
	if err := runTier(t, "new", "branch-b", "--on", "main", "--after", "branch-a"); err != nil {
		t.Fatalf("frond new branch-b: %v", err)
	}

	// Pre-create branch-c in mock so track can find it.
	mock.Branches["branch-c"] = true

	// Try self-dependency — should fail.
	err := runTier(t, "track", "branch-c", "--on", "main", "--after", "branch-c")
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "not tracked") && !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want cycle or dependency error", err.Error())
	}
}

func TestNewInheritsParentFromCurrentBranch(t *testing.T) {
	_, dir := setupTestEnv(t)

	// Create first branch.
	if err := runTier(t, "new", "base-feature"); err != nil {
		t.Fatalf("frond new base-feature: %v", err)
	}

	// We're now on base-feature (mock auto-checks out). Create another without --on.
	// It should inherit base-feature as parent.
	if err := runTier(t, "new", "sub-feature"); err != nil {
		t.Fatalf("frond new sub-feature: %v", err)
	}

	s := readState(t, dir)
	b := s.Branches["sub-feature"]
	if b.Parent != "base-feature" {
		t.Errorf("sub-feature parent = %q, want %q", b.Parent, "base-feature")
	}
}

func TestPushCreatesNewPR(t *testing.T) {
	mock, dir := setupTestEnv(t)

	// Create a tracked branch.
	if err := runTier(t, "new", "pr-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Ensure we're on the branch.
	if mock.CurrentBranchName != "pr-branch" {
		t.Fatalf("expected current branch pr-branch, got %s", mock.CurrentBranchName)
	}

	err := runTier(t, "push")
	if err != nil {
		t.Fatalf("frond push: %v", err)
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
	setupTestEnv(t)

	// Create a tracked branch.
	if err := runTier(t, "new", "sync-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Sync should succeed with "already up to date".
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("frond sync: %v", err)
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
	mock, _ := setupTestEnv(t)

	// Initialize state by creating one branch.
	if err := runTier(t, "new", "tracked-one"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Switch to an untracked branch.
	mock.Branches["untracked-branch"] = true
	mock.CurrentBranchName = "untracked-branch"

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
		t.Fatalf("frond new --json: %v", err)
	}
}

func TestNewWithAfterDeps(t *testing.T) {
	mock, dir := setupTestEnv(t)

	// Create two branches.
	if err := runTier(t, "new", "dep-a"); err != nil {
		t.Fatalf("frond new dep-a: %v", err)
	}
	// Go back to main so next new defaults to main.
	mock.CurrentBranchName = "main"
	if err := runTier(t, "new", "dep-b"); err != nil {
		t.Fatalf("frond new dep-b: %v", err)
	}

	// Go back to main.
	mock.CurrentBranchName = "main"

	// Create a branch with --after deps.
	if err := runTier(t, "new", "dep-c", "--on", "main", "--after", "dep-a,dep-b"); err != nil {
		t.Fatalf("frond new dep-c: %v", err)
	}

	s := readState(t, dir)
	b := s.Branches["dep-c"]
	if len(b.After) != 2 || b.After[0] != "dep-a" || b.After[1] != "dep-b" {
		t.Errorf("dep-c after = %v, want [dep-a, dep-b]", b.After)
	}
}

func TestNewInvalidBranchName(t *testing.T) {
	setupTestEnv(t)

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
	mock, _ := setupTestEnv(t)

	// Pre-create branch in mock.
	mock.Branches["json-track"] = true

	err := runTier(t, "track", "json-track", "--on", "main", "--json")
	if err != nil {
		t.Fatalf("frond track --json: %v", err)
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
		t.Fatalf("frond new: %v", err)
	}

	err := runTier(t, "untrack", "json-untrack", "--json")
	if err != nil {
		t.Fatalf("frond untrack --json: %v", err)
	}
}

func TestUntrackCurrentBranch(t *testing.T) {
	_, dir := setupTestEnv(t)

	// Create and stay on the branch.
	if err := runTier(t, "new", "current-br"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Untrack without specifying the branch name (should use current).
	err := runTier(t, "untrack")
	if err != nil {
		t.Fatalf("frond untrack (current): %v", err)
	}

	s := readState(t, dir)
	if _, ok := s.Branches["current-br"]; ok {
		t.Error("current-br should be untracked")
	}
}

func TestUntrackWithDepsAndChildren(t *testing.T) {
	mock, dir := setupTestEnv(t)

	// Create parent -> child chain with deps.
	if err := runTier(t, "new", "mid-branch"); err != nil {
		t.Fatalf("frond new mid-branch: %v", err)
	}
	if err := runTier(t, "new", "child-a", "--on", "mid-branch"); err != nil {
		t.Fatalf("frond new child-a: %v", err)
	}
	// Go back to main, create another that depends on mid-branch.
	mock.CurrentBranchName = "main"
	if err := runTier(t, "new", "dep-on-mid", "--on", "main", "--after", "mid-branch"); err != nil {
		t.Fatalf("frond new dep-on-mid: %v", err)
	}

	// Untrack mid-branch -> child-a should be reparented to main, dep-on-mid unblocked.
	err := runTier(t, "untrack", "mid-branch")
	if err != nil {
		t.Fatalf("frond untrack mid-branch: %v", err)
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
		t.Fatalf("frond completion bash: %v", err)
	}
}

func TestCompletionZsh(t *testing.T) {
	err := runTier(t, "completion", "zsh")
	if err != nil {
		t.Fatalf("frond completion zsh: %v", err)
	}
}

func TestCompletionFish(t *testing.T) {
	err := runTier(t, "completion", "fish")
	if err != nil {
		t.Fatalf("frond completion fish: %v", err)
	}
}

func TestCompletionInvalidShell(t *testing.T) {
	err := runTier(t, "completion", "powershell")
	if err == nil {
		t.Fatal("expected error for invalid shell")
	}
}

func TestPushExistingPRUpdates(t *testing.T) {
	_, dir := setupTestEnv(t)

	// Create a tracked branch.
	if err := runTier(t, "new", "update-pr-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// First push creates a PR.
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Second push should update the existing PR (not create new).
	err := runTier(t, "push")
	if err != nil {
		t.Fatalf("second push (update): %v", err)
	}

	// PR number should still be 42.
	s := readState(t, dir)
	b := s.Branches["update-pr-branch"]
	if b.PR == nil || *b.PR != 42 {
		t.Errorf("PR = %v, want 42", b.PR)
	}
}

func TestPushWithTitleAndDraft(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "draft-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	err := runTier(t, "push", "-t", "My Custom Title", "--draft")
	if err != nil {
		t.Fatalf("frond push with title and draft: %v", err)
	}
}

func TestPushWithJSONOutput(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "json-push"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	err := runTier(t, "push", "--json")
	if err != nil {
		t.Fatalf("frond push --json: %v", err)
	}
}

func TestSyncNoBranches(t *testing.T) {
	setupTestEnv(t)

	// Create a branch and immediately untrack it so state exists but has no branches.
	if err := runTier(t, "new", "temp-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}
	if err := runTier(t, "untrack", "temp-branch"); err != nil {
		t.Fatalf("frond untrack: %v", err)
	}

	// Sync with no branches should say "nothing to sync".
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("frond sync (no branches): %v", err)
	}
}

func TestSyncNoBranchesJSON(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "temp-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}
	if err := runTier(t, "untrack", "temp-branch"); err != nil {
		t.Fatalf("frond untrack: %v", err)
	}

	err := runTier(t, "sync", "--json")
	if err != nil {
		t.Fatalf("frond sync --json (no branches): %v", err)
	}
}

func TestSyncRebasesTrackedBranch(t *testing.T) {
	setupTestEnv(t)

	// Create tracked branch.
	if err := runTier(t, "new", "rebase-me"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Sync should rebase rebase-me onto main (mock rebase is no-op).
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("frond sync: %v", err)
	}
}

func TestSyncWithJSONOutput(t *testing.T) {
	setupTestEnv(t)

	if err := runTier(t, "new", "sync-json"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	err := runTier(t, "sync", "--json")
	if err != nil {
		t.Fatalf("frond sync --json: %v", err)
	}
}

func TestStatusWithPRStates(t *testing.T) {
	_, dir := setupTestEnv(t)

	// Create a tracked branch and manually set a PR number.
	if err := runTier(t, "new", "pr-status"); err != nil {
		t.Fatalf("frond new: %v", err)
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
	if err := os.WriteFile(filepath.Join(dir, ".git", "frond.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Status with --fetch should exercise fetchPRStates.
	err = runTier(t, "status", "--fetch")
	if err != nil {
		t.Fatalf("frond status --fetch: %v", err)
	}
}

func TestStatusFetchJSON(t *testing.T) {
	_, dir := setupTestEnv(t)

	if err := runTier(t, "new", "pr-json-status"); err != nil {
		t.Fatalf("frond new: %v", err)
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
	if err := os.WriteFile(filepath.Join(dir, ".git", "frond.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Status --fetch --json should exercise outputJSON with prStates.
	err = runTier(t, "status", "--fetch", "--json")
	if err != nil {
		t.Fatalf("frond status --fetch --json: %v", err)
	}
}

func TestPushWithUnmetDeps(t *testing.T) {
	mock, _ := setupTestEnv(t)

	// Create dep and dependent branches.
	if err := runTier(t, "new", "dep-branch"); err != nil {
		t.Fatalf("frond new dep-branch: %v", err)
	}

	mock.CurrentBranchName = "main"

	if err := runTier(t, "new", "with-deps", "--on", "main", "--after", "dep-branch"); err != nil {
		t.Fatalf("frond new with-deps: %v", err)
	}

	// Push should succeed but warn about unmet deps.
	err := runTier(t, "push")
	if err != nil {
		t.Fatalf("frond push (unmet deps): %v", err)
	}
}

func TestSyncBlockedBranch(t *testing.T) {
	mock, _ := setupTestEnv(t)

	// Create two branches: blocker and blocked.
	if err := runTier(t, "new", "blocker"); err != nil {
		t.Fatalf("frond new blocker: %v", err)
	}
	mock.CurrentBranchName = "main"
	if err := runTier(t, "new", "blocked-br", "--on", "main", "--after", "blocker"); err != nil {
		t.Fatalf("frond new blocked-br: %v", err)
	}

	// Sync should see blocked-br as blocked.
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("frond sync (blocked): %v", err)
	}
}

func TestPushSkipsStackCommentForSinglePR(t *testing.T) {
	_, dir := setupTestEnv(t)
	withFakeGH(t)

	recordFile := filepath.Join(dir, "gh_calls.log")
	t.Setenv("FAKEGH_RECORD", recordFile)

	// Create a single tracked branch.
	if err := runTier(t, "new", "solo-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	err := runTier(t, "push")
	if err != nil {
		t.Fatalf("frond push: %v", err)
	}

	// With only 1 PR, no comment API calls should be made.
	calls := readGHCalls(t, recordFile)
	for _, call := range calls {
		if strings.Contains(call, "api") && strings.Contains(call, "comments") {
			t.Errorf("expected no comment API calls for single PR, got: %s", call)
		}
	}
}

// setupPRCounter configures fakegh to use incrementing PR numbers.
func setupPRCounter(t *testing.T, dir string) {
	t.Helper()
	counterFile := filepath.Join(dir, "pr_counter")
	os.WriteFile(counterFile, []byte("42\n"), 0o644)
	t.Setenv("FAKEGH_PR_COUNTER", counterFile)
}

func TestPushCreatesStackComment(t *testing.T) {
	mock, dir := setupTestEnv(t)
	withFakeGH(t)

	recordFile := filepath.Join(dir, "gh_calls.log")
	t.Setenv("FAKEGH_RECORD", recordFile)

	// Use incrementing PR numbers so each push gets a unique PR.
	prCounter := 42
	mock.PushFn = func(_ context.Context, opts driver.PushOpts) (*driver.PushResult, error) {
		if opts.ExistingPR != nil {
			return &driver.PushResult{PRNumber: *opts.ExistingPR, Created: false}, nil
		}
		n := prCounter
		prCounter++
		return &driver.PushResult{PRNumber: n, Created: true}, nil
	}

	// Create two tracked branches so the stack has >= 2 PRs.
	if err := runTier(t, "new", "branch-a"); err != nil {
		t.Fatalf("frond new branch-a: %v", err)
	}

	// Push branch-a to create PR #42 (single PR, no comments yet).
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("frond push branch-a: %v", err)
	}

	// Create a second stacked branch.
	if err := runTier(t, "new", "branch-b", "--on", "branch-a"); err != nil {
		t.Fatalf("frond new branch-b: %v", err)
	}

	// Clear the record file so we only see calls from this push.
	os.Remove(recordFile)

	// Push branch-b — now there are 2 PRs (42 + 43), so stack comments should be posted.
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("frond push branch-b: %v", err)
	}

	// Verify state has distinct PR numbers.
	s := readState(t, dir)
	if s.Branches["branch-a"].PR == nil || s.Branches["branch-b"].PR == nil {
		t.Fatal("both branches should have PR numbers")
	}
	if *s.Branches["branch-a"].PR == *s.Branches["branch-b"].PR {
		t.Errorf("branches should have distinct PR numbers, both got %d", *s.Branches["branch-a"].PR)
	}

	// Verify gh api calls were made for comment operations.
	calls := readGHCalls(t, recordFile)
	var hasCommentList, hasCommentCreate bool
	for _, call := range calls {
		if strings.Contains(call, "api") && strings.Contains(call, "comments") {
			if strings.Contains(call, "--paginate") {
				hasCommentList = true
			}
			if strings.Contains(call, "body=") {
				hasCommentCreate = true
			}
		}
	}
	if !hasCommentList {
		t.Errorf("expected comment list API call, calls: %v", calls)
	}
	if !hasCommentCreate {
		t.Errorf("expected comment create API call, calls: %v", calls)
	}
}

func TestPushUpdatesStackComment(t *testing.T) {
	mock, dir := setupTestEnv(t)
	withFakeGH(t)

	recordFile := filepath.Join(dir, "gh_calls.log")
	t.Setenv("FAKEGH_RECORD", recordFile)
	t.Setenv("FAKEGH_EXISTING_COMMENT", "1")

	// Use incrementing PR numbers.
	prCounter := 42
	mock.PushFn = func(_ context.Context, opts driver.PushOpts) (*driver.PushResult, error) {
		if opts.ExistingPR != nil {
			return &driver.PushResult{PRNumber: *opts.ExistingPR, Created: false}, nil
		}
		n := prCounter
		prCounter++
		return &driver.PushResult{PRNumber: n, Created: true}, nil
	}

	// Create two tracked branches so the stack has >= 2 PRs.
	if err := runTier(t, "new", "update-branch-a"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Push branch-a to create its PR.
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("frond push update-branch-a: %v", err)
	}

	// Create second branch stacked on first.
	if err := runTier(t, "new", "update-branch-b", "--on", "update-branch-a"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Clear the record file so we only see calls from this push.
	os.Remove(recordFile)

	// Push branch-b — 2 PRs exist, FAKEGH_EXISTING_COMMENT is set,
	// so it should update (PATCH) the existing comment.
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("frond push update-branch-b: %v", err)
	}

	// Verify gh api PATCH call was made (update, not create).
	calls := readGHCalls(t, recordFile)
	var hasUpdate bool
	for _, call := range calls {
		if strings.Contains(call, "-X PATCH") && strings.Contains(call, "issues/comments/") {
			hasUpdate = true
			break
		}
	}
	if !hasUpdate {
		t.Errorf("expected comment update (PATCH) API call, calls: %v", calls)
	}
}

func TestPushStackCommentErrorNonFatal(t *testing.T) {
	mock, dir := setupTestEnv(t)
	withFakeGH(t)

	recordFile := filepath.Join(dir, "gh_calls.log")
	t.Setenv("FAKEGH_RECORD", recordFile)

	// Use incrementing PR numbers.
	prCounter := 42
	mock.PushFn = func(_ context.Context, opts driver.PushOpts) (*driver.PushResult, error) {
		if opts.ExistingPR != nil {
			return &driver.PushResult{PRNumber: *opts.ExistingPR, Created: false}, nil
		}
		n := prCounter
		prCounter++
		return &driver.PushResult{PRNumber: n, Created: true}, nil
	}

	// Create two branches with PRs so stack comments are attempted.
	if err := runTier(t, "new", "err-branch-a"); err != nil {
		t.Fatalf("frond new: %v", err)
	}
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("frond push err-branch-a: %v", err)
	}

	if err := runTier(t, "new", "err-branch-b", "--on", "err-branch-a"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Make only API calls fail — pr view/edit still work.
	t.Setenv("FAKEGH_FAIL_API", "1")

	// Push should still succeed; comment errors are warnings, not fatal.
	err := runTier(t, "push")
	if err != nil {
		t.Fatalf("push should succeed even when comment API fails: %v", err)
	}
}

func TestSyncUpdatesMergedComments(t *testing.T) {
	mock, dir := setupTestEnv(t)
	withFakeGH(t)

	recordFile := filepath.Join(dir, "gh_calls.log")
	t.Setenv("FAKEGH_RECORD", recordFile)

	// Use incrementing PR numbers.
	prCounter := 42
	mock.PushFn = func(_ context.Context, opts driver.PushOpts) (*driver.PushResult, error) {
		if opts.ExistingPR != nil {
			return &driver.PushResult{PRNumber: *opts.ExistingPR, Created: false}, nil
		}
		n := prCounter
		prCounter++
		return &driver.PushResult{PRNumber: n, Created: true}, nil
	}

	// Create two branches with PRs.
	if err := runTier(t, "new", "merge-branch-a"); err != nil {
		t.Fatalf("frond new: %v", err)
	}
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("frond push: %v", err)
	}

	if err := runTier(t, "new", "merge-branch-b", "--on", "merge-branch-a"); err != nil {
		t.Fatalf("frond new: %v", err)
	}
	if err := runTier(t, "push"); err != nil {
		t.Fatalf("frond push: %v", err)
	}

	// Make mock report all PRs as MERGED.
	mock.PRStateFn = func(_ context.Context, _ int) (string, error) {
		return "MERGED", nil
	}

	// Clear record to isolate sync calls.
	os.Remove(recordFile)

	// Sync should detect merges and post merged comments.
	err := runTier(t, "sync")
	if err != nil {
		t.Fatalf("frond sync: %v", err)
	}

	// Verify comment API calls were made for merged PR comments.
	calls := readGHCalls(t, recordFile)
	var hasCommentAPI bool
	for _, call := range calls {
		if strings.Contains(call, "api") && strings.Contains(call, "comments") {
			hasCommentAPI = true
			break
		}
	}
	if !hasCommentAPI {
		t.Errorf("expected comment API calls for merged PRs, calls: %v", calls)
	}
}

// readGHCalls reads the recorded gh CLI calls from the record file.
func readGHCalls(t *testing.T, recordFile string) []string {
	t.Helper()
	data, err := os.ReadFile(recordFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
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

func TestInitDefault(t *testing.T) {
	_, dir := setupTestEnv(t)

	err := runTier(t, "init")
	if err != nil {
		t.Fatalf("frond init: %v", err)
	}

	s := readState(t, dir)
	if s.Driver != "" {
		t.Errorf("driver = %q, want empty (native)", s.Driver)
	}
	if s.Trunk != "main" {
		t.Errorf("trunk = %q, want main", s.Trunk)
	}
}

func TestInitJSON(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "init", "--json")
	if err != nil {
		t.Fatalf("frond init --json: %v", err)
	}
}

func TestInitUnknownDriver(t *testing.T) {
	setupTestEnv(t)

	err := runTier(t, "init", "--driver", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
	if !strings.Contains(err.Error(), "unknown driver") {
		t.Errorf("error = %q, want containing 'unknown driver'", err.Error())
	}
}

func TestInitPreservesExistingState(t *testing.T) {
	_, dir := setupTestEnv(t)

	// Create some state first.
	if err := runTier(t, "new", "existing-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Init should not blow away existing branches.
	if err := runTier(t, "init"); err != nil {
		t.Fatalf("frond init: %v", err)
	}

	s := readState(t, dir)
	if _, ok := s.Branches["existing-branch"]; !ok {
		t.Error("init should preserve existing branches")
	}
}

func TestSyncMergedPR(t *testing.T) {
	mock, dir := setupTestEnv(t)

	// Create parent and child branches.
	if err := runTier(t, "new", "merged-branch"); err != nil {
		t.Fatalf("frond new merged-branch: %v", err)
	}
	if err := runTier(t, "new", "child-of-merged", "--on", "merged-branch"); err != nil {
		t.Fatalf("frond new child-of-merged: %v", err)
	}

	// Manually assign PR numbers to state.
	s := readState(t, dir)
	pr1 := 10
	b := s.Branches["merged-branch"]
	b.PR = &pr1
	s.Branches["merged-branch"] = b
	pr2 := 20
	c := s.Branches["child-of-merged"]
	c.PR = &pr2
	s.Branches["child-of-merged"] = c
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "frond.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Mock PRState to return MERGED for PR #10.
	mock.PRStateFn = func(_ context.Context, prNumber int) (string, error) {
		if prNumber == 10 {
			return "MERGED", nil
		}
		return "OPEN", nil
	}

	// Track retarget calls.
	var retargetCalls []int
	mock.RetargetPRFn = func(_ context.Context, prNumber int, _ string) error {
		retargetCalls = append(retargetCalls, prNumber)
		return nil
	}

	err = runTier(t, "sync")
	if err != nil {
		t.Fatalf("frond sync: %v", err)
	}

	// merged-branch should be removed, child reparented to main.
	s = readState(t, dir)
	if _, ok := s.Branches["merged-branch"]; ok {
		t.Error("merged-branch should be removed from state")
	}
	child := s.Branches["child-of-merged"]
	if child.Parent != "main" {
		t.Errorf("child parent = %q, want main", child.Parent)
	}

	// Child PR should have been retargeted.
	found := false
	for _, n := range retargetCalls {
		if n == 20 {
			found = true
		}
	}
	if !found {
		t.Error("expected RetargetPR called for child PR #20")
	}
}

func TestSyncRebaseConflict(t *testing.T) {
	mock, _ := setupTestEnv(t)

	if err := runTier(t, "new", "conflict-branch"); err != nil {
		t.Fatalf("frond new: %v", err)
	}

	// Mock rebase to return a conflict error.
	mock.RebaseFn = func(_ context.Context, _, branch string) error {
		return &driver.RebaseConflictError{Branch: branch, Detail: "CONFLICT in file.go"}
	}

	err := runTier(t, "sync")

	// Should return ExitError with code 2.
	if err == nil {
		t.Fatal("expected error from sync with conflict")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 2 {
		t.Errorf("exit code = %d, want 2", exitErr.Code)
	}
}

func TestResolveDriverFromState(t *testing.T) {
	// Empty driver resolves to native.
	st := &state.State{Driver: ""}
	drv, err := resolveDriver(st)
	if err != nil {
		t.Fatalf("resolveDriver empty: %v", err)
	}
	if drv.Name() != "native" {
		t.Errorf("Name() = %q, want native", drv.Name())
	}

	// Unknown driver errors.
	st = &state.State{Driver: "bogus"}
	_, err = resolveDriver(st)
	if err == nil {
		t.Fatal("expected error for unknown driver in state")
	}

	// driverOverride takes precedence.
	mock := driver.NewMock()
	driverOverride = mock
	defer func() { driverOverride = nil }()

	st = &state.State{Driver: "bogus"} // would fail without override
	drv, err = resolveDriver(st)
	if err != nil {
		t.Fatalf("resolveDriver with override: %v", err)
	}
	if drv.Name() != "mock" {
		t.Errorf("Name() = %q, want mock", drv.Name())
	}
}
