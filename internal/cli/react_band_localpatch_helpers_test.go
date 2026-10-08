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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Fixtures of the executor tests: a user checkout with a bare origin whose
// main matches refs/remotes/origin/main, a HOME for band git, a temp user
// cache directory, an in-memory ledger, and a test cleaner.

const (
	lpSeries          = "ci.failure_rate:CI"
	lpPatchClaimID    = "a1b2c3d4e5f60708a1b2c3d4e5f60708"
	lpDiagnoseClaimID = "0f1e2d3c4b5a69780f1e2d3c4b5a6978"
	lpKey             = "ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4"
	lpFooGo           = "package foo\n\nfunc Foo() int {\n\treturn 1\n}\n"
)

var lpT0 = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

// lpRecord is one appended record: its kind and value.
type lpRecord struct {
	kind  string
	value any
}

// lpMemLedger is an in-memory localPatchLedger with injectable faults.
type lpMemLedger struct {
	mu        sync.Mutex
	records   []lpRecord
	results   map[string]bool
	failPrep  bool
	failPhase string
}

func newLPMemLedger() *lpMemLedger { return &lpMemLedger{results: map[string]bool{}} }

var errLPInjected = errors.New("injected write error")

func (l *lpMemLedger) AppendPrep(_ context.Context, prep localPatchPrep) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failPrep {
		return false, errLPInjected
	}
	id := prep.ClaimID
	if id == "" {
		id = prep.DiagnoseClaimID
	}
	if l.results[id] {
		return false, nil
	}
	l.records = append(l.records, lpRecord{"prep", prep})
	return true, nil
}

func (l *lpMemLedger) AppendStage(_ context.Context, stage localPatchStage) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if stage.Phase == l.failPhase {
		return errLPInjected
	}
	l.records = append(l.records, lpRecord{"stage", stage})
	return nil
}

func (l *lpMemLedger) AppendResult(_ context.Context, result localPatchResult) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.results[result.ClaimID] {
		return false, nil
	}
	l.results[result.ClaimID] = true
	l.records = append(l.records, lpRecord{"result", result})
	return true, nil
}

// trail is the record sequence as kind or kind:phase, so tests compare order.
func (l *lpMemLedger) trail() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var trail []string
	for _, record := range l.records {
		if stage, ok := record.value.(localPatchStage); ok {
			trail = append(trail, "stage:"+stage.Phase)
			continue
		}
		trail = append(trail, record.kind)
	}
	return trail
}

func (l *lpMemLedger) prep(t *testing.T) localPatchPrep {
	t.Helper()
	for _, record := range l.records {
		if prep, ok := record.value.(localPatchPrep); ok {
			return prep
		}
	}
	t.Fatal("no prep record")
	return localPatchPrep{}
}

func (l *lpMemLedger) stage(t *testing.T, phase string) localPatchStage {
	t.Helper()
	for _, record := range l.records {
		if stage, ok := record.value.(localPatchStage); ok && stage.Phase == phase {
			return stage
		}
	}
	t.Fatalf("no %s stage", phase)
	return localPatchStage{}
}

func (l *lpMemLedger) result(t *testing.T) localPatchResult {
	t.Helper()
	for _, record := range l.records {
		if result, ok := record.value.(localPatchResult); ok {
			return result
		}
	}
	t.Fatal("no result record")
	return localPatchResult{}
}

// lpTestCleaner records its calls and removes a registered worktree that
// the claim's worktree_intent names, as Cleanup Rule 3 does for a clean one.
type lpTestCleaner struct {
	f     *lpFixture
	calls []string
}

func (c *lpTestCleaner) Cleanup(ctx context.Context, claimID string) ([]localPatchKept, error) {
	c.calls = append(c.calls, claimID)
	for _, record := range c.f.ledger.records {
		if stage, ok := record.value.(localPatchStage); ok && stage.Phase == lpPhaseWorktreeIntent && stage.ClaimID == claimID {
			_, _ = c.f.patcher.git.In(c.f.repo).Run(ctx, "worktree", "remove", "--force", stage.Path)
		}
	}
	return nil, nil
}

// lpFixture is the temp world of one executor test.
type lpFixture struct {
	t        *testing.T
	root     string
	repo     string
	base     string
	cacheDir string
	setupEnv []string
	ledger   *lpMemLedger
	cleaner  *lpTestCleaner
	patcher  *bandLocalPatcher
}

func newLPFixture(t *testing.T, harness *config.HarnessConfig) *lpFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	f := &lpFixture{t: t, root: root, repo: filepath.Join(root, "repo"), cacheDir: filepath.Join(root, "cache"), ledger: newLPMemLedger()}
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
	cache, err := resolveLocalPatchCache(context.Background(), runner, f.repo, func() (string, error) { return f.cacheDir, nil })
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.close() })
	if harness == nil {
		harness = lpHarness("claude", "", "", nil)
	}
	f.cleaner = &lpTestCleaner{f: f}
	f.patcher = newBandLocalPatcher(f.repo, harness, cache, f.ledger, f.cleaner)
	f.patcher.git, f.patcher.now, f.patcher.policyGit = runner, func() time.Time { return lpT0 }, lpPolicyGit(runner)
	f.patcher.statfs = func(string) (lpDiskSpace, error) { return lpDiskSpace{avail: 1 << 40, unit: 4096}, nil }
	return f
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
func (f *lpFixture) lp() string { return f.patcher.cache.dir }

// target is the S4 tier-3 diagnose claim with its local_patch claim.
func (f *lpFixture) target() localPatchTarget {
	z, tier := 4.2, 3
	event := healthband.Event{
		Schema: healthband.SchemaBandEvaluation, Seq: 42, Kind: healthband.EventKindEvaluation,
		Evaluation: healthband.Evaluation{Series: lpSeries, SampleKey: "1042", Z: &z, Tier: &tier},
		Action:     healthband.ActionDiagnose, EpisodeID: "e1042",
	}
	diagnose := healthband.DueClaim{
		Claim:  healthband.Claim{ID: lpDiagnoseClaimID, Kind: healthband.ClaimKindDiagnose, Owner: bandDiagnoseOwner, LeaseUntil: lpT0.Add(990 * time.Second)},
		Series: lpSeries, SampleKey: "1042", EpisodeID: "e1042", Tier: 3, Event: event,
	}
	return localPatchTarget{diagnose: diagnose, claimID: lpPatchClaimID, lease: lpT0.Add(1800 * time.Second)}
}

// lpPolicyGit is the Patch Policy runner of the tests: the production
// adapter, except that it runs the base listing `ls-tree -r -z <base>` with
// the policy argv and environment, because the W1 allowlist does not admit
// that form yet (reported in the T7 hand-off).
func lpPolicyGit(runner healthband.GitPolicyRunner) healthband.GitRunner {
	adapter := bandPolicyGit(runner)
	return healthband.GitRunnerFunc(func(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
		if len(args) == 4 && slices.Equal(args[:3], []string{"ls-tree", "-r", "-z"}) {
			cmd := exec.CommandContext(ctx, "git", healthband.GitPolicyArgv(args...)...)
			cmd.Dir, cmd.Env = dir, healthband.GitPolicyEnv(runner.Environ(), "")
			return cmd.Output()
		}
		return adapter.Run(ctx, dir, stdin, args...)
	})
}
