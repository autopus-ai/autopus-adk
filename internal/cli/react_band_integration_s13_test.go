package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandITHangingRunner holds one command, after signalling that it started,
// until the scenario releases it; the call then fails like a gh call cut
// off by its timeout. Every other command goes to the fake.
type bandITHangingRunner struct {
	*fakeBandRunner
	hung             string
	started, release chan struct{}
}

func (r *bandITHangingRunner) Run(ctx context.Context, command bandCommand) error {
	if !strings.HasPrefix(strings.Join(append([]string{command.Name}, command.Args...), " "), r.hung) {
		return r.fakeBandRunner.Run(ctx, command)
	}
	close(r.started)
	select {
	case <-r.release:
	case <-ctx.Done():
	}
	return context.DeadlineExceeded
}

// assertBandITStillDiagnoses is the S13 outcome every unavailable source
// keeps: exit 0, the source's reason, and the stored O2 series evaluated to
// tier 2 with its BS written.
func assertBandITStillDiagnoses(t *testing.T, p bandCmdProject, run bandCmdRun, reason string) {
	t.Helper()
	require.NoError(t, run.err)
	envelope := decodeBandEnvelope(t, run.stdout)
	assert.Equal(t, []string{reason}, envelope.Data.Reasons)
	ci := envelope.check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, map[string]string{"tier": "2", "action": "diagnose", "episode_id": "e1042", "bs_id": "BS-BAND-001"},
		pick(ci.Fields, "tier", "action", "episode_id", "bs_id"))
	assert.Equal(t, []string{"# BS-BAND-001: ci.failure_rate:CI tier 2 anomaly (e1042)"}, bandITHeads(t, p.dir))
}

// S13: each unavailable CI source ends ingest with its reason, and band
// still evaluates the stored O2 series to tier 2 and writes the BS. A gh run
// list that hangs past its 30 s budget is the deadline error that call gets.
func TestReactBandIT_S13_UnavailableSourcesFailOpen(t *testing.T) {
	t.Parallel()
	exit1 := errors.New("exit status 1")
	for _, tc := range []struct {
		name, origin, reason string
		edit                 func(*fakeBandRunner)
		wantGH               []string
	}{
		{"gh not on PATH", bandCmdOriginURL, healthband.ReasonGHMissing, func(f *fakeBandRunner) { f.noGH = true }, nil},
		{"gh auth status exit 1", bandCmdOriginURL, healthband.ReasonGHUnauthenticated, func(f *fakeBandRunner) {
			f.answers["gh auth status"] = fakeBandAnswer{err: exit1}
		}, []string{bandAuthArgv}},
		{"gh run list exit 1", bandCmdOriginURL, healthband.ReasonGHFetchFailed, func(f *fakeBandRunner) {
			f.answers["gh run list"] = fakeBandAnswer{err: exit1}
		}, []string{bandAuthArgv, bandAPIArgv, bandRunListArgv}},
		{"gh run list past its 30 s budget", bandCmdOriginURL, healthband.ReasonGHFetchFailed, func(f *fakeBandRunner) {
			f.answers["gh run list"] = fakeBandAnswer{err: context.DeadlineExceeded}
		}, []string{bandAuthArgv, bandAPIArgv, bandRunListArgv}},
		{"no origin", "", healthband.ReasonNoRemote, func(f *fakeBandRunner) {
			f.answers["git remote get-url"] = fakeBandAnswer{err: exit1}
		}, nil},
		{"gitlab origin", "https://gitlab.com/acme/app.git", healthband.ReasonRemoteNotGitHub, func(f *fakeBandRunner) {
			f.answers["gh auth status"] = fakeBandAnswer{err: exit1}
		}, []string{"gh auth status --hostname gitlab.com"}},
		{"empty default branch", bandCmdOriginURL, healthband.ReasonDefaultBranchUnknown, func(f *fakeBandRunner) {
			f.answers["gh api repos/acme/app"] = fakeBandAnswer{stdout: "\n"}
		}, []string{bandAuthArgv, bandAPIArgv}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newBandITProject(t)
			p.store(healthband.CIRunsFile, bandO2CI())
			runner := scriptedBandRunner(tc.origin, "main", "[]")
			tc.edit(runner)
			provider := &bandITProvider{}

			run := p.band(bandITRun{runner: runner, clock: fixedAt(bandITT0), provider: provider}, "--format", "json")

			assertBandITStillDiagnoses(t, p, run, tc.reason)
			assert.Equal(t, tc.wantGH, runner.argvs("gh"))
			for _, call := range runner.recorded("gh") {
				assert.Greater(t, call.timeout, 29*time.Second, call.argv)
				assert.LessOrEqual(t, call.timeout, 30*time.Second, "every gh call runs under its 30 s budget")
			}
			assert.Equal(t, 1, provider.count())
			assertBandITReadOnly(t, runner)
		})
	}
}

