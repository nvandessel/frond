package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nvandessel/frond/internal/dag"
	"github.com/nvandessel/frond/internal/gh"
	"github.com/nvandessel/frond/internal/state"
)

const commentMarker = "<!-- frond-stack -->"

// countPRs returns how many branches have a non-nil PR number.
func countPRs(branches map[string]state.Branch) int {
	n := 0
	for _, b := range branches {
		if b.PR != nil {
			n++
		}
	}
	return n
}

// updateStackComments posts or updates a frond stack comment on every PR in
// the tracked state. Each comment shows the full dependency tree with the
// current PR's branch highlighted. Skips when fewer than 2 PRs exist (a
// "stack" comment on a single PR is noise). Errors are logged as warnings
// and do not cause the calling command to fail.
func updateStackComments(ctx context.Context, st *state.State) {
	if countPRs(st.Branches) < 2 {
		return
	}

	dagBranches := stateToDag(st.Branches)
	readinessSlice := dag.ComputeReadiness(dagBranches)
	readinessMap := make(map[string]dag.ReadinessInfo, len(readinessSlice))
	for _, ri := range readinessSlice {
		readinessMap[ri.Name] = ri
	}

	prNumbers := make(map[string]*int, len(st.Branches))
	for name, b := range st.Branches {
		prNumbers[name] = b.PR
	}

	for name, b := range st.Branches {
		if b.PR == nil {
			continue
		}

		body := dag.RenderStackComment(st.Trunk, dagBranches, prNumbers, readinessMap, name)
		if err := upsertComment(ctx, *b.PR, body); err != nil {
			fmt.Fprintf(os.Stderr, "warning: stack comment on PR #%d: %v\n", *b.PR, err)
		}
	}
}

// updateMergedComments posts a final stack comment on each merged PR showing
// it as merged and displaying the remaining stack. Called from sync after
// merges are processed but before rebasing.
func updateMergedComments(ctx context.Context, st *state.State, mergedData map[string]state.Branch) {
	dagBranches := stateToDag(st.Branches)
	readinessSlice := dag.ComputeReadiness(dagBranches)
	readinessMap := make(map[string]dag.ReadinessInfo, len(readinessSlice))
	for _, ri := range readinessSlice {
		readinessMap[ri.Name] = ri
	}

	prNumbers := make(map[string]*int, len(st.Branches))
	for name, b := range st.Branches {
		prNumbers[name] = b.PR
	}

	for name, b := range mergedData {
		if b.PR == nil {
			continue
		}
		body := dag.RenderMergedStackComment(st.Trunk, dagBranches, prNumbers, readinessMap, name)
		if err := upsertComment(ctx, *b.PR, body); err != nil {
			fmt.Fprintf(os.Stderr, "warning: merged stack comment on PR #%d: %v\n", *b.PR, err)
		}
	}
}

// upsertComment finds an existing frond-stack comment on a PR and updates it,
// or creates a new one if none exists.
func upsertComment(ctx context.Context, prNumber int, body string) error {
	comments, err := gh.PRCommentList(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("listing comments: %w", err)
	}

	for _, c := range comments {
		if strings.Contains(c.Body, commentMarker) {
			return gh.PRCommentUpdate(ctx, c.ID, body)
		}
	}

	return gh.PRCommentCreate(ctx, prNumber, body)
}
