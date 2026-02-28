package dag

import (
	"strings"
	"testing"
)

// ─── DetectCycle Tests ───────────────────────────────────────────────────────

func TestDetectCycle_NoCycle(t *testing.T) {
	// A after B, B after C — adding D after A should not create a cycle
	branches := map[string]BranchInfo{
		"A": {After: []string{"B"}},
		"B": {After: []string{"C"}},
		"C": {},
	}
	path, hasCycle := DetectCycle(branches, "D", []string{"A"})
	if hasCycle {
		t.Errorf("expected no cycle, got cycle: %v", path)
	}
}

func TestDetectCycle_DirectCycle(t *testing.T) {
	// A after B — adding B after A creates A->B->A
	branches := map[string]BranchInfo{
		"A": {After: []string{"B"}},
		"B": {},
	}
	path, hasCycle := DetectCycle(branches, "B", []string{"A"})
	if !hasCycle {
		t.Fatal("expected cycle, got none")
	}
	if len(path) == 0 {
		t.Fatal("expected non-empty cycle path")
	}
	t.Logf("cycle path: %v", path)
}

func TestDetectCycle_IndirectCycle(t *testing.T) {
	// A after B, B after C — adding C after A creates A->B->C->A
	branches := map[string]BranchInfo{
		"A": {After: []string{"B"}},
		"B": {After: []string{"C"}},
		"C": {},
	}
	path, hasCycle := DetectCycle(branches, "C", []string{"A"})
	if !hasCycle {
		t.Fatal("expected cycle, got none")
	}
	if len(path) == 0 {
		t.Fatal("expected non-empty cycle path")
	}
	t.Logf("cycle path: %v", path)
}

func TestDetectCycle_SelfCycle(t *testing.T) {
	branches := map[string]BranchInfo{}
	path, hasCycle := DetectCycle(branches, "A", []string{"A"})
	if !hasCycle {
		t.Fatal("expected cycle for self-reference, got none")
	}
	t.Logf("cycle path: %v", path)
}

func TestDetectCycle_MultipleIndependentChains(t *testing.T) {
	// Two independent chains: A->B and C->D — adding E after A (no cycle)
	branches := map[string]BranchInfo{
		"A": {After: []string{"B"}},
		"B": {},
		"C": {After: []string{"D"}},
		"D": {},
	}
	path, hasCycle := DetectCycle(branches, "E", []string{"A"})
	if hasCycle {
		t.Errorf("expected no cycle with independent chains, got: %v", path)
	}
}

func TestDetectCycle_EmptyAfter(t *testing.T) {
	branches := map[string]BranchInfo{
		"A": {},
		"B": {},
	}
	path, hasCycle := DetectCycle(branches, "C", []string{})
	if hasCycle {
		t.Errorf("expected no cycle with empty after, got: %v", path)
	}
}

// ─── TopoSort Tests ─────────────────────────────────────────────────────────

