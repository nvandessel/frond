package cmd

import (
	"github.com/nvandessel/tier/internal/dag"
	"github.com/nvandessel/tier/internal/state"
)

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
