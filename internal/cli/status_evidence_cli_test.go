package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	assert.Equal(t, true, spec["completed"], "completed keeps meaning the declared status")
	assert.Equal(t, "documented_only", jsonPath(spec, "evidence", "label"))
	assert.Equal(t, "none", jsonPath(spec, "evidence", "verification", "state"))
	assert.Nil(t, jsonPath(spec, "evidence", "execution"))
	assert.EqualValues(t, 0, jsonPath(data, "summary", "verified"))
	assert.EqualValues(t, 1, jsonPath(data, "summary", "evidence", "documented_only"))
}

// S3, S6: a stopped run made by the real engine reports its attempt, stop
// reason, and counts — a restored phase that a fix dispatched again counts as
// executed — and the engine's own admission says it can resume.
func TestStatusEvidence_FailedRunIsResumableUntilHEADMoves(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code")
	f.probe = stubStatusProbe([]string{"codex"}, false, t.TempDir())
	f.writeSpec("SPEC-RUN-001", "approved")
	head := f.commit("baseline")
	cp := f.stoppedRun("SPEC-RUN-001")
	receipt := cp.Receipt

	text := f.runStatus()
	assert.Regexp(t, `SPEC-RUN-001\s+→ approved\s+failed\s`, text)
	assert.Contains(t, text, fmt.Sprintf("run: %s · task %s · attempt 2, resumed from %s · route full · platform codex",
		receipt.RunID, receipt.TaskID, receipt.ResumedFrom))
	assert.Contains(t, text, fmt.Sprintf("result: blocked (circuit_break_no_progress) · finished %s · base HEAD %s · "+
		".autopus/pipeline-state/SPEC-RUN-001.yaml", receipt.FinishedAt.UTC().Format(time.RFC3339), head[:12]))
	assert.Contains(t, text, "phases: 1 executed · 2 restored · 1 failed · 1 pending · 3 dispatches (7 cumulative)")
	assert.Contains(t, text, "blocker: phase validate: circuit_break_no_progress")
	assert.Contains(t, text, "resumable: yes — the engine admits it; restores plan, test_scaffold, implement; dispatches again none")
	assert.Contains(t, text, "resume command: auto pipeline run SPEC-RUN-001 --continue --platform codex")
	assert.Contains(t, text, "admission checked: git HEAD, SPEC snapshot, checkpoint identity and version, "+
		"dependencies, route, output records")

	execution := jsonPath(jsonSpec(t, f.statusJSON(), "SPEC-RUN-001"), "evidence", "execution")
	for key, want := range map[string]any{
		"terminal": "blocked", "stop_reason": "circuit_break_no_progress", "task_id": receipt.TaskID,
		"resumed_from": receipt.ResumedFrom, "route": "full", "platform": "codex", "base_head": head,
		"attempt": 2.0, "dispatches": 3.0, "cumulative_dispatches": 7.0, "root": f.root,
	} {
		assert.Equal(t, want, jsonPath(execution, key), key)
	}
	for key, want := range map[string]float64{"executed": 1, "restored": 2, "failed": 1, "pending": 1} {
		assert.Equal(t, want, jsonPath(execution, "phases", key), key)
	}
	assert.Equal(t, true, jsonPath(execution, "resume", "admitted"))
	assert.Equal(t, []any{"plan", "test_scaffold", "implement"}, jsonPath(execution, "resume", "restores"))

	f.write("pkg/app/app.go", "package app\n\n// moved on\n")
	f.commit("move HEAD")
	text = f.runStatus()
	assert.Contains(t, text, "resumable: no — HEAD moved since the run; --continue refuses a stale checkpoint")
	assert.NotContains(t, text, "resumable: yes")
	assert.Contains(t, text, "cd "+f.root+" && auto pipeline run SPEC-RUN-001 --platform codex\n")
}

// S3: a blocked sidecar newer than the checkpoint means the last attempt
// stopped before dispatch.
func TestStatusEvidence_RefusedResumeIsReported(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code")
	f.writeSpec("SPEC-RUN-002", "approved")
	f.commit("baseline")
	cp := f.stoppedRun("SPEC-RUN-002")
	past := time.Now().Add(-time.Hour)
	canonical := filepath.Join(f.root, pipelineStateDir, "SPEC-RUN-002.yaml")
	require.NoError(t, os.Chtimes(canonical, past, past))
	cp.Receipt.Blocker = "pipeline: resume blocked: checkpoint phase route mismatch: saved full, authorized compact"
	f.saveCheckpoint("SPEC-RUN-002", *cp, true)

	text := f.runStatus()
	assert.Contains(t, text,
		"last attempt blocked before dispatch: pipeline: resume blocked: checkpoint phase route mismatch")
	attempt := jsonPath(jsonSpec(t, f.statusJSON(), "SPEC-RUN-002"), "evidence", "execution", "blocked_attempt")
	assert.Equal(t, ".autopus/pipeline-state/SPEC-RUN-002.blocked.yaml", jsonPath(attempt, "source"))
	assert.Contains(t, jsonPath(attempt, "blocker"), "checkpoint phase route mismatch")
}

// S4: with no SPEC, a small change is offered without one, beside planning,
// in the syntax of every configured platform, each in an absolute directory.
func TestStatusNext_NoSpecOffersWorkWithoutASpec(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code", "codex")
	f.commit("baseline")

	text := f.runStatus()
	assert.Contains(t, text, "SPEC이 없습니다")
	assert.Contains(t, text, `/auto fix "<problem>" (claude-code) · $codex-auto-fix "<problem>" (codex)`)
	assert.Contains(t, text, `/auto plan "<outcome>" (claude-code) · $codex-auto-plan "<outcome>" (codex)`)
	assert.Contains(t, text, "in "+f.root+" · runs in agent_session")
	assert.NotContains(t, text, "`/auto plan`으로 시작하세요")

	byID := statusActions(t, f.statusJSON())
	require.Contains(t, byID, "fix")
	require.Contains(t, byID, "plan")
	for id, action := range byID {
		assert.Equal(t, false, action["auto_run"], id)
		assert.Equal(t, f.root, action["cwd"], id)
	}
	assert.Equal(t, "agent_session", byID["fix"]["runs_in"])
	assert.Equal(t, `/auto fix "<problem>"`, byID["fix"]["command"])
	assert.Equal(t, "$codex-auto-fix \"<problem>\"", jsonPath(byID["fix"]["forms"].([]any)[1], "command"))
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
