//go:build unix

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The world of the SPEC-SIGMABAND-002 integration tests (plan task T10):
// the S4 setup as a real user checkout, which is also the band project,
// with a bare origin whose main matches refs/remotes/origin/main, a dirty
// tracked file, an untracked .env, a stash entry, a tracked symlink whose
// link text leads out of a band worktree, the S4 identity settings, and
// marker-writing hooks of every hook type in .git/hooks and in a tracked,
// relative core.hooksPath. The test HOME is the user cache directory's
// parent and its global git configuration logs trace2 events, because band
// strips every GIT_* variable. gh and claude are fake binaries on PATH, and
// a git wrapper on PATH records every git call in order and injects one
// fault at a time (LPIT_GIT_FAULT).

// lpitConfig is the S4 configuration: the flag on and a subprocess claude
// (no backend) that 001's selection names, so the request asks for
// claude-opus-5-5.
const lpitConfig = "mode: full\nproject_name: band\nplatforms:\n  - claude-code\norchestra:\n  judge: claude\n" +
	"  providers:\n    claude:\n      binary: claude\n      args: [--print, --model, claude-opus-5-5]\n" +
	"health_band:\n  allow_local_patch: true\n"

// lpitHooks is every hook type of githooks(5).
var lpitHooks = strings.Fields("applypatch-msg pre-applypatch post-applypatch pre-commit pre-merge-commit " +
	"prepare-commit-msg commit-msg post-commit pre-rebase post-checkout post-merge pre-push pre-receive update " +
	"proc-receive post-receive post-update reference-transaction push-to-checkout pre-auto-gc post-rewrite " +
	"sendemail-validate fsmonitor-watchman p4-changelist p4-prepare-changelist p4-post-changelist p4-pre-submit " +
	"post-index-change")

const (
	lpitGhToken = "ghp_" + "S4synthetic0123456789abcdefABCDEF0123" // a synthetic token, never a real one
	lpitCanary  = "c4a1a2b3c4d5e6f708192a3b4c5d6e7f"               // 32 hex outside every band worktree
	lpitLink    = "../../../../canary.txt"                         // from <lp>/<key>/worktree/docs/
)

// lpitGitWrapper records each git call (cwd, argv, GIT_* variables) as
// $LPIT_GITREC/call.<seq> and then runs the real git, unless a fault of
// LPIT_GIT_FAULT applies to its subcommand.
const lpitGitWrapper = `#!/bin/sh
d="$LPIT_GITREC"
s=$(( $(cat "$d/seq" 2>/dev/null || echo 0) + 1 )); echo "$s" > "$d/seq"
{ echo GIT; pwd -P; for a in "$@"; do printf '%s\037' "$a"; done; echo; env | grep '^GIT_' | sort; } > "$d/call.$(printf %06d "$s")"
sub=""; skip=0
for a in "$@"; do
  if [ "$skip" = 1 ]; then skip=0; continue; fi
  case "$a" in -c|-C) skip=1; continue;; esac
  sub="$a"; break
done
case "$LPIT_GIT_FAULT:$sub" in
crash-after-commit:commit)
  "$LPIT_REALGIT" "$@"; rc=$?; kill -KILL "$PPID"; exit "$rc";;
stall-checkout:reset)
  printf 'partial\n' > README.md
  trap 'echo term >> "$d/stall.term"' TERM
  i=0; while [ "$i" -lt 600 ]; do sleep 0.1; i=$((i+1)); done
  echo done > "$d/stall.done"; exit 0;;
esac
exec "$LPIT_REALGIT" "$@"
`

// lpitFakeClaude records call n (argv, cwd, environment, stdin, and the
// bytes of every regular file under its cwd), takes a place in the git
// wrapper's sequence, sources hook.<n> when present, and prints stream.<n>.
const lpitFakeClaude = `#!/bin/sh
d="$LPIT_CLAUDE"
n=$(( $(cat "$d/count" 2>/dev/null || echo 0) + 1 )); echo "$n" > "$d/count"
s=$(( $(cat "$LPIT_GITREC/seq" 2>/dev/null || echo 0) + 1 )); echo "$s" > "$LPIT_GITREC/seq"
echo "CLAUDE $n" > "$LPIT_GITREC/call.$(printf %06d "$s")"
printf '%s\n' "$@" > "$d/argv.$n"; pwd -P > "$d/cwd.$n"; env > "$d/env.$n"; cat > "$d/stdin.$n"
find . -type f -exec cat {} + > "$d/files.$n" 2>/dev/null
if [ -f "$d/hook.$n" ]; then . "$d/hook.$n"; fi
cat "$d/stream.$n"
`

