package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

const (
	bandAuthArgv = "gh auth status --hostname github.com"
	bandAPIArgv  = "gh api repos/acme/app --hostname github.com --jq .default_branch"
)

// REQ-05 and S13: every unavailable source ends CI ingest with its reason
// code, runs no later gh step, and yields no observation; nothing fails.
func TestReactBandIngest_SkipReasons_FailOpen(t *testing.T) {
	t.Parallel()
	exit1 := errors.New("exit status 1")
	for _, tc := range []struct {
		name   string
		origin string
		edit   func(*fakeBandRunner)
		reason string
		wantGH []string
	}{
		{"gh not on PATH", "git@github.com:acme/app.git", func(f *fakeBandRunner) { f.noGH = true }, healthband.ReasonGHMissing, nil},
		{"gh disappears after the lookup", "git@github.com:acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh auth status"] = fakeBandAnswer{err: exec.ErrNotFound}
		}, healthband.ReasonGHMissing, []string{bandAuthArgv}},
		{"gh auth status exit 1", "git@github.com:acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh auth status"] = fakeBandAnswer{err: exit1}
		}, healthband.ReasonGHUnauthenticated, []string{bandAuthArgv}},
		{"gh run list exit 1", "git@github.com:acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh run list"] = fakeBandAnswer{err: exit1}
		}, healthband.ReasonGHFetchFailed, []string{bandAuthArgv, bandAPIArgv, bandRunListArgv}},
		{"gh run list prints no JSON", "git@github.com:acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh run list"] = fakeBandAnswer{stdout: "Unknown JSON field: \"attempt\"\n"}
		}, healthband.ReasonGHFetchFailed, []string{bandAuthArgv, bandAPIArgv, bandRunListArgv}},
		{"no origin", "", func(f *fakeBandRunner) {
			f.answers["git remote get-url"] = fakeBandAnswer{err: exit1}
		}, healthband.ReasonNoRemote, nil},
		{"blank origin", "  ", nil, healthband.ReasonNoRemote, nil},
		{"gitlab origin fails the host rule", "https://gitlab.com/acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh auth status"] = fakeBandAnswer{err: exit1}
		}, healthband.ReasonRemoteNotGitHub, []string{"gh auth status --hostname gitlab.com"}},
		{"local path origin", "/srv/git/app.git", nil, healthband.ReasonRemoteNotGitHub, nil},
		{"empty default branch", "git@github.com:acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh api repos/acme/app"] = fakeBandAnswer{stdout: "\n"}
		}, healthband.ReasonDefaultBranchUnknown, []string{bandAuthArgv, bandAPIArgv}},
		{"null default branch", "git@github.com:acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh api repos/acme/app"] = fakeBandAnswer{stdout: "null\n"}
		}, healthband.ReasonDefaultBranchUnknown, []string{bandAuthArgv, bandAPIArgv}},
		{"default branch lookup exit 1", "git@github.com:acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh api repos/acme/app"] = fakeBandAnswer{err: exit1}
		}, healthband.ReasonDefaultBranchUnknown, []string{bandAuthArgv, bandAPIArgv}},
		{"default branch output beyond its bound", "git@github.com:acme/app.git", func(f *fakeBandRunner) {
			f.answers["gh api repos/acme/app"] = fakeBandAnswer{stdout: strings.Repeat("m", bandTextOutputCap+1)}
		}, healthband.ReasonDefaultBranchUnknown, []string{bandAuthArgv, bandAPIArgv}},
	} {
		runner := scriptedBandRunner(tc.origin, "main", ghPayload(t, ghRun(500, "CI", "push", "main", "completed", "failure", 1)))
		if tc.edit != nil {
			tc.edit(runner)
		}
		fetch, err := testBandClient(runner).fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.reason, fetch.Reason, tc.name)
		assert.Empty(t, fetch.Observations, tc.name)
		assert.Equal(t, tc.wantGH, runner.argvs("gh"), tc.name)
		assert.Equal(t, []string{"git remote get-url origin"}, runner.argvs("git"), tc.name)
	}
}