func TestTopoSort_LinearChain(t *testing.T) {
	// A after B, B after C => C, B, A
	branches := map[string]BranchInfo{
		"A": {After: []string{"B"}},
		"B": {After: []string{"C"}},
		"C": {},
	}
	result, err := TopoSort(branches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"C", "B", "A"}
	if !equalSlice(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestTopoSort_Diamond(t *testing.T) {
	// D after B and C, B after A, C after A => A first, then B and C, then D
	branches := map[string]BranchInfo{
		"A": {},
		"B": {After: []string{"A"}},
		"C": {After: []string{"A"}},
		"D": {After: []string{"B", "C"}},
	}
	result, err := TopoSort(branches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A must come before B, C; B and C must come before D
	indexOf := make(map[string]int)
	for i, v := range result {
		indexOf[v] = i
	}

	if indexOf["A"] >= indexOf["B"] {
		t.Errorf("A should come before B: %v", result)
	}
	if indexOf["A"] >= indexOf["C"] {
		t.Errorf("A should come before C: %v", result)
	}
	if indexOf["B"] >= indexOf["D"] {
		t.Errorf("B should come before D: %v", result)
	}
	if indexOf["C"] >= indexOf["D"] {
		t.Errorf("C should come before D: %v", result)
	}
}

func TestTopoSort_IndependentBranches(t *testing.T) {
	branches := map[string]BranchInfo{
		"X": {},
		"Y": {},
		"Z": {},
	}
	result, err := TopoSort(branches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d", len(result))
	}
	// Should be alphabetically sorted since they're all independent
	expected := []string{"X", "Y", "Z"}
	if !equalSlice(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestTopoSort_Empty(t *testing.T) {
	result, err := TopoSort(map[string]BranchInfo{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestTopoSort_SingleBranch(t *testing.T) {
	branches := map[string]BranchInfo{
		"only": {},
	}
	result, err := TopoSort(branches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "only" {
		t.Errorf("expected [only], got %v", result)
	}
}

func TestTopoSort_CycleError(t *testing.T) {
	branches := map[string]BranchInfo{
		"A": {After: []string{"B"}},
		"B": {After: []string{"A"}},
	}
	_, err := TopoSort(branches)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestTopoSort_ExternalDepsIgnored(t *testing.T) {
	// A depends on "external" which is not in the map — should be ignored
	branches := map[string]BranchInfo{
		"A": {After: []string{"external"}},
		"B": {After: []string{"A"}},
	}
	result, err := TopoSort(branches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"A", "B"}
	if !equalSlice(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

// ─── ComputeReadiness Tests ─────────────────────────────────────────────────

func TestComputeReadiness_EmptyAfter(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature": {After: []string{}},
	}
	result := ComputeReadiness(branches)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].Ready {
		t.Error("expected ready for empty after")
	}
	if len(result[0].BlockedBy) != 0 {
		t.Errorf("expected no blocked_by, got %v", result[0].BlockedBy)
	}
}

func TestComputeReadiness_NilAfter(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature": {},
	}
	result := ComputeReadiness(branches)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].Ready {
		t.Error("expected ready for nil after")
	}
}

func TestComputeReadiness_AllDepsMerged(t *testing.T) {
	// After deps reference branches not in the map (they were merged)
	branches := map[string]BranchInfo{
		"feature": {After: []string{"merged-branch", "another-merged"}},
	}
	result := ComputeReadiness(branches)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].Ready {
		t.Error("expected ready when all deps are merged")
	}
}

func TestComputeReadiness_Blocked(t *testing.T) {
	branches := map[string]BranchInfo{
		"A": {},
		"B": {},
		"C": {After: []string{"A", "B"}},
	}
	result := ComputeReadiness(branches)

	// Find C's readiness
	var cReady ReadinessInfo
	for _, ri := range result {
		if ri.Name == "C" {
			cReady = ri
			break
		}
	}

	if cReady.Ready {
		t.Error("expected C to be blocked")
	}
	if len(cReady.BlockedBy) != 2 {
		t.Errorf("expected 2 blockers, got %d: %v", len(cReady.BlockedBy), cReady.BlockedBy)
	}
}

func TestComputeReadiness_Mixed(t *testing.T) {
	// C depends on A (in map) and "merged" (not in map)
	branches := map[string]BranchInfo{
		"A": {},
		"C": {After: []string{"A", "merged"}},
	}
	result := ComputeReadiness(branches)

	var cReady ReadinessInfo
	for _, ri := range result {
		if ri.Name == "C" {
			cReady = ri
			break
		}
	}

	if cReady.Ready {
		t.Error("expected C to be blocked (A still in map)")
	}
	if len(cReady.BlockedBy) != 1 || cReady.BlockedBy[0] != "A" {
		t.Errorf("expected blocked by [A], got %v", cReady.BlockedBy)
	}
}

func TestComputeReadiness_AllReady(t *testing.T) {
	branches := map[string]BranchInfo{
		"A": {},
		"B": {},
	}
	result := ComputeReadiness(branches)
	for _, ri := range result {
		if !ri.Ready {
			t.Errorf("expected %s to be ready", ri.Name)
		}
	}
}

// ─── RenderTree Tests ───────────────────────────────────────────────────────

func intPtr(n int) *int {
	return &n
}

func TestRenderTree_SingleBranch(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/x": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"feature/x": intPtr(42),
	}
	readiness := map[string]ReadinessInfo{
		"feature/x": {Name: "feature/x", Ready: true},
	}

	result := RenderTree("main", branches, prNumbers, readiness)
	expected := "main\n└── feature/x  #42  [ready]\n"
	if result != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
	}
}

func TestRenderTree_MultipleChildren(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/b": {Parent: "main"},
		"feature/a": {Parent: "main"},
		"feature/c": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"feature/a": intPtr(1),
		"feature/b": intPtr(2),
		"feature/c": intPtr(3),
	}
	readiness := map[string]ReadinessInfo{
		"feature/a": {Name: "feature/a", Ready: true},
		"feature/b": {Name: "feature/b", Ready: true},
		"feature/c": {Name: "feature/c", Ready: true},
	}

	result := RenderTree("main", branches, prNumbers, readiness)

	// Should be alphabetically sorted
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), result)
	}
	if !strings.Contains(lines[1], "feature/a") {
		t.Errorf("expected feature/a first, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "feature/b") {
		t.Errorf("expected feature/b second, got: %s", lines[2])
	}
	if !strings.Contains(lines[3], "feature/c") {
		t.Errorf("expected feature/c third, got: %s", lines[3])
	}

	// Verify box drawing: first two use ├──, last uses └──
	if !strings.HasPrefix(lines[1], "├── ") {
		t.Errorf("expected ├── prefix for first child, got: %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "├── ") {
		t.Errorf("expected ├── prefix for middle child, got: %s", lines[2])
	}
	if !strings.HasPrefix(lines[3], "└── ") {
		t.Errorf("expected └── prefix for last child, got: %s", lines[3])
	}
}

func TestRenderTree_DeepNesting(t *testing.T) {
	branches := map[string]BranchInfo{
		"level1": {Parent: "main"},
		"level2": {Parent: "level1"},
		"level3": {Parent: "level2"},
	}
	prNumbers := map[string]*int{
		"level1": intPtr(1),
		"level2": intPtr(2),
		"level3": intPtr(3),
	}
	readiness := map[string]ReadinessInfo{
		"level1": {Name: "level1", Ready: true},
		"level2": {Name: "level2", Ready: true},
		"level3": {Name: "level3", Ready: true},
	}

	result := RenderTree("main", branches, prNumbers, readiness)
	expected := "main\n" +
		"└── level1  #1  [ready]\n" +
		"    └── level2  #2  [ready]\n" +
		"        └── level3  #3  [ready]\n"

	if result != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
	}
}

func TestRenderTree_NotPushed(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/x": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"feature/x": nil, // no PR number
	}
	readiness := map[string]ReadinessInfo{
		"feature/x": {Name: "feature/x", Ready: true},
	}

	result := RenderTree("main", branches, prNumbers, readiness)
	if !strings.Contains(result, "(not pushed)") {
		t.Errorf("expected '(not pushed)', got:\n%s", result)
	}
}

func TestRenderTree_BlockedAnnotation(t *testing.T) {
	branches := map[string]BranchInfo{
		"pay/api-handlers": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"pay/api-handlers": intPtr(47),
	}
	readiness := map[string]ReadinessInfo{
		"pay/api-handlers": {
			Name:      "pay/api-handlers",
			Ready:     false,
			BlockedBy: []string{"pay/db-schema", "pay/stripe-client"},
		},
	}

	result := RenderTree("main", branches, prNumbers, readiness)
	if !strings.Contains(result, "[blocked: db-schema, stripe-client]") {
		t.Errorf("expected blocked annotation with short names, got:\n%s", result)
	}
}

func TestRenderTree_FullExample(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/payments":  {Parent: "main"},
		"pay/stripe-client": {Parent: "feature/payments"},
		"pay/stripe-tests":  {Parent: "pay/stripe-client"},
		"pay/db-schema":     {Parent: "feature/payments"},
		"pay/db-migrations": {Parent: "pay/db-schema"},
		"pay/api-handlers":  {Parent: "feature/payments"},
		"pay/e2e":           {Parent: "feature/payments"},
		"feature/auth":      {Parent: "main"},
		"auth/login":        {Parent: "feature/auth"},
	}
	prNumbers := map[string]*int{
		"feature/payments":  intPtr(42),
		"pay/stripe-client": intPtr(43),
		"pay/stripe-tests":  intPtr(44),
		"pay/db-schema":     intPtr(45),
		"pay/db-migrations": intPtr(46),
		"pay/api-handlers":  intPtr(47),
		"pay/e2e":           nil,
		"feature/auth":      intPtr(50),
		"auth/login":        intPtr(51),
	}
	readiness := map[string]ReadinessInfo{
		"feature/payments":  {Name: "feature/payments", Ready: true},
		"pay/stripe-client": {Name: "pay/stripe-client", Ready: true},
		"pay/stripe-tests":  {Name: "pay/stripe-tests", Ready: true},
		"pay/db-schema":     {Name: "pay/db-schema", Ready: true},
		"pay/db-migrations": {Name: "pay/db-migrations", Ready: true},
		"pay/api-handlers": {
			Name:      "pay/api-handlers",
			Ready:     false,
			BlockedBy: []string{"pay/stripe-client", "pay/db-schema"},
		},
		"pay/e2e": {
			Name:      "pay/e2e",
			Ready:     false,
			BlockedBy: []string{"pay/api-handlers", "pay/stripe-tests", "pay/db-migrations"},
		},
		"feature/auth": {Name: "feature/auth", Ready: true},
		"auth/login":   {Name: "auth/login", Ready: true},
	}

	result := RenderTree("main", branches, prNumbers, readiness)

	// Verify key structural elements
	if !strings.Contains(result, "main\n") {
		t.Error("missing trunk")
	}
	if !strings.Contains(result, "├── feature/auth") {
		t.Errorf("missing feature/auth with ├──:\n%s", result)
	}
	if !strings.Contains(result, "└── feature/payments") {
		t.Errorf("missing feature/payments with └──:\n%s", result)
	}
	if !strings.Contains(result, "[blocked: stripe-client, db-schema]") {
		t.Errorf("missing blocked annotation for api-handlers:\n%s", result)
	}
	if !strings.Contains(result, "(not pushed)") {
		t.Errorf("missing (not pushed) for e2e:\n%s", result)
	}

	t.Logf("Full tree:\n%s", result)
}

