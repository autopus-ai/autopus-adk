package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S1: a SPEC whose document says done, with no run behind it, is counted as
// documented, never as verified.
func TestStatusEvidence_DocumentedDoneWithoutReceiptIsNotVerified(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code")
	f.writeSpec("SPEC-DONE-001", "done")
	f.commit("baseline")

	text := f.runStatus()
	assert.Regexp(t, `SPEC-DONE-001\s+✓ done\s+documented_only\s`, text)
	assert.Contains(t, text, "전체: 1/1 완료")
	assert.Contains(t, text, "Evidence: 0 verified · 1 documented_only")

	data := f.statusJSON()
	spec := jsonSpec(t, data, "SPEC-DONE-001")
	assert.Equal(t, "done", spec["status"])
	assert.Equal(t, "documented_only", jsonPath(spec, "evidence", "label"))
	assert.Equal(t, "none", jsonPath(spec, "evidence", "verification", "state"))
	assert.Nil(t, jsonPath(spec, "evidence", "execution"))
	assert.EqualValues(t, 0, jsonPath(data, "summary", "verified"))
	assert.EqualValues(t, 1, jsonPath(data, "summary", "evidence", "documented_only"))
}

// S3, S6: a stopped run reports its attempt, stop reason, and counts with
// restored phases apart from executed ones, and is resumable until HEAD moves.
func TestStatusEvidence_FailedRunIsResumableUntilHEADMoves(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code")
	specDir := f.writeSpec("SPEC-RUN-001", "approved")
	head := f.commit("baseline")
	f.saveCheckpoint("SPEC-RUN-001", stoppedRunCheckpoint("SPEC-RUN-001", specDir, head), false)

	text := f.runStatus()
	assert.Regexp(t, `SPEC-RUN-001\s+→ approved\s+failed\s`, text)
	assert.Contains(t, text, "run: run-2 · attempt 2 · blocked (circuit_break_no_progress)")
	assert.Contains(t, text, "phases: 1 executed · 2 restored · 1 failed · 1 pending · 4 dispatches (7 cumulative)")
	assert.Contains(t, text, "blocker: validate: the fix resolved nothing")
	assert.Contains(t, text, "resumable: yes — auto pipeline run SPEC-RUN-001 --continue --platform codex")

	execution := jsonPath(jsonSpec(t, f.statusJSON(), "SPEC-RUN-001"), "evidence", "execution")
	assert.Equal(t, "blocked", jsonPath(execution, "terminal"))
	assert.Equal(t, "circuit_break_no_progress", jsonPath(execution, "stop_reason"))
	assert.Equal(t, "task-1", jsonPath(execution, "task_id"))
	assert.Equal(t, "run-1", jsonPath(execution, "resumed_from"))
	assert.EqualValues(t, 2, jsonPath(execution, "attempt"))
	assert.EqualValues(t, 1, jsonPath(execution, "phases", "executed"))
	assert.EqualValues(t, 2, jsonPath(execution, "phases", "restored"))
	assert.EqualValues(t, 1, jsonPath(execution, "phases", "failed"))
	assert.EqualValues(t, 1, jsonPath(execution, "phases", "pending"))
	assert.EqualValues(t, 4, jsonPath(execution, "dispatches"))
	assert.EqualValues(t, 7, jsonPath(execution, "cumulative_dispatches"))
	assert.Equal(t, true, jsonPath(execution, "resumable"))
	assert.Equal(t, "auto pipeline run SPEC-RUN-001 --continue --platform codex", jsonPath(execution, "resume_command"))

	f.write("pkg/app/app.go", "package app\n\n// moved on\n")
	f.commit("move HEAD")
	text = f.runStatus()
	assert.Contains(t, text, "resumable: no — HEAD moved since the run; --continue refuses a stale checkpoint")
	assert.NotContains(t, text, "resumable: yes")
	assert.Contains(t, text, ". auto pipeline run SPEC-RUN-001\n")
}

