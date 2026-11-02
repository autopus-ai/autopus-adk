package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureEvidence reads one SPEC's evidence the way status does.
func fixtureEvidence(t *testing.T, f *statusFixture, specID string) statusEvidence {
	t.Helper()
	specs := scanAllSpecs(f.root)
	reader := newStatusEvidenceReader(f.root, specs)
	for _, entry := range specs {
		if entry.id == specID {
			return reader.read(entry)
		}
	}
	require.FailNow(t, "SPEC not scanned", specID)
	return statusEvidence{}
}

// recordFixtureVerification records the baseline a completed run leaves.
func recordFixtureVerification(t *testing.T, f *statusFixture, specID, specDir, runID string) {
	t.Helper()
	cp := completedRunCheckpoint(specID, specDir, f.head(), runID)
	f.saveCheckpoint(specID, cp, false)
	baseline, err := newPipelineVerificationBaseline(f.root, specID, specDir, *cp.Receipt)
	require.NoError(t, err)
	stateDir := filepath.Join(f.root, pipelineStateDir)
	require.NoError(t, writePipelineVerification(pipelineVerificationPath(stateDir, specID), baseline))
}

// S2: verification stays fresh when the verified tree is committed or only a
// status line moves, and goes stale when code or requirements change.
func TestStatusEvidence_VerificationGoesStaleWhenCodeChanges(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-VER-001"
	f := newStatusFixture(t, "claude-code")
	specDir := f.writeSpec(specID, "approved")
	f.commit("baseline")
	f.write("pkg/app/app.go", "package app\n\n// implemented\n")
	recordFixtureVerification(t, f, specID, specDir, "run-9")
	check := func(label, state string, reasons ...string) {
		t.Helper()
		evidence := fixtureEvidence(t, f, specID)
		assert.Equal(t, label, evidence.Label)
		assert.Equal(t, state, evidence.Verification.State)
		assert.Equal(t, reasons, evidence.Verification.Reasons)
	}

	check("verified", "fresh")
	f.commit("commit the verified tree")
	check("verified", "fresh")

	f.write("pkg/app/app.go", "package app\n\n// edited after verification\n")
	check("stale", "stale", "working tree changed since verification: pkg/app/app.go")
	runGitCommand(t, f.root, "checkout", "--", "pkg/app/app.go")
	check("verified", "fresh")

	// Review promotion and sync rewrite the status line; that is no change.
	spec := f.read(".autopus/specs/" + specID + "/spec.md")
	f.write(".autopus/specs/"+specID+"/spec.md", strings.Replace(spec, "**Status**: approved", "**Status**: completed", 1))
	check("verified", "fresh")
	f.write(".autopus/specs/"+specID+"/acceptance.md", f.read(".autopus/specs/"+specID+"/acceptance.md")+"And errors are reported.\n")
	check("stale", "stale", "SPEC documents changed since verification")

	require.NoError(t, os.Remove(pipelineVerificationPath(filepath.Join(f.root, pipelineStateDir), specID)))
	check("unverified", "unknown", "no verification baseline")
	assert.Contains(t, f.runStatus(), "verification: unknown (run run-9) — no verification baseline")
}

