package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

var bandTestTarget = bandGHTarget{Host: "github.com", Owner: "acme", Repo: "app"}

// S19 and S11 at the fetch seam: each failed-step log is fetched for the
// attempt of its observation and returned only after the Untrusted Input
// Contract (controls stripped, secrets and local paths redacted).
func TestReactBandGH_FailedRunLog_PinsTheAttemptAndSanitizes(t *testing.T) {
	t.Parallel()
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
	runner.answers["gh run view 4242 -R acme/app --attempt 1 --log-failed"] = fakeBandAnswer{stdout: "\x1b[31mstep 3 failed\x1b[0m\n" +
		"using ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA for auth\n" +
		"open /Users/alice/work/repo/.env and /Users/alice/.ssh/config failed\n"}
	runner.answers["gh run view 4243 -R acme/app --attempt 2 --log-failed"] = fakeBandAnswer{stdout: "step 9 failed: exit 2\n"}
	client := testBandClient(runner)

	dirty, err := client.failedRunEvidence(t.Context(), bandTestTarget, "/Users/alice/work/repo", 4242, 1)
	require.NoError(t, err)
	assert.Equal(t, "step 3 failed\nusing [REDACTED_SECRET] for auth\nopen <project>/.env and ~/.ssh/config failed", dirty.Text)
	assert.Equal(t, []string{healthband.ReasonSecretRisk}, dirty.Reasons)
	assert.Equal(t, promptlayer.RedactionRedacted, dirty.RedactionStatus)

	clean, err := client.failedRunEvidence(t.Context(), bandTestTarget, "/Users/alice/work/repo", 4243, 2)
	require.NoError(t, err)
	assert.Equal(t, "step 9 failed: exit 2", clean.Text)
	assert.Empty(t, clean.Reasons)
	assert.Equal(t, promptlayer.RedactionPassed, clean.RedactionStatus)

	assert.Equal(t, []string{
		"gh run view 4242 -R acme/app --attempt 1 --log-failed",
		"gh run view 4243 -R acme/app --attempt 2 --log-failed",
	}, runner.argvs("gh"))
	for _, call := range runner.recorded("gh") {
		assert.Equal(t, "/Users/alice/work/repo", call.dir)
		assert.Contains(t, call.env, "GH_REPO=acme/app")
		assert.Contains(t, call.env, "GH_HOST=github.com")
		assert.Greater(t, call.timeout, 59*time.Second)
		assert.LessOrEqual(t, call.timeout, 60*time.Second)
	}
}

// S11 at the seam: a log larger than the capture keeps only its tail, which
// is redacted whole and then cut to 8 KiB, so the failing line survives with
// size_cap and a token inside the kept tail is redacted. The unterminated
// key marker sits before the capture window: an unbounded capture would see
// it and redact to the end of the text, the failing line included. The
// capture is 4 MiB in production (pinned by UsesContractDefaults) and 64 KiB
// here, because SanitizeContent over 4 MiB takes about 15 s under -race; T8
// covers a real 6 MiB log.
func TestReactBandGH_FailedRunLog_KeepsTheTailOfAHugeLog(t *testing.T) {
	t.Parallel()
	head := "-----BEGIN RSA PRIVATE KEY-----\n" + strings.Repeat("MIIEowIBAAKCAQEAsyntheticsyntheticsynthetic\n", 20)
	noise := strings.Repeat("noise line 0123456789abcdef\n", (96<<10)/28)
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
	runner.answers["gh run view"] = fakeBandAnswer{stdout: head + noise + "using ghp_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBB for auth\n" +
		strings.Repeat("y", 500) + "\nstep 7 failed: exit 1\n"}
	client := testBandClient(runner)
	client.logCapture = 64 << 10

	evidence, err := client.failedRunEvidence(t.Context(), bandTestTarget, t.TempDir(), 77, 3)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(evidence.Text, "\nstep 7 failed: exit 1"))
	assert.Contains(t, evidence.Text, "using [REDACTED_SECRET] for auth")
	assert.LessOrEqual(t, len(evidence.Text), healthband.CILogExcerptBytes)
	assert.NotContains(t, evidence.Text, "ghp_")
	assert.NotContains(t, evidence.Text, "MIIEow")
	assert.Equal(t, []string{healthband.ReasonSecretRisk, healthband.ReasonSizeCap}, evidence.Reasons)

	unbounded := testBandClient(runner)
	unbounded.logCapture = 1 << 20
	whole, err := unbounded.failedRunEvidence(t.Context(), bandTestTarget, t.TempDir(), 77, 3)
	require.NoError(t, err)
	assert.False(t, strings.HasSuffix(whole.Text, "step 7 failed: exit 1"), "control: the marker is live when captured")
}

// A failed, hung, or ill-formed log fetch returns an error and no text; an
// id or attempt outside the contract never reaches gh.
func TestReactBandGH_FailedRunLog_FailsClosed(t *testing.T) {
	t.Parallel()
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
	runner.answers["gh run view 1 -R acme/app --attempt 1 --log-failed"] = fakeBandAnswer{stdout: "partial ghp_CCCCCCCCCCCCCCCCCCCCCCCCCCCCCC", err: errors.New("exit status 1")}
	runner.answers["gh run view 2 -R acme/app --attempt 1 --log-failed"] = fakeBandAnswer{hang: true}
	client := testBandClient(runner)
	client.logTimeout = 20 * time.Millisecond

	for _, runID := range []int64{1, 2} {
		evidence, err := client.failedRunEvidence(t.Context(), bandTestTarget, t.TempDir(), runID, 1)
		assert.Error(t, err, runID)
		assert.Empty(t, evidence.Text, runID)
	}
	for _, bad := range [][2]int64{{0, 1}, {-5, 1}, {4242, 0}} {
		_, err := client.failedRunEvidence(t.Context(), bandTestTarget, t.TempDir(), bad[0], int(bad[1]))
		assert.Error(t, err, bad)
	}
	assert.Len(t, runner.argvs("gh"), 2)
}
