package cmd

import (
	"fmt"
	"strings"

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
	ctx := cmd.Context()
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

	// Check if branch already exists in git
	exists, err := git.BranchExists(ctx, name)
	if err != nil {
		return fmt.Errorf("checking branch existence: %w", err)
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

	// 5. Validate --after deps and check for cycles
	if err := validateAfterDeps(s.Branches, name, after); err != nil {
		return err
	}

	// 6. git.CreateBranch (also checks it out)
	if err := git.CreateBranch(ctx, name, parent); err != nil {
		return fmt.Errorf("creating branch: %w", err)
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
		return fmt.Errorf("writing state: %w", err)
	}

	// 9. Output
	if jsonOut {
		return printJSON(newResult{
			Name:   name,
			Parent: parent,
			After:  after,
		})
	}
	fmt.Printf("Created and checked out branch '%s' (parent: %s)\n", name, parent)
	if len(after) > 0 {
		fmt.Printf("Dependencies: %s\n", strings.Join(after, ", "))
	}

	return nil
}
