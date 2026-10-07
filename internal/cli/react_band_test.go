package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// newBandFixtureProject stores the O2 fixture as ci.failure_rate:CI and the
// O4 fixture as canary.failure_rate:local (S15, S20).
func newBandFixtureProject(t *testing.T) bandCmdProject {
	t.Helper()
	p := newBandCmdProject(t)
	p.store(healthband.CIRunsFile, bandO2CI())
	p.store(healthband.CanaryRunsFile, bandO4Canary())
	return p
}

func emptyRunList() *fakeBandRunner { return scriptedBandRunner(bandCmdOriginURL, "main", "[]") }

// S20 (help part) and REQ-21: the command is registered under auto react,
// reads the T15 help text, and exposes every REQ-14 and REQ-19 flag.
func TestReactBand_RegisteredUnderReactWithHelpAndFlags(t *testing.T) {
	t.Parallel()
	cmd := newReactCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"band", "--help"})

	require.NoError(t, cmd.Execute())

	help := out.String()
	for _, want := range []string{
		"z = (x - μ) / max(sd, 1/K)", "K=4", "W=30", "N_min=20", "Tier 3 is diagnosis-only", "cron", "/auto schedule",
		"--project-dir", "--no-fetch", "--no-agent", "--dry-run", "--series", "--limit", "--json", "--format",
	} {
		assert.Contains(t, help, want)
	}
	band, _, err := newReactCmd().Find([]string{"band"})
	require.NoError(t, err)
	assert.Equal(t, reactBandShort, band.Short)
}

// S15: the JSON envelope holds one band.<series> check per series with the
// expected values; absent values are absent keys, and the claim ran.
func TestReactBand_JSONEnvelopeReportsEachSeries(t *testing.T) {
	t.Parallel()
	p := newBandFixtureProject(t)

	run := p.run(emptyRunList(), nil, "--format", "json")

	require.NoError(t, run.err)
	envelope := decodeBandEnvelope(t, run.stdout)
	assert.Equal(t, "warn", envelope.Status)
	ci := envelope.check(t, "band.ci.failure_rate:CI")
	for key, want := range map[string]string{
		"n": "20", "x": "0.5", "mu": "0", "sd": "0", "sd_eff": "0.25", "z": "2", "tier": "2", "action": "diagnose",
		"episode_id": "e1042", "bs_id": "BS-BAND-001", "claim_status": "done", "diagnosis_status": "unavailable(provider_unconfigured)",
	} {
		assert.Equal(t, want, ci.Fields[key], key)
	}
	assert.Equal(t, "warn", ci.Status)
	canary := envelope.check(t, "band.canary.failure_rate:local")
	for key, want := range map[string]string{"n": "19", "x": "1", "action": "log", "reasons": "insufficient_samples"} {
		assert.Equal(t, want, canary.Fields[key], key)
	}
	assert.Equal(t, "pass", canary.Status)
	data := envelope.series(t, "canary.failure_rate:local")
	assert.InDelta(t, 19, data["n"], 0)
	assert.InDelta(t, 1, data["x"], 0)
	for _, key := range []string{"mu", "sd", "sd_eff", "z", "tier"} {
		assert.NotContains(t, canary.Fields, key)
		assert.NotContains(t, data, key)
	}
	assert.InDelta(t, 0.25, envelope.series(t, "ci.failure_rate:CI")["sd_eff"], 1e-12)
	assert.FileExists(t, filepath.Join(p.dir, ".autopus", "brainstorms", "BS-BAND-001.md"))
}

// S15 and REQ-14: --dry-run writes nothing, takes no lock, and calls no
// provider; the same project without --dry-run calls the provider once.
func TestReactBand_DryRunWritesNothingAndCallsNoProvider(t *testing.T) {
	t.Parallel()
	p := newBandFixtureProject(t)
	before := bandTreeHashes(t, p.dir)
	calls, argv := 0, []string(nil)

	run := p.run(emptyRunList(), bandCodexDiagnosis(&calls, &argv), "--dry-run", "--format", "json")

	require.NoError(t, run.err)
	assert.Equal(t, before, bandTreeHashes(t, p.dir))
	assert.NoFileExists(t, p.metrics(healthband.LockFile))
	assert.Zero(t, calls)
	envelope := decodeBandEnvelope(t, run.stdout)
	assert.True(t, envelope.Data.DryRun)
	ci := envelope.check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, "diagnose", ci.Fields["planned_action"])
	assert.Equal(t, "planned", ci.Fields["claim_status"])
	assert.NotContains(t, ci.Fields, "action")
	assert.NotContains(t, ci.Fields, "bs_id")

	run = p.run(emptyRunList(), bandCodexDiagnosis(&calls, &argv), "--format", "json")

	require.NoError(t, run.err)
	assert.Equal(t, 1, calls)
	assert.Contains(t, strings.Join(argv, " "), "--sandbox read-only")
	assert.Equal(t, "ok", decodeBandEnvelope(t, run.stdout).check(t, "band.ci.failure_rate:CI").Fields["diagnosis_status"])
}

