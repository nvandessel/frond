package driver

import (
	"strings"
	"testing"
)

func TestParseSubmitResult(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		branch      string
		wantPR      int
		wantCreated bool
		wantErr     string
	}{
		{
			name:        "created on .com domain",
			output:      "my-feature: https://app.graphite.com/github/pr/owner/repo/42 (created)",
			branch:      "my-feature",
			wantPR:      42,
			wantCreated: true,
		},
		{
			name:        "updated on .dev domain",
			output:      "my-feature: https://app.graphite.dev/github/pr/owner/repo/99 (updated)",
			branch:      "my-feature",
			wantPR:      99,
			wantCreated: false,
		},
		{
			name: "multi-branch stack matches correct branch",
			output: `pp--06-14-part_1: https://app.graphite.com/github/pr/withgraphite/repo/100 (created)
pp--06-14-part_2: https://app.graphite.com/github/pr/withgraphite/repo/101 (created)
pp--06-14-part_3: https://app.graphite.com/github/pr/withgraphite/repo/102 (created)`,
			branch:      "pp--06-14-part_2",
			wantPR:      101,
			wantCreated: true,
		},
		{
			name:    "branch not found",
			output:  "other-branch: https://app.graphite.com/github/pr/owner/repo/42 (created)",
			branch:  "my-feature",
			wantErr: `branch "my-feature" not found in gt submit output`,
		},
		{
			name:    "malformed URL no trailing number",
			output:  "my-feature: https://app.graphite.com/github/pr/owner/repo/ (created)",
			branch:  "my-feature",
			wantErr: "malformed PR URL",
		},
		{
			name:    "empty output",
			output:  "",
			branch:  "my-feature",
			wantErr: `branch "my-feature" not found in gt submit output`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPR, gotCreated, err := parseSubmitResult(tt.output, tt.branch)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotPR != tt.wantPR {
				t.Errorf("prNumber = %d, want %d", gotPR, tt.wantPR)
			}
			if gotCreated != tt.wantCreated {
				t.Errorf("created = %v, want %v", gotCreated, tt.wantCreated)
			}
		})
	}
}
