package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nvandessel/frond/internal/dag"
	"github.com/nvandessel/frond/internal/gh"
	"github.com/nvandessel/frond/internal/git"
	"github.com/nvandessel/frond/internal/state"
	"github.com/spf13/cobra"
)

// syncResult collects all actions performed during sync for JSON output.
type syncResult struct {
	Merged     []string            `json:"merged"`
	Reparented map[string]string   `json:"reparented"`
	Rebased    []string            `json:"rebased"`
	Unblocked  []string            `json:"unblocked"`
	Blocked    map[string][]string `json:"blocked"`
	Conflicts  []string            `json:"conflicts"`
}

// syncAction represents a single line of human-readable output.
type syncAction struct {
	symbol  string
	message string
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch, detect merged branches, clean up dependencies, rebase unblocked branches",
	Example: `  # Sync all tracked branches
  frond sync

  # Sync with JSON output
  frond sync --json`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Step 1: Lock state, defer unlock.
	unlock, err := state.Lock(ctx)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer unlock()

	// Step 2: Read state.
	st, err := state.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// Edge case: no tracked branches.
	if len(st.Branches) == 0 {
		if jsonOut {
			return printJSON(newEmptySyncResult())
		}
		fmt.Println("nothing to sync")
		return nil
	}

	// Step 3: Fetch from origin.
	if err := git.Fetch(ctx); err != nil {
		return fmt.Errorf("fetching: %w", err)
	}

	// Save current branch before any operations so we can restore it.
	originalBranch, err := git.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	result := newEmptySyncResult()
	var actions []syncAction

	// Step 4: Detect merged branches.
	var mergedBranches []string
	mergedData := make(map[string]state.Branch) // preserve data before deletion
	for name, b := range st.Branches {
		if b.PR == nil {
			continue
		}
		info, err := gh.PRView(ctx, *b.PR)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not check PR #%d for %s: %v\n", *b.PR, name, err)
			continue
		}
		if info.State == gh.PRStateMerged {
			mergedBranches = append(mergedBranches, name)
			mergedData[name] = b
		}
	}

	// Step 5: Process merged branches.
	// reparentedFrom tracks what the old parent was for each reparented child.
	reparentedFrom := make(map[string]string)

	for _, merged := range mergedBranches {
		mergedBranch := mergedData[merged]
		mergedParent := mergedBranch.Parent

		result.Merged = append(result.Merged, merged)
		actions = append(actions, syncAction{
			symbol:  "\u2713",
			message: fmt.Sprintf("%s merged \u2192 removed", merged),
		})

		// 5a: Reparent children whose parent was the merged branch.
		for childName, childBranch := range st.Branches {
			if childBranch.Parent == merged {
				childBranch.Parent = mergedParent
				st.Branches[childName] = childBranch
				result.Reparented[childName] = mergedParent
				reparentedFrom[childName] = merged

				// 5b: Update child PRs to point to new parent.
				if childBranch.PR != nil {
					if err := gh.PREdit(ctx, *childBranch.PR, mergedParent); err != nil {
						fmt.Fprintf(os.Stderr, "warning: could not retarget PR #%d for %s: %v\n", *childBranch.PR, childName, err)
					}
				}
			}
		}

		// 5c: Clean after lists — remove merged branch from ALL branches' after arrays.
		for name, b := range st.Branches {
			b.After = removeFromSlice(b.After, merged)
			st.Branches[name] = b
		}

		// 5d: Remove merged branch from state.
		delete(st.Branches, merged)
	}

	// Write state BEFORE rebasing so that if rebase fails, state is still consistent.
	if err := state.Write(ctx, st); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	// Step 6: Rebase remaining branches in topological order.
	dagBranches := stateToDag(st.Branches)

	topoOrder, err := dag.TopoSort(dagBranches)
	if err != nil {
		return fmt.Errorf("computing topological order: %w", err)
	}

	readiness := dag.ComputeReadiness(dagBranches)
	readinessMap := make(map[string]dag.ReadinessInfo, len(readiness))
	for _, ri := range readiness {
		readinessMap[ri.Name] = ri
	}

	// Determine which branches became unblocked due to merged branch removal.
	// A branch is "unblocked" if it is now ready AND was reparented from a merged branch.
	unblockedSet := make(map[string]bool)
	for name := range result.Reparented {
		if ri, ok := readinessMap[name]; ok && ri.Ready {
			unblockedSet[name] = true
		}
	}

	var conflictBranch string
	for _, name := range topoOrder {
		ri := readinessMap[name]
		if ri.Ready {
			parent := st.Branches[name].Parent
			if err := git.Rebase(ctx, parent, name); err != nil {
				var conflictErr *git.RebaseConflictError
				if errors.As(err, &conflictErr) {
					conflictBranch = name
					result.Conflicts = append(result.Conflicts, name)
					break
				}
				return fmt.Errorf("rebasing %s: %w", name, err)
			}
			result.Rebased = append(result.Rebased, name)

			if unblockedSet[name] {
				result.Unblocked = append(result.Unblocked, name)
				oldParent := reparentedFrom[name]
				actions = append(actions, syncAction{
					symbol:  "\u2191",
					message: fmt.Sprintf("%s now unblocked [was blocked: %s]", name, oldParent),
				})
			} else if oldParent, reparented := reparentedFrom[name]; reparented {
				actions = append(actions, syncAction{
					symbol:  "\u2191",
					message: fmt.Sprintf("%s rebased onto %s (was: %s)", name, parent, oldParent),
				})
			} else {
				actions = append(actions, syncAction{
					symbol:  "\u2191",
					message: fmt.Sprintf("%s rebased onto %s", name, parent),
				})
			}
		} else {
			result.Blocked[name] = ri.BlockedBy
			actions = append(actions, syncAction{
				symbol:  "\u25cf",
				message: fmt.Sprintf("%s still blocked by: %s", name, strings.Join(ri.BlockedBy, ", ")),
			})
		}
	}

	// Restore original branch after rebasing.
	if len(result.Rebased) > 0 || conflictBranch != "" {
		if err := git.Checkout(ctx, originalBranch); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not restore branch %s: %v\n", originalBranch, err)
		}
	}

	// Edge case: nothing happened at all.
	if len(mergedBranches) == 0 && len(result.Rebased) == 0 && len(result.Blocked) == 0 && conflictBranch == "" {
		if jsonOut {
			return printJSON(result)
		}
		fmt.Println("already up to date")
		return nil
	}

	// Step 8: Print summary.
	if jsonOut {
		if err := printJSON(result); err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}
	} else {
		fmt.Println("Synced:")
		for _, a := range actions {
			fmt.Printf("  %s %s\n", a.symbol, a.message)
		}
	}

	// If there was a conflict, print conflict message and exit with code 2.
	if conflictBranch != "" {
		if !jsonOut {
			fmt.Fprintf(os.Stderr, "conflict: %s \u2014 resolve and run 'frond sync' again\n", conflictBranch)
		}
		return &ExitError{Code: 2}
	}

	return nil
}

// removeFromSlice returns a new slice with all occurrences of val removed.
// Returns nil if the result would be empty.
func removeFromSlice(s []string, val string) []string {
	var result []string
	for _, v := range s {
		if v != val {
			result = append(result, v)
		}
	}
	return result
}

// newEmptySyncResult returns a syncResult with initialized maps and slices
// so JSON output always has arrays/objects instead of nulls.
func newEmptySyncResult() *syncResult {
	return &syncResult{
		Merged:     []string{},
		Reparented: make(map[string]string),
		Rebased:    []string{},
		Unblocked:  []string{},
		Blocked:    make(map[string][]string),
		Conflicts:  []string{},
	}
}