// lpitFakeGH answers the gh Invocation Table of acme/app and logs each call.
const lpitFakeGH = `#!/bin/sh
printf '%s\n' "$*" >> "$LPIT_GH/calls"
case "$1 $2" in
"auth status") exit 0;;
"api repos/acme/app") echo main; exit 0;;
"run list") cat "$LPIT_GH/runs.json"; exit 0;;
"run view") echo "step 3 failed: TestFoo in pkg/foo/foo.go (run $3)"; exit 0;;
esac
exit 1
`

// lpitWorld is one integration world.
type lpitWorld struct {
	t                                  *testing.T
	root, repo, bare, home, cache, bin string
	markers, trace, gh, claude, gitrec string
	tmp, realGit, base                 string
	setupEnv                           []string
}

// lpitTracked adds files of the world to the base commit, by repo path.
type lpitTracked func(w *lpitWorld) map[string]string

func newLPITWorld(t *testing.T, config string, tracked ...lpitTracked) *lpitWorld {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	w := &lpitWorld{t: t, root: root, repo: filepath.Join(root, "repo"), bare: filepath.Join(root, "origin.git"),
		home: filepath.Join(root, "home"), realGit: realGit}
	w.cache = lpitCacheDir(w.home)
	for _, name := range []string{"bin", "markers", "trace2", "gh", "claude", "gitrec", "tmp", "setup-home/.config"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, name), 0o700))
	}
	w.bin, w.markers, w.trace = filepath.Join(root, "bin"), filepath.Join(root, "markers"), filepath.Join(root, "trace2")
	w.gh, w.claude, w.gitrec, w.tmp = filepath.Join(root, "gh"), filepath.Join(root, "claude"), filepath.Join(root, "gitrec"), filepath.Join(root, "tmp")
	require.NoError(t, os.MkdirAll(filepath.Join(w.cache, "autopus", "local-patches"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(w.home, ".config"), 0o700))
	setupHome := filepath.Join(root, "setup-home")
	w.writeAbs(filepath.Join(setupHome, ".gitconfig"), "[user]\n\tname = Setup\n\temail = setup@example.invalid\n[init]\n\tdefaultBranch = main\n", 0o600)
	w.setupEnv = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + setupHome, "XDG_CONFIG_HOME=" + filepath.Join(setupHome, ".config"), "TMPDIR=" + os.TempDir()}
	w.writeAbs(filepath.Join(w.home, ".gitconfig"), "[trace2]\n\teventTarget = "+w.trace+"\n[author]\n\temail = global@example.invalid\n", 0o600)
	w.writeAbs(filepath.Join(w.cache, "autopus", "local-patches", "canary.txt"), lpitCanary+"\n", 0o600)
	w.buildRepo(config, tracked)
	w.installFakes()
	return w
}

// lpitCacheDir is the user cache directory that os.UserCacheDir names for
// a HOME without XDG_CACHE_HOME.
func lpitCacheDir(home string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Caches")
	}
	return filepath.Join(home, ".cache")
}

// buildRepo is the S4 user checkout; hooks go live only after the setup
// commands, which run with hooks off.
func (w *lpitWorld) buildRepo(config string, tracked []lpitTracked) {
	w.git(w.root, "init", "-q", "--bare", w.bare)
	w.git(w.root, "init", "-q", w.repo)
	w.write("pkg/foo/foo.go", lpFooGo, 0o644)
	w.write("README.md", "readme\n", 0o644)
	w.write(".gitignore", ".autopus/\n", 0o644)
	w.write("autopus.yaml", config, 0o644)
	for _, files := range tracked {
		for name, content := range files(w) {
			w.write(name, content, 0o644)
		}
	}
	for _, hook := range lpitHooks {
		w.write(".githooks-band/"+hook, w.markerHook("tracked-"+hook), 0o755)
	}
	require.NoError(w.t, os.MkdirAll(filepath.Join(w.repo, "docs"), 0o755))
	require.NoError(w.t, os.Symlink(lpitLink, filepath.Join(w.repo, "docs", "canary.md")))
	w.git(w.repo, "add", "-A")
	w.git(w.repo, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "base")
	w.git(w.repo, "remote", "add", "origin", bandCmdOriginURL)
	w.git(w.repo, "config", "remote.origin.pushurl", w.bare)
	w.git(w.repo, "-c", "core.hooksPath=/dev/null", "push", "-q", "-u", "origin", "main")
	w.git(w.repo, "remote", "set-head", "origin", "main")
	w.base = strings.TrimSpace(w.git(w.repo, "rev-parse", "HEAD"))
	w.write("README.md", "stashed change\n", 0o644)
	w.git(w.repo, "-c", "core.hooksPath=/dev/null", "stash", "push", "-q", "-m", "lpit-stash")
	w.write("README.md", "dirty change\n", 0o644)
	w.write(".env", "GITHUB_TOKEN="+lpitGhToken+"\n", 0o600)
	for key, value := range map[string]string{"author.name": "User", "author.email": "user@example.invalid",
		"committer.email": "user@example.invalid", "core.hooksPath": ".githooks-band"} {
		w.git(w.repo, "config", key, value)
	}
	for _, hook := range lpitHooks {
		w.writeAbs(filepath.Join(w.repo, ".git", "hooks", hook), w.markerHook("git-"+hook), 0o755)
	}
}

