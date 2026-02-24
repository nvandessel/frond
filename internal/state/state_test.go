package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupGitRepo creates a minimal git repo in a temp dir and overrides
// gitCommonDir to point there. It returns the git-common-dir path and a
// cleanup function that restores the original gitCommonDir.
func setupGitRepo(t *testing.T) (dir string) {
	t.Helper()

	dir = t.TempDir()

	// Initialise a real git repo so git commands work.
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")

	// Create an initial commit so branches can exist.
	dummy := filepath.Join(dir, "README.md")
	if err := os.WriteFile(dummy, []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("writing dummy file: %v", err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")

	gitDir := filepath.Join(dir, ".git")

	// Override the package-level gitCommonDir so all functions in this
	// package resolve paths inside our temp repo.
	orig := gitCommonDir
	gitCommonDir = func(_ context.Context) (string, error) {
		return gitDir, nil
	}
	t.Cleanup(func() { gitCommonDir = orig })

	return dir
}

// run executes a command in the given directory and fails the test on error.
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func TestPath(t *testing.T) {
	dir := setupGitRepo(t)
	ctx := context.Background()

	p, err := Path(ctx)
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}

	want := filepath.Join(dir, ".git", "tier.json")
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}

func TestReadMissingFile(t *testing.T) {
	setupGitRepo(t)
	ctx := context.Background()

	s, err := Read(ctx)
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Read() error = %v, want ErrNotInitialized", err)
	}
	if s != nil {
		t.Errorf("Read() on missing file returned non-nil state: %+v", s)
	}
}

func TestReadMalformedJSON(t *testing.T) {
	dir := setupGitRepo(t)
	ctx := context.Background()

	// Write garbage to tier.json.
	p := filepath.Join(dir, ".git", stateFile)
	if err := os.WriteFile(p, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("writing malformed file: %v", err)
	}

	_, err := Read(ctx)
	if err == nil {
		t.Fatal("Read() should return error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error = %q, want to contain 'parsing'", err.Error())
	}
}

func TestWriteThenRead(t *testing.T) {
	setupGitRepo(t)
	ctx := context.Background()

	pr := 42
	want := &State{
		Version: 1,
		Trunk:   "main",
		Branches: map[string]Branch{
			"feature/foo": {
				Parent: "main",
				After:  []string{"feature/bar"},
				PR:     &pr,
			},
			"feature/bar": {
				Parent: "main",
				After:  nil,
				PR:     nil,
			},
		},
	}

	if err := Write(ctx, want); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	got, err := Read(ctx)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got == nil {
		t.Fatal("Read() returned nil after Write()")
	}

	if got.Version != want.Version {
		t.Errorf("Version = %d, want %d", got.Version, want.Version)
	}
	if got.Trunk != want.Trunk {
		t.Errorf("Trunk = %q, want %q", got.Trunk, want.Trunk)
	}
	if len(got.Branches) != len(want.Branches) {
		t.Fatalf("len(Branches) = %d, want %d", len(got.Branches), len(want.Branches))
	}

	fooBranch := got.Branches["feature/foo"]
	if fooBranch.Parent != "main" {
		t.Errorf("feature/foo Parent = %q, want %q", fooBranch.Parent, "main")
	}
	if fooBranch.PR == nil || *fooBranch.PR != 42 {
		t.Errorf("feature/foo PR = %v, want 42", fooBranch.PR)
	}
	if len(fooBranch.After) != 1 || fooBranch.After[0] != "feature/bar" {
		t.Errorf("feature/foo After = %v, want [feature/bar]", fooBranch.After)
	}

	barBranch := got.Branches["feature/bar"]
	if barBranch.PR != nil {
		t.Errorf("feature/bar PR = %v, want nil", barBranch.PR)
	}
}

func TestLockUnlock(t *testing.T) {
	dir := setupGitRepo(t)
	ctx := context.Background()

	// Acquire the lock.
	unlock, err := Lock(ctx)
	if err != nil {
		t.Fatalf("Lock() error: %v", err)
	}

	// Lockfile should exist.
	lockPath := filepath.Join(dir, ".git", lockFile)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lockfile does not exist after Lock(): %v", err)
	}

	// Second lock should fail because the lockfile is fresh (not stale).
	_, err = Lock(ctx)
	if err == nil {
		t.Fatal("second Lock() should have failed while lock is held")
	}

	// Release the lock.
	unlock()

	// Lockfile should be gone.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lockfile still exists after unlock: %v", err)
	}

	// Should be able to re-acquire.
	unlock2, err := Lock(ctx)
	if err != nil {
		t.Fatalf("Lock() after unlock error: %v", err)
	}
	unlock2()
}

