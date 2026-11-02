package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// continueDryRun is the executed probe of probe B2: the real
// `auto pipeline run <SPEC> --continue --dry-run`, which runs the CLI's and
// the engine's admission and dispatches nothing.
func continueDryRun(t *testing.T, root, specID string) error {
	t.Helper()
	chdirForTest(t, root)
	cmd := newPipelineRunCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{specID, "--continue", "--dry-run", "--platform", "codex"})
	return cmd.Execute()
}

// S3 and probe B2: for every admission condition, status says what
// `--continue` then does — admits, or refuses for the same reason.
func TestStatusEvidence_AdmissionAgreesWithContinue(t *testing.T) {
	const specID = "SPEC-ADMIT-001"
	cases := []struct {
		name       string
		mutate     func(f *statusFixture)
		statusWhy  string
		continueIs string
	}{
		{name: "unchanged"},
		{name: "HEAD moved", mutate: func(f *statusFixture) {
			f.write("pkg/app/app.go", "package app\n\n// moved on\n")
			f.commit("move HEAD")
		}, statusWhy: "HEAD moved since the run", continueIs: "is stale"},
		{name: "SPEC changed", mutate: func(f *statusFixture) {
			f.write(".autopus/specs/"+specID+"/acceptance.md", "# Acceptance\n\nA new criterion.\n")
		}, statusWhy: "checkpoint snapshot mismatch", continueIs: "checkpoint snapshot mismatch"},
		{name: "output tampered", mutate: func(f *statusFixture) {
			f.editCheckpoint(specID, func(cp *pipeline.Checkpoint) {
				for i := range cp.PhaseOutputs {
					if cp.PhaseOutputs[i].PhaseID == pipeline.PhaseImplement {
						cp.PhaseOutputs[i].Content += " edited"
					}
				}
			})
		}, statusWhy: "phase output of implement cannot be restored", continueIs: "phase output of implement cannot be restored"},
		{name: "dependency broken", mutate: func(f *statusFixture) {
			f.editCheckpoint(specID, func(cp *pipeline.Checkpoint) {
				cp.TaskStatus[string(pipeline.PhaseTestScaffold)] = pipeline.CheckpointStatusPending
			})
		}, statusWhy: "is done but dependency test_scaffold is not done", continueIs: "is done but dependency test_scaffold is not done"},
		{name: "unknown version", mutate: func(f *statusFixture) {
			f.editCheckpoint(specID, func(cp *pipeline.Checkpoint) { cp.Version = "pipeline_checkpoint.v9" })
		}, statusWhy: "checkpoint identity version mismatch", continueIs: "checkpoint identity version mismatch"},
		{name: "route changed", mutate: func(f *statusFixture) {
			writeRouteContract(t, f.root, specID,
				routeContractDocument(specID, "bugfix_existing_contract", "low", "compact_contract", "pkg/app"))
			f.write("pkg/app/app.go", "package app\n\n// fix\n")
		}, statusWhy: "checkpoint phase route mismatch", continueIs: "checkpoint phase route mismatch"},
		{name: "git unavailable", mutate: func(f *statusFixture) {
			require.NoError(t, os.Rename(filepath.Join(f.root, ".git"), filepath.Join(f.root, ".git-away")))
		}, statusWhy: "git HEAD unavailable", continueIs: "without git HEAD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newStatusFixture(t, "claude-code")
			f.probe = stubStatusProbe([]string{"codex"}, false, t.TempDir())
			f.writeSpec(specID, "approved")
			f.commit("baseline")
			f.stoppedRun(specID)
			if tc.mutate != nil {
				tc.mutate(f)
			}
			canonical := filepath.Join(f.root, pipelineStateDir, specID+".yaml")
			before, err := os.ReadFile(canonical)
			require.NoError(t, err)

			resume := f.evidence(specID).Execution.Resume
			require.NotNil(t, resume)
			continueErr := continueDryRun(t, f.root, specID)

			if tc.statusWhy == "" {
				assert.True(t, resume.Admitted, resume.Reason)
				assert.NoError(t, continueErr, "status said admitted")
			} else {
				assert.False(t, resume.Admitted)
				assert.Contains(t, resume.Reason, tc.statusWhy)
				require.Error(t, continueErr, "status said refused")
				assert.Contains(t, continueErr.Error(), tc.continueIs)
			}
			after, err := os.ReadFile(canonical)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "a dry run never rewrites the canonical checkpoint")
		})
	}
}

