package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a temporary git repo with an initial commit and returns
// its path. It sets the working directory for git commands via the GIT_WORK_TREE
// and GIT_DIR environment variables on the returned context-cancel pair, and
// also changes the process working directory for the duration of the test.
func initRepo(t *testing.T) (dir string, ctx context.Context) {
	t.Helper()

	dir = t.TempDir()
	ctx = context.Background()

	// Set env vars so git doesn't rely on global config.
	gitEnv := []string{
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME=" + dir,
	}

	// Helper to run git commands in the temp dir during setup.
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

	// Change to the temp dir so all git commands in the package run there.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	// Also set the env vars for the test process so run() picks them up.
	for _, e := range gitEnv {
		parts := strings.SplitN(e, "=", 2)
		orig, hadOrig := os.LookupEnv(parts[0])
		os.Setenv(parts[0], parts[1])
		if hadOrig {
			t.Cleanup(func() { os.Setenv(parts[0], orig) })
		} else {
			t.Cleanup(func() { os.Unsetenv(parts[0]) })
		}
	}

	return dir, ctx
}

func TestCommonDir(t *testing.T) {
	dir, ctx := initRepo(t)

	got, err := CommonDir(ctx)
	if err != nil {
		t.Fatalf("CommonDir() error: %v", err)
	}

	// CommonDir should return a path that resolves to <repo>/.git
	want := filepath.Join(dir, ".git")
	// Resolve both to absolute paths for comparison (CommonDir might return relative).
	absGot, err := filepath.Abs(got)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", got, err)
	}
	if absGot != want {
		t.Errorf("CommonDir() = %q (abs: %q), want %q", got, absGot, want)
	}
}

func TestCurrentBranch(t *testing.T) {
	_, ctx := initRepo(t)

	got, err := CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if got != "main" {
		t.Errorf("CurrentBranch() = %q, want %q", got, "main")
	}
}

func TestBranchExists(t *testing.T) {
	_, ctx := initRepo(t)

	t.Run("existing branch", func(t *testing.T) {
		exists, err := BranchExists(ctx, "main")
		if err != nil {
			t.Fatalf("BranchExists(main) error: %v", err)
		}
		if !exists {
			t.Error("BranchExists(main) = false, want true")
		}
	})

	t.Run("non-existing branch", func(t *testing.T) {
		exists, err := BranchExists(ctx, "no-such-branch")
		if err != nil {
			t.Fatalf("BranchExists(no-such-branch) error: %v", err)
		}
		if exists {
			t.Error("BranchExists(no-such-branch) = true, want false")
		}
	})
}

func TestCreateBranch(t *testing.T) {
	_, ctx := initRepo(t)

	err := CreateBranch(ctx, "feature-x", "main")
	if err != nil {
		t.Fatalf("CreateBranch() error: %v", err)
	}

	// Verify we're now on the new branch.
	got, err := CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if got != "feature-x" {
		t.Errorf("after CreateBranch, CurrentBranch() = %q, want %q", got, "feature-x")
	}

	// Verify the branch exists.
	exists, err := BranchExists(ctx, "feature-x")
	if err != nil {
		t.Fatalf("BranchExists(feature-x) error: %v", err)
	}
	if !exists {
		t.Error("BranchExists(feature-x) = false after CreateBranch")
	}
}

func TestCheckout(t *testing.T) {
	_, ctx := initRepo(t)

	// Create a second branch to switch between.
	err := CreateBranch(ctx, "other", "main")
	if err != nil {
		t.Fatalf("CreateBranch() error: %v", err)
	}

	// Switch back to main.
	err = Checkout(ctx, "main")
	if err != nil {
		t.Fatalf("Checkout(main) error: %v", err)
	}

	got, err := CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if got != "main" {
		t.Errorf("after Checkout(main), CurrentBranch() = %q, want %q", got, "main")
	}
}

func TestRebase(t *testing.T) {
	dir, ctx := initRepo(t)

	// Helper for creating a file and committing.
	commitFile := func(filename, content, msg string) {
		t.Helper()
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", filename)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add: %s\n%s", err, out)
		}
		cmd = exec.Command("git", "commit", "-m", msg)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %s\n%s", err, out)
		}
	}

	t.Run("clean rebase", func(t *testing.T) {
		// Create a feature branch off main.
		err := CreateBranch(ctx, "feature", "main")
		if err != nil {
			t.Fatalf("CreateBranch: %v", err)
		}
		commitFile("feature.txt", "feature content\n", "add feature file")

		// Go back to main and add a commit.
		err = Checkout(ctx, "main")
		if err != nil {
			t.Fatalf("Checkout: %v", err)
		}
		commitFile("main.txt", "main content\n", "add main file")

		// Rebase feature onto main (no conflicts since different files).
		err = Rebase(ctx, "main", "feature")
		if err != nil {
			t.Fatalf("Rebase() error: %v", err)
		}

		// Verify we're on the feature branch after rebase.
		got, err := CurrentBranch(ctx)
		if err != nil {
			t.Fatalf("CurrentBranch() error: %v", err)
		}
		if got != "feature" {
			t.Errorf("after Rebase, CurrentBranch() = %q, want %q", got, "feature")
		}
	})
}

func TestRebaseConflict(t *testing.T) {
	dir, ctx := initRepo(t)

	// Helper for creating a file and committing.
	commitFile := func(filename, content, msg string) {
		t.Helper()
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", filename)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add: %s\n%s", err, out)
		}
		cmd = exec.Command("git", "commit", "-m", msg)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %s\n%s", err, out)
		}
	}

	// Both branches modify the same file to create a conflict.
	commitFile("shared.txt", "original\n", "add shared file")

	err := CreateBranch(ctx, "conflict-branch", "main")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	commitFile("shared.txt", "conflict-branch change\n", "modify shared on conflict-branch")

	err = Checkout(ctx, "main")
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	commitFile("shared.txt", "main change\n", "modify shared on main")

	// Rebase should detect the conflict.
	err = Rebase(ctx, "main", "conflict-branch")
	if err == nil {
		t.Fatal("Rebase() expected conflict error, got nil")
	}

	var conflictErr *RebaseConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("Rebase() error type = %T, want *RebaseConflictError; error: %v", err, err)
	}
	if conflictErr.Branch != "conflict-branch" {
		t.Errorf("RebaseConflictError.Branch = %q, want %q", conflictErr.Branch, "conflict-branch")
	}
}

func TestPush(t *testing.T) {
	dir, ctx := initRepo(t)

	// Set up a bare remote.
	remoteDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = remoteDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}

	// Push should succeed.
	err := Push(ctx, "main")
	if err != nil {
		t.Fatalf("Push() error: %v", err)
	}
}

func TestFetch(t *testing.T) {
	dir, ctx := initRepo(t)

	// Set up a bare remote.
	remoteDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = remoteDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s\n%s", err, out)
	}
	cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s\n%s", err, out)
	}
	// Push main so there's something to fetch.
	cmd = exec.Command("git", "push", "origin", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push: %s\n%s", err, out)
	}

	// Fetch should succeed.
	err := Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
}

func TestGitError(t *testing.T) {
	_, ctx := initRepo(t)

	// Run a git command that will fail.
	_, err := run(ctx, "checkout", "nonexistent-branch-xyz")
	if err == nil {
		t.Fatal("expected error for checkout of nonexistent branch")
	}

	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("error type = %T, want *GitError", err)
	}
	if len(gitErr.Args) == 0 {
		t.Error("GitError.Args is empty")
	}
	if gitErr.Stderr == "" {
		t.Error("GitError.Stderr is empty")
	}
}
