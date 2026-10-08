//go:build unix

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Fixtures of the executor tests: a user checkout with a bare origin whose
// main matches refs/remotes/origin/main, a HOME for band git, a temp user
// cache directory, and plan task T2's store and <lp> behind the executor,
// with a ledger that can inject a write error.

const (
	lpSeries          = "ci.failure_rate:CI"
	lpPatchClaimID    = "a1b2c3d4e5f60708a1b2c3d4e5f60708"
	lpDiagnoseClaimID = "0f1e2d3c4b5a69780f1e2d3c4b5a6978"
	lpKey             = "ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4"
	lpFooGo           = "package foo\n\nfunc Foo() int {\n\treturn 1\n}\n"
)

var lpT0 = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

var errLPInjected = errors.New("injected write error")

// lpLedger is T2's store as the executor's ledger, except that a prep, a
// stage of failPhase, or, with failResult, a result fails as a write error.
type lpLedger struct {
	t          *testing.T
	store      *healthband.Store
	failPrep   bool
	failPhase  string
	failResult bool
}

func (l *lpLedger) AppendLocalPatchPrep(ctx context.Context, prep healthband.LocalPatchRecord) (bool, error) {
	if l.failPrep {
		return false, errLPInjected
	}
	return l.store.AppendLocalPatchPrep(ctx, prep)
}

func (l *lpLedger) AppendLocalPatchStage(ctx context.Context, stage healthband.LocalPatchRecord) error {
	if stage.Phase == l.failPhase {
		return errLPInjected
	}
	return l.store.AppendLocalPatchStage(ctx, stage)
}

func (l *lpLedger) AppendLocalPatchResult(ctx context.Context, result healthband.LocalPatchRecord) (bool, error) {
	if l.failResult {
		return false, errLPInjected
	}
	return l.store.AppendLocalPatchResult(ctx, result)
}

// records are the store's records in seq order.
func (l *lpLedger) records() []healthband.LocalPatchRecord {
	l.t.Helper()
	log, err := l.store.ReadLocalPatchLog()
	require.NoError(l.t, err)
	return log.Records
}

// trail is the record sequence as kind or stage:phase, so tests compare order.
func (l *lpLedger) trail() []string {
	var trail []string
	for _, record := range l.records() {
		if record.Kind == healthband.LocalPatchKindStage {
			trail = append(trail, "stage:"+record.Phase)
			continue
		}
		trail = append(trail, record.Kind)
	}
	return trail
}

// find returns the first record of kind (and phase for a stage).
func (l *lpLedger) find(kind, phase string) healthband.LocalPatchRecord {
	l.t.Helper()
	for _, record := range l.records() {
		if record.Kind == kind && (phase == "" || record.Phase == phase) {
			return record
		}
	}
	l.t.Fatalf("no %s %s record", kind, phase)
	return healthband.LocalPatchRecord{}
}

func (l *lpLedger) prep() healthband.LocalPatchRecord {
	return l.find(healthband.LocalPatchKindPrep, "")
}

func (l *lpLedger) stage(phase string) healthband.LocalPatchRecord {
	return l.find(healthband.LocalPatchKindStage, phase)
}

func (l *lpLedger) result() healthband.LocalPatchRecord {
	return l.find(healthband.LocalPatchKindResult, "")
}

// lpUnsealed drops the fields that the append sets, so a returned record
// compares with its stored copy.
func lpUnsealed(record healthband.LocalPatchRecord) healthband.LocalPatchRecord {
	record.Schema, record.Seq = "", 0
	return record
}

// lpFixture is the temp world of one executor test.
type lpFixture struct {
	t        *testing.T
	root     string
	repo     string
	base     string
	cacheDir string
	setupEnv []string
	ledger   *lpLedger
	patcher  *bandLocalPatcher
}

