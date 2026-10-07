package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Run values of the O3 and O6 rows of acceptance S1: O3 has 20 zero baseline
// blocks and x = 0.75; O6 has the O5 baseline (0.25 ×4, 0.5 ×1, 0.0 ×15)
// and x = 1.00.
var (
	bandO3Values = strings.Repeat("0", 81) + "111"
	bandO6Values = strings.Repeat("1000", 4) + "1100" + strings.Repeat("0000", 15) + "1111"
)

// bandITRealGit runs git for real and answers gh from the fake; the fake
// records both, so the recorder sees every subprocess band started.
type bandITRealGit struct{ *fakeBandRunner }

func (r bandITRealGit) Run(ctx context.Context, command bandCommand) error {
	if command.Name != "git" {
		return r.fakeBandRunner.Run(ctx, command)
	}
	r.mu.Lock()
	r.calls = append(r.calls, fakeBandCall{argv: append([]string{command.Name}, command.Args...), env: command.Env, dir: command.Dir})
	r.mu.Unlock()
	return execBandRunner{}.Run(ctx, command)
}

// bandITRepo is a real repository with a dirty tree, a branch, a tag, a
// stash entry, and the generated .gitignore, whose origin is acme/app.
func bandITRepo(t *testing.T) (dir string, snapshot func() string) {
	t.Helper()
	dir = t.TempDir()
	initSyncRepo(t, dir)
	syncWrite(t, dir, ".gitignore", strings.Join(gitignorePatterns, "\n")+"\n")
	syncWrite(t, dir, "autopus.yaml", bandITConfig)
	syncWrite(t, dir, "README.md", "band\n")
	syncGit(t, dir, "add", ".gitignore", "autopus.yaml", "README.md")
	syncGit(t, dir, "commit", "-m", "project")
	syncGit(t, dir, "branch", "feature")
	syncGit(t, dir, "tag", "v1")
	syncGit(t, dir, "remote", "add", "origin", bandCmdOriginURL)
	syncWrite(t, dir, "README.md", "stashed\n")
	syncGit(t, dir, "stash", "push", "-m", "before band")
	syncWrite(t, dir, "README.md", "work in progress\n")
	return dir, func() string {
		var out strings.Builder
		for _, args := range [][]string{
			{"status", "--porcelain=v1", "--untracked-files=all"}, {"rev-parse", "HEAD"},
			{"for-each-ref", "--format=%(refname) %(objectname)"}, {"stash", "list"}, {"worktree", "list", "--porcelain"},
		} {
			out.WriteString(syncGitOut(t, dir, args...))
		}
		return out.String()
	}
}

// S14: tier 3 stays diagnosis-only. In a real repository and with an
// autopus.yaml without health_band, the O3 and O6 fixtures each record
// diagnose at tier 3 with a tier-3 BS, 2 read-only provider calls run, the
// recorder holds no git worktree, commit, push, stash, or gh pr call and
// only GET gh api calls, and status, HEAD, refs, stash list, and worktrees
// are byte-identical before and after.
func TestReactBandIT_S14_TierThreeNeverMutatesGitOrGitHub(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir, snapshot := bandITRepo(t)
	before := snapshot()
	require.Contains(t, before, "refs/stash")
	require.Contains(t, before, " M README.md")
	p := bandCmdProject{t: t, dir: dir, cache: t.TempDir()}
	fake := bandITGH(t, append(bandITRows("CI", bandITRuns(959, bandO3Values)), bandITRows("Lint", bandITRuns(2959, bandO6Values))...)...)
	provider := &bandITProvider{}

	run := p.band(bandITRun{runner: bandITRealGit{fake}, clock: fixedAt(bandITT0), provider: provider}, "--format", "json")

	require.NoError(t, run.err)
	envelope := decodeBandEnvelope(t, run.stdout)
	for series, want := range map[string]map[string]string{
		"ci.failure_rate:CI":   {"tier": "3", "action": "diagnose", "episode_id": "e1042", "bs_id": "BS-BAND-001", "reasons": "zero_variance"},
		"ci.failure_rate:Lint": {"tier": "3", "action": "diagnose", "episode_id": "e3042", "bs_id": "BS-BAND-002", "reasons": "variance_floor_applied"},
	} {
		check := envelope.check(t, "band."+series)
		assert.Equal(t, want, pick(check.Fields, "tier", "action", "episode_id", "bs_id", "reasons"), series)
		assert.Equal(t, "ok", check.Fields["diagnosis_status"], series)
	}
	lint := envelope.series(t, "ci.failure_rate:Lint")
	for key, want := range map[string]float64{"mu": 0.075, "sd": 0.142810, "sd_eff": 0.25, "z": 3.7} {
		assert.InDelta(t, want, lint[key], 5e-7, key)
	}
	assert.InDelta(t, 3, envelope.series(t, "ci.failure_rate:CI")["z"], 5e-7)
	assert.Equal(t, []string{
		"# BS-BAND-001: ci.failure_rate:CI tier 3 anomaly (e1042)",
		"# BS-BAND-002: ci.failure_rate:Lint tier 3 anomaly (e3042)",
	}, bandITHeads(t, dir))
	assert.Equal(t, 2, provider.count())
	for _, argv := range provider.argv {
		assert.Contains(t, strings.Join(argv, " "), "--sandbox read-only")
	}
	assertBandITReadOnly(t, fake)
	assert.Equal(t, []string{"git remote get-url origin"}, fake.argvs("git"), "the only git call is the read-only origin lookup")
	assert.Equal(t, before, snapshot())
}

