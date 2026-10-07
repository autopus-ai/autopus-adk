package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Integration scenarios of auto react band (SPEC-SIGMABAND-001 T16): each
// one drives the real command through a fake gh, a fake clock, and a fake
// provider call, then reads the files band wrote. Expected numbers come from
// acceptance.md or from an independent Python detector, never from the
// package under test.

// bandITConfig configures codex as orchestra.judge, so provider selection,
// the shared read-only projection, and the control check all run for real;
// only the provider call itself is faked. It has no health_band key.
const bandITConfig = bandCmdConfig + "orchestra:\n  judge: codex\n  providers:\n    codex:\n" +
	"      binary: codex\n      args: [exec, --json, --sandbox, workspace-write]\n"

// bandITT0 is the phase A time of the first run of every scenario.
var bandITT0 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func newBandITProject(t *testing.T) bandCmdProject {
	t.Helper()
	p := newBandCmdProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "autopus.yaml"), []byte(bandITConfig), 0o600))
	return p
}

// storeBytes writes a store file as is, without taking the lock.
func (p bandCmdProject) storeBytes(name string, data []byte) {
	p.t.Helper()
	require.NoError(p.t, os.MkdirAll(filepath.Dir(p.metrics(name)), 0o700))
	require.NoError(p.t, os.WriteFile(p.metrics(name), data, 0o600))
}

// bandITClock is a settable fake clock shared by the phases of one run.
type bandITClock struct {
	mu  sync.Mutex
	now time.Time
}

func newBandITClock(at time.Time) *bandITClock { return &bandITClock{now: at} }

func (c *bandITClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *bandITClock) Set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = at
}

// bandITProvider fakes the provider call: it counts calls, records each
// projected argv, and lets a scenario hold a call (hold runs inside it).
type bandITProvider struct {
	mu    sync.Mutex
	calls int
	argv  [][]string
	hold  func(call int)
}

func (f *bandITProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *bandITProvider) install(d *bandDiagnoser) {
	d.run = func(_ context.Context, _ orchestra.OrchestraConfig, provider orchestra.ProviderConfig, _ string) (*orchestra.ProviderResponse, error) {
		f.mu.Lock()
		f.calls++
		call, hold := f.calls, f.hold
		f.argv = append(f.argv, slices.Clone(provider.Args))
		f.mu.Unlock()
		if hold != nil {
			hold(call)
		}
		return &orchestra.ProviderResponse{Output: "### Summary\nThe flaky step failed.\n"}, nil
	}
}

// bandITRun is one band invocation: its runner, clock, and provider fake,
// the production store lock wait unless lockWait is set, and its context.
type bandITRun struct {
	runner   bandRunner
	clock    func() time.Time
	provider *bandITProvider
	lockWait time.Duration
	ctx      context.Context
}

// band runs the command in p. It never calls require, so concurrent
// scenarios may run it from their own goroutines.
func (p bandCmdProject) band(r bandITRun, args ...string) bandCmdRun {
	if r.provider == nil {
		r.provider = &bandITProvider{} // a configured codex must never run for real
	}
	if r.lockWait == 0 {
		r.lockWait = healthband.StoreLockWait
	}
	if r.ctx == nil {
		r.ctx = context.Background()
	}
	cmd := newReactBandCmdWith(reactBandDeps{runner: r.runner, clock: r.clock, lockWait: r.lockWait, prepare: func(d *bandDiagnoser) {
		d.bsOptions = brainstorm.Options{CacheDir: func() (string, error) { return p.cache, nil }}
		r.provider.install(d)
	}})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"--project-dir", p.dir}, args...))
	err := cmd.ExecuteContext(r.ctx)
	return bandCmdRun{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// fixedAt is a clock stopped at one instant.
func fixedAt(instant time.Time) func() time.Time { return func() time.Time { return instant } }

// bandITRows spells observations as gh run list rows of one workflow, newest
// first like gh prints them; value 1 is a failure.
func bandITRows(workflow string, observations []healthband.Observation) []map[string]any {
	rows := make([]map[string]any, 0, len(observations))
	for i := len(observations) - 1; i >= 0; i-- {
		observation := observations[i]
		conclusion := "success"
		if observation.Value == 1 {
			conclusion = "failure"
		}
		id, _ := strconv.ParseInt(observation.SampleKey, 10, 64)
		rows = append(rows, ghRunAt(id, workflow, "push", "main", "completed", conclusion, observation.Attempt, observation.ObservedAt))
	}
	return rows
}

// bandITRuns are CI observations of runs first..first+len(values)-1, run i
// created at the fixed origin plus i minutes.
func bandITRuns(first int64, values string) []healthband.Observation {
	observations := make([]healthband.Observation, len(values))
	for i := range values {
		observations[i] = ciObservationAt(first+int64(i), float64(values[i]-'0'))
	}
	return observations
}

// bandITGH answers the gh Invocation Table of acme/app with these rows.
func bandITGH(t *testing.T, rows ...map[string]any) *fakeBandRunner {
	t.Helper()
	return scriptedBandRunner(bandCmdOriginURL, "main", ghPayload(t, rows...))
}

func bandITEvents(t *testing.T, p bandCmdProject) (evaluations, results []healthband.Event) {
	t.Helper()
	data, err := os.ReadFile(p.metrics(healthband.EventsFile))
	require.NoError(t, err)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var event healthband.Event
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &event), scanner.Text())
		if event.Kind == healthband.EventKindEvaluation {
			evaluations = append(evaluations, event)
		} else {
			results = append(results, event)
		}
	}
	require.NoError(t, scanner.Err())
	return evaluations, results
}

func bandITState(t *testing.T, p bandCmdProject) healthband.Checkpoint {
	t.Helper()
	data, err := os.ReadFile(p.metrics(healthband.StateFile))
	require.NoError(t, err)
	var state healthband.Checkpoint
	require.NoError(t, json.Unmarshal(data, &state))
	return state
}

func bandITFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

// bandITHeads returns line 1 of every BS file in the project.
func bandITHeads(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".autopus", "brainstorms", "BS-BAND-*.md"))
	require.NoError(t, err)
	heads := make([]string, 0, len(matches))
	for _, match := range matches {
		head, _, _ := strings.Cut(string(bandITFile(t, match)), "\n")
		heads = append(heads, head)
	}
	return heads
}

// assertBandITReadOnly is the S14 recorder check, applied to every
// scenario's calls (S17 too): no argv starts with git worktree, git commit,
// git push, git stash, or gh pr, none runs react apply, and every gh api
// call is a GET (no method, field, or input flag).
func assertBandITReadOnly(t *testing.T, runner *fakeBandRunner) {
	t.Helper()
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for _, call := range runner.calls {
		argv := strings.Join(call.argv, " ")
		for _, forbidden := range []string{"git worktree", "git commit", "git push", "git stash", "gh pr"} {
			assert.False(t, strings.HasPrefix(argv, forbidden), argv)
		}
		assert.NotContains(t, argv, "react apply")
		if len(call.argv) > 1 && call.argv[0] == "gh" && call.argv[1] == "api" {
			for _, arg := range call.argv[2:] {
				write := strings.HasPrefix(arg, "-X") || strings.HasPrefix(arg, "--method") || strings.HasPrefix(arg, "--input") ||
					strings.HasPrefix(arg, "--field") || strings.HasPrefix(arg, "--raw-field") ||
					(strings.HasPrefix(arg, "-f") || strings.HasPrefix(arg, "-F")) && !strings.HasPrefix(arg, "--")
				assert.False(t, write, "gh api must stay a GET: %s", argv)
			}
		}
	}
}
