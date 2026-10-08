package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-002 Git Execution Policy item 4 (plan task T8): the network
// step keeps the default branch it resolved, so the local patch base is the
// remote-tracking ref of that branch; a lookup that failed leaves it empty,
// which makes the base fall back to refs/remotes/origin/HEAD.
func TestFetchCI_DefaultBranch_KeepsTheResolvedBranch(t *testing.T) {
	t.Parallel()
	runs := ghPayload(t, ghRun(501, "CI", "push", "trunk", "completed", "success", 1))
	fetch, err := testBandClient(scriptedBandRunner(bandCmdOriginURL, "trunk", runs)).fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
	require.NoError(t, err)
	assert.Empty(t, fetch.Reason)
	assert.Equal(t, "trunk", fetch.DefaultBranch)
	assert.Len(t, fetch.Observations, 1, "the run on the default branch counts")
}

func TestFetchCI_DefaultBranch_EmptyWhenTheLookupFails(t *testing.T) {
	t.Parallel()
	for name, branch := range map[string]string{"null": "null", "empty": "", "two lines": "main\nother"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fetch, err := testBandClient(scriptedBandRunner(bandCmdOriginURL, branch, "[]")).fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
			require.NoError(t, err)
			assert.Equal(t, healthband.ReasonDefaultBranchUnknown, fetch.Reason)
			assert.Empty(t, fetch.DefaultBranch)
		})
	}
}
