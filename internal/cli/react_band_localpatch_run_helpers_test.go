//go:build unix

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Fixtures of the run wiring tests (SPEC-SIGMABAND-002 plan task T8): the
// band command over plan task T7's fixture repository as the project, with
// a stored series, a recorder around production's diagnose side
// ((*bandDiagnoser).enableLocalPatch), and the fake claude binary that
// answers both confined requests.

// bandLPConfig turns the flag on with health_band.local_patch_provider
// claude over this repository's OMP claude entry, so the patch request asks
// for claude-opus-5-5. Its judge is a subprocess codex, so 001's diagnosis,
// which runs unchanged in this slice, can run on a faked provider call.
const bandLPConfig = bandITConfig + "    claude:\n      backend: omp\n      model: anthropic/claude-opus-5-5:max\n" +
	"health_band:\n  allow_local_patch: true\n  local_patch_provider: claude\n"

// bandLPT0 is the phase A time of every wiring run.
var bandLPT0 = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

// bandLPEnable records what the run hands over to the diagnose side and
// delegates to production's (*bandDiagnoser).enableLocalPatch, whose
// AfterRecord hook it wraps to record each call. With reports set it stands
// in for the diagnose side instead: Run stays 001's and the hook only
// injects results, so the run's own report plumbing is tested alone.
type bandLPEnable struct {
	calls   int
	patcher *bandLocalPatcher
	plan    []healthband.LocalPatchRecord
	order   *[]string // "enable" and "prepare" in call order
	after   []string  // the claim id and BS ID of every AfterRecord call
	reports func(report func(healthband.LocalPatchRecord), claim healthband.DueClaim)
	during  func(claim healthband.DueClaim) // runs first inside AfterRecord
}

func (f *bandLPEnable) enable(d *bandDiagnoser, patcher *bandLocalPatcher, plan []healthband.LocalPatchRecord, report func(healthband.LocalPatchRecord)) func(context.Context, healthband.DueClaim, healthband.ClaimOutcome, healthband.Recorded) {
	f.calls++
	f.patcher, f.plan = patcher, plan
	*f.order = append(*f.order, "enable")
	var after func(context.Context, healthband.DueClaim, healthband.ClaimOutcome, healthband.Recorded)
	if f.reports == nil {
		after = d.enableLocalPatch(patcher, plan, report)
	}
	return func(ctx context.Context, claim healthband.DueClaim, outcome healthband.ClaimOutcome, recorded healthband.Recorded) {
		f.after = append(f.after, claim.ID+" "+outcome.BSID)
		if f.during != nil {
			f.during(claim)
		}
		if f.reports != nil {
			f.reports(report, claim)
			return
		}
		after(ctx, claim, outcome, recorded)
	}
}

// bandLPWorld is one flag-on band world: T7's fixture repository holds the
// project, its autopus.yaml, and its metric store.
type bandLPWorld struct {
	*lpFixture
	project  bandCmdProject
	fake     lpFakeClaude
	enable   *bandLPEnable
	order    []string
	runner   *fakeBandRunner
	provider *bandITProvider // 001's provider call
	workDirs []string        // the working directory of each 001 provider call
	ids      []string        // local_patch claim ids, drawn in order
	deps     func(*bandLocalPatchDeps)
}

func newBandLPWorld(t *testing.T, config string) *bandLPWorld {
	t.Helper()
	fake := installLPFakeClaude(t)
	f := newLPFixture(t, nil)
	w := &bandLPWorld{
		lpFixture: f, project: bandCmdProject{t: t, dir: f.repo, cache: t.TempDir()}, fake: fake,
		runner: scriptedBandRunner(bandCmdOriginURL, "main", "[]"), provider: &bandITProvider{},
		ids: []string{lpPatchClaimID, "b2c3d4e5f60708a1b2c3d4e5f60708a1"},
	}
	w.enable = &bandLPEnable{order: &w.order}
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, "autopus.yaml"), []byte(config), 0o600))
	fake.setStream(t, lpStream(lpInit55, lpAssistant55, lpResult(lpReplyWith(lpFooDiff))))
	return w
}

