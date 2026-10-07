package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Shared fixtures of the diagnose tests (S12, S19, S20).

const (
	bandDiagnoseOwner  = "test-host:4242:0123456789abcdef"
	bandDiagnoseSeries = "ci.failure_rate:CI"
)

var bandDiagnoseT0 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// bandDiagnoseFixture is one tier-2 diagnose claim that a real phase A
// planned over the O2 fixture of ci.failure_rate:CI (20 zero baseline blocks
// and a current block 0, 0, 1, 1, so x = 0.50 and z = 2), ending at sample
// key 1042 in episode e1042, inside a temp project.
type bandDiagnoseFixture struct {
	projectDir string
	cacheDir   string
	claim      healthband.DueClaim
}

func newBandDiagnoseFixture(t *testing.T) bandDiagnoseFixture {
	t.Helper()
	dir := t.TempDir()
	origin := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	observations := make([]healthband.Observation, 84)
	for i := range observations {
		runID := int64(1042 - 83 + i)
		observations[i] = healthband.Observation{
			Schema: healthband.SchemaObservation, Series: bandDiagnoseSeries, SampleKey: strconv.FormatInt(runID, 10),
			ObservedAt: origin.Add(time.Duration(runID) * time.Minute), Tiebreak: runID, Attempt: 1, Source: healthband.SourceGH,
		}
	}
	observations[82].Value, observations[83].Value = 1, 1
	locked, err := healthband.NewStore(dir).Lock(context.Background(), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, locked.Unlock()) }()
	_, err = locked.MergeObservations(healthband.CIRunsFile, observations)
	require.NoError(t, err)
	wal, err := locked.OpenWAL(bandDiagnoseT0)
	require.NoError(t, err)
	series, _, err := locked.Store().ReadSeries()
	require.NoError(t, err)
	plan, err := wal.Plan(series, healthband.PlanOptions{Owner: bandDiagnoseOwner})
	require.NoError(t, err)
	require.NoError(t, wal.Commit(plan))
	require.Len(t, plan.Claims, 1)
	require.Equal(t, "e1042", plan.Claims[0].EpisodeID)
	require.Equal(t, 2, plan.Claims[0].Tier)
	return bandDiagnoseFixture{projectDir: dir, cacheDir: t.TempDir(), claim: plan.Claims[0]}
}

// diagnoser returns the production diagnoser of the fixture with a fixed
// clock and the per-user BS lock in a temp cache dir.
func (f bandDiagnoseFixture) diagnoser(harness *config.HarnessConfig, evidence bandEvidenceSource) *bandDiagnoser {
	d := newBandDiagnoser(f.projectDir, harness, false, evidence)
	d.now = func() time.Time { return bandDiagnoseT0 }
	d.bsOptions = brainstorm.Options{CacheDir: func() (string, error) { return f.cacheDir, nil }}
	return d
}

// bsFiles returns the names of the BS files written in the project.
func (f bandDiagnoseFixture) bsFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(f.projectDir, ".autopus", "brainstorms", "BS-BAND-*.md"))
	require.NoError(t, err)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, filepath.Base(match))
	}
	return names
}

func (f bandDiagnoseFixture) bs(t *testing.T, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.projectDir, ".autopus", "brainstorms", id+".md"))
	require.NoError(t, err)
	return string(data)
}

// bandHarness is an autopus.yaml with only the keys provider selection reads.
func bandHarness(judge, diagnosisProvider string, providers map[string]config.ProviderEntry) *config.HarnessConfig {
	return &config.HarnessConfig{
		Orchestra:  config.OrchestraConf{Judge: judge, Providers: providers},
		HealthBand: config.HealthBandConf{DiagnosisProvider: diagnosisProvider},
	}
}

// bandFakeProviderScript stands in for every provider binary: it records its
// call, argv, cwd, and stdin under BAND_FAKE_LOG and answers by BAND_FAKE_MODE.
const bandFakeProviderScript = `#!/bin/sh
name=$(basename "$0")
echo "$name" >> "$BAND_FAKE_LOG/calls"
printf '%s\n' "$@" > "$BAND_FAKE_LOG/$name.argv"
pwd -P > "$BAND_FAKE_LOG/$name.cwd"
cat > "$BAND_FAKE_LOG/$name.stdin"
case "$BAND_FAKE_MODE" in
sleep) exec sleep 5 ;;
exit3) echo partial; exit 3 ;;
blank) printf '  \n\t\n' ;;
*) printf '### Summary\nThe flaky step failed.\n' ;;
esac
`

// bandFakeProviders is the record of the fake provider binaries.
type bandFakeProviders struct{ logs string }

// installBandFakeProviders makes fake claude, codex, agy, omp, and opencode
// the only provider binaries on PATH, so no test reaches a real provider.
func installBandFakeProviders(t *testing.T, names ...string) bandFakeProviders {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake provider binaries are POSIX shell scripts")
	}
	bin, logs := t.TempDir(), t.TempDir()
	if len(names) == 0 {
		names = []string{"claude", "codex", "agy", "omp", "opencode"}
	}
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(bandFakeProviderScript), 0o755))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
	t.Setenv("BAND_FAKE_LOG", logs)
	t.Setenv("BAND_FAKE_MODE", "ok")
	return bandFakeProviders{logs: logs}
}

// calls returns the binaries that ran, in order.
func (p bandFakeProviders) calls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.logs, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Fields(string(data))
}

func (p bandFakeProviders) record(t *testing.T, name, kind string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.logs, name+"."+kind))
	require.NoError(t, err)
	return string(data)
}

// argv returns the argv one fake binary received.
func (p bandFakeProviders) argv(t *testing.T, name string) []string {
	t.Helper()
	return strings.Split(strings.TrimSuffix(p.record(t, name, "argv"), "\n"), "\n")
}

// bandFakeBackend is a registered OMP backend that records its requests.
type bandFakeBackend struct {
	mu       sync.Mutex
	requests []orchestra.ProviderRequest
	response orchestra.ProviderResponse
}

func (b *bandFakeBackend) Execute(_ context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = append(b.requests, req)
	response := b.response
	return &response, nil
}

func (*bandFakeBackend) Name() string { return config.ProviderBackendOMP }

func (b *bandFakeBackend) calls() []orchestra.ProviderRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]orchestra.ProviderRequest(nil), b.requests...)
}

// bandStaticEvidence returns the same sanitized evidence for every claim.
type bandStaticEvidence struct {
	logs    []healthband.RunLog
	reports []healthband.ReactReport
}

func (e bandStaticEvidence) Evidence(context.Context, healthband.DueClaim) ([]healthband.RunLog, []healthband.ReactReport) {
	return e.logs, e.reports
}

// bandS19Evidence is the S19 evidence: run 4242 attempt 1 holds a synthetic
// ghp_ token, run 4243 attempt 2 is clean.
func bandS19Evidence(projectDir string) bandStaticEvidence {
	return bandStaticEvidence{logs: []healthband.RunLog{
		{RunID: 4242, Attempt: 1, Evidence: healthband.SanitizeCILog("using ghp_"+strings.Repeat("A", 36)+" for auth\nstep 3 failed\n", false, projectDir)},
		{RunID: 4243, Attempt: 2, Evidence: healthband.SanitizeCILog("step 9 failed: exit 2\n", false, projectDir)},
	}}
}