func newLPFixture(t *testing.T, harness *config.HarnessConfig) *lpFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	f := &lpFixture{t: t, root: root, repo: filepath.Join(root, "repo"), cacheDir: filepath.Join(root, "cache")}
	f.setupEnv = f.home("setup-home", "[user]\n\tname = Setup\n\temail = setup@example.invalid\n[init]\n\tdefaultBranch = main\n")
	bandEnv := f.home("band-home", "")
	require.NoError(t, os.MkdirAll(f.cacheDir, 0o700))
	f.git(root, "init", "-q", "--bare", filepath.Join(root, "origin.git"))
	f.git(root, "init", "-q", f.repo)
	f.write("pkg/foo/foo.go", lpFooGo)
	f.write("README.md", "readme\n")
	f.git(f.repo, "add", "-A")
	f.git(f.repo, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "base")
	f.git(f.repo, "remote", "add", "origin", filepath.Join(root, "origin.git"))
	f.git(f.repo, "push", "-q", "-u", "origin", "main")
	f.git(f.repo, "remote", "set-head", "origin", "main")
	f.base = strings.TrimSpace(f.git(f.repo, "rev-parse", "HEAD"))
	runner := healthband.GitPolicyRunner{Environ: func() []string { return slices.Clone(bandEnv) }}
	f.ledger = &lpLedger{t: t, store: healthband.NewStore(filepath.Join(root, "project"))}
	if harness == nil {
		harness = lpHarness("claude", "", "", nil)
	}
	f.patcher = newBandLocalPatcher(f.repo, harness, f.location(f.cacheDir, runner), f.ledger)
	f.patcher.git, f.patcher.now = runner, func() time.Time { return lpT0 }
	f.patcher.statfs = func(string) (lpDiskSpace, error) { return lpDiskSpace{avail: 1 << 40, unit: 4096}, nil }
	// Open creates <lp>, so a test can place artifacts there first.
	dir, code, err := f.patcher.location.Open(context.Background(), runner.In(f.repo))
	require.NoError(t, err)
	require.Empty(t, code)
	require.NoError(t, dir.Close())
	return f
}

// location resolves <lp> of the fixture's checkout below cacheDir.
func (f *lpFixture) location(cacheDir string, runner healthband.GitPolicyRunner) *healthband.LocalPatchLocation {
	f.t.Helper()
	loc, err := healthband.ResolveLocalPatchLocation(context.Background(), runner.In(f.repo), cacheDir)
	require.NoError(f.t, err)
	return &loc
}

// home writes a HOME with a .gitconfig and returns its environment.
func (f *lpFixture) home(name, gitconfig string) []string {
	home := filepath.Join(f.root, name)
	require.NoError(f.t, os.MkdirAll(filepath.Join(home, ".config"), 0o700))
	require.NoError(f.t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitconfig), 0o600))
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "TMPDIR=" + os.TempDir()}
}

func (f *lpFixture) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, f.setupEnv
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(f.t, cmd.Run(), "git %s: %s", strings.Join(args, " "), stderr.String())
	return stdout.String()
}

func (f *lpFixture) write(name, content string) {
	path := filepath.Join(f.repo, name)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(content), 0o644))
}

// lp is the absolute <lp> of the fixture.
func (f *lpFixture) lp() string { return f.patcher.location.Path }

// absent reports that nothing is at <lp>/<name>.
func (f *lpFixture) absent(name string) bool {
	_, err := os.Lstat(filepath.Join(f.lp(), name))
	return os.IsNotExist(err)
}

// target is the S4 tier-3 diagnose claim with its local_patch claim.
func (f *lpFixture) target() localPatchTarget {
	z, tier := 4.2, 3
	diagnoseClaim := healthband.Claim{ID: lpDiagnoseClaimID, Kind: healthband.ClaimKindDiagnose, Owner: bandDiagnoseOwner, LeaseUntil: lpT0.Add(990 * time.Second)}
	event := healthband.Event{
		Schema: healthband.SchemaBandEvaluation, Seq: 42, Kind: healthband.EventKindEvaluation,
		Evaluation: healthband.Evaluation{Series: lpSeries, SampleKey: "1042", Z: &z, Tier: &tier},
		Action:     healthband.ActionDiagnose, EpisodeID: "e1042", Claims: []healthband.Claim{diagnoseClaim},
	}
	diagnose := healthband.DueClaim{Claim: diagnoseClaim, Series: lpSeries, SampleKey: "1042", EpisodeID: "e1042", Tier: 3, Event: event}
	return localPatchTarget{diagnose: diagnose, claimID: lpPatchClaimID, lease: lpT0.Add(1800 * time.Second)}
}