// A skipped ingest merges nothing, so no history file appears.
func TestReactBandIngest_SkippedFetchWritesNoHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
	runner.noGH = true
	fetch, appended := ingestBandCI(t, testBandClient(runner), dir, bandDefaultLimit)
	assert.Equal(t, healthband.ReasonGHMissing, fetch.Reason)
	assert.Zero(t, appended)
	assert.NoFileExists(t, filepath.Join(dir, ".autopus", "metrics", healthband.CIRunsFile))
}

// Host rule: a GitHub Enterprise host that passes gh auth status is used for
// --hostname and GH_HOST.
func TestReactBandIngest_HostRule_AcceptsAnAuthenticatedEnterpriseHost(t *testing.T) {
	t.Parallel()
	runner := scriptedBandRunner("git@ghe.example.com:acme/app.git", "trunk",
		ghPayload(t, ghRun(500, "CI", "push", "trunk", "completed", "failure", 1), ghRun(501, "CI", "push", "main", "completed", "failure", 1)))
	fetch, err := testBandClient(runner).fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
	require.NoError(t, err)
	require.Empty(t, fetch.Reason)
	require.Len(t, fetch.Observations, 1)
	assert.Equal(t, "500", fetch.Observations[0].SampleKey, "only the resolved default branch trunk is trusted")
	assert.Equal(t, []string{
		"gh auth status --hostname ghe.example.com",
		"gh api repos/acme/app --hostname ghe.example.com --jq .default_branch",
		bandRunListArgv,
	}, runner.argvs("gh"))
	for _, call := range runner.recorded("gh") {
		assert.Contains(t, call.env, "GH_HOST=ghe.example.com")
		assert.Contains(t, call.env, "GH_REPO=acme/app")
	}
}

// S13: a gh call that hangs is cut off by its own timeout and ends ingest
// with the reason of its step instead of blocking band.
func TestReactBandIngest_HangingCallsEndAtTheirTimeout(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		hung   string
		reason string
	}{
		{"gh run list", healthband.ReasonGHFetchFailed},
		{"gh api repos/acme/app", healthband.ReasonDefaultBranchUnknown},
		{"gh auth status", healthband.ReasonGHUnauthenticated},
		{"git remote get-url", healthband.ReasonNoRemote},
	} {
		runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
		runner.answers[tc.hung] = fakeBandAnswer{hang: true}
		client := testBandClient(runner)
		client.callTimeout = 20 * time.Millisecond
		started := time.Now()
		fetch, err := client.fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
		require.NoError(t, err, tc.hung)
		assert.Equal(t, tc.reason, fetch.Reason, tc.hung)
		assert.Less(t, time.Since(started), 5*time.Second, tc.hung)
	}
}

// REQ-19: a limit outside 1–1000 is an invalid flag, rejected before any
// subprocess starts.
func TestReactBandIngest_RejectsAnOutOfRangeLimitBeforeAnyCall(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{0, 1001} {
		runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
		_, err := testBandClient(runner).fetchCI(t.Context(), t.TempDir(), limit)
		assert.Error(t, err, limit)
		assert.Empty(t, runner.calls, limit)
	}
}

// The default client reads the real environment and keeps the contract
// bounds: 30 s per gh call, 60 s per failed-step log, and its last 4 MiB.
func TestReactBandGH_NewClient_UsesContractDefaults(t *testing.T) {
	t.Parallel()
	client := newBandGHClient(execBandRunner{})
	assert.Equal(t, 30*time.Second, client.callTimeout)
	assert.Equal(t, 60*time.Second, client.logTimeout)
	assert.Equal(t, 4<<20, client.logCapture)
	assert.Equal(t, os.Environ(), client.environ())
}