// S3: a blocked sidecar newer than the checkpoint means the last attempt
// stopped before dispatch.
func TestStatusEvidence_RefusedResumeIsReported(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code")
	specDir := f.writeSpec("SPEC-RUN-002", "approved")
	head := f.commit("baseline")
	f.saveCheckpoint("SPEC-RUN-002", stoppedRunCheckpoint("SPEC-RUN-002", specDir, head), false)
	past := time.Now().Add(-time.Hour)
	canonical := filepath.Join(f.root, ".autopus", "pipeline-state", "SPEC-RUN-002.yaml")
	require.NoError(t, os.Chtimes(canonical, past, past))
	refused := stoppedRunCheckpoint("SPEC-RUN-002", specDir, head)
	refused.Receipt.Blocker = "pipeline: resume blocked: checkpoint phase route mismatch: saved full, authorized compact"
	f.saveCheckpoint("SPEC-RUN-002", refused, true)

	text := f.runStatus()
	assert.Contains(t, text,
		"last attempt blocked before dispatch: pipeline: resume blocked: checkpoint phase route mismatch")
	attempt := jsonPath(jsonSpec(t, f.statusJSON(), "SPEC-RUN-002"), "evidence", "execution", "blocked_attempt")
	assert.Equal(t, ".autopus/pipeline-state/SPEC-RUN-002.blocked.yaml", jsonPath(attempt, "source"))
	assert.Contains(t, jsonPath(attempt, "blocker"), "checkpoint phase route mismatch")
}

// S4: with no SPEC, a small change is offered without one, beside planning,
// in the syntax of every configured platform.
func TestStatusNext_NoSpecOffersWorkWithoutASpec(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code", "codex")
	f.commit("baseline")

	text := f.runStatus()
	assert.Contains(t, text, "SPEC이 없습니다")
	assert.Contains(t, text, `/auto fix "<problem>" (claude-code) · $codex-auto-fix "<problem>" (codex)`)
	assert.Contains(t, text, `/auto plan "<outcome>" (claude-code) · $codex-auto-plan "<outcome>" (codex)`)
	assert.NotContains(t, text, "`/auto plan`으로 시작하세요")

	actions, _ := f.statusJSON()["next_actions"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range actions {
		action, _ := raw.(map[string]any)
		byID[action["id"].(string)] = action
		assert.Equal(t, false, action["auto_run"])
	}
	require.Contains(t, byID, "fix")
	require.Contains(t, byID, "plan")
	assert.Equal(t, "agent_session", byID["fix"]["runs_in"])
	assert.Equal(t, `/auto fix "<problem>"`, byID["fix"]["command"])
	assert.Equal(t, "$codex-auto-fix \"<problem>\"", jsonPath(byID["fix"]["forms"].([]any)[1], "command"))
}

// S12: status reads; it leaves every project file as it was.
func TestStatusEvidence_StatusWritesNothing(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code")
	specDir := f.writeSpec("SPEC-RUN-003", "approved")
	head := f.commit("baseline")
	f.saveCheckpoint("SPEC-RUN-003", stoppedRunCheckpoint("SPEC-RUN-003", specDir, head), false)
	f.write(".autopus/specs/SPEC-RUN-003/review-receipt.json",
		`{"schema":"spec_review_promotion_receipt.v1","run_id":"r","finished_at":"2026-11-01T00:00:00Z","verdict":"PASS"}`)
	f.write("pkg/app/app.go", "package app\n\n// uncommitted\n")
	before := statusTreeSnapshot(t, f.root)

	f.runStatus()
	f.runStatus("--json")

	assert.Equal(t, before, statusTreeSnapshot(t, f.root))
}

// statusTreeSnapshot records every project file outside .git with its mode,
// content digest, and modification time.
func statusTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		record := info.Mode().String() + " " + info.ModTime().UTC().Format(time.RFC3339Nano)
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			record += " " + hex.EncodeToString(sum[:])
		}
		rel, _ := filepath.Rel(root, path)
		snapshot[rel] = record
		return nil
	}))
	return snapshot
}
