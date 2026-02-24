//go:build !windows

package state

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

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
