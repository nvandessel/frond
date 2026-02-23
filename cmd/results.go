package cmd

import "github.com/nvandessel/tier/internal/dag"

// Typed result structs for JSON output. Each command that emits JSON uses
// a named struct here instead of map[string]any for compile-time safety.

// newResult is the JSON output of "tier new".
type newResult struct {
	Name   string   `json:"name"`
	Parent string   `json:"parent"`
	After  []string `json:"after"`
}

// trackResult is the JSON output of "tier track".
type trackResult struct {
	Name   string   `json:"name"`
	Parent string   `json:"parent"`
	After  []string `json:"after"`
}

// pushResult is the JSON output of "tier push".
type pushResult struct {
	Branch  string `json:"branch"`
	PR      int    `json:"pr"`
	Created bool   `json:"created"`
}

// untrackResult is the JSON output of "tier untrack".
type untrackResult struct {
	Name       string   `json:"name"`
	Reparented []string `json:"reparented"`
	Unblocked  []string `json:"unblocked"`
}

// statusJSONResult is the JSON output of "tier status" (without --fetch PR states).
type statusJSONResult struct {
	Trunk    string           `json:"trunk"`
	Branches []dag.JSONBranch `json:"branches"`
}

// statusFetchResult is the JSON output of "tier status --fetch" with PR states.
type statusFetchResult struct {
	Trunk    string         `json:"trunk"`
	Branches []statusBranch `json:"branches"`
}
