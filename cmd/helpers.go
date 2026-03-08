package cmd

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/nvandessel/frond/internal/dag"
	"github.com/nvandessel/frond/internal/driver"
	"github.com/nvandessel/frond/internal/state"
)

// driverOverride is nil in production; tests set it to inject a mock driver.
var driverOverride driver.Driver

// resolveDriver returns the active driver. If driverOverride is set (tests),
// it is returned directly. Otherwise the driver is resolved from state.
func resolveDriver(st *state.State) (driver.Driver, error) {
	if driverOverride != nil {
		return driverOverride, nil
	}
	return driver.Resolve(st.Driver)
}

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

// validateAfterDeps checks that all --after dependencies exist in state and that
// adding the branch would not create a dependency cycle.
func validateAfterDeps(branches map[string]state.Branch, name string, after []string) error {
	for _, dep := range after {
		if _, tracked := branches[dep]; !tracked {
			return fmt.Errorf("'%s' is not tracked. Track it first with 'frond track'", dep)
		}
	}
	dagBranches := stateToDag(branches)
	if cyclePath, hasCycle := dag.DetectCycle(dagBranches, name, after); hasCycle {
		return fmt.Errorf("dependency cycle: %s", strings.Join(cyclePath, " → "))
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
