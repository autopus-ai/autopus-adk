package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

const backendSecret = "sk-proj-backendsecret0123456789abcdef"

// S13: a secret inside a backend error reaches neither the receipt, the
// checkpoint file, nor the error the caller prints, while the error keeps
// its identity for errors.Is.
func TestSubprocessEngine_BackendErrorSecretNeverPersistsOrSurfaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sentinel := errors.New("provider rejected the call")
	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: func(context.Context, int) (*pipeline.PhaseResponse, error) {
			return nil, fmt.Errorf("%w: Authorization: Bearer %s", sentinel, backendSecret)
		},
	}}

	result, err := compactEngine("SPEC-REDACT-001", backend, dir).Run(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "redaction keeps the error's identity")
	assert.NotContains(t, err.Error(), backendSecret)
	assert.Contains(t, err.Error(), "[REDACTED_SECRET]")
	receipt, marshalErr := json.Marshal(result.Receipt)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(receipt), backendSecret, "blocker, transitions, phase errors, and failures")
	assert.Contains(t, result.Receipt.Blocker, "[REDACTED_SECRET]")
	persisted, readErr := os.ReadFile(filepath.Join(dir, "SPEC-REDACT-001.yaml"))
	require.NoError(t, readErr)
	assert.NotContains(t, string(persisted), backendSecret)
}

// S13: a stored prior receipt may carry raw secrets from an older binary or
// an edit; the resumed run's lineage, its checkpoint, and a blocked sidecar
// carry them redacted.
func TestSubprocessEngine_ResumeRedactsPriorReceiptSecrets(t *testing.T) {
	t.Parallel()

	legacy := func(fixture resumeFixture, done ...pipeline.PhaseID) *pipeline.Checkpoint {
		statuses := map[string]pipeline.CheckpointStatus{}
		for _, phase := range done {
			statuses[string(phase)] = pipeline.CheckpointStatusDone
		}
		return &pipeline.Checkpoint{
			Version: pipeline.LegacyCheckpointVersionV1, RouteVersion: pipeline.PipelineRouteVersion,
			SpecID: fixture.specID, SnapshotHash: fixture.snapshot, GitCommitHash: "git-a", TaskStatus: statuses,
			Receipt: &pipeline.OrchestrationRunReceipt{
				RunID: "pipeline-older", Terminal: pipeline.TerminalBlocked,
				Blocker: "phase implement: OPENAI_API_KEY=" + backendSecret,
				Failures: []pipeline.PhaseFailure{{
					PhaseID: pipeline.PhaseImplement, Kind: pipeline.FailureTransport,
					Summary: "token=" + backendSecret, Excerpt: "Authorization: Bearer " + backendSecret,
				}},
			},
		}
	}

	t.Run("blocked resume writes a redacted sidecar", func(t *testing.T) {
		t.Parallel()
		fixture := newResumeFixture(t, "SPEC-REDACT-002", false)
		_, err := fixture.run(context.Background(), &scriptedBackend{},
			legacy(fixture, pipeline.PhasePlan, pipeline.PhaseTestScaffold))
		require.ErrorContains(t, err, "pipeline: resume blocked:")
		sidecar := filepath.Join(fixture.stateDir, fixture.specID+".blocked.yaml")
		raw, readErr := os.ReadFile(sidecar)
		require.NoError(t, readErr)
		assert.NotContains(t, string(raw), backendSecret)
		blocked, loadErr := pipeline.LoadFile(sidecar)
		require.NoError(t, loadErr)
		require.Len(t, blocked.Receipt.PriorAttempts, 1)
		assert.Contains(t, blocked.Receipt.PriorAttempts[0].Blocker, "[REDACTED_SECRET]")
	})

	t.Run("completed resume keeps a redacted lineage", func(t *testing.T) {
		t.Parallel()
		fixture := newResumeFixture(t, "SPEC-REDACT-003", false)
		result, err := fixture.run(context.Background(), &scriptedBackend{}, legacy(fixture, pipeline.PhasePlan))
		require.NoError(t, err)
		require.Len(t, result.Receipt.PriorAttempts, 1)
		attempt := result.Receipt.PriorAttempts[0]
		assert.Contains(t, attempt.Blocker, "[REDACTED_SECRET]")
		require.Len(t, attempt.Failures, 1)
		assert.NotContains(t, attempt.Failures[0].Summary+attempt.Failures[0].Excerpt, backendSecret)
		raw, readErr := os.ReadFile(fixture.checkpointPath())
		require.NoError(t, readErr)
		assert.NotContains(t, string(raw), backendSecret)
	})
}