// storeO3 stores the O3 fixture (key 1042 opens e1042 at tier 3).
func (w *bandLPWorld) storeO3() {
	w.project.store(healthband.CIRunsFile, bandITRuns(959, bandO3Values))
}

// band runs the command in the repository with the world's seams; 001's
// provider call is faked, as in the 001 integration tests.
func (w *bandLPWorld) band(args ...string) bandCmdRun {
	w.t.Helper()
	lp := bandLocalPatchDeps{
		enable: w.enable.enable, git: w.patcher.git, cacheDir: w.cacheDir, recoveryWait: 100 * time.Millisecond,
		newClaimID: func() string { id := w.ids[0]; w.ids = w.ids[1:]; return id },
		patcher: func(p *bandLocalPatcher) {
			p.statfs = func(string) (lpDiskSpace, error) { return lpDiskSpace{avail: 1 << 40, unit: 4096}, nil }
		},
	}
	if w.deps != nil {
		w.deps(&lp)
	}
	cmd := newReactBandCmdWith(reactBandDeps{
		runner: w.runner, clock: fixedAt(bandLPT0), lockWait: 100 * time.Millisecond, localPatch: lp,
		prepare: func(d *bandDiagnoser) {
			w.order = append(w.order, "prepare")
			d.bsOptions = brainstorm.Options{CacheDir: func() (string, error) { return w.project.cache, nil }}
			d.backends = func(orchestra.OrchestraConfig) map[string]orchestra.ExecutionBackend { return nil }
			w.provider.install(d)
			run := d.run
			d.run = func(ctx context.Context, cfg orchestra.OrchestraConfig, provider orchestra.ProviderConfig, prompt string) (*orchestra.ProviderResponse, error) {
				w.workDirs = append(w.workDirs, provider.WorkDir)
				return run(ctx, cfg, provider, prompt)
			}
		},
	})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"--project-dir", w.repo}, args...))
	err := cmd.ExecuteContext(context.Background())
	return bandCmdRun{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// records are the local patch records of the project's store.
func (w *bandLPWorld) records() []healthband.LocalPatchRecord {
	w.t.Helper()
	log, err := healthband.NewStore(w.repo).ReadLocalPatchLog()
	require.NoError(w.t, err)
	return log.Records
}

// trail spells the records as kind or stage:phase.
func (w *bandLPWorld) trail() []string {
	var trail []string
	for _, record := range w.records() {
		if record.Kind == healthband.LocalPatchKindStage {
			trail = append(trail, "stage:"+record.Phase)
			continue
		}
		trail = append(trail, record.Kind)
	}
	return trail
}

func (w *bandLPWorld) record(kind string) healthband.LocalPatchRecord {
	w.t.Helper()
	for _, record := range w.records() {
		if record.Kind == kind {
			return record
		}
	}
	w.t.Fatalf("no %s record", kind)
	return healthband.LocalPatchRecord{}
}

// diagnoseClaim is the diagnose claim that 001's events hold for the series.
func (w *bandLPWorld) diagnoseClaim(series string) healthband.Claim {
	w.t.Helper()
	evaluations, _ := bandITEvents(w.t, w.project)
	for _, event := range evaluations {
		if event.Series == series && len(event.Claims) == 1 {
			return event.Claims[0]
		}
	}
	w.t.Fatalf("no diagnose claim of %s", series)
	return healthband.Claim{}
}

// bandLPEnvelope is the local_patches[] part of the JSON envelope.
type bandLPEnvelope struct {
	Status string `json:"status"`
	Data   struct {
		Reasons      []string               `json:"reasons"`
		LocalPatches []bandLocalPatchReport `json:"local_patches"`
	} `json:"data"`
	raw map[string]any
}

func decodeBandLPEnvelope(t *testing.T, stdout string) bandLPEnvelope {
	t.Helper()
	var envelope bandLPEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), stdout)
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope.raw))
	return envelope
}

// worktrees is the user repository's git worktree list.
func (w *bandLPWorld) worktrees() string { return w.git(w.repo, "worktree", "list", "--porcelain") }