// S13: while gh run list hangs, band holds no store lock, so a concurrent
// auto canary appends its observation within 1 s; band then fails open.
func TestReactBandIT_S13_CanaryAppendsWhileGHRunListHangs(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	p.store(healthband.CIRunsFile, bandO2CI())
	runner := &bandITHangingRunner{fakeBandRunner: emptyRunList(), hung: "gh run list", started: make(chan struct{}), release: make(chan struct{})}
	var run bandCmdRun
	done := make(chan struct{})
	go func() {
		defer close(done)
		run = p.band(bandITRun{runner: runner, clock: fixedAt(bandITT0)}, "--format", "json")
	}()
	select {
	case <-runner.started:
	case <-time.After(30 * time.Second):
		require.FailNow(t, "gh run list never started")
	}

	begin := time.Now()
	appendErr := appendCanaryHistory(context.Background(), p.dir, "canary.failure_rate:local", 1)
	elapsed := time.Since(begin)

	close(runner.release)
	<-done
	require.NoError(t, appendErr)
	assert.Less(t, elapsed, time.Second, "the canary append never waits for band's network step")
	canary, _, err := healthband.NewStore(p.dir).ReadObservations(healthband.CanaryRunsFile)
	require.NoError(t, err)
	require.Len(t, canary, 1)
	assert.Equal(t, "c1", canary[0].SampleKey)
	assertBandITStillDiagnoses(t, p, run, healthband.ReasonGHFetchFailed)
	assertBandITReadOnly(t, runner.fakeBandRunner)
}

// S13: another process holding .autopus/metrics/.lock past the 5 s wait
// yields store_locked: no file under .autopus/ changes, no evaluation runs,
// no provider is called, and band exits 0.
func TestReactBandIT_S13_StoreLockedPastFiveSeconds(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	p.store(healthband.CIRunsFile, bandO2CI())
	holder, err := filelock.Acquire(context.Background(), p.metrics(healthband.LockFile), time.Second)
	require.NoError(t, err)
	defer func() { assert.NoError(t, holder.Unlock()) }()
	before := bandTreeHashes(t, p.dir)
	provider := &bandITProvider{}
	runner := emptyRunList()
	begin := time.Now()

	run := p.band(bandITRun{runner: runner, clock: fixedAt(bandITT0), provider: provider}, "--format", "json")

	require.NoError(t, run.err)
	assert.GreaterOrEqual(t, time.Since(begin), healthband.StoreLockWait)
	envelope := decodeBandEnvelope(t, run.stdout)
	assert.Equal(t, []string{healthband.ReasonStoreLocked}, envelope.Data.Reasons)
	assert.Empty(t, envelope.Checks, "no evaluation runs")
	assert.Equal(t, before, bandTreeHashes(t, p.dir))
	assert.NoFileExists(t, p.metrics(healthband.EventsFile))
	assert.Zero(t, provider.count())
	assertBandITReadOnly(t, runner)
}