func TestRenderTree_BoxDrawing(t *testing.T) {
	// Verify that when a non-last child has children, the │ continues
	branches := map[string]BranchInfo{
		"a":       {Parent: "main"},
		"b":       {Parent: "main"},
		"a-child": {Parent: "a"},
	}
	prNumbers := map[string]*int{
		"a":       intPtr(1),
		"b":       intPtr(2),
		"a-child": intPtr(3),
	}
	readiness := map[string]ReadinessInfo{
		"a":       {Name: "a", Ready: true},
		"b":       {Name: "b", Ready: true},
		"a-child": {Name: "a-child", Ready: true},
	}

	result := RenderTree("main", branches, prNumbers, readiness)
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")

	// Line 0: main
	// Line 1: ├── a  #1  [ready]
	// Line 2: │   └── a-child  #3  [ready]
	// Line 3: └── b  #2  [ready]
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), result)
	}
	if !strings.HasPrefix(lines[2], "│   └── ") {
		t.Errorf("expected │   └── prefix for nested child, got: %q", lines[2])
	}
}

// ─── RenderJSON Tests ───────────────────────────────────────────────────────

func TestRenderJSON_AllFields(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/x": {Parent: "main", After: []string{}},
		"feature/y": {Parent: "main", After: []string{"feature/x"}},
	}
	prNumbers := map[string]*int{
		"feature/x": intPtr(42),
		"feature/y": nil,
	}

	result := RenderJSON("main", branches, prNumbers)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Results are sorted alphabetically
	x := result[0]
	y := result[1]

	if x.Name != "feature/x" {
		t.Errorf("expected feature/x, got %s", x.Name)
	}
	if x.Parent != "main" {
		t.Errorf("expected parent main, got %s", x.Parent)
	}
	if x.PR == nil || *x.PR != 42 {
		t.Errorf("expected PR 42, got %v", x.PR)
	}
	if !x.Ready {
		t.Error("expected feature/x to be ready")
	}

	if y.Name != "feature/y" {
		t.Errorf("expected feature/y, got %s", y.Name)
	}
	if y.PR != nil {
		t.Errorf("expected nil PR, got %v", y.PR)
	}
	if y.Ready {
		t.Error("expected feature/y to be blocked")
	}
	if len(y.BlockedBy) != 1 || y.BlockedBy[0] != "feature/x" {
		t.Errorf("expected blocked by [feature/x], got %v", y.BlockedBy)
	}
}

