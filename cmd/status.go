package cmd

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/nvandessel/tier/internal/dag"
	"github.com/nvandessel/tier/internal/gh"
	"github.com/nvandessel/tier/internal/state"
	"github.com/spf13/cobra"
)

// statusBranch wraps dag.JSONBranch with an optional PR state field
// for --fetch output.
type statusBranch struct {
	dag.JSONBranch
	PRState string `json:"pr_state,omitempty"`
}

var fetchFlag bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the dependency graph with readiness indicators",
	Long:  "Display the branch dependency tree with PR numbers, readiness status, and optionally live PR states from GitHub.",
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&fetchFlag, "fetch", false, "Fetch live PR states from GitHub (slower)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// 1. Read state (do NOT create state if missing).
	s, err := state.Read(ctx)
	if err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("no tier state found. Run 'tier new' or 'tier track' first")
	}

	// 2. Convert state.Branch -> dag.BranchInfo for all branches.
	branches := stateToDag(s.Branches)

	// 3. Build prNumbers map from state branches' PR fields.
	prNumbers := make(map[string]*int, len(s.Branches))
	for name, b := range s.Branches {
		prNumbers[name] = b.PR
	}

	// 4. Compute readiness.
	readinessSlice := dag.ComputeReadiness(branches)
	readinessMap := make(map[string]dag.ReadinessInfo, len(readinessSlice))
	for _, ri := range readinessSlice {
		readinessMap[ri.Name] = ri
	}

	// 5. If --fetch, get live PR states from GitHub.
	prStates := make(map[string]string)
	if fetchFlag {
		prStates = fetchPRStates(ctx, prNumbers)
	}

	// 6. Output.
	if jsonOut {
		return outputJSON(s.Trunk, branches, prNumbers, prStates)
	}
	return outputHuman(s.Trunk, branches, prNumbers, readinessMap, prStates)
}

// fetchPRStates calls gh.PRView for each branch that has a PR number.
// On individual failures it warns to stderr and continues.
func fetchPRStates(ctx context.Context, prNumbers map[string]*int) map[string]string {
	states := make(map[string]string)
	for name, pr := range prNumbers {
		if pr == nil {
			continue
		}
		info, err := gh.PRView(ctx, *pr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to fetch PR #%d for %s: %v\n", *pr, name, err)
			continue
		}
		states[name] = info.State
	}
	return states
}

// outputJSON renders JSON output using dag.RenderJSON, optionally enriched
// with PR state information from --fetch.
func outputJSON(trunk string, branches map[string]dag.BranchInfo, prNumbers map[string]*int, prStates map[string]string) error {
	jsonBranches := dag.RenderJSON(trunk, branches, prNumbers)

	if len(prStates) > 0 {
		// Wrap with statusBranch to include pr_state.
		wrapped := make([]statusBranch, len(jsonBranches))
		for i, jb := range jsonBranches {
			wrapped[i] = statusBranch{
				JSONBranch: jb,
				PRState:    prStates[jb.Name],
			}
		}
		printJSON(struct {
			Trunk    string         `json:"trunk"`
			Branches []statusBranch `json:"branches"`
		}{
			Trunk:    trunk,
			Branches: wrapped,
		})
	} else {
		printJSON(struct {
			Trunk    string           `json:"trunk"`
			Branches []dag.JSONBranch `json:"branches"`
		}{
			Trunk:    trunk,
			Branches: jsonBranches,
		})
	}

	return nil
}

// outputHuman renders the ASCII tree and optionally a PR states section.
func outputHuman(trunk string, branches map[string]dag.BranchInfo, prNumbers map[string]*int, readiness map[string]dag.ReadinessInfo, prStates map[string]string) error {
	tree := dag.RenderTree(trunk, branches, prNumbers, readiness)
	fmt.Print(tree)

	if len(prStates) > 0 {
		fmt.Println()
		fmt.Println("PR states:")

		// Collect and sort by branch name for deterministic output.
		type prEntry struct {
			name   string
			number int
			state  string
		}
		var entries []prEntry
		for name, st := range prStates {
			if pr, ok := prNumbers[name]; ok && pr != nil {
				entries = append(entries, prEntry{name: name, number: *pr, state: st})
			}
		}
		slices.SortFunc(entries, func(a, b prEntry) int {
			return cmp.Compare(a.name, b.name)
		})
		for _, e := range entries {
			fmt.Printf("  #%d %s — %s\n", e.number, e.name, e.state)
		}
	}

	return nil
}
