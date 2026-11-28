package harnessfb

import (
	"context"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// PathIndex resolves the paths reviewers cite against the repository: the
// roots of its worktrees and the files it has. Observe builds it; without
// one (a bare transcript read) paths are taken relative to the session cwd.
type PathIndex struct {
	Roots  []string            // worktree roots, longest first
	Files  map[string]bool     // repository-relative paths
	ByBase map[string][]string // file name -> paths
}

// resolve makes a cited path repository-relative, or reports it is not a
// file of this repository.
func (x *PathIndex) resolve(p string) (string, bool) {
	if strings.HasPrefix(p, "/") {
		rel := ""
		for _, r := range x.Roots {
			if strings.HasPrefix(p, r+"/") {
				rel = strings.TrimPrefix(p, r+"/")
				break
			}
		}
		if rel == "" {
			return "", false
		}
		p = rel
	}
	if x.Files[p] || len(x.Files) == 0 {
		return p, true
	}
	if !strings.Contains(p, "/") {
		// A bare file name, as reviewers often write: only a unique one.
		if hits := x.ByBase[p]; len(hits) == 1 {
			return hits[0], true
		}
		return "", false
	}
	// A path under a worktree since removed (.claude/worktrees/<name>/...).
	if rest, ok := strings.CutPrefix(p, ".claude/worktrees/"); ok {
		if _, after, found := strings.Cut(rest, "/"); found {
			return after, true
		}
	}
	// A one-level alias a reviewer put in front (W/, R/): only when that
	// directory is not the repository's and the rest is a tracked file. Any
	// other path is kept as written; it may be a new file or on a branch.
	if first, rest, ok := strings.Cut(p, "/"); ok && !x.top(first) && x.Files[rest] {
		return rest, true
	}
	return p, true
}

// top reports a top-level directory of the repository.
func (x *PathIndex) top(dir string) bool {
	for f := range x.Files {
		if strings.HasPrefix(f, dir+"/") {
			return true
		}
	}
	return false
}

// LoadPathIndex reads the worktree roots and files of root's repository,
// or returns nil when git cannot say.
func LoadPathIndex(ctx context.Context, root string) *PathIndex {
	wt, err := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	x := &PathIndex{Files: map[string]bool{}, ByBase: map[string][]string{}}
	for _, line := range strings.Split(string(wt), "\n") {
		if r, ok := strings.CutPrefix(line, "worktree "); ok {
			x.Roots = append(x.Roots, strings.TrimSuffix(r, "/"))
			if real, err := filepath.EvalSymlinks(r); err == nil && real != r {
				x.Roots = append(x.Roots, strings.TrimSuffix(real, "/"))
			}
		}
	}
	sort.Slice(x.Roots, func(i, j int) bool { return len(x.Roots[i]) > len(x.Roots[j]) })
	files, err := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return x
	}
	for _, f := range strings.Split(string(files), "\x00") {
		if f != "" {
			x.Files[f] = true
			x.ByBase[path.Base(f)] = append(x.ByBase[path.Base(f)], f)
		}
	}
	return x
}
