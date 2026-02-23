// Package state manages tier.json — the single state file that tracks all
// branch metadata for the tier CLI. The state file lives at
// <git-common-dir>/tier.json so it is shared across worktrees.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nvandessel/tier/internal/git"
)

// Branch holds metadata for a single tracked branch.
type Branch struct {
	Parent string   `json:"parent"`
	After  []string `json:"after"`
	PR     *int     `json:"pr"`
}

// State is the top-level structure persisted to tier.json.
type State struct {
	Version  int               `json:"version"`
	Trunk    string            `json:"trunk"`
	Branches map[string]Branch `json:"branches"`
}

// ErrNotInitialized is returned by Read when tier.json does not exist.
var ErrNotInitialized = errors.New("no tier state found; run 'tier new' or 'tier track' first")

const (
	stateFile = "tier.json"
	lockFile  = "tier.json.lock"
	tmpFile   = "tier.json.tmp"

	lockStaleDuration = 5 * time.Minute
	stateVersion      = 1
)

// gitCommonDir is a package-level variable so tests can override it.
var gitCommonDir = func(ctx context.Context) (string, error) {
	dir, err := git.CommonDir(ctx)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}
	return abs, nil
}

// Path returns the absolute path to tier.json.
func Path(ctx context.Context) (string, error) {
	dir, err := gitCommonDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFile), nil
}

// Read parses tier.json and returns the state. If the file does not exist,
// it returns ErrNotInitialized.
func Read(ctx context.Context) (*State, error) {
	p, err := Path(ctx)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotInitialized
		}
		return nil, fmt.Errorf("reading %s: %w", p, err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}
	return &s, nil
}

// Write atomically persists state to tier.json. It writes to a temporary
// file first, then renames it into place so readers never see partial data.
func Write(ctx context.Context, s *State) error {
	p, err := Path(ctx)
	if err != nil {
		return err
	}

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling state: %w", err)
	}
	data = append(data, '\n')

	tmp := filepath.Join(dir, tmpFile)
	if err := rejectSymlink(tmp); err != nil {
		return err
	}
	if err := rejectSymlink(p); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp file %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, p); err != nil {
		// Best-effort cleanup of the temp file on rename failure.
		os.Remove(tmp)
		return fmt.Errorf("renaming %s to %s: %w", tmp, p, err)
	}

	return nil
}

// Lock acquires an exclusive lockfile (tier.json.lock) to serialise
// concurrent access from multiple worktrees. It returns an unlock function
// that removes the lockfile. If a lockfile older than 5 minutes exists it
// is treated as stale, removed, and the lock is retried once.
//
// Usage:
//
//	unlock, err := state.Lock(ctx)
//	if err != nil { ... }
//	defer unlock()
func Lock(ctx context.Context) (unlock func(), err error) {
	dir, err := gitCommonDir(ctx)
	if err != nil {
		return noop, err
	}

	lockPath := filepath.Join(dir, lockFile)

	acquired, err := tryLock(lockPath)
	if err != nil {
		return noop, err
	}
	if !acquired {
		// Check for staleness: lock is stale if mtime exceeds threshold
		// OR if the PID recorded in the lockfile is no longer running.
		info, statErr := os.Stat(lockPath)
		if statErr != nil {
			return noop, fmt.Errorf("stat lockfile %s: %w", lockPath, statErr)
		}
		stale := time.Since(info.ModTime()) > lockStaleDuration || !lockPIDAlive(lockPath)
		if stale {
			// Stale lock — remove and retry once.
			if removeErr := os.Remove(lockPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return noop, fmt.Errorf("removing stale lockfile %s: %w", lockPath, removeErr)
			}
			acquired, err = tryLock(lockPath)
			if err != nil {
				return noop, err
			}
			if !acquired {
				return noop, fmt.Errorf("failed to acquire lock after removing stale lockfile %s", lockPath)
			}
		} else {
			return noop, fmt.Errorf("lockfile %s is held by another process", lockPath)
		}
	}

	return func() {
		os.Remove(lockPath)
	}, nil
}

// tryLock attempts to create the lockfile exclusively. Returns true if
// the lock was acquired. It writes the current PID for stale detection.
func tryLock(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("creating lockfile %s: %w", path, err)
	}
	// Write PID so stale lock detection can check process liveness.
	fmt.Fprintf(f, "%d\n", os.Getpid())
	if err := f.Close(); err != nil {
		// Close failed — lock may not be durable. Clean up and report.
		os.Remove(path)
		return false, fmt.Errorf("closing lockfile %s: %w", path, err)
	}
	return true, nil
}

func noop() {}

// lockPIDAlive reads the PID from a lockfile and checks if that process
// is still running. Returns false if the PID cannot be read or the process
// is not alive.
func lockPIDAlive(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	// Signal 0 checks process existence without sending a real signal.
	return syscall.Kill(pid, 0) == nil
}

// rejectSymlink returns an error if the given path is a symlink.
// This is a defense-in-depth measure to prevent symlink attacks.
func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // path doesn't exist yet, that's fine
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink — refusing to write", path)
	}
	return nil
}

// ReadOrInit reads existing state from tier.json. If no state file exists,
// it creates an initial state with auto-detected trunk and writes it out.
func ReadOrInit(ctx context.Context) (*State, error) {
	s, err := Read(ctx)
	if err != nil && !errors.Is(err, ErrNotInitialized) {
		return nil, err
	}
	if s != nil {
		return s, nil
	}

	trunk, err := detectTrunk(ctx)
	if err != nil {
		return nil, fmt.Errorf("detecting trunk branch: %w", err)
	}

	s = &State{
		Version:  stateVersion,
		Trunk:    trunk,
		Branches: make(map[string]Branch),
	}

	if err := Write(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// detectTrunk determines the trunk branch name. It checks for "main" first,
// then "master", defaulting to "main" if neither exists.
func detectTrunk(ctx context.Context) (string, error) {
	for _, name := range []string{"main", "master"} {
		exists, err := git.BranchExists(ctx, name)
		if err != nil {
			return "", fmt.Errorf("checking branch %s: %w", name, err)
		}
		if exists {
			return name, nil
		}
	}
	return "main", nil
}