func TestRenderJSON_EmptyAfter(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/x": {Parent: "main"},
	}
	prNumbers := map[string]*int{}

	result := RenderJSON("main", branches, prNumbers)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// After should be empty slice, not nil (for clean JSON)
	if result[0].After == nil {
		t.Error("expected non-nil After slice")
	}
	if len(result[0].After) != 0 {
		t.Errorf("expected empty After, got %v", result[0].After)
	}
}

func TestRenderJSON_BlockedByComputed(t *testing.T) {
	branches := map[string]BranchInfo{
		"A": {Parent: "main"},
		"B": {Parent: "main"},
		"C": {Parent: "main", After: []string{"A", "B"}},
	}
	prNumbers := map[string]*int{
		"A": intPtr(1),
		"B": intPtr(2),
		"C": intPtr(3),
	}

	result := RenderJSON("main", branches, prNumbers)

	var cBranch JSONBranch
	for _, jb := range result {
		if jb.Name == "C" {
			cBranch = jb
			break
		}
	}

	if cBranch.Ready {
		t.Error("expected C to be blocked")
	}
	if len(cBranch.BlockedBy) != 2 {
		t.Errorf("expected 2 blockers, got %v", cBranch.BlockedBy)
	}
}

// ─── RenderStackComment Tests ───────────────────────────────────────────────