// S5: the JSON and the text carry the same labels, counts, reasons, and next
// commands.
func TestStatusEvidence_JSONAndTextAgree(t *testing.T) {
	stubStatusProbe(t, []string{"codex"}, false, t.TempDir())
	f := newStatusFixture(t, "claude-code", "codex")
	runDir := f.writeSpec("SPEC-RUN-001", "approved")
	f.writeSpec("SPEC-DONE-001", "done")
	f.writeSpec("SPEC-NEXT-001", "approved")
	head := f.commit("baseline")
	f.saveCheckpoint("SPEC-RUN-001", stoppedRunCheckpoint("SPEC-RUN-001", runDir, head), false)

	text := f.runStatus()
	data := f.statusJSON()
	for _, id := range []string{"SPEC-RUN-001", "SPEC-DONE-001", "SPEC-NEXT-001"} {
		label := jsonPath(jsonSpec(t, data, id), "evidence", "label").(string)
		assert.Regexp(t, regexp.QuoteMeta(id)+`\s+\S+ \S+\s+`+label+`\s`, text)
	}
	execution := jsonPath(jsonSpec(t, data, "SPEC-RUN-001"), "evidence", "execution")
	assert.Contains(t, text, fmt.Sprintf("run: %v · attempt %v · %v (%v)", jsonPath(execution, "run_id"),
		jsonPath(execution, "attempt"), jsonPath(execution, "terminal"), jsonPath(execution, "stop_reason")))
	phases := jsonPath(execution, "phases")
	assert.Contains(t, text, fmt.Sprintf("phases: %v executed · %v restored · %v failed · %v pending · %v dispatches (%v cumulative)",
		jsonPath(phases, "executed"), jsonPath(phases, "restored"), jsonPath(phases, "failed"),
		jsonPath(phases, "pending"), jsonPath(execution, "dispatches"), jsonPath(execution, "cumulative_dispatches")))
	assert.Contains(t, text, fmt.Sprintf("resumable: yes — %v (%v)", jsonPath(execution, "resume_command"), jsonPath(execution, "resume_reason")))
	counts, _ := jsonPath(data, "summary", "evidence").(map[string]any)
	assert.Contains(t, text, fmt.Sprintf("Evidence: %v verified", counts["verified"]))
	for label, count := range counts {
		if count.(float64) > 0 {
			assert.Contains(t, text, fmt.Sprintf("%v %s", count, label))
		}
	}
	env := jsonPath(data, "environment")
	assert.Contains(t, text, "AI CLIs on PATH: "+joinJSONStrings(jsonPath(env, "installed_clis")))
	assert.Contains(t, text, "Configured platforms: "+joinJSONStrings(jsonPath(env, "configured_platforms")))
	assert.Contains(t, text, fmt.Sprintf("auto pipeline run platform: %v — %v",
		jsonPath(env, "pipeline_platform", "platform"), jsonPath(env, "pipeline_platform", "reason")))
	actions, _ := data["next_actions"].([]any)
	require.NotEmpty(t, actions)
	for _, raw := range actions {
		action := raw.(map[string]any)
		assert.Equal(t, false, action["auto_run"])
		forms, _ := action["forms"].([]any)
		if len(forms) == 0 {
			assert.Contains(t, text, action["command"])
		}
		for _, form := range forms {
			assert.Contains(t, text, jsonPath(form, "command"))
		}
		assert.Contains(t, text, action["purpose"])
		assert.Contains(t, text, fmt.Sprintf("runs in %v · provider: %v · calls: %v · writes: %v",
			action["runs_in"], action["provider"], action["external_calls"], action["writes"]))
	}
	assert.Equal(t, "resume", jsonPath(actions[0], "id"))
}

func joinJSONStrings(value any) string {
	items, _ := value.([]any)
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprint(item)
	}
	return strings.Join(parts, ", ")
}

// A stopped or unfinished last run outranks older verification, a dry run is
// not execution evidence, and only fresh verification yields verified.
func TestStatusEvidence_LabelPrecedence(t *testing.T) {
	t.Parallel()
	fresh, none := statusVerification{State: "fresh"}, statusVerification{State: "none"}
	cases := []struct {
		name         string
		declaredDone bool
		execution    *statusExecution
		verification statusVerification
		want         string
	}{
		{"unfinished outranks fresh", true, &statusExecution{}, fresh, "unfinished"},
		{"stopped outranks fresh", true, &statusExecution{Terminal: "failed_budget"}, fresh, "failed"},
		{"completed and fresh", false, &statusExecution{Terminal: "completed"}, fresh, "verified"},
		{"dry run is no evidence", true, &statusExecution{Terminal: "dry_run"}, none, "documented_only"},
		{"unreadable checkpoint is no evidence", false, &statusExecution{Error: "checkpoint unreadable"}, none, "none"},
		{"unknown verification", true, nil, statusVerification{State: "unknown"}, "unverified"},
		{"stale verification", true, nil, statusVerification{State: "stale"}, "stale"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, evidenceLabel(tc.declaredDone, tc.execution, tc.verification), tc.name)
	}
}
