package driver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBinDir holds the directory containing the built fakegt binary.
// Set by TestMain before any tests run.
var fakeBinDir string

func TestMain(m *testing.M) {
	// Build fakegt and prepend its directory to PATH so that
	// exec.LookPath("gt") and runGT find our test double.
	dir, err := os.MkdirTemp("", "fakegt-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	gtBin := filepath.Join(dir, "gt")
	cmd := exec.Command("go", "build", "-o", gtBin, "./testdata/fakegt")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakegt: %v\n", err)
		os.Exit(1)
	}

	fakeBinDir = dir
	os.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	os.Exit(m.Run())
}

// initGitRepo creates a temp git repo with an initial commit, chdir's into it,
// and restores the original directory on cleanup. This is needed because the
// git package operates on the current working directory.
func initGitRepo(t *testing.T) (dir string, ctx context.Context) {
	t.Helper()
	dir = t.TempDir()
	ctx = context.Background()

	gitEnv := []string{
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME=" + dir,
	}

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

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	for _, e := range gitEnv {
		parts := strings.SplitN(e, "=", 2)
		t.Setenv(parts[0], parts[1])
	}

	return dir, ctx
}

// --- Unit tests for parseSubmitResult ---

func TestParseSubmitResult(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		branch      string
		wantPR      int
		wantCreated bool
		wantErr     string
	}{
		{
			name:        "created on .com domain",
			output:      "my-feature: https://app.graphite.com/github/pr/owner/repo/42 (created)",
			branch:      "my-feature",
			wantPR:      42,
			wantCreated: true,
		},
		{
			name:        "updated on .dev domain",
			output:      "my-feature: https://app.graphite.dev/github/pr/owner/repo/99 (updated)",
			branch:      "my-feature",
			wantPR:      99,
			wantCreated: false,
		},
		{
			name: "multi-branch stack matches correct branch",
			output: `pp--06-14-part_1: https://app.graphite.com/github/pr/withgraphite/repo/100 (created)
pp--06-14-part_2: https://app.graphite.com/github/pr/withgraphite/repo/101 (created)
pp--06-14-part_3: https://app.graphite.com/github/pr/withgraphite/repo/102 (created)`,
			branch:      "pp--06-14-part_2",
			wantPR:      101,
			wantCreated: true,
		},
		{
			name:    "branch not found",
			output:  "other-branch: https://app.graphite.com/github/pr/owner/repo/42 (created)",
			branch:  "my-feature",
			wantErr: `branch "my-feature" not found in gt submit output`,
		},
		{
			name:    "malformed URL no trailing number",
			output:  "my-feature: https://app.graphite.com/github/pr/owner/repo/ (created)",
			branch:  "my-feature",
			wantErr: "malformed PR URL",
		},
		{
			name:    "empty output",
			output:  "",
			branch:  "my-feature",
			wantErr: `branch "my-feature" not found in gt submit output`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPR, gotCreated, err := parseSubmitResult(tt.output, tt.branch)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotPR != tt.wantPR {
				t.Errorf("prNumber = %d, want %d", gotPR, tt.wantPR)
			}
			if gotCreated != tt.wantCreated {
				t.Errorf("created = %v, want %v", gotCreated, tt.wantCreated)
			}
		})
	}
}

// --- Integration tests using fakegt + temp git repos ---

func TestNewGraphiteWithFakeGT(t *testing.T) {
	// fakegt is on PATH via TestMain, so NewGraphite should succeed.
	g, err := NewGraphite()
	if err != nil {
		t.Fatalf("NewGraphite() with fakegt on PATH: %v", err)
	}
	if g.Name() != "graphite" {
		t.Errorf("Name() = %q, want %q", g.Name(), "graphite")
	}
}