// The execution owner comes from the run's own receipt: an orca run keeps its
// owner even after a later attempt rewrote the owner sidecar.
func TestStatusEvidence_OwnerComesFromTheRunReceipt(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-OWNER-STATUS-001"
	cases := map[string]struct {
		backend string
		command string
		note    string
	}{
		"orca":      {backend: "orca", command: "auto pipeline run " + specID + " --continue --platform omp --execution-owner orca"},
		"omp":       {backend: "rpc-canonical-full", command: "auto pipeline run " + specID + " --continue --platform omp"},
		"no record": {command: "auto pipeline run " + specID + " --continue --platform omp", note: "execution owner is unknown"},
	}
	for name, tc := range cases {
		f := newStatusFixture(t, "omp")
		f.probe = stubStatusProbe([]string{"omp"}, false, t.TempDir())
		f.writeSpec(specID, "approved")
		f.commit("baseline")
		f.stoppedRun(specID)
		f.editCheckpoint(specID, func(cp *pipeline.Checkpoint) {
			cp.Receipt.ResolvedProviders = []string{"omp"}
			cp.Receipt.ProviderReceipts = nil
			if tc.backend != "" {
				cp.Receipt.ProviderReceipts = []pipeline.ProviderRunReceipt{{Provider: "omp", Backend: tc.backend, Role: "implement"}}
			}
		})
		// A later attempt under the other owner rewrote the sidecar before
		// its admission was refused; status must not read it.
		f.write(".autopus/pipeline-state/"+specID+".execution-owner.json", `{"schema":"pipeline_execution_owner_receipt.v1","owner":"omp"}`)

		resume := f.evidence(specID).Execution.Resume
		require.NotNil(t, resume, name)
		assert.Equal(t, tc.command, resume.Command, name)
		assert.Contains(t, resume.CommandNote, tc.note, name)
	}
}

// When a module root and the workspace root both hold evidence for a SPEC,
// the newest wins and the other root is named.
func TestStatusEvidence_NewestStateRootWins(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-ROOTS-001"
	f := newStatusFixture(t, "claude-code")
	moduleRoot := filepath.Join(f.root, "mod")
	f.write("mod/autopus.yaml", "mode: full\nproject_name: mod\nplatforms:\n  - claude-code\n")
	f.write("mod/.autopus/specs/"+specID+"/spec.md", "# "+specID+": module feature\n\n**Status**: approved\n\n## Requirements\n\nIt works.\n")
	f.write("mod/.autopus/specs/"+specID+"/plan.md", "# Plan\n\n## Tasks\n\n- T1\n")
	f.write("mod/.autopus/specs/"+specID+"/acceptance.md", "# Acceptance\n\nGiven it, then it works.\n")
	f.commit("baseline")
	specDir := filepath.Join(moduleRoot, ".autopus", "specs", specID)
	moduleRun, err := f.runEngineAt(context.Background(), moduleRoot, specDir, specID, &statusEngineBackend{}, nil)
	require.NoError(t, err)
	workspaceRun, err := f.runEngineAt(context.Background(), f.root, specDir, specID, &statusEngineBackend{}, nil)
	require.NoError(t, err)

	execution := f.evidence(specID).Execution
	require.NotNil(t, execution)
	assert.Equal(t, workspaceRun.Receipt.RunID, execution.RunID, "the later run, started from the workspace, wins")
	assert.Equal(t, f.root, execution.Root)
	assert.Equal(t, []string{moduleRoot}, execution.OtherRoots)

	past := time.Now().Add(-time.Hour)
	f.editStateFile(filepath.Join(f.root, pipelineStateDir, specID+".yaml"), func(cp *pipeline.Checkpoint) {
		cp.Receipt.StartedAt, cp.Receipt.FinishedAt = past, &past
	}, past)
	execution = f.evidence(specID).Execution
	assert.Equal(t, moduleRun.Receipt.RunID, execution.RunID, "now the module run is the newest")
	assert.Equal(t, moduleRoot, execution.Root)
	assert.Contains(t, f.runStatus(), "older evidence also in: "+f.root)
}

// editStateFile rewrites a checkpoint and dates the file.
func (f *statusFixture) editStateFile(path string, edit func(cp *pipeline.Checkpoint), at time.Time) {
	f.t.Helper()
	cp, err := pipeline.LoadFile(path)
	require.NoError(f.t, err)
	edit(cp)
	require.NoError(f.t, cp.SaveFile(path))
	require.NoError(f.t, os.Chtimes(path, at, at))
}
