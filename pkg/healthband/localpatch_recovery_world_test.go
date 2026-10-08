//go:build unix

package healthband

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// lpWorld is a repository with a store, an <lp> under a temp cache, and one
// local_patch claim of the acceptance fixture whose artifacts a test builds
// step by step as the live flow does, through the policy runner. Every band
// git command of the world and of the code under test goes through a
// recording wrapper, so tests count invocations.
type lpWorld struct {
	*lpRepo
	t        *testing.T
	ctx      context.Context
	store    *Store
	cache    string
	lp       *LocalPatchDir
	paths    LocalPatchPaths
	recorder string
	temp     string
	diff     []byte
	message  []byte
	expected string
	commit   string
	patch    []byte
}

// lpWorldDiff adds one line to the tracked pkg/foo/foo.go.
const lpWorldDiff = "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n" +
	"@@ -1 +1,2 @@\n package foo\n+// patched by band\n"

var lpWorldFiles = map[string]string{
	"pkg/foo/foo.go": "package foo\n", "pkg/foo/bar.go": "package foo\n\nvar Bar = 1\n", ".gitignore": "secret.env\n.autopus/\n",
}

func newLPWorld(t *testing.T, files map[string]string) *lpWorld {
	t.Helper()
	f := newGPFixture(t)
	if files == nil {
		files = lpWorldFiles
	}
	dir, base := f.repo("repo", files)
	real, err := exec.LookPath("git")
	require.NoError(t, err)
	w := &lpWorld{t: t, ctx: gpContext(t), recorder: filepath.Join(f.root, "git-argv.log"), temp: filepath.Join(f.root, "band-tmp")}
	wrapper := f.script("git-rec", "printf '%s\\037' \"$@\" >> '"+w.recorder+"'\nprintf '\\n' >> '"+w.recorder+"'\nexec '"+real+"' \"$@\"\n")
	git := f.runner(f.setupEnv, dir)
	git.Binary = wrapper
	w.lpRepo = &lpRepo{f: f, dir: dir, base: base, git: git}
	w.store, w.cache, w.diff = NewStore(dir), filepath.Join(f.root, "cache"), []byte(lpWorldDiff)
	require.NoError(t, os.MkdirAll(w.temp, 0o700))
	dirOpened, code := w.open(t, w.cache)
	require.Empty(t, code)
	w.lp, w.paths = dirOpened, dirOpened.Paths(lpFixtureK)
	return w
}

// wt is the policy runner in the claim's worktree.
func (w *lpWorld) wt() GitPolicyRunner { return w.git.In(w.paths.Worktree) }

func (w *lpWorld) stage(phase string, set func(*LocalPatchRecord)) {
	w.t.Helper()
	record := NewLocalPatchStage(lpClaimID, phase)
	if set != nil {
		set(&record)
	}
	require.NoError(w.t, w.store.AppendLocalPatchStage(w.ctx, record))
}

// claimed appends the phase A records with the local_patch lease and the
// prep record of the diagnose claim (base SHA, code ok).
func (w *lpWorld) claimed(lease time.Time) {
	w.t.Helper()
	appendPhaseA(w.t, w.store, lpDecision(), lpClaim(w.lp.LocalPatchLocation, lease))
	prep := lpPrep(LocalPatchCodeOK)
	prep.BaseSHA = w.base
	appended, err := w.store.AppendLocalPatchPrep(w.ctx, prep)
	require.NoError(w.t, err)
	require.True(w.t, appended)
}

// checkedOut runs steps 2 up to worktree_done.
func (w *lpWorld) checkedOut() {
	w.t.Helper()
	w.checkout()
	w.worktreeDone()
}

func (w *lpWorld) checkout() {
	w.t.Helper()
	w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	w.f.must(w.git.Run(w.ctx, "worktree", "add", "--no-checkout", "--detach", w.paths.Worktree, w.base))
	w.f.must(w.wt().Run(w.ctx, "reset", "--hard", "--no-recurse-submodules", "--quiet"))
}

func (w *lpWorld) worktreeDone() {
	w.t.Helper()
	sum, err := WorktreeStatusSHA256(w.ctx, w.wt())
	require.NoError(w.t, err)
	w.stage(StageWorktreeDone, func(r *LocalPatchRecord) { r.StatusSHA256 = sum })
}

// messaged records the final message and its hash (step 6).
func (w *lpWorld) messaged() {
	w.t.Helper()
	w.message = []byte("fix(band): ci-failure-rate-ci anomaly local patch (e1042)\n\nPatch model: claude-opus-5-5\n")
	w.stage(StageMessage, func(r *LocalPatchRecord) { r.MessageSHA256 = lpSHA256(w.message) })
}

