//go:build windows

package state

import (
	"os"
	"strconv"
	"strings"
)

// lockPIDAlive reads the PID from a lockfile and checks if that process
// is still running. On Windows, os.FindProcess always succeeds, so we
// attempt to open the process handle to verify liveness.
func lockPIDAlive(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, Signal(os.Signal(nil)) is not supported.
	// FindProcess succeeds even for dead PIDs, but we can attempt
	// Signal(nil) which returns os.ErrProcessDone for dead processes
	// on Go 1.20+.
	err = p.Signal(nil)
	return err == nil
}
