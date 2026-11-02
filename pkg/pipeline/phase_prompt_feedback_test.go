package pipeline_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

func manifestEntry(t *testing.T, manifest pipeline.PromptManifest, id string) pipeline.PromptManifestEntry {
	t.Helper()
	for _, entry := range manifest.Entries {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("manifest has no entry %q: %+v", id, manifest.Entries)
	return pipeline.PromptManifestEntry{}
}

// S16: restored outputs and remediation feedback are ephemeral manifest
// layers, so a fix dispatch and a changed failure are visible as exactly one
// changed entry, and a restored output keeps the hash it had before.
func TestPhasePromptBuilder_FeedbackAndRestoredOutputsAreManifestLayers(t *testing.T) {
	t.Parallel()

	builder := pipeline.NewPhasePromptBuilder(writeEnginePromptSpec(t))
	previous := map[pipeline.PhaseID]string{
		pipeline.PhasePlan: planMarker, pipeline.PhaseTestScaffold: scaffoldMarker,
	}
	failure := func(fingerprint, actual string) *pipeline.PhaseFailure {
		return &pipeline.PhaseFailure{
			PhaseID: pipeline.PhaseValidate, Attempt: 1, Kind: pipeline.FailureTestFailure,
			Fingerprint: fingerprint, Summary: "validation failed: go test ./x",
			Evidence: []pipeline.FailureEvidence{{Command: "go test ./x", Expected: "ok", Actual: actual}},
		}
	}
	build := func(feedback *pipeline.PhaseFailure) (string, pipeline.PromptManifest) {
		prompt, manifest, err := builder.BuildPromptWithManifest(pipeline.PhaseImplement, pipeline.PhaseContext{
			PreviousResults: previous, FrozenRequiredDocuments: true, Feedback: feedback,
		})
		require.NoError(t, err)
		return prompt, manifest
	}

	plain, plainManifest := build(nil)
	fix, fixManifest := build(failure("sha256:aaaa", "boom"))
	_, nextManifest := build(failure("sha256:bbbb", "bang"))

	for _, id := range []string{"phase:previous:plan", "phase:previous:test_scaffold"} {
		entry := manifestEntry(t, fixManifest, id)
		assert.Equal(t, promptlayer.KindEphemeral, entry.Kind, id)
		assert.Equal(t, manifestEntry(t, plainManifest, id).Hash, entry.Hash, id)
	}
	feedback := manifestEntry(t, fixManifest, "phase:remediation-feedback")
	assert.Equal(t, promptlayer.KindEphemeral, feedback.Kind)
	assert.Equal(t, promptlayer.GroupTaskContext, feedback.Group)
	assert.Equal(t, "remediation:sha256:aaaa", feedback.SourceRef)
	assert.False(t, feedback.CacheEligible)
	assert.NotContains(t, plain, "## Remediation Feedback")
	assert.Contains(t, fix, "## Remediation Feedback")

	added := promptlayer.CompareManifests(plainManifest, fixManifest)
	require.Len(t, added, 1, "the fix dispatch adds only the feedback layer")
	assert.Equal(t, "phase:remediation-feedback", added[0].ID)
	changed := promptlayer.CompareManifests(fixManifest, nextManifest)
	require.Len(t, changed, 1, "a new failure changes only the feedback layer")
	assert.Equal(t, "phase:remediation-feedback", changed[0].ID)
	assert.Equal(t, promptlayer.InvalidationEphemeralChanged, changed[0].Reason)
}