func TestRenderStackComment_SingleBranch(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/x": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"feature/x": intPtr(10),
	}
	readiness := map[string]ReadinessInfo{
		"feature/x": {Name: "feature/x", Ready: true},
	}

	result := RenderStackComment("main", branches, prNumbers, readiness, "feature/x", "https://github.com/owner/repo")

	if !strings.Contains(result, "<!-- frond-stack -->") {
		t.Error("missing frond-stack marker")
	}
	if !strings.Contains(result, "### 🌴 Frond Stack") {
		t.Error("missing header")
	}
	if !strings.Contains(result, `<a href="https://github.com/owner/repo/pull/10">#10</a>  👈`) {
		t.Errorf("missing linked+highlighted PR on feature/x:\n%s", result)
	}
	if !strings.Contains(result, "[ready]") {
		t.Error("missing [ready] annotation")
	}
	if !strings.Contains(result, "Managed by [frond]") {
		t.Error("missing footer")
	}
}

func TestRenderStackComment_MultiBranch(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/payments":  {Parent: "main"},
		"pay/stripe-client": {Parent: "feature/payments"},
		"pay/stripe-tests":  {Parent: "pay/stripe-client"},
		"pay/api-handlers":  {Parent: "feature/payments"},
	}
	prNumbers := map[string]*int{
		"feature/payments":  intPtr(10),
		"pay/stripe-client": intPtr(11),
		"pay/stripe-tests":  intPtr(12),
		"pay/api-handlers":  nil,
	}
	readiness := map[string]ReadinessInfo{
		"feature/payments":  {Name: "feature/payments", Ready: true},
		"pay/stripe-client": {Name: "pay/stripe-client", Ready: true},
		"pay/stripe-tests":  {Name: "pay/stripe-tests", Ready: true},
		"pay/api-handlers": {
			Name:      "pay/api-handlers",
			Ready:     false,
			BlockedBy: []string{"pay/stripe-client"},
		},
	}

	result := RenderStackComment("main", branches, prNumbers, readiness, "pay/stripe-client", "https://github.com/owner/repo")

	// Highlight should be on stripe-client with linked PR, not others.
	if !strings.Contains(result, `<a href="https://github.com/owner/repo/pull/11">#11</a>  👈`) {
		t.Errorf("missing linked+highlighted PR on pay/stripe-client:\n%s", result)
	}
	// Other branches should NOT have the highlight.
	if strings.Contains(result, "#10</a>  👈") {
		t.Error("feature/payments should not be highlighted")
	}
	if strings.Contains(result, "#12</a>  👈") {
		t.Error("pay/stripe-tests should not be highlighted")
	}
	// api-handlers should show (not pushed) and blocked.
	if !strings.Contains(result, "(not pushed)") {
		t.Errorf("missing (not pushed) for api-handlers:\n%s", result)
	}
	if !strings.Contains(result, "[blocked: stripe-client]") {
		t.Errorf("missing blocked annotation:\n%s", result)
	}
}

