package harnessfb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/procgroup"
)

// proofDirPrefix names the temporary directory of one proof; a proof that
// was killed before it could clean up is swept by a later one.
const proofDirPrefix = "auto-harness-proof-"

// staleProof is how old a leftover proof directory must be before a later
// proof removes it: longer than three check runs can take.
var staleProof = 2 * time.Hour

var (
	localEnvOnce sync.Once
	localEnvVars map[string]bool
)

// cleanEnv is the environment without the variables that point git at a
// particular repository, index or object store (`git rev-parse
// --local-env-vars`). Run from a git hook, auto inherits them; kept, they
// would send the snapshot's writes into the user's own index.
func cleanEnv(extra ...string) []string {
	localEnvOnce.Do(func() {
		localEnvVars = map[string]bool{}
		cmd := exec.Command("git", "rev-parse", "--local-env-vars")
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
		if out, err := cmd.Output(); err == nil {
			for _, name := range strings.Fields(string(out)) {
				localEnvVars[name] = true
			}
		}
		for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_PREFIX", "GIT_NAMESPACE",
			"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_IMPLICIT_WORK_TREE"} {
			localEnvVars[name] = true
		}
	})
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if localEnvVars[name] || strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, extra...)
}

func gitIn(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = cleanEnv(env...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := tail(stderr.String())
		return msg, fmt.Errorf("git %s: %w: %s", subcommand(args), err, msg)
	}
	return string(out), nil
}

// subcommand names a git call in errors, past any leading -c key=value.
func subcommand(args []string) string {
	for len(args) > 1 && args[0] == "-c" {
		args = args[2:]
	}
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

var harnessIdent = []string{
	"GIT_AUTHOR_NAME=auto harness", "GIT_AUTHOR_EMAIL=harness@localhost",
	"GIT_COMMITTER_NAME=auto harness", "GIT_COMMITTER_EMAIL=harness@localhost",
}

// proofCopy is a throwaway repository holding a snapshot of a work tree.
type proofCopy struct {
	tmp, dir, hooks, lfs string
}

// git runs in the copy with every hook off, the user's global hooksPath
// included, and with the project's LFS objects for smudge.
func (p proofCopy) git(ctx context.Context, args ...string) (string, error) {
	pre := []string{"-c", "core.hooksPath=" + p.hooks}
	if p.lfs != "" {
		pre = append(pre, "-c", "lfs.storage="+p.lfs)
	}
	return gitIn(ctx, p.dir, harnessIdent, append(pre, args...)...)
}

// snapshot copies tree as it is now, uncommitted and untracked (not
// ignored) files included, into a new repository in a temporary directory.
// It reads the tree through a throwaway index and commits it with
// commit-tree, so the user's tree, index, refs and stash stay as they were;
// the copy borrows the user's objects through an alternates file and has
// no worktree entry, hooks, refs or stash of the user's.
func snapshot(ctx context.Context, tree string) (proofCopy, func(), error) {
	sweepProofs(time.Now())
	tmp, err := os.MkdirTemp("", proofDirPrefix)
	if err != nil {
		return proofCopy{}, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	p := proofCopy{tmp: tmp, dir: filepath.Join(tmp, "tree"), hooks: filepath.Join(tmp, "hooks")}
	fail := func(err error) (proofCopy, func(), error) {
		cleanup()
		return proofCopy{}, nil, err
	}
	if err := os.Mkdir(p.hooks, 0o700); err != nil {
		return fail(err)
	}
	indexPath, err := gitIn(ctx, tree, nil, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return fail(err)
	}
	tmpIndex := filepath.Join(tmp, "index")
	if data, err := os.ReadFile(strings.TrimSpace(indexPath)); err == nil {
		if err := os.WriteFile(tmpIndex, data, 0o600); err != nil {
			return fail(err)
		}
	}
	idx := []string{"GIT_INDEX_FILE=" + tmpIndex}
	if _, err := gitIn(ctx, tree, idx, "add", "-A", "--sparse", "--", "."); err != nil {
		return fail(err)
	}
	treeID, err := gitIn(ctx, tree, idx, "write-tree")
	if err != nil {
		return fail(err)
	}
	args := []string{"commit-tree", strings.TrimSpace(treeID), "-m", "auto harness proof snapshot"}
	if head, err := gitIn(ctx, tree, nil, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		args = append(args, "-p", strings.TrimSpace(head))
	}
	commit, err := gitIn(ctx, tree, harnessIdent, args...)
	if err != nil {
		return fail(err)
	}
	common, err := gitIn(ctx, tree, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fail(err)
	}
	format := "sha1"
	if out, err := gitIn(ctx, tree, nil, "rev-parse", "--show-object-format"); err == nil && strings.TrimSpace(out) != "" {
		format = strings.TrimSpace(out)
	}
	if _, err := gitIn(ctx, tmp, nil, "init", "-q", "--template=", "--object-format="+format, p.dir); err != nil {
		return fail(err)
	}
	common = strings.TrimSpace(common)
	if fi, err := os.Stat(filepath.Join(common, "lfs")); err == nil && fi.IsDir() {
		p.lfs = filepath.Join(common, "lfs")
	}
	alt := filepath.Join(common, "objects") + "\n"
	if err := os.WriteFile(filepath.Join(p.dir, ".git", "objects", "info", "alternates"), []byte(alt), 0o600); err != nil {
		return fail(err)
	}
	// Branches, remote branches and tags, so a check that compares with
	// origin/main or describes a tag sees what it sees in the project. The
	// objects are already there through alternates; nothing is copied.
	if _, err := p.git(ctx, "fetch", "-q", "--no-tags", "--update-head-ok", tree,
		"+refs/heads/*:refs/heads/*", "+refs/remotes/*:refs/remotes/*", "+refs/tags/*:refs/tags/*"); err != nil {
		return fail(err)
	}
	if _, err := p.git(ctx, "checkout", "-q", "--detach", strings.TrimSpace(commit)); err != nil {
		return fail(err)
	}
	return p, cleanup, nil
}

// sweepProofs removes proof directories a killed proof left behind.
func sweepProofs(now time.Time) {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), proofDirPrefix+"*"))
	for _, m := range matches {
		if fi, err := os.Lstat(m); err == nil && fi.IsDir() && now.Sub(fi.ModTime()) > staleProof {
			_ = os.RemoveAll(m)
		}
	}
}

