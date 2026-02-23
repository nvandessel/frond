package gh

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

// setupFakeGH creates a fake gh script in a temp directory and prepends it
// to PATH so that exec.LookPath and exec.Command find it first.
// The fake script records all invocations to a record file and returns
// canned JSON responses based on the first two arguments.
func setupFakeGH(t *testing.T) (recordFile string) {
	t.Helper()

	dir := t.TempDir()
	recordFile = filepath.Join(dir, "gh_calls.log")

	script := filepath.Join(dir, "gh")
	content := fmt.Sprintf(`#!/bin/bash
echo "$@" >> "%s"
# Return canned responses based on subcommand
if [[ "$1" == "--version" ]]; then
    echo "gh version 2.50.0 (2024-05-01)"
    exit 0
fi
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
exit 0
`, recordFile)
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	// Prepend the fake gh directory to PATH
	origPATH := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPATH)

	return recordFile
}

// setupFailingGH creates a fake gh that always exits non-zero.
func setupFailingGH(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	recordFile := filepath.Join(dir, "gh_calls.log")

	script := filepath.Join(dir, "gh")
	content := fmt.Sprintf(`#!/bin/bash
echo "$@" >> "%s"
echo "fatal: something went wrong" >&2
exit 1
`, recordFile)
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	origPATH := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPATH)

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

	num, err := PRCreate(ctx, "main", "feature/foo", "My PR", "Some body", false)
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

	num, err := PRCreate(ctx, "main", "feature/bar", "Draft PR", "WIP", true)
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

	_, err := PRCreate(ctx, "main", "feature/fail", "Fail", "body", false)
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
