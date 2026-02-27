// Package dag handles dependency graph operations on branch state.
// It works with branch data structures but does NOT import the state package,
// keeping it pure and testable.
package dag

import (
	"fmt"
	"slices"
	"strings"
)

// BranchInfo represents the metadata for computing DAG operations.
type BranchInfo struct {
	Parent string
	After  []string
}

// ReadinessInfo is the computed status for a branch.
type ReadinessInfo struct {
	Name      string   `json:"name"`
	Ready     bool     `json:"ready"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// JSONBranch is the structured data for JSON output.
type JSONBranch struct {
	Name      string   `json:"name"`
	Parent    string   `json:"parent"`
	After     []string `json:"after"`
	PR        *int     `json:"pr"`
	Ready     bool     `json:"ready"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// DetectCycle checks if adding a new branch with the given after dependencies
// would create a cycle in the dependency graph. Returns the cycle path and true
// if a cycle exists. Uses DFS on the "after" edge graph.
func DetectCycle(branches map[string]BranchInfo, newName string, newAfter []string) ([]string, bool) {
	// Build adjacency list: branch -> its after dependencies
	// An edge from A to B means "A depends on B" (A is after B).
	// A cycle means A -> B -> ... -> A through after edges.
	adj := make(map[string][]string)
	for name, info := range branches {
		if len(info.After) > 0 {
			adj[name] = info.After
		}
	}
	adj[newName] = newAfter

	// DFS cycle detection with coloring:
	// white (0) = unvisited, gray (1) = in current path, black (2) = finished
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int)
	parent := make(map[string]string)

	var cyclePath []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = gray
		for _, dep := range adj[node] {
			if color[dep] == gray {
				// Found a cycle. Reconstruct the path.
				cyclePath = []string{dep}
				cur := node
				for cur != dep {
					cyclePath = append(cyclePath, cur)
					cur = parent[cur]
				}
				cyclePath = append(cyclePath, dep)
				// Reverse to get: dep -> ... -> node -> dep
				for i, j := 0, len(cyclePath)-1; i < j; i, j = i+1, j-1 {
					cyclePath[i], cyclePath[j] = cyclePath[j], cyclePath[i]
				}
				return true
			}
			if color[dep] == white {
				parent[dep] = node
				if dfs(dep) {
					return true
				}
			}
		}
		color[node] = black
		return false
	}

	// Collect all nodes
	allNodes := make(map[string]bool)
	for name := range branches {
		allNodes[name] = true
	}
	allNodes[newName] = true
	for _, dep := range newAfter {
		allNodes[dep] = true
	}
	for _, info := range branches {
		for _, dep := range info.After {
			allNodes[dep] = true
		}
	}

	// Sort for deterministic behavior
	sorted := make([]string, 0, len(allNodes))
	for n := range allNodes {
		sorted = append(sorted, n)
	}
	slices.Sort(sorted)

	for _, node := range sorted {
		if color[node] == white {
			if dfs(node) {
				return cyclePath, true
			}
		}
	}

	return nil, false
}

// TopoSort performs a topological sort of branches based on the "after"
// dependency edges. Returns branch names in dependency order (dependencies
// first). Returns an error if a cycle is detected.
func TopoSort(branches map[string]BranchInfo) ([]string, error) {
	if len(branches) == 0 {
		return nil, nil
	}

	// Kahn's algorithm for topological sort.
	// Edge: A depends on B (A is "after" B) means B must come before A.
	inDegree := make(map[string]int)
	// dependents[B] = list of branches that depend on B
	dependents := make(map[string][]string)

	for name := range branches {
		inDegree[name] = 0
	}

	for name, info := range branches {
		for _, dep := range info.After {
			if _, exists := branches[dep]; exists {
				inDegree[name]++
				dependents[dep] = append(dependents[dep], name)
			}
		}
	}

	// Start with nodes that have no in-edges (no unresolved deps)
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	slices.Sort(queue)

	var result []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		deps := dependents[node]
		slices.Sort(deps)
		for _, dep := range deps {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(result) != len(branches) {
		return nil, fmt.Errorf("cycle detected in dependency graph")
	}

	return result, nil
}

// ComputeReadiness computes whether each branch is ready or blocked.
// A branch is "ready" when its after list is empty OR all branches in after
// are no longer tracked (not in the map). A branch is "blocked" when some
// after deps still exist in the map.
func ComputeReadiness(branches map[string]BranchInfo) []ReadinessInfo {
	var result []ReadinessInfo

	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		info := branches[name]
		ri := ReadinessInfo{Name: name, Ready: true}

		for _, dep := range info.After {
			if _, exists := branches[dep]; exists {
				ri.Ready = false
				ri.BlockedBy = append(ri.BlockedBy, dep)
			}
		}

		if ri.BlockedBy != nil {
			slices.Sort(ri.BlockedBy)
		}

		result = append(result, ri)
	}

	return result
}

