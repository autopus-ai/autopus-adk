package content

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// readGeneratedTeamJS reads the generated route_team.workflow.js.tmpl from a
// template dir produced by generateWorkflowTemplates.
func readGeneratedTeamJS(t *testing.T, templateDir string) string {
	t.Helper()
	path := filepath.Join(templateDir, "claude", "workflows", "route_team.workflow.js.tmpl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated team js %q: %v", path, err)
	}
	return string(data)
}

var teamCanonicalPhases = []string{
	"gate_build_test", "implementation", "planning",
	"release_hygiene", "review", "test_scaffold", "testing",
} // sorted

// TestS1S19_TeamDeterministicGeneration verifies route_team generation from the
// real content dir is deterministic (byte-identical across runs), carries the
// generated warning, exposes the declared team phases, and emits the per-phase
// structure the dispatch layer relies on.
func TestS1S19_TeamDeterministicGeneration(t *testing.T) {
	t.Parallel()
	contentDir := repoContentDir(t)

	tmpl1 := t.TempDir()
	tmpl2 := t.TempDir()
	if err := generateWorkflowTemplates(contentDir, tmpl1); err != nil {
		t.Fatalf("generate run 1: %v", err)
	}
	if err := generateWorkflowTemplates(contentDir, tmpl2); err != nil {
		t.Fatalf("generate run 2: %v", err)
	}

	js1 := readGeneratedTeamJS(t, tmpl1)
	js2 := readGeneratedTeamJS(t, tmpl2)
	if js1 != js2 {
		t.Fatalf("generated route_team js.tmpl not byte-identical across runs")
	}

	if firstLine := strings.SplitN(js1, "\n", 2)[0]; !strings.Contains(firstLine, "GENERATED") || !strings.Contains(firstLine, "DO NOT EDIT") {
		t.Errorf("first line missing generated warning: %q", firstLine)
	}

	if !strings.Contains(js1, "explicit retained Workflow") ||
		strings.Contains(js1, "Workflow substrate (claude-code --team)") {
		t.Errorf("route_team metadata must not claim the native --team substrate")
	}

	jsIDs := extractPhaseIDsFromJS(js1)
	jsSet := make([]string, 0, len(jsIDs))
	for id := range jsIDs {
		jsSet = append(jsSet, id)
	}
	sort.Strings(jsSet)
	if !equalStrs(jsSet, teamCanonicalPhases) {
		t.Errorf("team js phase set = %v, want %v", jsSet, teamCanonicalPhases)
	}

	implBlock := phaseJSBlock(js1, "implementation")
	for _, want := range []string{"parallel(", "agent(taskPrompt", "shared working tree", "agentType: 'executor'", "fan_out_cap=5", "RT.implementation", "'claude-opus-5'"} {
		if !strings.Contains(implBlock, want) {
			t.Errorf("implementation block missing %q, got:\n%s", want, implBlock)
		}
	}

	reviewBlock := phaseJSBlock(js1, "review")
	for _, want := range []string{"agent(`Review changes for SPEC", "agent(`Perform OWASP security audit", "agent(`Synthesize review results for SPEC", "agentType: 'reviewer'", "agentType: 'security-auditor'"} {
		if !strings.Contains(reviewBlock, want) {
			t.Errorf("review block missing %q, got:\n%s", want, reviewBlock)
		}
	}
	// FIDELITY-001 F4: both the verify-vote loop call and the synthesis pass call
	// carry agentType: 'reviewer' (2 occurrences); the audit pass carries exactly
	// one security-auditor. This locks synthesis agentType consistency.
	if got := strings.Count(reviewBlock, "agentType: 'reviewer'"); got != 2 {
		t.Errorf("review block reviewer agentType count = %d, want 2 (vote loop + synthesis)", got)
	}
	if got := strings.Count(reviewBlock, "agentType: 'security-auditor'"); got != 1 {
		t.Errorf("review block security-auditor agentType count = %d, want 1", got)
	}

	gateBlock := phaseJSBlock(js1, "gate_build_test")
	if gateBlock == "" {
		t.Errorf("gate_build_test block must exist")
	}
	if !strings.Contains(gateBlock, "log(") {
		t.Errorf("gate_build_test block must emit a log() marker")
	}
	if strings.Contains(gateBlock, "agent.exec(") {
		t.Errorf("gate_build_test block must not call agent.exec (gate runs outside the JS in the Go dispatcher)")
	}

	hygieneBlock := phaseJSBlock(js1, "release_hygiene")
	if hygieneBlock == "" {
		t.Errorf("release_hygiene block must exist")
	}
	if !strings.Contains(hygieneBlock, "log(") {
		t.Errorf("release_hygiene block must emit a log() marker")
	}
	if strings.Contains(hygieneBlock, "agent.exec(") {
		t.Errorf("release_hygiene block must not call agent.exec (gate runs outside the JS in the Go dispatcher)")
	}
}

// TestS19_RouteARegressionGolden verifies that adding route_team generation does
// NOT alter the route_a generated surface: regenerating route_a from the real
// content dir reproduces the committed templates/claude/workflows/route_a.workflow.js.tmpl
// byte-for-byte.
func TestS19_RouteARegressionGolden(t *testing.T) {
	t.Parallel()
	contentDir := repoContentDir(t)
	tmpl := t.TempDir()
	if err := generateWorkflowTemplates(contentDir, tmpl); err != nil {
		t.Fatalf("generate: %v", err)
	}
	got := readGeneratedJS(t, tmpl)

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	committedPath := filepath.Join(filepath.Dir(thisFile), "..", "..",
		"templates", "claude", "workflows", "route_a.workflow.js.tmpl")
	wantBytes, err := os.ReadFile(committedPath)
	if err != nil {
		t.Fatalf("read committed route_a tmpl: %v", err)
	}
	if got != string(wantBytes) {
		t.Errorf("regenerated route_a diverges from committed golden\n--- got ---\n%s\n--- want ---\n%s", got, string(wantBytes))
	}
}