// S20 and REQ-20: text rows are sorted by series ID, and the CI row reads
// exactly as the acceptance oracle states.
func TestReactBand_TextRowsAreSortedBySeries(t *testing.T) {
	t.Parallel()
	p := newBandFixtureProject(t)

	run := p.run(emptyRunList(), nil)

	require.NoError(t, run.err)
	lines := strings.Split(run.stdout, "\n")
	canaryAt := slices.Index(lines, "canary.failure_rate:local n=19/20 x=1.000000 μ=- sd_eff=- z=- tier=- action=log episode=-")
	ciAt := slices.Index(lines, "ci.failure_rate:CI n=20/20 x=0.500000 μ=0.000000 sd_eff=0.250000 z=2.000000 tier=2 action=diagnose episode=e1042")
	require.GreaterOrEqual(t, canaryAt, 0, run.stdout)
	assert.Greater(t, ciAt, canaryAt, run.stdout)
	assert.Equal(t, []string{"  reasons: zero_variance", "  claim: diagnose 1042 done bs=BS-BAND-001 diagnosis=unavailable(provider_unconfigured)"},
		lines[ciAt+1:ciAt+3], "the layout docs/health-band.md shows")
	assert.Equal(t, "  reasons: insufficient_samples", lines[canaryAt+1])
	assert.Equal(t, "ci: 0 runs, 0 trusted, 0 new", lines[0])
}

// S20 and REQ-19: --limit reaches gh; values outside 1 to 1000 are invalid
// flags that exit non-zero before any command or file write.
func TestReactBand_LimitReachesGHAndRejectsOutOfRange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		want string
	}{{nil, "--limit 200 "}, {[]string{"--limit", "50"}, "--limit 50 "}} {
		runner := emptyRunList()
		run := newBandCmdProject(t).run(runner, nil, tc.args...)
		require.NoError(t, run.err)
		list := runner.argvs("gh")
		require.Len(t, list, 3)
		assert.Contains(t, list[2], "gh run list -R acme/app "+tc.want)
	}
	for _, args := range [][]string{{"--limit", "0"}, {"--limit", "1001"}, {"--no-fetch", "--limit", "0"}} {
		p := newBandCmdProject(t)
		runner := emptyRunList()
		run := p.run(runner, nil, args...)
		require.Error(t, run.err, args)
		assert.Empty(t, runner.calls, args)
		assert.NoDirExists(t, filepath.Join(p.dir, ".autopus"), args)
	}
}

// S15: --series evaluates only that series; an unknown series exits 0 with
// reason series_not_found.
func TestReactBand_SeriesFilterSelectsOneSeries(t *testing.T) {
	t.Parallel()
	p := newBandFixtureProject(t)

	run := p.run(emptyRunList(), nil, "--series", "ci.failure_rate:CI", "--format", "json")

	require.NoError(t, run.err)
	envelope := decodeBandEnvelope(t, run.stdout)
	require.Len(t, envelope.Checks, 1)
	assert.Equal(t, "band.ci.failure_rate:CI", envelope.Checks[0].ID)
	events, err := os.ReadFile(p.metrics(healthband.EventsFile))
	require.NoError(t, err)
	assert.NotContains(t, string(events), "canary.failure_rate:local")

	run = p.run(emptyRunList(), nil, "--series", "nope", "--format", "json")

	require.NoError(t, run.err)
	envelope = decodeBandEnvelope(t, run.stdout)
	assert.Equal(t, []string{"series_not_found"}, envelope.Data.Reasons)
	assert.Equal(t, []string{"nope"}, envelope.Data.NotFound)
	assert.Empty(t, envelope.Checks)
	assert.Equal(t, "warn", envelope.Status)

	// An unknown ID is echoed only through the identifier filter.
	text := p.run(emptyRunList(), nil, "--series", "bad\x1b[2Jname")
	require.NoError(t, text.err)
	assert.Contains(t, text.stdout, "reason: series_not_found bad2Jname#")
	assert.NotContains(t, text.stdout, "\x1b")
}

// REQ-14: invalid flags and an invalid autopus.yaml exit non-zero before any
// git, gh, or store access.
func TestReactBand_InvalidInvocationExitsNonZero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, config, want string
		args               []string
	}{
		{name: "unknown flag", args: []string{"--bogus"}, want: "bogus"},
		{name: "unknown format", args: []string{"--format", "yaml"}, want: "yaml"},
		{name: "positional argument", args: []string{"extra"}, want: "extra"},
		{name: "missing project dir", args: []string{"--project-dir", "missing"}, want: "not a directory"},
		{name: "allow_draft_pr", config: bandCmdConfig + "health_band:\n  allow_draft_pr: true\n", want: "allow_draft_pr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newBandFixtureProject(t)
			if tc.config != "" {
				require.NoError(t, os.WriteFile(filepath.Join(p.dir, "autopus.yaml"), []byte(tc.config), 0o600))
			}
			args := tc.args
			if tc.name == "missing project dir" {
				args = []string{"--project-dir", filepath.Join(p.dir, "missing")}
			}
			before := bandTreeHashes(t, p.dir)
			runner := emptyRunList()

			run := p.run(runner, nil, args...)

			require.Error(t, run.err)
			assert.Contains(t, run.err.Error(), tc.want)
			assert.Empty(t, runner.calls)
			assert.Equal(t, before, bandTreeHashes(t, p.dir))
			assert.NoDirExists(t, filepath.Join(p.dir, "missing"))
		})
	}
}

// S13 (CLI part): a missing gh skips CI ingest with gh_missing, and band
// still evaluates the stored series to tier 2 and writes the BS.
func TestReactBand_MissingGHStillEvaluatesStoredSeries(t *testing.T) {
	t.Parallel()
	p := newBandFixtureProject(t)
	runner := &fakeBandRunner{noGH: true, answers: map[string]fakeBandAnswer{"git remote get-url": {stdout: bandCmdOriginURL + "\n"}}}

	run := p.run(runner, nil, "--format", "json")

	require.NoError(t, run.err)
	envelope := decodeBandEnvelope(t, run.stdout)
	assert.Equal(t, []string{"gh_missing"}, envelope.Data.Reasons)
	ci := envelope.check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, "2", ci.Fields["tier"])
	assert.Equal(t, "BS-BAND-001", ci.Fields["bs_id"])
	assert.Empty(t, runner.argvs("gh"))
}