// errCannotRun is a check command the shell could not start (exit 126/127).
var errCannotRun = errors.New("검사 명령을 실행할 수 없습니다 (exit 126/127)")

// runCheck runs the check with sh in dir, its whole process tree stopped
// on timeout or interrupt, with the repository variables removed.
func runCheck(ctx context.Context, dir, check string) (int, string, error) {
	cctx, cancel := context.WithTimeout(ctx, ProofTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", check)
	cmd.Dir = dir
	cmd.Env = cleanEnv()
	out := &capped{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, out
	group := procgroup.New(cmd, procgroup.Options{Grace: procgroup.DefaultGrace})
	if err := group.Start(); err != nil {
		return 0, "", fmt.Errorf("검사를 시작하지 못했습니다: %w", err)
	}
	err := cmd.Wait()
	_ = group.Close()
	switch {
	case errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil:
		return 0, tail(out.String()), fmt.Errorf("검사가 %s 안에 끝나지 않았습니다 (%s)", ProofTimeout, dir)
	case ctx.Err() != nil:
		return 0, tail(out.String()), fmt.Errorf("검사가 중단됐습니다: %w", ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code == 126 || code == 127 {
			return code, tail(out.String()), errCannotRun
		}
		return code, tail(out.String()), nil
	}
	return 0, tail(out.String()), err
}

// capped keeps the last limit bytes a command printed.
type capped struct {
	buf   []byte
	limit int
}

func (c *capped) Write(p []byte) (int, error) {
	c.buf = append(c.buf, p...)
	if len(c.buf) > 2*c.limit {
		c.buf = append([]byte(nil), c.buf[len(c.buf)-c.limit:]...)
	}
	return len(p), nil
}

func (c *capped) String() string {
	if len(c.buf) > c.limit {
		return string(c.buf[len(c.buf)-c.limit:])
	}
	return string(c.buf)
}

// tail keeps the last 2 KiB of a command's output.
func tail(s string) string {
	const keep = 2 << 10
	s = strings.TrimSpace(s)
	if len(s) > keep {
		return "…" + s[len(s)-keep:]
	}
	return s
}