// shortName returns the last segment of a branch name after the last '/'.
func shortName(name string) string {
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

// renderOpts controls optional rendering behavior.
type renderOpts struct {
	highlight string // branch name to mark with 👈
}

// RenderTree renders an ASCII tree showing the branch hierarchy based on
// parent relationships. Annotations include PR numbers and readiness status.
func RenderTree(trunk string, branches map[string]BranchInfo, prNumbers map[string]*int, readiness map[string]ReadinessInfo) string {
	return renderTree(trunk, branches, prNumbers, readiness, renderOpts{})
}

func renderTree(trunk string, branches map[string]BranchInfo, prNumbers map[string]*int, readiness map[string]ReadinessInfo, opts renderOpts) string {
	// Build children map from parent relationships
	children := make(map[string][]string)
	for name, info := range branches {
		children[info.Parent] = append(children[info.Parent], name)
	}

	// Sort children alphabetically
	for p := range children {
		slices.Sort(children[p])
	}

	var sb strings.Builder
	sb.WriteString(trunk)
	sb.WriteString("\n")

	renderChildren(&sb, trunk, children, prNumbers, readiness, "", opts)

	return sb.String()
}

func renderChildren(sb *strings.Builder, node string, children map[string][]string, prNumbers map[string]*int, readiness map[string]ReadinessInfo, prefix string, opts renderOpts) {
	kids := children[node]
	for i, child := range kids {
		isLast := i == len(kids)-1

		connector := "├── "
		if isLast {
			connector = "└── "
		}

		sb.WriteString(prefix)
		sb.WriteString(connector)
		sb.WriteString(child)

		// PR number
		if prNumbers != nil {
			if pr, ok := prNumbers[child]; ok && pr != nil {
				sb.WriteString(fmt.Sprintf("  #%d", *pr))
			} else {
				sb.WriteString("  (not pushed)")
			}
		}

		// Highlight marker
		if opts.highlight != "" && child == opts.highlight {
			sb.WriteString("  👈")
		}

		// Readiness
		if readiness != nil {
			if ri, ok := readiness[child]; ok {
				if ri.Ready {
					sb.WriteString("  [ready]")
				} else if len(ri.BlockedBy) > 0 {
					short := make([]string, len(ri.BlockedBy))
					for j, dep := range ri.BlockedBy {
						short[j] = shortName(dep)
					}
					sb.WriteString(fmt.Sprintf("  [blocked: %s]", strings.Join(short, ", ")))
				}
			}
		}

		sb.WriteString("\n")

		childPrefix := prefix + "│   "
		if isLast {
			childPrefix = prefix + "    "
		}
		renderChildren(sb, child, children, prNumbers, readiness, childPrefix, opts)
	}
}

// CommentMarker is the HTML comment used to identify frond stack comments
// on GitHub PRs. Used by both rendering (here) and upsert detection (cmd).
const CommentMarker = "<!-- frond-stack -->"

// RenderStackComment renders a full stack comment for a GitHub PR.
// The highlight parameter marks the current PR's branch with the pointer emoji.
// Returns a markdown string wrapped with the frond-stack marker.
func RenderStackComment(trunk string, branches map[string]BranchInfo, prNumbers map[string]*int, readiness map[string]ReadinessInfo, highlight string) string {
	tree := renderTree(trunk, branches, prNumbers, readiness, renderOpts{highlight: highlight})

	var sb strings.Builder
	sb.WriteString(CommentMarker + "\n")
	sb.WriteString("### 🌴 Frond Stack\n\n")
	sb.WriteString("```\n")
	sb.WriteString(tree)
	sb.WriteString("```\n\n")
	sb.WriteString("*Managed by [frond](https://github.com/nvandessel/frond)*\n")
	return sb.String()
}

// RenderMergedStackComment renders a final stack comment for a merged PR.
// It shows the branch as merged and displays the remaining stack tree.
func RenderMergedStackComment(trunk string, branches map[string]BranchInfo, prNumbers map[string]*int, readiness map[string]ReadinessInfo, mergedBranch string) string {
	var sb strings.Builder
	sb.WriteString(CommentMarker + "\n")
	sb.WriteString("### 🌴 Frond Stack\n\n")
	sb.WriteString(fmt.Sprintf("**%s** has been merged. :tada:\n\n", mergedBranch))

	if len(branches) > 0 {
		tree := renderTree(trunk, branches, prNumbers, readiness, renderOpts{})
		sb.WriteString("Remaining stack:\n")
		sb.WriteString("```\n")
		sb.WriteString(tree)
		sb.WriteString("```\n\n")
	}

	sb.WriteString("*Managed by [frond](https://github.com/nvandessel/frond)*\n")
	return sb.String()
}

// RenderJSON returns the structured data for JSON output.
func RenderJSON(trunk string, branches map[string]BranchInfo, prNumbers map[string]*int) []JSONBranch {
	readinessSlice := ComputeReadiness(branches)
	readinessMap := make(map[string]ReadinessInfo)
	for _, ri := range readinessSlice {
		readinessMap[ri.Name] = ri
	}

	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	slices.Sort(names)

	var result []JSONBranch
	for _, name := range names {
		info := branches[name]
		ri := readinessMap[name]

		jb := JSONBranch{
			Name:      name,
			Parent:    info.Parent,
			After:     info.After,
			Ready:     ri.Ready,
			BlockedBy: ri.BlockedBy,
		}

		if jb.After == nil {
			jb.After = []string{}
		}

		if prNumbers != nil {
			if pr, ok := prNumbers[name]; ok {
				jb.PR = pr
			}
		}

		result = append(result, jb)
	}

	return result
}
