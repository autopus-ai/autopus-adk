//go:build windows

package intake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The name carries PlatformUnsupported so the windows-runtime CI job lists
// and runs it with the other Windows write refusals.

func TestReject_OnWindows_PlatformUnsupported(t *testing.T) {
	root := t.TempDir()

	_, err := Reject(RejectRequest{Root: root, CandidateID: "GTC-8e80c7a18029", Reason: "not a harness issue", Redactor: noRedaction})

	var runErr *RunError
	require.ErrorAs(t, err, &runErr)
	assert.Equal(t, ReasonPlatformUnsupported, runErr.Reason)
	assert.NoDirExists(t, root+`\evals`)
}