// intended computes the expected tree in a temp index, appends apply_intent,
// and writes the diff file (step 8 up to git apply --index).
func (w *lpWorld) intended() {
	w.t.Helper()
	indexed := w.wt().WithIndexFile(filepath.Join(w.temp, "index"))
	w.f.must(indexed.Run(w.ctx, "read-tree", w.base))
	w.f.must(indexed.RunInput(w.ctx, w.diff, "apply", "--cached"))
	w.expected = gpTrim(w.f.must(indexed.Run(w.ctx, "write-tree")))
	w.stage(StageApplyIntent, func(r *LocalPatchRecord) { r.DiffSHA256, r.Tree = lpSHA256(w.diff), w.expected })
	require.NoError(w.t, os.WriteFile(w.paths.Diff, w.diff, 0o600))
}

func (w *lpWorld) applied() {
	w.t.Helper()
	w.f.must(w.wt().RunInput(w.ctx, w.diff, "apply", "--index"))
}

// committed commits the -F file and reads the commit OID (step 9).
func (w *lpWorld) committed() {
	w.t.Helper()
	file := filepath.Join(w.temp, "msg")
	require.NoError(w.t, os.WriteFile(file, w.message, 0o600))
	w.f.must(w.wt().Run(w.ctx, "commit", "--no-verify", "--cleanup=verbatim", "-F", file))
	w.commit = gpTrim(w.f.must(w.wt().Run(w.ctx, "rev-parse", "HEAD")))
}

func (w *lpWorld) branched() {
	w.t.Helper()
	w.stage(StageBranchIntent, func(r *LocalPatchRecord) { r.CommitOID = w.commit })
	w.f.must(w.git.Run(w.ctx, "update-ref", "--no-deref", w.paths.Ref, w.commit, ""))
}

// patched writes the canonical format-patch through its temp file and the
// rename (step 11 up to patch_done).
func (w *lpWorld) patched() {
	w.t.Helper()
	w.patch = w.f.must(w.git.Run(w.ctx, GitFormatPatchArgs(w.base, w.commit)...))
	w.stage(StagePatchIntent, func(r *LocalPatchRecord) { r.Path, r.PatchSHA256 = w.paths.Patch, lpSHA256(w.patch) })
	require.NoError(w.t, os.WriteFile(w.paths.PatchTemp, w.patch, 0o600))
	require.NoError(w.t, os.Rename(w.paths.PatchTemp, w.paths.Patch))
}

// throughPatchDone builds every artifact of a done claim without its result.
func (w *lpWorld) throughPatchDone() {
	w.t.Helper()
	w.claimed(lpT0.Add(1800 * time.Second))
	w.checkedOut()
	w.messaged()
	w.intended()
	w.applied()
	w.stage(StageApplyDone, func(r *LocalPatchRecord) { r.Tree = w.expected })
	w.committed()
	w.stage(StageCommitDone, func(r *LocalPatchRecord) { r.CommitOID = w.commit })
	w.branched()
	w.stage(StageBranchDone, nil)
	w.patched()
	w.stage(StagePatchDone, func(r *LocalPatchRecord) { r.PatchSHA256 = lpSHA256(w.patch) })
}

// records returns the claim's records as the live claim holds them.
func (w *lpWorld) records() []LocalPatchRecord {
	w.t.Helper()
	log, err := w.store.ReadLocalPatchLog()
	require.NoError(w.t, err)
	return log.ClaimRecords(lpClaimID)
}

// mark starts a new invocation window of the recorder.
func (w *lpWorld) mark() int { return len(w.invocations(0)) }

// invocations returns the recorded git argv after the policy -c flags, from
// window start on, each joined by spaces.
func (w *lpWorld) invocations(start int) []string {
	data, err := os.ReadFile(w.recorder)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(w.t, err)
	var calls []string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		args := strings.Split(strings.TrimSuffix(line, "\x1f"), "\x1f")
		for len(args) > 1 && args[0] == "-c" {
			args = args[2:]
		}
		calls = append(calls, strings.Join(args, " "))
	}
	return calls[min(start, len(calls)):]
}

// count counts the window's invocations that start with prefix.
func (w *lpWorld) count(start int, prefix string) int {
	n := 0
	for _, call := range w.invocations(start) {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

// admin reports whether the repository still registers the claim's worktree.
func (w *lpWorld) admin() bool {
	_, found := findWorktree(parseWorktreeList([]byte(w.f.git(w.f.setupEnv, w.dir, "worktree", "list", "--porcelain", "-z"))), w.paths.Worktree)
	return found
}
