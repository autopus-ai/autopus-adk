package pipeline_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// S8: an output record that is missing, edited, re-hashed, borrowed from
// another SPEC, or made under another policy is never restored. Because the
// chain reaches test_scaffold, which wrote the working tree, the resume is
// blocked before any call and the canonical checkpoint is left as it was.
func TestSubprocessEngine_ResumeRejectsUnverifiableOutputs(t *testing.T) {
	t.Parallel()

	other := newResumeFixture(t, "SPEC-RESUME-OTHER-001", false)
	other.interruptAt(t, pipeline.PhaseImplement)
	foreign := other.load(t).PhaseOutputs

	cases := []struct {
		name   string
		mutate func(*pipeline.Checkpoint)
		reason string
	}{
		{name: "plan record removed", reason: "no persisted output record",
			mutate: func(cp *pipeline.Checkpoint) { cp.PhaseOutputs = cp.PhaseOutputs[1:] }},
		{name: "plan content edited", reason: "content hash mismatch",
			mutate: func(cp *pipeline.Checkpoint) { cp.PhaseOutputs[0].Content += " tampered" }},
		{name: "plan content edited and re-hashed", reason: "input fingerprint mismatch",
			mutate: func(cp *pipeline.Checkpoint) {
				cp.PhaseOutputs[0].Content += " tampered"
				sum := sha256.Sum256([]byte(cp.PhaseOutputs[0].Content))
				cp.PhaseOutputs[0].ContentHash = "sha256:" + hex.EncodeToString(sum[:])
			}},
		{name: "records of another SPEC", reason: "input fingerprint mismatch",
			mutate: func(cp *pipeline.Checkpoint) {
				cp.PhaseOutputs = append([]pipeline.PhaseOutputRecord(nil), foreign...)
			}},
		{name: "plan record from another output policy", reason: "output policy",
			mutate: func(cp *pipeline.Checkpoint) { cp.PhaseOutputs[0].Policy = "pipeline_phase_output_policy.v0" }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := newResumeFixture(t, "SPEC-RESUME-TAMPER-001", false)
			fixture.interruptAt(t, pipeline.PhaseImplement)
			saved := fixture.load(t)
			require.Len(t, saved.PhaseOutputs, 2)
			tc.mutate(saved)
			before := fixture.save(t, saved)
			backend := &scriptedBackend{}

			_, err := fixture.run(context.Background(), backend, fixture.load(t))

			require.ErrorContains(t, err, "pipeline: resume blocked:")
			assert.ErrorContains(t, err, "test_scaffold")
			assert.ErrorContains(t, err, tc.reason)
			assert.Empty(t, backend.calls(), "a refused resume dispatches nothing")
			after, readErr := os.ReadFile(fixture.checkpointPath())
			require.NoError(t, readErr)
			assert.Equal(t, before, after, "the canonical checkpoint is left as it was")
			blocked, loadErr := pipeline.LoadFile(filepath.Join(fixture.stateDir, fixture.specID+".blocked.yaml"))
			require.NoError(t, loadErr)
			require.NotNil(t, blocked.Receipt)
			assert.Equal(t, pipeline.TerminalBlocked, blocked.Receipt.Terminal)
			assert.Empty(t, blocked.PhaseOutputs)
		})
	}
}

// A checkpoint for another SPEC is refused by the identity check before
// any output is read.
func TestSubprocessEngine_ResumeUnderAnotherSpecIsRefused(t *testing.T) {
	t.Parallel()

	fixture := newResumeFixture(t, "SPEC-RESUME-TAMPER-002", false)
	fixture.interruptAt(t, pipeline.PhaseImplement)
	saved := fixture.load(t)
	elsewhere := newResumeFixture(t, "SPEC-RESUME-TAMPER-003", false)
	backend := &scriptedBackend{}

	_, err := elsewhere.run(context.Background(), backend, saved)

	require.ErrorContains(t, err, "checkpoint SPEC mismatch")
	assert.Empty(t, backend.calls())
}
