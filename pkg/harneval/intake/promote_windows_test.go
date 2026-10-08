//go:build windows

package intake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The name carries PlatformUnsupported so the windows-runtime CI job lists
// and runs it.
func TestPromote_OnWindows_PlatformUnsupported(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, promoteCandidateAt, "{}\n")
	before := treeDigest(t, root)

	_, err := Promote(context.Background(), PromoteRequest{Root: root, CandidateID: promoteCandidateID})
	requirePromoteRefusal(t, err, ReasonPlatformUnsupported, "")

	_, err = Promote(context.Background(), PromoteRequest{Root: root, CandidateID: "../x"})
	requirePromoteRefusal(t, err, ReasonCandidateIDInvalid, "")
	assert.Equal(t, before, treeDigest(t, root))
}
