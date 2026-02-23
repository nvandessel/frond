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

var newCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new tracked branch and check it out",
	Args:  cobra.ExactArgs(1),
	RunE:  runNew,
}

func init() {
	newCmd.Flags().String("on", "", "Git parent branch (PR base)")
	newCmd.Flags().String("after", "", "Comma-separated logical dependencies")
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	name := args[0]

	if err := validateBranchName(name); err != nil {
		return err
	}

	// 1. Lock state, defer unlock
	unlock, err := state.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	// 2. ReadOrInit state
	s, err := state.ReadOrInit(ctx)
	if err != nil {
		return err
	}

	// Check if branch already exists in git
	exists, err := git.BranchExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("branch '%s' already exists. Use 'tier track' to add it", name)
	}

	// 3. Resolve parent: --on flag -> current branch if tracked -> trunk
	onFlag, _ := cmd.Flags().GetString("on")
	parent := s.Trunk
	if onFlag != "" {
		parent = onFlag
	} else {
		current, err := git.CurrentBranch(ctx)
		if err == nil {
			if _, tracked := s.Branches[current]; tracked {
				parent = current
			}
		}
	}

	// 4. Parse --after
	afterFlag, _ := cmd.Flags().GetString("after")
	var after []string
	if afterFlag != "" {
		after = strings.Split(afterFlag, ",")
	}

	// Validate --after branches all exist in tier.json
	for _, dep := range after {
		if _, tracked := s.Branches[dep]; !tracked {
			return fmt.Errorf("'%s' is not tracked. Track it first with 'tier track'", dep)
		}
	}

	// 5. Cycle detection
	dagBranches := stateToDag(s.Branches)
	if cyclePath, hasCycle := dag.DetectCycle(dagBranches, name, after); hasCycle {
		return fmt.Errorf("dependency cycle: %s", strings.Join(cyclePath, " → "))
	}

	// 6. git.CreateBranch (also checks it out)
	if err := git.CreateBranch(ctx, name, parent); err != nil {
		return err
	}

	// 7. Write branch to state.Branches
	if after == nil {
		after = []string{}
	}
	s.Branches[name] = state.Branch{
		Parent: parent,
		After:  after,
	}

	// 8. Write state
	if err := state.Write(ctx, s); err != nil {
		return err
	}

	// 9. Output
	if jsonOut {
		printJSON(map[string]any{
			"name":   name,
			"parent": parent,
			"after":  after,
		})
	} else {
		fmt.Printf("Created and checked out branch '%s' (parent: %s)\n", name, parent)
		if len(after) > 0 {
			fmt.Printf("Dependencies: %s\n", strings.Join(after, ", "))
		}
	}

	return nil
}
