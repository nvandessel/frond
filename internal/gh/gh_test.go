package gh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeGHBin is the path to the pre-built fake gh binary, built once in TestMain.
var fakeGHBin string

func TestMain(m *testing.M) {
	// Find module root.
	dir, err := os.Getwd()
	if err != nil {
		panic("gh_test: " + err.Error())
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("gh_test: could not find go.mod")
		}
		dir = parent
	}

	// Build the fake gh binary once.
	tmpDir, err := os.MkdirTemp("", "fakegh-*")
	if err != nil {
		panic("gh_test: " + err.Error())
	}
	binName := "gh"
	if runtime.GOOS == "windows" {
		binName = "gh.exe"
	}
	fakeGHBin = filepath.Join(tmpDir, binName)

	cmd := exec.Command("go", "build", "-o", fakeGHBin, "./internal/gh/testdata/fakegh")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("gh_test: building fakegh: %s\n%s", err, out))
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// installFakeGH copies the pre-built binary into a temp dir and returns that dir.
func installFakeGH(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	binName := "gh"
	if runtime.GOOS == "windows" {
		binName = "gh.exe"
	}
	dst := filepath.Join(dir, binName)

	if err := os.Link(fakeGHBin, dst); err != nil {
		data, err := os.ReadFile(fakeGHBin)
		if err != nil {
			t.Fatalf("reading fakegh: %v", err)
		}
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			t.Fatalf("writing fakegh: %v", err)
		}
	}

	return dir
}

