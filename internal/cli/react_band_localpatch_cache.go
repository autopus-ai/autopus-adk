package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// <lp> = <UserCacheDir>/autopus/local-patches/<repo-hash> (SPEC-SIGMABAND-002
// REQ-14, Data Contracts Derived paths). Band opens the real path of the
// user cache directory as an os.Root, creates or opens autopus,
// local-patches, and <repo-hash> through it with Lstat showing a directory
// and never a symlink at each component, and opens every file below <lp>
// through it. <lp> must be a directory of the current user without group or
// other permission bits, and its real path must neither equal nor lie inside
// the repository's top level, its common directory, or any registered
// worktree; that check runs before anything is created.

// lpRepoHashHex is the length of <repo-hash>: the first 12 hex digits of the
// SHA-256 of the absolute common directory (without its newline).
const lpRepoHashHex = 12

// lpRetentionCap is the number of kept keys that fills <lp> (CD-3 M5).
const lpRetentionCap = 5

var errLocalPatchCache = errors.New("react band: the local patch cache directory is unavailable")

// lpDiskSpace is the free space of the file system that holds <lp>: Bavail
// blocks of the unit in which the file system counts Bavail.
type lpDiskSpace struct {
	avail, unit uint64
}

// localPatchCache is <lp> of one repository, held below an os.Root opened at
// the real user cache directory.
type localPatchCache struct {
	root *os.Root
	rel  string // autopus/local-patches/<repo-hash>, relative to root
	dir  string // absolute real path of <lp>
}

// resolveLocalPatchCache resolves <lp> of the checkout before the store
// lock; userCacheDir nil uses os.UserCacheDir. Any refusal wraps
// errLocalPatchCache, which Local Patch Flow step 1 reports as
// cache_unavailable.
func resolveLocalPatchCache(ctx context.Context, git healthband.GitPolicyRunner, checkout string, userCacheDir func() (string, error)) (*localPatchCache, error) {
	run := git.In(checkout)
	common, errCommon := lpGitPath(ctx, run, "--git-common-dir")
	top, errTop := lpGitPath(ctx, run, "--show-toplevel")
	listing, errList := run.Run(ctx, "worktree", "list", "--porcelain", "-z")
	if err := errors.Join(errCommon, errTop, errList); err != nil {
		return nil, errors.Join(errLocalPatchCache, err)
	}
	if userCacheDir == nil {
		userCacheDir = os.UserCacheDir
	}
	base, err := userCacheDir()
	if err == nil {
		base, err = filepath.EvalSymlinks(base)
	}
	if err != nil {
		return nil, errors.Join(errLocalPatchCache, err)
	}
	sum := sha256.Sum256([]byte(common))
	rel := filepath.Join("autopus", "local-patches", hex.EncodeToString(sum[:])[:lpRepoHashHex])
	dir := filepath.Join(base, rel)
	for _, path := range append([]string{top, common}, lpWorktreePaths(listing)...) {
		if lpWithin(dir, lpRealPath(path)) {
			return nil, errLocalPatchCache
		}
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, errors.Join(errLocalPatchCache, err)
	}
	cache := &localPatchCache{root: root, rel: rel, dir: dir}
	if err := cache.walk(); err != nil {
		_ = root.Close()
		return nil, err
	}
	return cache, nil
}

// lpGitPath is one absolute path that git rev-parse prints.
func lpGitPath(ctx context.Context, run healthband.GitPolicyRunner, flag string) (string, error) {
	out, err := run.Run(ctx, "rev-parse", "--path-format=absolute", flag)
	path := strings.TrimSuffix(string(out), "\n")
	if err == nil && !filepath.IsAbs(path) {
		err = errLocalPatchCache
	}
	return path, err
}

// lpWorktreePaths reads the worktree paths of git worktree list --porcelain -z.
func lpWorktreePaths(listing []byte) []string {
	var paths []string
	for _, field := range strings.Split(string(listing), "\x00") {
		if path, ok := strings.CutPrefix(field, "worktree "); ok && path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// lpRealPath is the real path of path, or the cleaned path when it does not
// resolve (a pruned worktree).
func lpRealPath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Clean(path)
}

// lpWithin reports that path equals parent or lies inside it.
func lpWithin(path, parent string) bool {
	return path == parent || strings.HasPrefix(path, strings.TrimSuffix(parent, string(filepath.Separator))+string(filepath.Separator))
}

// walk creates or opens each component of <lp> through the root, refusing a
// symlink or a non-directory at any of them, and checks <lp> itself.
func (c *localPatchCache) walk() error {
	parts := strings.Split(c.rel, string(filepath.Separator))
	for i := range parts {
		name := filepath.Join(parts[:i+1]...)
		info, err := c.root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			if err = c.root.Mkdir(name, 0o700); err == nil {
				info, err = c.root.Lstat(name)
			}
		}
		if err != nil || !info.IsDir() {
			return errors.Join(errLocalPatchCache, err)
		}
	}
	if !c.available() {
		return errLocalPatchCache
	}
	return nil
}

// available is REQ-14's rule for <lp>: a directory of the current user
// without group or other permission bits, reached without a symlink.
func (c *localPatchCache) available() bool {
	info, err := c.root.Lstat(c.rel)
	return err == nil && info.IsDir() && info.Mode().Perm()&0o077 == 0 && lpOwnedByCurrentUser(info)
}

// close releases the root.
func (c *localPatchCache) close() error {
	if c == nil {
		return nil
	}
	return c.root.Close()
}

// path is the absolute path of name below <lp>.
func (c *localPatchCache) path(name string) string { return filepath.Join(c.dir, name) }

func (c *localPatchCache) relPath(name string) string { return filepath.Join(c.rel, name) }

// artifactExists reports an entry of any type at <lp>/<key>/, <key>.patch,
// <key>.diff, or <key>.lock; a read fault is an error.
func (c *localPatchCache) artifactExists(key string) (bool, error) {
	for _, name := range []string{key, key + ".patch", key + ".diff", key + ".lock"} {
		_, err := c.root.Lstat(c.relPath(name))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

// retained is the retention count of <lp>: the distinct keys that have a
// <key>/ directory or a <key>.patch entry, by one ReadDir (no git).
func (c *localPatchCache) retained() (int, error) {
	dir, err := c.root.Open(c.rel)
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return 0, err
	}
	keys := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			keys[entry.Name()] = true
		} else if key, ok := strings.CutSuffix(entry.Name(), ".patch"); ok {
			keys[key] = true
		}
	}
	return len(keys), nil
}

// create writes a new file below <lp> with mode 0600 through O_CREAT,
// O_EXCL, and O_NOFOLLOW; a partial file is removed.
func (c *localPatchCache) create(name string, data []byte) error {
	file, err := c.root.OpenFile(c.relPath(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL|lpNoFollowFlag, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		_ = c.root.Remove(c.relPath(name))
	}
	return err
}

// mkdirKey creates <lp>/<key>/ with mode 0700; it fails when anything is there.
func (c *localPatchCache) mkdirKey(key string) error { return c.root.Mkdir(c.relPath(key), 0o700) }

// removeEmpty removes <lp>/<name> when it is an empty directory or a file;
// a directory with entries stays.
func (c *localPatchCache) removeEmpty(name string) error { return c.root.Remove(c.relPath(name)) }

// rename moves <lp>/<from> to <lp>/<to>.
func (c *localPatchCache) rename(from, to string) error {
	return c.root.Rename(c.relPath(from), c.relPath(to))
}