func TestGraphiteCreateBranch(t *testing.T) {
	_, ctx := initGitRepo(t)

	g := &Graphite{}

	// CreateBranch checks out the parent (real git) then calls gt create (fakegt).
	if err := g.CreateBranch(ctx, "my-feature", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
}

func TestGraphitePush(t *testing.T) {
	tests := []struct {
		name        string
		submitOut   string
		branch      string
		opts        PushOpts
		wantPR      int
		wantCreated bool
		wantErr     string
	}{
		{
			name:      "new PR created",
			submitOut: "feat-a: https://app.graphite.com/github/pr/owner/repo/77 (created)",
			branch:    "feat-a",
			opts: PushOpts{
				Branch: "feat-a",
				Base:   "main",
				Title:  "Add feature A",
			},
			wantPR:      77,
			wantCreated: true,
		},
		{
			name:      "existing PR updated by gt",
			submitOut: "feat-b: https://app.graphite.com/github/pr/owner/repo/88 (updated)",
			branch:    "feat-b",
			opts: PushOpts{
				Branch: "feat-b",
				Base:   "main",
			},
			wantPR:      88,
			wantCreated: false,
		},
		{
			name:      "existing PR via ExistingPR field",
			submitOut: "feat-c: https://app.graphite.com/github/pr/owner/repo/99 (updated)",
			branch:    "feat-c",
			opts: PushOpts{
				Branch:     "feat-c",
				Base:       "main",
				ExistingPR: intPtr(55),
			},
			wantPR:      55,
			wantCreated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FAKEGT_SUBMIT_OUTPUT", tt.submitOut)
			ctx := context.Background()
			g := &Graphite{}

			result, err := g.Push(ctx, tt.opts)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Push: %v", err)
			}
			if result.PRNumber != tt.wantPR {
				t.Errorf("PRNumber = %d, want %d", result.PRNumber, tt.wantPR)
			}
			if result.Created != tt.wantCreated {
				t.Errorf("Created = %v, want %v", result.Created, tt.wantCreated)
			}
		})
	}
}

func TestGraphitePushPassesBodyAndTitle(t *testing.T) {
	// Verify that --title and --description flags are passed to gt submit.
	recordFile := filepath.Join(t.TempDir(), "record.txt")
	t.Setenv("FAKEGT_RECORD", recordFile)
	t.Setenv("FAKEGT_SUBMIT_OUTPUT", "my-feat: https://app.graphite.com/github/pr/o/r/1 (created)")

	ctx := context.Background()
	g := &Graphite{}

	_, err := g.Push(ctx, PushOpts{
		Branch: "my-feat",
		Base:   "main",
		Title:  "My title",
		Body:   "My description",
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	recorded, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("reading record file: %v", err)
	}
	args := string(recorded)

	if !strings.Contains(args, "--title My title") {
		t.Errorf("expected --title in args, got: %s", args)
	}
	if !strings.Contains(args, "--description My description") {
		t.Errorf("expected --description in args, got: %s", args)
	}
}

func TestGraphitePushDraft(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "record.txt")
	t.Setenv("FAKEGT_RECORD", recordFile)
	t.Setenv("FAKEGT_SUBMIT_OUTPUT", "my-feat: https://app.graphite.com/github/pr/o/r/1 (created)")

	ctx := context.Background()
	g := &Graphite{}

	_, err := g.Push(ctx, PushOpts{
		Branch: "my-feat",
		Base:   "main",
		Title:  "Draft PR",
		Draft:  true,
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	recorded, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("reading record file: %v", err)
	}
	if !strings.Contains(string(recorded), "--draft") {
		t.Errorf("expected --draft in args, got: %s", recorded)
	}
}

func TestGraphitePushFailure(t *testing.T) {
	t.Setenv("FAKEGT_FAIL", "1")
	ctx := context.Background()
	g := &Graphite{}

	_, err := g.Push(ctx, PushOpts{Branch: "feat", Base: "main"})
	if err == nil {
		t.Fatal("expected error when gt submit fails")
	}
	if !strings.Contains(err.Error(), "gt submit") {
		t.Errorf("error = %q, want containing 'gt submit'", err.Error())
	}
}

func TestGraphiteRebase(t *testing.T) {
	ctx := context.Background()
	g := &Graphite{}

	// Normal restack succeeds.
	if err := g.Rebase(ctx, "main", "feature"); err != nil {
		t.Fatalf("Rebase: %v", err)
	}
}

func TestGraphiteRebaseConflict(t *testing.T) {
	t.Setenv("FAKEGT_CONFLICT", "1")
	ctx := context.Background()
	g := &Graphite{}

	err := g.Rebase(ctx, "main", "feature")
	if err == nil {
		t.Fatal("expected error on conflict")
	}

	var conflictErr *RebaseConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected RebaseConflictError, got %T: %v", err, err)
	}
	if !strings.Contains(conflictErr.Detail, "CONFLICT") {
		t.Errorf("Detail = %q, want containing 'CONFLICT'", conflictErr.Detail)
	}
}

func intPtr(n int) *int { return &n }
