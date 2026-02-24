package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nvandessel/tier/internal/dag"
	"github.com/nvandessel/tier/internal/git"
	"github.com/nvandessel/tier/internal/state"
	"github.com/spf13/cobra"
)

var trackCmd = &cobra.Command{
	Use:   "track <branch>",
	Short: "Retroactively track an existing branch",
	Args:  cobra.ExactArgs(1),
	RunE:  runTrack,
}

func init() {
	trackCmd.Flags().String("on", "", "Git parent branch (PR base) [required]")
	trackCmd.Flags().String("after", "", "Comma-separated logical dependencies")
	_ = trackCmd.MarkFlagRequired("on")
	rootCmd.AddCommand(trackCmd)
}

func runTrack(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	name := args[0]

	if err := validateBranchName(name); err != nil {
		return err
	}

	// 1. Lock state, defer unlock
	unlock, err := state.Lock(ctx)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer unlock()

	// 2. ReadOrInit state
	s, err := state.ReadOrInit(ctx)
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// 3. Validate branch exists locally
	exists, err := git.BranchExists(ctx, name)
	if err != nil {
		return fmt.Errorf("checking branch existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("branch '%s' does not exist", name)
	}

	// Check if already tracked
	if _, tracked := s.Branches[name]; tracked {
		return fmt.Errorf("branch '%s' is already tracked", name)
	}

	// 4. Validate --on branch exists (trunk or tracked)
	onFlag, _ := cmd.Flags().GetString("on")
	if onFlag != s.Trunk {
		if _, tracked := s.Branches[onFlag]; !tracked {
			// Also check if branch exists in git at all
			onExists, err := git.BranchExists(ctx, onFlag)
			if err != nil {
				return fmt.Errorf("checking parent branch: %w", err)
			}
			if !onExists {
				return fmt.Errorf("branch '%s' does not exist", onFlag)
			}
			return fmt.Errorf("'%s' is not tracked. Track it first with 'tier track'", onFlag)
		}
	}
	parent := onFlag

	// 5. Parse --after
	afterFlag, _ := cmd.Flags().GetString("after")
	var after []string
	if afterFlag != "" {
		after = strings.Split(afterFlag, ",")
	}

	// Validate --after deps exist in tier.json
	for _, dep := range after {
		if _, tracked := s.Branches[dep]; !tracked {
			return fmt.Errorf("'%s' is not tracked. Track it first with 'tier track'", dep)
		}
	}

	// 6. Cycle detection
	dagBranches := stateToDag(s.Branches)
	if cyclePath, hasCycle := dag.DetectCycle(dagBranches, name, after); hasCycle {
		return fmt.Errorf("dependency cycle: %s", strings.Join(cyclePath, " → "))
	}

	// 7. Add to state.Branches (no checkout, no git branch creation)
	if after == nil {
		after = []string{}
	}
	s.Branches[name] = state.Branch{
		Parent: parent,
		After:  after,
	}

	// 8. Write state
	if err := state.Write(ctx, s); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	// 9. Output
	if jsonOut {
		printJSON(map[string]any{
			"name":   name,
			"parent": parent,
			"after":  after,
		})
	} else {
		fmt.Printf("Tracking branch '%s' (parent: %s)\n", name, parent)
		if len(after) > 0 {
			fmt.Printf("Dependencies: %s\n", strings.Join(after, ", "))
		}
	}

	return nil
}
