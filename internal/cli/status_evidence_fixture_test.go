package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// statusFixture is a git project with SPECs and pipeline state for the status
// evidence tests (SPEC-STATUSEVIDENCE-001). It only uses the status command
// and the pipeline types, so the same tests run against code without the
// evidence report.
type statusFixture struct {
	t    *testing.T
	root string
}

// newStatusFixture makes a committed project. With platforms it also writes
// an autopus.yaml naming them.
func newStatusFixture(t *testing.T, platforms ...string) *statusFixture {
	t.Helper()
	f := &statusFixture{t: t, root: t.TempDir()}
	initTestGitRepo(t, f.root)
	if len(platforms) > 0 {
		f.write("autopus.yaml", "mode: full\nproject_name: fixture\nplatforms:\n  - "+strings.Join(platforms, "\n  - ")+"\n")
	}
	f.write("pkg/app/app.go", "package app\n")
	return f
}

func (f *statusFixture) write(rel, body string) {
	f.t.Helper()
	path := filepath.Join(f.root, filepath.FromSlash(rel))
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(body), 0o644))
}

func (f *statusFixture) read(rel string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	require.NoError(f.t, err)
	return string(data)
}

// writeSpec writes a SPEC with the given declared status and returns its
// directory.
func (f *statusFixture) writeSpec(id, status string) string {
	f.t.Helper()
	base := ".autopus/specs/" + id + "/"
	f.write(base+"spec.md", "# "+id+": fixture feature\n\n**Status**: "+status+"\n\n## Requirements\n\nThe fixture works.\n")
	f.write(base+"plan.md", "# Plan\n\n## Tasks\n\n- T1: build it\n")
	f.write(base+"acceptance.md", "# Acceptance\n\n## Scenarios\n\nGiven the fixture, when it runs, then it works.\n")
	return filepath.Join(f.root, ".autopus", "specs", id)
}

func (f *statusFixture) commit(message string) string {
	f.t.Helper()
	runGitCommand(f.t, f.root, "add", "-A")
	runGitCommand(f.t, f.root, "commit", "-q", "--no-verify", "-m", message)
	return f.head()
}

func (f *statusFixture) head() string {
	f.t.Helper()
	return strings.TrimSpace(runGitCommand(f.t, f.root, "rev-parse", "HEAD"))
}

// saveCheckpoint writes a SPEC's canonical checkpoint, or its blocked
// sidecar when blocked is set.
func (f *statusFixture) saveCheckpoint(specID string, cp pipeline.Checkpoint, blocked bool) {
	f.t.Helper()
	name := specID + ".yaml"
	if blocked {
		name = specID + ".blocked.yaml"
	}
	dir := filepath.Join(f.root, ".autopus", "pipeline-state")
	require.NoError(f.t, os.MkdirAll(dir, 0o755))
	require.NoError(f.t, cp.SaveFile(filepath.Join(dir, name)))
}

// runStatus runs `auto status --dir <root>` with extra arguments.
func (f *statusFixture) runStatus(args ...string) string {
	f.t.Helper()
	cmd := newStatusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"--dir", f.root}, args...))
	require.NoError(f.t, cmd.Execute(), out.String())
	return statusANSI.ReplaceAllString(out.String(), "")
}

// statusANSI matches terminal styling, which the assertions read past.
var statusANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// statusJSON runs `auto status --json` and returns the envelope's data.
func (f *statusFixture) statusJSON() map[string]any {
	f.t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	out := f.runStatus("--json")
	require.NoError(f.t, json.Unmarshal([]byte(out), &envelope), out)
	return envelope.Data
}

// jsonSpec returns the JSON entry of one SPEC.
func jsonSpec(t *testing.T, data map[string]any, specID string) map[string]any {
	t.Helper()
	specs, _ := data["specs"].([]any)
	for _, raw := range specs {
		if spec, _ := raw.(map[string]any); spec["id"] == specID {
			return spec
		}
	}
	require.FailNow(t, "SPEC missing from status JSON", specID)
	return nil
}