func TestRenderStackComment_NoHighlight(t *testing.T) {
	branches := map[string]BranchInfo{
		"feature/x": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"feature/x": intPtr(10),
	}
	readiness := map[string]ReadinessInfo{
		"feature/x": {Name: "feature/x", Ready: true},
	}

	result := RenderStackComment("main", branches, prNumbers, readiness, "", "https://github.com/owner/repo")

	if strings.Contains(result, "👈") {
		t.Error("no branch should be highlighted with empty highlight")
	}
}

func TestRenderMergedStackComment(t *testing.T) {
	branches := map[string]BranchInfo{
		"pay/stripe-tests": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"pay/stripe-tests": intPtr(12),
	}
	readiness := map[string]ReadinessInfo{
		"pay/stripe-tests": {Name: "pay/stripe-tests", Ready: true},
	}

	result := RenderMergedStackComment("main", branches, prNumbers, readiness, "pay/stripe-client", "https://github.com/owner/repo")

	if !strings.Contains(result, "<!-- frond-stack -->") {
		t.Error("missing frond-stack marker")
	}
	if !strings.Contains(result, "**pay/stripe-client** has been merged") {
		t.Errorf("missing merged message:\n%s", result)
	}
	if !strings.Contains(result, "Remaining stack:") {
		t.Errorf("missing remaining stack header:\n%s", result)
	}
	if !strings.Contains(result, `<a href="https://github.com/owner/repo/pull/12">#12</a>`) {
		t.Errorf("missing linked PR in remaining tree:\n%s", result)
	}
	// Merged branch should NOT have a highlight.
	if strings.Contains(result, "👈") {
		t.Error("merged comment should not have a highlight")
	}
}

func TestRenderMergedStackComment_NoRemainingBranches(t *testing.T) {
	branches := map[string]BranchInfo{}
	prNumbers := map[string]*int{}
	readiness := map[string]ReadinessInfo{}

	result := RenderMergedStackComment("main", branches, prNumbers, readiness, "last-branch", "https://github.com/owner/repo")

	if !strings.Contains(result, "**last-branch** has been merged") {
		t.Errorf("missing merged message:\n%s", result)
	}
	if strings.Contains(result, "Remaining stack:") {
		t.Error("should not show remaining stack when no branches left")
	}
}

func TestRenderStackComment_MarkerAndPreTag(t *testing.T) {
	branches := map[string]BranchInfo{
		"feat": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"feat": intPtr(1),
	}
	readiness := map[string]ReadinessInfo{
		"feat": {Name: "feat", Ready: true},
	}

	result := RenderStackComment("main", branches, prNumbers, readiness, "feat", "https://github.com/owner/repo")

	// Verify it starts with the HTML comment marker.
	if !strings.HasPrefix(result, "<!-- frond-stack -->") {
		t.Error("result should start with frond-stack marker")
	}
	// Verify <pre> wraps the tree (not code fences).
	if !strings.Contains(result, "<pre>\nmain\n") {
		t.Errorf("expected <pre> around tree:\n%s", result)
	}
	if strings.Contains(result, "```") {
		t.Errorf("should not contain code fences:\n%s", result)
	}
}

func TestRenderStackComment_EmptyRepoURL(t *testing.T) {
	branches := map[string]BranchInfo{
		"feat": {Parent: "main"},
	}
	prNumbers := map[string]*int{
		"feat": intPtr(1),
	}
	readiness := map[string]ReadinessInfo{
		"feat": {Name: "feat", Ready: true},
	}

	result := RenderStackComment("main", branches, prNumbers, readiness, "feat", "")

	// With empty repoURL, PR numbers should be plain text (no <a> links).
	if strings.Contains(result, "<a href=") {
		t.Errorf("should not contain links with empty repoURL:\n%s", result)
	}
	if !strings.Contains(result, "feat  #1") {
		t.Errorf("missing plain PR number:\n%s", result)
	}
	// Should still use <pre> tags.
	if !strings.Contains(result, "<pre>") {
		t.Errorf("missing <pre> tag:\n%s", result)
	}
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