// setupFakeGH installs the fake gh binary and prepends it to PATH.
// Returns the path to the record file where invocations are logged.
func setupFakeGH(t *testing.T) (recordFile string) {
	t.Helper()

	ghDir := installFakeGH(t)
	recordFile = filepath.Join(ghDir, "gh_calls.log")

	t.Setenv("FAKEGH_RECORD", recordFile)
	t.Setenv("FAKEGH_FAIL", "")
	t.Setenv("PATH", ghDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return recordFile
}

// setupFailingGH installs the fake gh in fail mode.
func setupFailingGH(t *testing.T) string {
	t.Helper()

	ghDir := installFakeGH(t)
	recordFile := filepath.Join(ghDir, "gh_calls.log")

	t.Setenv("FAKEGH_RECORD", recordFile)
	t.Setenv("FAKEGH_FAIL", "1")
	t.Setenv("PATH", ghDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return recordFile
}

func readRecord(t *testing.T, recordFile string) []string {
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

func TestAvailable(t *testing.T) {
	_ = setupFakeGH(t)
	if err := Available(); err != nil {
		t.Fatalf("Available() returned error: %v", err)
	}
}

func TestAvailable_NotFound(t *testing.T) {
	// Set PATH to empty so gh cannot be found
	t.Setenv("PATH", t.TempDir())
	err := Available()
	if err == nil {
		t.Fatal("Available() should return error when gh not on PATH")
	}
	if !strings.Contains(err.Error(), "gh CLI is required") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestPRCreate(t *testing.T) {
	recordFile := setupFakeGH(t)
	ctx := context.Background()

	num, err := PRCreate(ctx, PRCreateOpts{
		Base: "main", Head: "feature/foo", Title: "My PR", Body: "Some body",
	})
	if err != nil {
		t.Fatalf("PRCreate() error: %v", err)
	}
	if num != 42 {
		t.Fatalf("PRCreate() = %d, want 42", num)
	}

	calls := readRecord(t, recordFile)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %v", len(calls), calls)
	}
	call := calls[0]
	if !strings.Contains(call, "pr create") {
		t.Fatalf("expected 'pr create' in call, got: %s", call)
	}
	if !strings.Contains(call, "--base main") {
		t.Fatalf("expected '--base main' in call, got: %s", call)
	}
	if !strings.Contains(call, "--head feature/foo") {
		t.Fatalf("expected '--head feature/foo' in call, got: %s", call)
	}
	if strings.Contains(call, "--draft") {
		t.Fatalf("--draft should not be present when draft=false, got: %s", call)
	}
}

func TestPRCreate_Draft(t *testing.T) {
	recordFile := setupFakeGH(t)
	ctx := context.Background()

	num, err := PRCreate(ctx, PRCreateOpts{
		Base: "main", Head: "feature/bar", Title: "Draft PR", Body: "WIP", Draft: true,
	})
	if err != nil {
		t.Fatalf("PRCreate() error: %v", err)
	}
	if num != 42 {
		t.Fatalf("PRCreate() = %d, want 42", num)
	}

	calls := readRecord(t, recordFile)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if !strings.Contains(calls[0], "--draft") {
		t.Fatalf("expected '--draft' in call, got: %s", calls[0])
	}
}

func TestPRView(t *testing.T) {
	_ = setupFakeGH(t)
	ctx := context.Background()

	info, err := PRView(ctx, 42)
	if err != nil {
		t.Fatalf("PRView() error: %v", err)
	}
	if info.Number != 42 {
		t.Fatalf("PRView().Number = %d, want 42", info.Number)
	}
	if info.State != "OPEN" {
		t.Fatalf("PRView().State = %q, want OPEN", info.State)
	}
	if info.BaseRefName != "main" {
		t.Fatalf("PRView().BaseRefName = %q, want main", info.BaseRefName)
	}
}

func TestPREdit(t *testing.T) {
	recordFile := setupFakeGH(t)
	ctx := context.Background()

	err := PREdit(ctx, 42, "develop")
	if err != nil {
		t.Fatalf("PREdit() error: %v", err)
	}

	calls := readRecord(t, recordFile)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	call := calls[0]
	if !strings.Contains(call, "pr edit") {
		t.Fatalf("expected 'pr edit' in call, got: %s", call)
	}
	if !strings.Contains(call, "42") {
		t.Fatalf("expected '42' in call, got: %s", call)
	}
	if !strings.Contains(call, "--base develop") {
		t.Fatalf("expected '--base develop' in call, got: %s", call)
	}
}

func TestPRState(t *testing.T) {
	_ = setupFakeGH(t)
	ctx := context.Background()

	state, err := PRState(ctx, 42)
	if err != nil {
		t.Fatalf("PRState() error: %v", err)
	}
	if state != "OPEN" {
		t.Fatalf("PRState() = %q, want OPEN", state)
	}
}

func TestPRCreate_Error(t *testing.T) {
	_ = setupFailingGH(t)
	ctx := context.Background()

	_, err := PRCreate(ctx, PRCreateOpts{
		Base: "main", Head: "feature/fail", Title: "Fail", Body: "body",
	})
	if err == nil {
		t.Fatal("PRCreate() should return error when gh fails")
	}

	var ghErr *GHError
	if !errors.As(err, &ghErr) {
		t.Fatalf("expected *GHError, got %T: %v", err, err)
	}
	if !strings.Contains(ghErr.Stderr, "something went wrong") {
		t.Fatalf("expected stderr in GHError, got: %q", ghErr.Stderr)
	}
}

func TestPRView_Error(t *testing.T) {
	_ = setupFailingGH(t)
	ctx := context.Background()

	_, err := PRView(ctx, 999)
	if err == nil {
		t.Fatal("PRView() should return error when gh fails")
	}
	var ghErr2 *GHError
	if !errors.As(err, &ghErr2) {
		t.Fatalf("expected *GHError, got %T", err)
	}
}

func TestPREdit_Error(t *testing.T) {
	_ = setupFailingGH(t)
	ctx := context.Background()

	err := PREdit(ctx, 999, "main")
	if err == nil {
		t.Fatal("PREdit() should return error when gh fails")
	}
	var ghErr3 *GHError
	if !errors.As(err, &ghErr3) {
		t.Fatalf("expected *GHError, got %T", err)
	}
}

func TestGHError_Error(t *testing.T) {
	e := &GHError{
		Args:   []string{"pr", "view", "42"},
		Stderr: "not found\n",
		Err:    &exec.ExitError{},
	}
	msg := e.Error()
	if !strings.Contains(msg, "gh pr view 42") {
		t.Fatalf("expected args in error message, got: %s", msg)
	}
	if !strings.Contains(msg, "not found") {
		t.Fatalf("expected stderr in error message, got: %s", msg)
	}
	if strings.Contains(msg, "\n") {
		t.Fatalf("error message should not contain newlines, got: %q", msg)
	}
}

func TestGHError_Unwrap(t *testing.T) {
	inner := &exec.ExitError{}
	e := &GHError{Err: inner}
	if e.Unwrap() != inner {
		t.Fatal("Unwrap() should return the inner error")
	}
}
