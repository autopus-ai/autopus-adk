package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Fixtures of the auto react band command tests (T12).

var (
	bandCmdOrigin = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	bandCmdNow    = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
)

// bandCmdConfig configures no provider, so no command test can reach a real
// provider: every diagnosis is unavailable(provider_unconfigured).
const bandCmdConfig = "mode: full\nproject_name: band\nplatforms:\n  - claude-code\n"

const bandCmdOriginURL = "git@github.com:acme/app.git"

// bandCmdProject is a temp project with that autopus.yaml and its own
// per-user BS lock directory.
type bandCmdProject struct {
	t     *testing.T
	dir   string
	cache string
}

func newBandCmdProject(t *testing.T) bandCmdProject {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(bandCmdConfig), 0o600))
	return bandCmdProject{t: t, dir: dir, cache: t.TempDir()}
}

func (p bandCmdProject) metrics(name string) string {
	return filepath.Join(p.dir, ".autopus", "metrics", name)
}

// store writes observations as store lines without taking the lock, so a
// fresh copy holds no .lock file.
func (p bandCmdProject) store(name string, observations []healthband.Observation) {
	p.t.Helper()
	var buf bytes.Buffer
	for _, observation := range observations {
		line, err := json.Marshal(observation)
		require.NoError(p.t, err)
		buf.Write(append(line, '\n'))
	}
	require.NoError(p.t, os.MkdirAll(filepath.Dir(p.metrics(name)), 0o700))
	require.NoError(p.t, os.WriteFile(p.metrics(name), buf.Bytes(), 0o600))
}

// ciObservationAt is a CI observation of run id at origin + id minutes.
func ciObservationAt(runID int64, value float64) healthband.Observation {
	return healthband.Observation{
		Schema: healthband.SchemaObservation, Series: "ci.failure_rate:CI", SampleKey: strconv.FormatInt(runID, 10),
		ObservedAt: bandCmdOrigin.Add(time.Duration(runID) * time.Minute), Tiebreak: runID, Value: value,
		Attempt: 1, Source: healthband.SourceGH,
	}
}

// bandO2CI is the O2 fixture as ci.failure_rate:CI: runs 959 to 1042, 20 zero
// baseline blocks and a current block 0, 0, 1, 1 (x = 0.5, z = 2, e1042).
func bandO2CI() []healthband.Observation {
	observations := make([]healthband.Observation, 84)
	for i := range observations {
		observations[i] = ciObservationAt(int64(959+i), 0)
	}
	observations[82].Value, observations[83].Value = 1, 1
	return observations
}

// bandO4Canary is the O4 fixture as canary.failure_rate:local: c1 to c80, 19
// zero baseline blocks and a current block of four failures (n = 19, x = 1).
func bandO4Canary() []healthband.Observation {
	observations := make([]healthband.Observation, 80)
	for i := range observations {
		sequence := int64(i + 1)
		observations[i] = healthband.Observation{
			Schema: healthband.SchemaObservation, Series: "canary.failure_rate:local", SampleKey: "c" + strconv.FormatInt(sequence, 10),
			ObservedAt: bandCmdOrigin.Add(time.Duration(sequence) * time.Hour), Tiebreak: sequence, Attempt: 1, Source: healthband.SourceCanary,
		}
		if i >= 76 {
			observations[i].Value = 1
		}
	}
	return observations
}

// bandCmdRun is one execution of the band command.
type bandCmdRun struct {
	stdout string
	stderr string
	err    error
}

// run executes the band command with a fake runner, a fixed clock, a short
// store lock wait, and the per-user BS lock in the project's cache dir;
// prepare adjusts the diagnoser further.
func (p bandCmdProject) run(runner bandRunner, prepare func(*bandDiagnoser), args ...string) bandCmdRun {
	p.t.Helper()
	cmd := newReactBandCmdWith(reactBandDeps{
		runner: runner, clock: func() time.Time { return bandCmdNow }, lockWait: 100 * time.Millisecond,
		prepare: func(d *bandDiagnoser) {
			d.bsOptions = brainstorm.Options{CacheDir: func() (string, error) { return p.cache, nil }}
			if prepare != nil {
				prepare(d)
			}
		},
	})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"--project-dir", p.dir}, args...))
	err := cmd.ExecuteContext(context.Background())
	return bandCmdRun{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// bandCmdEnvelope is the part of the JSON envelope the tests read.
type bandCmdEnvelope struct {
	Status string      `json:"status"`
	Checks []jsonCheck `json:"checks"`
	Data   struct {
		DryRun   bool             `json:"dry_run"`
		Reasons  []string         `json:"reasons"`
		NotFound []string         `json:"not_found"`
		CI       map[string]any   `json:"ci"`
		Series   []map[string]any `json:"series"`
	} `json:"data"`
}

func decodeBandEnvelope(t *testing.T, stdout string) bandCmdEnvelope {
	t.Helper()
	var envelope bandCmdEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), stdout)
	return envelope
}

func (e bandCmdEnvelope) check(t *testing.T, id string) jsonCheck {
	t.Helper()
	for _, check := range e.Checks {
		if check.ID == id {
			return check
		}
	}
	require.Failf(t, "missing check", "%s in %+v", id, e.Checks)
	return jsonCheck{}
}

func (e bandCmdEnvelope) series(t *testing.T, id string) map[string]any {
	t.Helper()
	for _, series := range e.Data.Series {
		if series["series"] == id {
			return series
		}
	}
	require.Failf(t, "missing series", "%s in %+v", id, e.Data.Series)
	return nil
}

// bandTreeHashes returns the SHA-256 of every file under dir/.autopus.
func bandTreeHashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	hashes := make(map[string]string)
	root := filepath.Join(dir, ".autopus")
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		hashes[path] = hex.EncodeToString(sum[:])
		return nil
	}))
	return hashes
}

// bandCodexDiagnosis selects a configured codex and replaces the provider run
// with a fake that counts calls and records the projected argv.
func bandCodexDiagnosis(calls *int, argv *[]string) func(*bandDiagnoser) {
	return func(d *bandDiagnoser) {
		d.harness = bandHarness("codex", "", map[string]config.ProviderEntry{"codex": config.CodexProviderEntryForQuality(config.QualityConf{})})
		d.run = func(_ context.Context, _ orchestra.OrchestraConfig, provider orchestra.ProviderConfig, _ string) (*orchestra.ProviderResponse, error) {
			*calls++
			*argv = provider.Args
			return &orchestra.ProviderResponse{Output: "### Summary\nThe flaky step failed.\n"}, nil
		}
	}
}
