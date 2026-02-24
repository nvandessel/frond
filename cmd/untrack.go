package cmd

import (
	"fmt"

	"github.com/nvandessel/frond/internal/git"
	"github.com/nvandessel/frond/internal/state"
	"github.com/spf13/cobra"
)

var untrackCmd = &cobra.Command{
	Use:   "untrack [<branch>]",
	Short: "Remove a branch from tracking",
	Example: `  # Untrack the current branch
  frond untrack

  # Untrack a specific branch
  frond untrack my-feature`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUntrack,
}

func init() {
	rootCmd.AddCommand(untrackCmd)
}

func runUntrack(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// 1. Lock state, defer unlock
	unlock, err := state.Lock(ctx)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer unlock()

	// 2. Read state (not ReadOrInit — if no state, error)
	s, err := state.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// 3. Resolve branch: arg or current branch
	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		current, err := git.CurrentBranch(ctx)
		if err != nil {
			return fmt.Errorf("getting current branch: %w", err)
		}
		name = current
	}

	// 4. Must be tracked
	branch, tracked := s.Branches[name]
	if !tracked {
		return fmt.Errorf("branch '%s' is not tracked", name)
	}

	removedParent := branch.Parent

	// 5. Remove from state.Branches
	delete(s.Branches, name)

	// 6. Remove from ALL other branches' after lists
	// 7. Reparent children: any branch whose parent was this branch -> set parent to this branch's parent
	var reparented []string
	var unblocked []string

	for bName, b := range s.Branches {
		// Remove from after lists
		newAfter := make([]string, 0, len(b.After))
		wasInAfter := false
		for _, dep := range b.After {
			if dep == name {
				wasInAfter = true
			} else {
				newAfter = append(newAfter, dep)
			}
		}
		if wasInAfter {
			unblocked = append(unblocked, bName)
			b.After = newAfter
		}

		// Reparent children
		if b.Parent == name {
			b.Parent = removedParent
			reparented = append(reparented, bName)
		}

		s.Branches[bName] = b
	}

	// 8. Write state
	if err := state.Write(ctx, s); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	// 9. Output
	if jsonOut {
		if reparented == nil {
			reparented = []string{}
		}
		if unblocked == nil {
			unblocked = []string{}
		}
		return printJSON(untrackResult{
			Name:       name,
			Reparented: reparented,
			Unblocked:  unblocked,
		})
	}
	fmt.Printf("Untracked branch '%s'\n", name)
	if len(reparented) > 0 {
		for _, child := range reparented {
			fmt.Printf("  Reparented '%s' to '%s'\n", child, removedParent)
		}
	}
	if len(unblocked) > 0 {
		for _, dep := range unblocked {
			fmt.Printf("  Removed '%s' from '%s' dependencies\n", name, dep)
		}
	}

	return nil
}