// jsonPath walks nested JSON objects by key.
func jsonPath(value any, keys ...string) any {
	for _, key := range keys {
		object, _ := value.(map[string]any)
		value = object[key]
	}
	return value
}

// stoppedRunCheckpoint is attempt 2 of a task: a resume restored plan and
// test_scaffold, implemented, and stopped in validate when the fix made no
// progress.
func stoppedRunCheckpoint(specID, specDir, head string) pipeline.Checkpoint {
	snapshot, _ := pipeline.SpecSnapshotHash(specDir)
	finished := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	statuses := map[pipeline.PhaseID]pipeline.CheckpointStatus{
		pipeline.PhasePlan: pipeline.CheckpointStatusDone, pipeline.PhaseTestScaffold: pipeline.CheckpointStatusDone,
		pipeline.PhaseImplement: pipeline.CheckpointStatusDone, pipeline.PhaseValidate: pipeline.CheckpointStatusFailed,
		pipeline.PhaseReview: pipeline.CheckpointStatusPending,
	}
	receipt := pipeline.OrchestrationRunReceipt{
		SchemaVersion: pipeline.OrchestrationRunReceiptVersion, RunID: "run-2", TaskID: "task-1",
		ResumedFrom: "run-1", RouteVersion: pipeline.PipelineRouteVersion, RoutePhaseSet: pipeline.RouteFull,
		SpecID: specID, ResolvedProviders: []string{"codex"}, DispatchCount: 4, CumulativeDispatchCount: 7,
		Terminal: pipeline.TerminalBlocked, Blocker: "validate: the fix resolved nothing",
		Failures: []pipeline.PhaseFailure{{
			PhaseID: pipeline.PhaseValidate, Attempt: 2, Decision: pipeline.DecisionNoProgress, Summary: "same failure",
		}},
		PriorAttempts: []pipeline.RunAttemptRecord{{RunID: "run-1", Terminal: pipeline.TerminalCancelled, DispatchCount: 3}},
		Resume: &pipeline.ResumeRecord{
			CheckpointVersion: pipeline.CheckpointVersion,
			RestoredPhases:    []pipeline.PhaseID{pipeline.PhasePlan, pipeline.PhaseTestScaffold},
		},
		FinishedAt: &finished,
	}
	taskStatus := map[string]pipeline.CheckpointStatus{}
	for _, phase := range pipeline.DefaultPhases() {
		receipt.Phases = append(receipt.Phases, pipeline.PhaseReceipt{PhaseID: phase.ID, Status: statuses[phase.ID], Gate: phase.Gate})
		taskStatus[string(phase.ID)] = statuses[phase.ID]
	}
	return pipeline.Checkpoint{
		Version: pipeline.CheckpointVersion, RouteVersion: pipeline.PipelineRouteVersion, SpecID: specID,
		SnapshotHash: snapshot, GitCommitHash: head, TaskStatus: taskStatus, Receipt: &receipt,
	}
}

// completedRunCheckpoint is a completed first run that dispatched every phase
// once.
func completedRunCheckpoint(specID, specDir, head, runID string) pipeline.Checkpoint {
	cp := stoppedRunCheckpoint(specID, specDir, head)
	receipt := *cp.Receipt
	receipt.RunID, receipt.ResumedFrom, receipt.PriorAttempts, receipt.Resume = runID, "", nil, nil
	receipt.Failures, receipt.Blocker, receipt.Terminal = nil, "", pipeline.TerminalCompleted
	receipt.DispatchCount, receipt.CumulativeDispatchCount = 5, 5
	receipt.Phases = append([]pipeline.PhaseReceipt(nil), receipt.Phases...)
	for i := range receipt.Phases {
		receipt.Phases[i].Status = pipeline.CheckpointStatusDone
		cp.TaskStatus[string(receipt.Phases[i].PhaseID)] = pipeline.CheckpointStatusDone
	}
	cp.Receipt = &receipt
	return cp
}
