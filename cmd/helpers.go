package cmd

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/nvandessel/tier/internal/dag"
	"github.com/nvandessel/tier/internal/state"
)

// validateBranchName checks that a branch name is safe to use with git commands.
func validateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("branch name cannot be empty")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("branch name %q cannot start with '-'", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("branch name %q cannot contain '..'", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("branch name %q contains control characters", name)
		}
	}
	return nil
}

// stateToDag converts state.Branch map to dag.BranchInfo map for use with dag functions.
func stateToDag(branches map[string]state.Branch) map[string]dag.BranchInfo {
	result := make(map[string]dag.BranchInfo, len(branches))
	for name, b := range branches {
		result[name] = dag.BranchInfo{
			Parent: b.Parent,
			After:  b.After,
		}
	}
	return result
}
