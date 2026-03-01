package cmd

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/nvandessel/frond/internal/driver"
	"github.com/nvandessel/frond/internal/state"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push current branch and create/update its GitHub PR",
	Example: `  # Push and create/update PR with auto-generated title
  frond push

  # Push with a custom title and as draft
  frond push -t "Add user auth" --draft

  # Push with JSON output for scripting
  frond push --json`,
	RunE: runPush,
}

func init() {
	pushCmd.Flags().StringP("title", "t", "", "PR title (default: branch name humanized)")
	pushCmd.Flags().StringP("body", "b", "", "PR body")
	pushCmd.Flags().Bool("draft", false, "Create as draft PR")
	rootCmd.AddCommand(pushCmd)
}

// humanizeTitle converts a branch name into a human-readable title.
// "pay/stripe-client" becomes "Pay Stripe Client".
func humanizeTitle(branch string) string {
	s := strings.NewReplacer("/", " ", "-", " ").Replace(branch)
	words := strings.Fields(s)
	for i, w := range words {
		runes := []rune(w)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
		}
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

func runPush(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// 1. Lock state, defer unlock.
	unlock, err := state.Lock(ctx)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer unlock()

	// 2. Read state (not ReadOrInit).
	st, err := state.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// 3. Resolve driver.
	drv, err := resolveDriver(st)
	if err != nil {
		return err
	}

	// 4. Get current branch.
	branch, err := drv.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// 5. Current branch must be tracked.
	br, ok := st.Branches[branch]
	if !ok {
		return fmt.Errorf("current branch '%s' is not tracked", branch)
	}

	// 6. Build push opts.
	title, _ := cmd.Flags().GetString("title")
	if title == "" {
		title = humanizeTitle(branch)
	}
	body, _ := cmd.Flags().GetString("body")
	draft, _ := cmd.Flags().GetBool("draft")

	opts := driver.PushOpts{
		Branch:     branch,
		Base:       br.Parent,
		Title:      title,
		Body:       body,
		Draft:      draft,
		ExistingPR: br.PR,
	}

	// 7. Push (creates or updates PR).
	result, err := drv.Push(ctx, opts)
	if err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	// 8. Write PR number to state if created.
	if result.Created {
		br.PR = &result.PRNumber
		st.Branches[branch] = br
		if err := state.Write(ctx, st); err != nil {
			return fmt.Errorf("writing state: %w", err)
		}
	}

	// 9. Update stack comments on all PRs (skip for drivers that manage their own).
	if drv.SupportsStackComments() {
		updateStackComments(ctx, st)
	}

	// 10. Check for unmet --after deps: warn if any are still tracked.
	if len(br.After) > 0 {
		var unmet []string
		for _, dep := range br.After {
			if _, tracked := st.Branches[dep]; tracked {
				unmet = append(unmet, dep)
			}
		}
		if len(unmet) > 0 {
			fmt.Fprintf(os.Stderr, "warning: unmet dependencies: %s\n", strings.Join(unmet, ", "))
		}
	}

	// 11. Output.
	if jsonOut {
		return printJSON(pushResult{
			Branch:  branch,
			PR:      result.PRNumber,
			Created: result.Created,
		})
	}
	action := "updated"
	if result.Created {
		action = "created"
	}
	fmt.Printf("Pushed %s. PR #%d [%s]\n", branch, result.PRNumber, action)

	return nil
}
