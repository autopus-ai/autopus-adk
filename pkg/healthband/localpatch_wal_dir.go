package healthband

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// <lp> itself (REQ-14, CD-3 F-008): band opens the real user cache directory
// as an os.Root, creates or opens autopus, local-patches, and <repo-hash>
// through it with Lstat showing a directory and never a symlink at each
// component, and reaches every file below <lp> through an os.Root at <lp>.

var (
	// ErrLocalPatchKeyLocked reports that another process holds the key lock
	// of a claim: the live claim stops with no record, and recovery skips the
	// claim with reason recovery_key_locked.
	ErrLocalPatchKeyLocked = errors.New("healthband: another process holds the local patch key lock")
	errLocalPatchKey       = errors.New("healthband: local patch key outside [a-z0-9-]")
)

// LocalPatchDir is an opened <lp>.
type LocalPatchDir struct {
	LocalPatchLocation
	root *os.Root // at <lp>
}

// Open checks and opens <lp> for Local Patch Flow step 1 and recovery. The
// code is cache_unavailable when the real path of <lp> equals or lies inside
// the repository's top level, its common directory, or any worktree that git
// worktree list --porcelain -z lists, when a component below the user cache
// directory is a symlink or no directory, when <lp> is not a directory of
// the current user without group or other permission bits, or when it
// cannot be created with mode 0700. The location check runs before anything
// is created, so a refusal creates nothing. Only a done context or an
// allowlist refusal is an error.
func (loc LocalPatchLocation) Open(ctx context.Context, git GitPolicyRunner) (*LocalPatchDir, string, error) {
	if code, err := loc.checkOutside(ctx, git); code != "" || err != nil {
		return nil, code, err
	}
	if err := os.MkdirAll(loc.CacheDir, 0o700); err != nil {
		return nil, LocalPatchCodeCacheUnavailable, nil
	}
	if real, err := filepath.EvalSymlinks(loc.CacheDir); err != nil || real != loc.CacheDir {
		return nil, LocalPatchCodeCacheUnavailable, nil
	}
	cacheRoot, err := os.OpenRoot(loc.CacheDir)
	if err != nil {
		return nil, LocalPatchCodeCacheUnavailable, nil
	}
	defer func() { _ = cacheRoot.Close() }()
	rel := ""
	var info os.FileInfo
	for _, part := range append(strings.Split(localPatchCacheParts, "/"), loc.RepoHash) {
		rel = filepath.Join(rel, part)
		info, err = cacheRoot.Lstat(rel)
		if errors.Is(err, os.ErrNotExist) {
			if err = cacheRoot.Mkdir(rel, 0o700); err == nil || errors.Is(err, os.ErrExist) {
				info, err = cacheRoot.Lstat(rel)
			}
		}
		if err != nil || !info.IsDir() {
			return nil, LocalPatchCodeCacheUnavailable, nil
		}
	}
	if !privateDir(info) {
		return nil, LocalPatchCodeCacheUnavailable, nil
	}
	root, err := cacheRoot.OpenRoot(rel)
	if err != nil {
		return nil, LocalPatchCodeCacheUnavailable, nil
	}
	if opened, err := root.Stat("."); err != nil || !os.SameFile(opened, info) {
		_ = root.Close()
		return nil, LocalPatchCodeCacheUnavailable, nil
	}
	return &LocalPatchDir{LocalPatchLocation: loc, root: root}, "", nil
}

// checkOutside refuses an <lp> whose real path is a repository path or lies
// inside one; a worktree list that git cannot give is refused too.
func (loc LocalPatchLocation) checkOutside(ctx context.Context, git GitPolicyRunner) (string, error) {
	out, err := git.Run(ctx, "worktree", "list", "--porcelain", "-z")
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if err != nil {
		return LocalPatchCodeCacheUnavailable, nil
	}
	lp := realPathOf(loc.Path)
	repoPaths := []string{loc.TopLevel, loc.CommonDir}
	for _, worktree := range parseWorktreeList(out) {
		repoPaths = append(repoPaths, worktree.path)
	}
	for _, path := range repoPaths {
		if inside(lp, realPathOf(path)) {
			return LocalPatchCodeCacheUnavailable, nil
		}
	}
	return "", nil
}

