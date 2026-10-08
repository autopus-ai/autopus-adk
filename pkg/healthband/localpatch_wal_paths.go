package healthband

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Derived paths (Data Contracts, CD-3 L3): every artifact of a claim lives
// at a path derived from <lp> and the claim's <key>, and band never uses a
// path that a record stores.

const (
	localPatchHashHex = 12
	// localPatchCacheParts are the components below the user cache directory.
	localPatchCacheParts = "autopus/local-patches"
)

var errLocalPatchLocation = errors.New("healthband: the repository's git common directory is unavailable")

// LocalPatchKey returns <series-slug>-<h8>-<episode-id>-<c8>: both slugs
// follow the slug rule (BandSlug: lowercase, runs outside [a-z0-9] folded to
// one "-", trimmed, then cut to 40 bytes), <h8> is H8 of the series ID, and
// <c8> the first 8 hex of the record claim id. The key alphabet is
// [a-z0-9-]. The live executor, phase A, and recovery all derive the key
// here, so a record that one writes is the key that another derives.
func LocalPatchKey(series, episodeID, claimID string) string {
	return BandSlug(series) + "-" + H8(series) + "-" + BandSlug(episodeID) + "-" + claimID[:min(len(claimID), 8)]
}

// LocalPatchRepoHash is <repo-hash>: the first 12 hex of the SHA-256 of the
// absolute git common directory, so every worktree of one repository shares
// one <lp>.
func LocalPatchRepoHash(commonDir string) string {
	sum := sha256.Sum256([]byte(commonDir))
	return hex.EncodeToString(sum[:])[:localPatchHashHex]
}

// LocalPatchLocation is where <lp> lives for one repository, as a path:
// resolving it creates nothing (Open creates and checks it).
type LocalPatchLocation struct {
	CacheDir  string // real path of the user cache directory
	RepoHash  string
	Path      string // <CacheDir>/autopus/local-patches/<RepoHash>
	TopLevel  string // the repository's top level, which <lp> must stay outside
	CommonDir string // its git common directory, likewise
}

// LocalPatchPaths are the derived artifacts of one key.
type LocalPatchPaths struct {
	Key       string
	KeyDir    string // <lp>/<key>
	Worktree  string // <lp>/<key>/worktree
	Patch     string // <lp>/<key>.patch
	PatchTemp string // <lp>/<key>.patch.tmp-<c8>
	Diff      string // <lp>/<key>.diff
	Lock      string // <lp>/<key>.lock
	Ref       string // refs/heads/autopus/band/<key>
	Branch    string // autopus/band/<key>, the form records store
}

// Paths derives every artifact path of key below the location.
func (loc LocalPatchLocation) Paths(key string) LocalPatchPaths {
	c8 := key[max(strings.LastIndexByte(key, '-')+1, 0):]
	return LocalPatchPaths{
		Key: key, KeyDir: filepath.Join(loc.Path, key), Worktree: filepath.Join(loc.Path, key, "worktree"),
		Patch: filepath.Join(loc.Path, key+".patch"), PatchTemp: filepath.Join(loc.Path, key+".patch.tmp-"+c8),
		Diff: filepath.Join(loc.Path, key+".diff"), Lock: filepath.Join(loc.Path, key+".lock"),
		Ref: BandBranchRef(key), Branch: strings.TrimPrefix(BandBranchRef(key), "refs/heads/"),
	}
}

func localPatchDirOf(cacheDir, repoHash string) string {
	return filepath.Join(cacheDir, filepath.FromSlash(localPatchCacheParts), repoHash)
}

// ResolveLocalPatchLocation resolves <lp> before phase A's store.Lock: the
// git common directory and top level of the checkout that git runs in, and
// the real path of cacheDir ("" is os.UserCacheDir). A cache directory that
// does not exist yet gets the real path it would have; Open creates it.
func ResolveLocalPatchLocation(ctx context.Context, git GitPolicyRunner, cacheDir string) (LocalPatchLocation, error) {
	common, err := git.Run(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return LocalPatchLocation{}, errors.Join(errLocalPatchLocation, err)
	}
	top, err := git.Run(ctx, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return LocalPatchLocation{}, errors.Join(errLocalPatchLocation, err)
	}
	loc := LocalPatchLocation{CommonDir: gitLine(common), TopLevel: gitLine(top)}
	if !filepath.IsAbs(loc.CommonDir) || !filepath.IsAbs(loc.TopLevel) {
		return LocalPatchLocation{}, errLocalPatchLocation
	}
	if cacheDir == "" {
		if cacheDir, err = os.UserCacheDir(); err != nil {
			return LocalPatchLocation{}, err
		}
	}
	if cacheDir, err = filepath.Abs(cacheDir); err != nil {
		return LocalPatchLocation{}, err
	}
	// <lp> derives from the real path, the form a worktree is registered
	// under and the form every stored path is compared with.
	loc.CacheDir, loc.RepoHash = realPathOf(cacheDir), LocalPatchRepoHash(loc.CommonDir)
	loc.Path = localPatchDirOf(loc.CacheDir, loc.RepoHash)
	return loc, nil
}

// gitLine is one line of git output without its newline.
func gitLine(out []byte) string { return strings.TrimSuffix(string(out), "\n") }
