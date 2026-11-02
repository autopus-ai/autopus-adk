package cli

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S5: every field of the JSON contract appears in the text as the contract
// table in spec.md spells it, for a project with a stopped run, a verified
// run, an approved SPEC, a reviewed draft, an installed Desktop, and an
// installed codex.
func TestStatusEvidence_JSONAndTextAgree(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code", "codex")
	f.probe = stubStatusProbe([]string{"codex"}, true, t.TempDir())
	f.writeSpec("SPEC-RUN-001", "approved")
	f.writeSpec("SPEC-VER-002", "done")
	f.writeSpec("SPEC-NEXT-003", "approved")
	draftDir := f.writeSpec("SPEC-DRAFT-004", "draft")
	f.commit("baseline")
	f.recordBaseline("SPEC-VER-002", f.completedRun("SPEC-VER-002"))
	f.stoppedRun("SPEC-RUN-001")
	fingerprint, err := specRequirementsFingerprint(draftDir)
	require.NoError(t, err)
	writeReviewReceipt(t, f, "SPEC-DRAFT-004", fingerprint, time.Now())
	f.write(".autopus/specs/SPEC-DRAFT-004/review-session.json",
		`{"schema":"spec_review_session.v1","state":"converged","attempts":[{}],"calls":[{"call_id":"c1"}]}`)

	text, data := f.runStatus(), f.statusJSON()
	has := func(fragment string) { t.Helper(); assert.Contains(t, text, fragment) }
	str := func(value any) string { return fmt.Sprint(value) }

	for _, raw := range data["specs"].([]any) {
		spec := raw.(map[string]any)
		assert.Regexp(t, regexp.QuoteMeta(str(spec["id"]))+`\s+\S+ `+str(spec["status"])+`\s+`+
			str(jsonPath(spec, "evidence", "label"))+`\s`, text)
		if execution, ok := jsonPath(spec, "evidence", "execution").(map[string]any); ok {
			attempt := "attempt " + str(execution["attempt"])
			if from, ok := execution["resumed_from"]; ok {
				attempt += ", resumed from " + str(from)
			}
			has(fmt.Sprintf("run: %s · task %s · %s · route %s · platform %s",
				execution["run_id"], execution["task_id"], attempt, execution["route"], execution["platform"]))
			result := str(execution["terminal"])
			if reason, ok := execution["stop_reason"]; ok && reason != execution["terminal"] {
				result += " (" + str(reason) + ")"
			}
			has(fmt.Sprintf("result: %s · finished %s · base HEAD %s · %s", result, execution["finished_at"],
				shortCommit(str(execution["base_head"])), execution["source"]))
			phases := execution["phases"]
			has(fmt.Sprintf("phases: %v executed · %v restored · %v failed · %v pending · %v dispatches (%v cumulative)",
				jsonPath(phases, "executed"), jsonPath(phases, "restored"), jsonPath(phases, "failed"),
				jsonPath(phases, "pending"), execution["dispatches"], execution["cumulative_dispatches"]))
			if blocker, ok := execution["blocker"]; ok {
				has("blocker: " + str(blocker))
			}
			if resume, ok := execution["resume"].(map[string]any); ok {
				require.Equal(t, true, resume["admitted"])
				has(fmt.Sprintf("resumable: yes — the engine admits it; restores %s; dispatches again none",
					joinJSONStrings(resume["restores"])))
				has("resume command: " + str(resume["command"]))
				has("admission checked: " + joinJSONStrings(resume["checked"]))
			}
		}
		if verification := jsonPath(spec, "evidence", "verification").(map[string]any); verification["state"] != "none" {
			has(fmt.Sprintf("verification: %s (run %s, base HEAD %s, recorded %s)", verification["state"],
				verification["run_id"], shortCommit(str(verification["base_head"])), verification["recorded_at"]))
		}
		if review, ok := jsonPath(spec, "evidence", "review").(map[string]any); ok {
			has(fmt.Sprintf("review: %s (gate %s, loop %s, run %s) at %s — %s", review["verdict"],
				review["gate_status"], review["loop_status"], review["run_id"], review["finished_at"], review["state"]))
			session := review["session"]
			has(fmt.Sprintf("review session: %s · %v attempts · %v calls",
				jsonPath(session, "state"), jsonPath(session, "attempts"), jsonPath(session, "calls")))
		}
	}
	counts := jsonPath(data, "summary", "evidence").(map[string]any)
	has(fmt.Sprintf("Evidence: %v verified", counts["verified"]))
	for label, count := range counts {
		if count.(float64) > 0 {
			has(fmt.Sprintf("%v %s", count, label))
		}
	}
	env := data["environment"]
	has("Directory: " + str(jsonPath(env, "dir")))
	has("AI CLIs on PATH: " + joinJSONStrings(jsonPath(env, "installed_clis")))
	has("Configured platforms: " + joinJSONStrings(jsonPath(env, "configured_platforms")))
	has(fmt.Sprintf("auto pipeline run platform: %s — %s",
		jsonPath(env, "pipeline_platform", "platform"), jsonPath(env, "pipeline_platform", "reason")))
	has(fmt.Sprintf("Desktop: %s (%s)", desktopStateText(str(jsonPath(env, "desktop", "state"))),
		jsonPath(env, "desktop", "note")))

	actions, _ := data["next_actions"].([]any)
	ids := []string{}
	for i, raw := range actions {
		action := raw.(map[string]any)
		ids = append(ids, str(action["id"]))
		assert.Equal(t, false, action["auto_run"])
		line := str(action["shell"])
		if forms, ok := action["forms"].([]any); ok {
			parts := []string{}
			for _, form := range forms {
				parts = append(parts, fmt.Sprintf("%s (%s)", jsonPath(form, "command"), joinJSONStrings(jsonPath(form, "platforms"))))
			}
			line = strings.Join(parts, " · ")
		}
		facts := fmt.Sprintf("in %s · runs in %s · provider: %s · calls: %s · writes: %s", action["cwd"],
			action["runs_in"], action["provider"], action["external_calls"], action["writes"])
		if action["optional"] == true {
			facts = "optional · " + facts
		}
		has(fmt.Sprintf("  %d. %s\n     %s\n     %s\n", i+1, line, action["purpose"], facts))
	}
	assert.Equal(t, []string{"resume", "implement", "fix", "plan", "desktop_connect"}, ids)
}

func joinJSONStrings(value any) string {
	items, _ := value.([]any)
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprint(item)
	}
	return strings.Join(parts, ", ")
}
