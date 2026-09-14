package run

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/desktopobserve"
	"github.com/insajin/autopus-adk/pkg/qa/journey"
)

func coveragePack() journey.Pack {
	pack := journey.Pack{ID: "journey-desktop"}
	pack.Adapter.ID = "desktop-observe"
	return pack
}

// Guards that a contract error is reported as a blocked adapter result with a
// cause, not as a pass or an empty summary the operator cannot act on.
func TestDesktopObservationContractFailureCarriesCause(t *testing.T) {
	t.Parallel()

	result := desktopObservationContractFailure(coveragePack())
	assert.Equal(t, "blocked", result.Status)
	assert.Equal(t, "journey-desktop", result.JourneyID)
	assert.Equal(t, "desktop-observe", result.Adapter)
	assert.NotEmpty(t, result.FailureSummary)
	assert.Nil(t, result.DesktopObservation, "a failed contract must publish no evidence")
}

// Guards the index check emitted for the same failure: it must be blocked and
// record the expected/actual pair, otherwise the index shows a silent hole.
func TestDesktopObservationErrorCheckIsBlocked(t *testing.T) {
	t.Parallel()

	check := desktopObservationErrorCheck(coveragePack())
	assert.Equal(t, desktopobserve.DeterministicCheckSemanticLandmarks, check.ID)
	assert.Equal(t, "blocked", check.Status)
	assert.Equal(t, "journey-desktop", check.JourneyID)
	assert.Equal(t, "desktop-observe", check.Adapter)
	assert.NotEqual(t, check.Expected, check.Actual)
	assert.NotEmpty(t, check.FailureSummary)
}

// Guards the observation artifact writer: it must emit newline-terminated JSON
// that round-trips, and its cleanup must actually remove the temp file so runs
// do not leak evidence into the system temp dir.
func TestWriteDesktopObservationArtifactRoundTrips(t *testing.T) {
	evidence := desktopobserve.ObservationEvidence{
		DeterministicChecks: []desktopobserve.DeterministicCheck{
			{ID: desktopobserve.DeterministicCheckSemanticLandmarks},
		},
	}

	path, cleanup, err := writeDesktopObservationArtifact(evidence)
	require.NoError(t, err)
	require.NotEmpty(t, path)

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Greater(t, len(body), 0)
	assert.Equal(t, byte('\n'), body[len(body)-1])

	var decoded desktopobserve.ObservationEvidence
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Len(t, decoded.DeterministicChecks, 1)
	assert.Equal(t,
		desktopobserve.DeterministicCheckSemanticLandmarks,
		decoded.DeterministicChecks[0].ID,
	)

	cleanup()
	_, statErr := os.Stat(path)
	assert.True(t, errors.Is(statErr, os.ErrNotExist))
}

// Guards the writer's failure path: when the temp directory is unusable it
// must return an error plus a safe no-op cleanup rather than a bogus path.
func TestWriteDesktopObservationArtifactTempFailure(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "absent"))

	path, cleanup, err := writeDesktopObservationArtifact(desktopobserve.ObservationEvidence{})
	require.Error(t, err)
	assert.Empty(t, path)
	require.NotNil(t, cleanup)
	cleanup()
}

// Guards the tri-state semantic read: only an explicitly true pointer counts.
// Nil (unknown) and unrecognized keys must not be treated as satisfied, which
// would let a provider pass landmark checks by omitting state.
func TestDesktopStateIsTrueTriState(t *testing.T) {
	t.Parallel()

	yes, no := true, false
	full := desktopobserve.SemanticState{
		Enabled: &yes, Focused: &no, Selected: &yes, Expanded: &no,
	}

	assert.True(t, desktopStateIsTrue(full, desktopobserve.StateEnabled))
	assert.False(t, desktopStateIsTrue(full, desktopobserve.StateFocused))
	assert.True(t, desktopStateIsTrue(full, desktopobserve.StateSelected))
	assert.False(t, desktopStateIsTrue(full, desktopobserve.StateExpanded))

	assert.False(t, desktopStateIsTrue(desktopobserve.SemanticState{}, desktopobserve.StateEnabled))
	assert.False(t, desktopStateIsTrue(full, desktopobserve.SemanticStateKey("checked")))
}

// Guards the normalized provider failure sentinel: it must satisfy error and
// stay matchable through errors.As after wrapping, which is how the runner
// recovers the provider-authored receipt.
func TestDesktopProviderFailureIsMatchableError(t *testing.T) {
	t.Parallel()

	failure := desktopProviderFailure{
		condition: desktopobserve.FailureOperationMissing,
		operation: desktopobserve.OperationCapabilities,
		validated: true,
	}
	assert.NotEmpty(t, failure.Error())

	var recovered desktopProviderFailure
	require.True(t, errors.As(errors.Join(errors.New("outer"), failure), &recovered))
	assert.Equal(t, desktopobserve.FailureOperationMissing, recovered.condition)
	assert.True(t, recovered.validated)
}