// realPathOf resolves the nearest existing ancestor of path and appends the
// components below it, so a path that does not exist yet has the real path
// it would get.
func realPathOf(path string) string {
	path = filepath.Clean(path)
	var rest []string
	for {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Join(append([]string{real}, rest...)...)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Join(append([]string{path}, rest...)...)
		}
		rest = append([]string{filepath.Base(path)}, rest...)
		path = parent
	}
}

// inside reports whether path equals dir or lies below it.
func inside(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// Close releases <lp>'s root. Release key locks before closing.
func (d *LocalPatchDir) Close() error { return d.root.Close() }

// Paths derives every artifact path of key below <lp>.
func (d *LocalPatchDir) Paths(key string) LocalPatchPaths { return d.LocalPatchLocation.Paths(key) }

// KeptKeys is the retention count of Local Patch Flow step 1, read by one
// directory read of <lp>: the distinct keys with a <key>/ directory or a
// <key>.patch file.
func (d *LocalPatchDir) KeptKeys() (int, error) {
	file, err := d.root.Open(".")
	if err != nil {
		return 0, err
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return 0, err
	}
	return countKeys(entries), nil
}

// CountKeptKeys is the retention count of phase A (Decision Table row 4),
// read by one os.ReadDir of <lp>, local IO only; a missing <lp> has none.
func CountKeptKeys(lp string) (int, error) {
	entries, err := os.ReadDir(lp)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return countKeys(entries), nil
}

// countKeys counts a <key> directory (or a symlink in its place) and a
// <key>.patch entry of any type once per key.
func countKeys(entries []os.DirEntry) int {
	keys := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if key, isPatch := strings.CutSuffix(name, ".patch"); isPatch && localPatchKeyPattern.MatchString(key) {
			keys[key] = true
		} else if localPatchKeyPattern.MatchString(name) && (entry.IsDir() || entry.Type()&os.ModeSymlink != 0) {
			keys[name] = true
		}
	}
	return len(keys)
}

// LocalPatchKeyLock is a held <lp>/<key>.lock.
type LocalPatchKeyLock struct {
	mu   sync.Mutex
	lock *filelock.Lock
	path string
	file os.FileInfo
	done bool
}

// AcquireKeyLock takes the key lock with a zero wait (filelock.Acquire tries
// once, creates a 0600 file, and refuses a symlink). A lock that another
// process holds is ErrLocalPatchKeyLocked.
func (d *LocalPatchDir) AcquireKeyLock(ctx context.Context, key string) (*LocalPatchKeyLock, error) {
	if !localPatchKeyPattern.MatchString(key) {
		return nil, errLocalPatchKey
	}
	path := d.Paths(key).Lock
	lock, err := filelock.Acquire(ctx, path, 0)
	if errors.Is(err, filelock.ErrTimeout) {
		return nil, ErrLocalPatchKeyLocked
	}
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.Join(err, lock.Unlock())
	}
	return &LocalPatchKeyLock{lock: lock, path: path, file: info}, nil
}

// Release unlinks the lock file while it still names the locked file and
// then releases the lock, so no later holder ever locks an unlinked file. A
// second call is a no-op.
func (l *LocalPatchKeyLock) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.done {
		return nil
	}
	l.done = true
	var removeErr error
	if info, err := os.Lstat(l.path); err == nil && os.SameFile(info, l.file) {
		removeErr = os.Remove(l.path)
	}
	return errors.Join(removeErr, l.lock.Unlock())
}

// CheckNoArtifacts is the artifact check of Local Patch Flow step 1: no
// <key>/, <key>.patch, <key>.diff, or <key>.lock below <lp> and no
// refs/heads/autopus/band/<key>, a symbolic ref included. Anything there,
// or a check that cannot prove absence, is artifact_exists.
func (d *LocalPatchDir) CheckNoArtifacts(ctx context.Context, git GitPolicyRunner, key string) (string, error) {
	for _, name := range []string{key, key + ".patch", key + ".diff", key + ".lock"} {
		if _, err := d.root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return LocalPatchCodeArtifactExists, nil
		}
	}
	ref := d.Paths(key).Ref
	for _, args := range [][]string{{"rev-parse", "--verify", "--quiet", ref}, {"symbolic-ref", "-q", "--no-recurse", ref}} {
		_, err := git.Run(ctx, args...)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if GitExitCode(err) != 1 {
			return LocalPatchCodeArtifactExists, nil
		}
	}
	return "", nil
}