// S15 end to end: CI runs arrive through the fake gh and canary results
// through the append auto canary uses. The envelope holds the expected
// values and leaves absent ones out; --dry-run on a fresh copy changes no
// file, takes no lock, and calls no provider; --series selects; an unknown
// series exits 0; an invalid flag or config exits non-zero.
func TestReactBandIT_S15_EnvelopeDryRunSeriesAndExitCodes(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	for i := range 80 {
		require.NoError(t, appendCanaryHistory(context.Background(), p.dir, "canary.failure_rate:local", float64(i/76)))
	}
	fresh := newBandITProject(t)
	fresh.storeBytes(healthband.CanaryRunsFile, bandITFile(t, p.metrics(healthband.CanaryRunsFile)))
	rows := bandITRows("CI", bandITRuns(959, bandO2Values))
	provider := &bandITProvider{}

	run := p.band(bandITRun{runner: bandITGH(t, rows...), clock: fixedAt(bandITT0), provider: provider}, "--format", "json")

	require.NoError(t, run.err)
	envelope := decodeBandEnvelope(t, run.stdout)
	ci := envelope.check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, map[string]string{
		"n": "20", "x": "0.5", "mu": "0", "sd_eff": "0.25", "z": "2", "tier": "2", "action": "diagnose", "episode_id": "e1042", "bs_id": "BS-BAND-001",
	}, pick(ci.Fields, "n", "x", "mu", "sd_eff", "z", "tier", "action", "episode_id", "bs_id"))
	canary := envelope.check(t, "band.canary.failure_rate:local")
	assert.Equal(t, map[string]string{"n": "19", "x": "1", "action": "log", "reasons": "insufficient_samples"},
		pick(canary.Fields, "n", "x", "action", "reasons", "mu", "sd", "sd_eff", "z", "tier"), "no mu, sd, sd_eff, z, or tier key")
	assert.Equal(t, 1, provider.count())

	before := bandTreeHashes(t, fresh.dir)
	dry := fresh.band(bandITRun{runner: bandITGH(t, rows...), clock: fixedAt(bandITT0), provider: provider}, "--dry-run", "--format", "json")
	require.NoError(t, dry.err)
	assert.Equal(t, before, bandTreeHashes(t, fresh.dir))
	assert.NoFileExists(t, fresh.metrics(healthband.LockFile))
	assert.Equal(t, 1, provider.count(), "--dry-run calls no provider")
	assert.Equal(t, "diagnose", decodeBandEnvelope(t, dry.stdout).check(t, "band.ci.failure_rate:CI").Fields["planned_action"])

	only := fresh.band(bandITRun{runner: bandITGH(t, rows...), clock: fixedAt(bandITT0), provider: provider}, "--series", "ci.failure_rate:CI", "--format", "json")
	require.NoError(t, only.err)
	checks := decodeBandEnvelope(t, only.stdout).Checks
	require.Len(t, checks, 1)
	assert.Equal(t, "band.ci.failure_rate:CI", checks[0].ID)
	evaluations, _ := bandITEvents(t, fresh)
	require.Len(t, evaluations, 1)
	assert.Equal(t, "ci.failure_rate:CI", evaluations[0].Series)

	nope := fresh.band(bandITRun{runner: &fakeBandRunner{}, clock: fixedAt(bandITT0), provider: provider}, "--no-fetch", "--series", "nope", "--format", "json")
	require.NoError(t, nope.err)
	assert.Equal(t, []string{healthband.ReasonSeriesNotFound}, decodeBandEnvelope(t, nope.stdout).Data.Reasons)
	assert.Error(t, fresh.band(bandITRun{runner: &fakeBandRunner{}, clock: fixedAt(bandITT0), provider: provider}, "--bogus").err)
	require.NoError(t, os.WriteFile(filepath.Join(fresh.dir, "autopus.yaml"), []byte(bandITConfig+"health_band:\n  allow_draft_pr: true\n"), 0o600))
	invalid := fresh.band(bandITRun{runner: &fakeBandRunner{}, clock: fixedAt(bandITT0), provider: provider}, "--no-fetch")
	require.Error(t, invalid.err)
	assert.Contains(t, invalid.err.Error(), "allow_draft_pr")
}
