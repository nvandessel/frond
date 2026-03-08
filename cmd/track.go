package cmd

import (
	"fmt"
	"strings"

	"github.com/nvandessel/frond/internal/state"
	"github.com/spf13/cobra"
)

var trackCmd = &cobra.Command{
	Use:   "track <branch>",
	Short: "Retroactively track an existing branch",
	Example: `  # Track an existing branch with its parent
  frond track my-feature --on main

  # Track with a dependency
  frond track step-2 --on step-1 --after step-1`,
	Args: cobra.ExactArgs(1),
	RunE: runTrack,
}

func init() {
	trackCmd.Flags().String("on", "", "Git parent branch (PR base) [required]")
	trackCmd.Flags().String("after", "", "Comma-separated logical dependencies")
	_ = trackCmd.MarkFlagRequired("on")
	rootCmd.AddCommand(trackCmd)
}

func runTrack(cmd *cobra.Command, args []string) error {
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

	// 3. Resolve driver
	drv, err := resolveDriver(s)
	if err != nil {
		return err
	}

	// 4. Validate branch exists locally
	exists, err := drv.BranchExists(ctx, name)
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

	// 5. Validate --on branch exists (trunk or tracked)
	onFlag, _ := cmd.Flags().GetString("on")
	if onFlag != s.Trunk {
		if _, tracked := s.Branches[onFlag]; !tracked {
			// Also check if branch exists in git at all
			onExists, err := drv.BranchExists(ctx, onFlag)
			if err != nil {
				return fmt.Errorf("checking parent branch: %w", err)
			}
			if !onExists {
				return fmt.Errorf("branch '%s' does not exist", onFlag)
			}
			return fmt.Errorf("'%s' is not tracked. Track it first with 'frond track'", onFlag)
		}
	}
	parent := onFlag

	// 6. Parse --after
	afterFlag, _ := cmd.Flags().GetString("after")
	var after []string
	if afterFlag != "" {
		after = strings.Split(afterFlag, ",")
	}

	// 7. Validate --after deps and check for cycles
	if err := validateAfterDeps(s.Branches, name, after); err != nil {
		return err
	}

	// 8. Add to state.Branches (no checkout, no git branch creation)
	if after == nil {
		after = []string{}
	}
	s.Branches[name] = state.Branch{
		Parent: parent,
		After:  after,
	}

	// 9. Write state
	if err := state.Write(ctx, s); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	// 10. Output
	if jsonOut {
		return printJSON(trackResult{
			Name:   name,
			Parent: parent,
			After:  after,
		})
	}
	fmt.Printf("Tracking branch '%s' (parent: %s)\n", name, parent)
	if len(after) > 0 {
		fmt.Printf("Dependencies: %s\n", strings.Join(after, ", "))
	}

	return nil
}
