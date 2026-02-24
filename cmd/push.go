package cmd

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/nvandessel/tier/internal/gh"
	"github.com/nvandessel/tier/internal/git"
	"github.com/nvandessel/tier/internal/state"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push current branch and create/update its GitHub PR",
	Example: `  # Push and create/update PR with auto-generated title
  tier push

  # Push with a custom title and as draft
  tier push -t "Add user auth" --draft

  # Push with JSON output for scripting
  tier push --json`,
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

	// 1. Check gh is available.
	if err := gh.Available(); err != nil {
		return fmt.Errorf("gh CLI is required. Install: https://cli.github.com")
	}

	// 2. Get current branch.
	branch, err := git.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// 3. Lock state, defer unlock.
	unlock, err := state.Lock(ctx)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer unlock()

	// 4. Read state (not ReadOrInit).
	st, err := state.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// 5. Current branch must be tracked.
	br, ok := st.Branches[branch]
	if !ok {
		return fmt.Errorf("current branch '%s' is not tracked", branch)
	}

	// 6. Push to origin.
	if err := git.Push(ctx, branch); err != nil {
		return fmt.Errorf("pushing to origin: %w", err)
	}

	created := false
	var prNumber int

	// 7. If no PR exists, create one.
	if br.PR == nil {
		title, _ := cmd.Flags().GetString("title")
		if title == "" {
			title = humanizeTitle(branch)
		}
		body, _ := cmd.Flags().GetString("body")
		draft, _ := cmd.Flags().GetBool("draft")

		prNumber, err = gh.PRCreate(ctx, gh.PRCreateOpts{
			Base:  br.Parent,
			Head:  branch,
			Title: title,
			Body:  body,
			Draft: draft,
		})
		if err != nil {
			return fmt.Errorf("creating PR: %w", err)
		}

		br.PR = &prNumber
		st.Branches[branch] = br
		if err := state.Write(ctx, st); err != nil {
			return fmt.Errorf("writing state: %w", err)
		}
		created = true
	} else {
		// 8. PR exists — check if base needs retargeting.
		prNumber = *br.PR

		info, err := gh.PRView(ctx, prNumber)
		if err != nil {
			return fmt.Errorf("viewing PR #%d: %w", prNumber, err)
		}

		if info.BaseRefName != br.Parent {
			if err := gh.PREdit(ctx, prNumber, br.Parent); err != nil {
				return fmt.Errorf("retargeting PR #%d: %w", prNumber, err)
			}
		}
	}

	// 9. Check for unmet --after deps: warn if any are still tracked.
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

	// 10. Output.
	if jsonOut {
		return printJSON(pushResult{
			Branch:  branch,
			PR:      prNumber,
			Created: created,
		})
	}
	action := "updated"
	if created {
		action = "created"
	}
	fmt.Printf("Pushed %s. PR #%d [%s]\n", branch, prNumber, action)

	return nil
}