func (w *lpitWorld) markerHook(name string) string {
	return "#!/bin/sh\ntouch " + filepath.Join(w.markers, name) + "\n"
}

// installFakes puts the fakes on PATH, the O3 run list behind gh, and the
// inherited variables that band must keep from git and from the provider.
func (w *lpitWorld) installFakes() {
	w.writeAbs(filepath.Join(w.bin, "git"), lpitGitWrapper, 0o755)
	w.writeAbs(filepath.Join(w.bin, "claude"), lpitFakeClaude, 0o755)
	w.writeAbs(filepath.Join(w.bin, "gh"), lpitFakeGH, 0o755)
	w.writeAbs(filepath.Join(w.gh, "runs.json"), ghPayload(w.t, bandITRows("CI", bandITRuns(959, bandO3Values))...), 0o600)
	for key, value := range map[string]string{
		"HOME": w.home, "XDG_CONFIG_HOME": filepath.Join(w.home, ".config"), "XDG_CACHE_HOME": "",
		"PATH":    w.bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"LPIT_GH": w.gh, "LPIT_CLAUDE": w.claude, "LPIT_GITREC": w.gitrec, "LPIT_REALGIT": w.realGit, "LPIT_GIT_FAULT": "",
		"GH_TOKEN": lpitGhToken, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "s4-oidc", "AWS_CONTAINER_CREDENTIALS_FULL_URI": "http://169.254.170.2/s4",
		"GIT_DIR": filepath.Join(w.repo, ".git"), "GIT_INDEX_FILE": filepath.Join(w.repo, ".git", "index"),
		"GIT_CONFIG_PARAMETERS": "'core.worktree'='" + w.repo + "'",
	} {
		w.t.Setenv(key, value)
	}
}

// answer sets the stream of fake claude call n; hook sets a shell snippet
// that call n sources before it prints the stream.
func (w *lpitWorld) answer(n int, stream string) {
	w.writeAbs(filepath.Join(w.claude, "stream."+strconv.Itoa(n)), stream, 0o600)
}

func (w *lpitWorld) hook(n int, script string) {
	w.writeAbs(filepath.Join(w.claude, "hook."+strconv.Itoa(n)), script, 0o600)
}

// answerS4 gives the S4 replies: a short diagnosis, then one diff fence
// changing two lines of pkg/foo/foo.go, both on the requested model.
func (w *lpitWorld) answerS4() {
	w.answer(1, lpStream(lpInit55, lpAssistant55, lpResult(dlpDiagnosisText)))
	w.answer(2, lpStream(lpInit55, lpAssistant55, lpResult(lpReplyWith(lpFooDiff))))
}

// git runs the real git with the fixture's own environment (no hook, no
// trace2, no inherited GIT_* variable) and returns its stdout.
func (w *lpitWorld) git(dir string, args ...string) string {
	w.t.Helper()
	cmd := exec.Command(w.realGit, args...)
	cmd.Dir, cmd.Env = dir, w.setupEnv
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(w.t, cmd.Run(), "git %s: %s", strings.Join(args, " "), stderr.String())
	return stdout.String()
}

// inspect is git in the user checkout without hooks and optional locks.
func (w *lpitWorld) inspect(args ...string) string {
	w.t.Helper()
	return w.git(w.repo, append([]string{"--no-optional-locks", "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}, args...)...)
}

func (w *lpitWorld) write(name, content string, mode os.FileMode) {
	w.writeAbs(filepath.Join(w.repo, name), content, mode)
}

func (w *lpitWorld) writeAbs(path, content string, mode os.FileMode) {
	w.t.Helper()
	require.NoError(w.t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(w.t, os.WriteFile(path, []byte(content), mode))
}

// lp is <lp> by the spec's definition: <UserCacheDir>/autopus/local-patches/
// and the first 12 hex of the SHA-256 of the absolute common directory.
func (w *lpitWorld) lp() string {
	common := strings.TrimSpace(w.git(w.repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	sum := sha256.Sum256([]byte(common))
	return filepath.Join(w.cache, "autopus", "local-patches", hex.EncodeToString(sum[:])[:12])
}

// run is the band command of deps in the user checkout.
func (w *lpitWorld) run(cmdDeps *reactBandDeps, args ...string) bandCmdRun {
	w.t.Helper()
	cmd := newReactBandCmd()
	if cmdDeps != nil {
		cmd = newReactBandCmdWith(*cmdDeps)
	}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"--project-dir", w.repo}, args...))
	err := cmd.ExecuteContext(context.Background())
	return bandCmdRun{stdout: stdout.String(), stderr: stderr.String(), err: err}
}