func TestLockStaleness(t *testing.T) {
	dir := setupGitRepo(t)
	ctx := context.Background()

	lockPath := filepath.Join(dir, ".git", lockFile)

	// Create a lockfile manually with a mod time in the past.
	f, err := os.Create(lockPath)
	if err != nil {
		t.Fatalf("creating lockfile: %v", err)
	}
	f.Close()

	staleTime := time.Now().Add(-6 * time.Minute)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatalf("setting lockfile mtime: %v", err)
	}

	// Lock should succeed because the existing lockfile is stale.
	unlock, err := Lock(ctx)
	if err != nil {
		t.Fatalf("Lock() with stale lockfile error: %v", err)
	}
	unlock()
}

func TestReadOrInit(t *testing.T) {
	dir := setupGitRepo(t)
	ctx := context.Background()

	// The setupGitRepo created a "main" branch by default (git init default).
	// Ensure the default branch is named "main" for this test.
	run(t, dir, "git", "branch", "-M", "main")

	s, err := ReadOrInit(ctx)
	if err != nil {
		t.Fatalf("ReadOrInit() error: %v", err)
	}

	if s.Version != 1 {
		t.Errorf("Version = %d, want 1", s.Version)
	}
	if s.Trunk != "main" {
		t.Errorf("Trunk = %q, want %q", s.Trunk, "main")
	}
	if s.Branches == nil {
		t.Error("Branches is nil, want empty map")
	}
	if len(s.Branches) != 0 {
		t.Errorf("len(Branches) = %d, want 0", len(s.Branches))
	}

	// File should now exist on disk.
	p, _ := Path(ctx)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("tier.json does not exist after ReadOrInit(): %v", err)
	}

	// Calling ReadOrInit again should return the same state (not re-create).
	s2, err := ReadOrInit(ctx)
	if err != nil {
		t.Fatalf("second ReadOrInit() error: %v", err)
	}
	if s2.Trunk != s.Trunk || s2.Version != s.Version {
		t.Errorf("second ReadOrInit() returned different state: %+v vs %+v", s2, s)
	}
}

func TestReadOrInitMasterBranch(t *testing.T) {
	dir := setupGitRepo(t)
	ctx := context.Background()

	// Rename default branch to "master" so trunk detection picks it up.
	run(t, dir, "git", "branch", "-M", "master")

	// Override detectTrunk's git commands to run inside our temp dir.
	origGitCommonDir := gitCommonDir
	gitDir := filepath.Join(dir, ".git")
	gitCommonDir = func(_ context.Context) (string, error) {
		return gitDir, nil
	}
	t.Cleanup(func() { gitCommonDir = origGitCommonDir })

	// We need detectTrunk to run git commands in the right repo.
	// Override the PATH-relative git commands by changing to the dir.
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	s, err := ReadOrInit(ctx)
	if err != nil {
		t.Fatalf("ReadOrInit() error: %v", err)
	}
	if s.Trunk != "master" {
		t.Errorf("Trunk = %q, want %q", s.Trunk, "master")
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := setupGitRepo(t)
	ctx := context.Background()
	gitDir := filepath.Join(dir, ".git")

	// Write initial state.
	s1 := &State{
		Version:  1,
		Trunk:    "main",
		Branches: map[string]Branch{},
	}
	if err := Write(ctx, s1); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Verify the temp file does not linger after a successful write.
	tmpPath := filepath.Join(gitDir, tmpFile)
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file %s should not exist after successful Write()", tmpPath)
	}

	// Verify the state file is valid JSON.
	p := filepath.Join(gitDir, stateFile)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}
	var parsed State
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("state file contains invalid JSON: %v", err)
	}
	if parsed.Version != 1 {
		t.Errorf("parsed Version = %d, want 1", parsed.Version)
	}

	// Overwrite with new state — the file should be replaced atomically.
	pr := 7
	s2 := &State{
		Version: 1,
		Trunk:   "main",
		Branches: map[string]Branch{
			"feature/x": {Parent: "main", PR: &pr},
		},
	}
	if err := Write(ctx, s2); err != nil {
		t.Fatalf("second Write() error: %v", err)
	}

	got, err := Read(ctx)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if len(got.Branches) != 1 {
		t.Errorf("len(Branches) = %d, want 1", len(got.Branches))
	}
	if got.Branches["feature/x"].PR == nil || *got.Branches["feature/x"].PR != 7 {
		t.Errorf("feature/x PR = %v, want 7", got.Branches["feature/x"].PR)
	}
}

func TestWriteCreatesParentDirs(t *testing.T) {
	// Use a temp dir with a nested non-existent path as the git common dir.
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "deeply", "nested", "gitdir")

	orig := gitCommonDir
	gitCommonDir = func(_ context.Context) (string, error) {
		return nestedDir, nil
	}
	t.Cleanup(func() { gitCommonDir = orig })

	ctx := context.Background()
	s := &State{
		Version:  1,
		Trunk:    "main",
		Branches: map[string]Branch{},
	}

	if err := Write(ctx, s); err != nil {
		t.Fatalf("Write() with nested dirs error: %v", err)
	}

	p := filepath.Join(nestedDir, stateFile)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("state file not created at %s: %v", p, err)
	}
}

func TestWriteReadOnlyDir(t *testing.T) {
	// Write should fail if the directory is read-only (can't write temp file).
	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatal(err)
	}

	orig := gitCommonDir
	gitCommonDir = func(_ context.Context) (string, error) {
		return roDir, nil
	}
	t.Cleanup(func() { gitCommonDir = orig })

	// Make it read-only AFTER creating the dir.
	os.Chmod(roDir, 0o555)
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	ctx := context.Background()
	s := &State{Version: 1, Trunk: "main", Branches: map[string]Branch{}}
	err := Write(ctx, s)
	if err == nil {
		t.Fatal("Write() should fail on read-only directory")
	}
}

func TestLockDoubleLockFails(t *testing.T) {
	setupGitRepo(t)
	ctx := context.Background()

	// First lock should succeed.
	unlock1, err := Lock(ctx)
	if err != nil {
		t.Fatalf("first Lock() error: %v", err)
	}

	// Second lock should fail (lockfile is fresh, not stale).
	_, err = Lock(ctx)
	if err == nil {
		t.Fatal("second Lock() should fail while first is held")
	}
	if !strings.Contains(err.Error(), "held by another process") {
		t.Errorf("error = %q, want 'held by another process'", err.Error())
	}

	unlock1()
}

func TestPathError(t *testing.T) {
	// Override gitCommonDir to return an error.
	orig := gitCommonDir
	gitCommonDir = func(_ context.Context) (string, error) {
		return "", fmt.Errorf("git not found")
	}
	t.Cleanup(func() { gitCommonDir = orig })

	ctx := context.Background()

	_, err := Path(ctx)
	if err == nil {
		t.Fatal("Path() should fail when gitCommonDir fails")
	}

	_, err = Read(ctx)
	if err == nil {
		t.Fatal("Read() should fail when Path() fails")
	}

	err = Write(ctx, &State{Version: 1, Trunk: "main", Branches: map[string]Branch{}})
	if err == nil {
		t.Fatal("Write() should fail when Path() fails")
	}

	_, err = Lock(ctx)
	if err == nil {
		t.Fatal("Lock() should fail when gitCommonDir fails")
	}
}

func TestReadOrInitExistingState(t *testing.T) {
	dir := setupGitRepo(t)
	ctx := context.Background()

	run(t, dir, "git", "branch", "-M", "main")

	// First call creates state.
	s1, err := ReadOrInit(ctx)
	if err != nil {
		t.Fatalf("ReadOrInit() error: %v", err)
	}

	// Add a branch to distinguish from a fresh init.
	s1.Branches["test-branch"] = Branch{Parent: "main", After: []string{}}
	if err := Write(ctx, s1); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Second call should read existing state (not re-init).
	s2, err := ReadOrInit(ctx)
	if err != nil {
		t.Fatalf("second ReadOrInit() error: %v", err)
	}
	if _, ok := s2.Branches["test-branch"]; !ok {
		t.Error("ReadOrInit() re-initialized instead of reading existing state")
	}
}
