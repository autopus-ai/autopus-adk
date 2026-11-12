package diskspace

import (
	"context"
	"path/filepath"
	"strings"
)

// SkipIgnored keeps a worktree outside the harness directories that holds
// git-ignored files other than regenerable build output. `git worktree
// remove` deletes ignored files without --force, and in a worktree the user
// made those may be local configuration or data (.env, a local database).
const SkipIgnored = "ignored_files"

// ignoredFiles reports whether the worktree at path holds anything git
// ignores outside the build output directories regenerableDirs names. Any
// status entry that is not an ignored one also counts as True, the side
// that keeps the worktree; a failed status is Unknown.
func ignoredFiles(ctx context.Context, run Runner, path string) Tristate {
	out, err := git(ctx, run, path, "status", "--porcelain=v1", "-z", "--ignored=matching", "--untracked-files=normal")
	if err != nil {
		return Unknown
	}
	for _, record := range strings.Split(string(out), "\x00") {
		if record == "" {
			continue
		}
		rel, ignored := strings.CutPrefix(record, "!! ")
		if !ignored || !regenerableEntry(path, rel) {
			return True
		}
	}
	return False
}

// regenerableEntry reports whether rel, a path git printed relative to the
// worktree root, lies inside a regenerable build output directory: one of
// its directory components is named in regenerableDirs and that
// component's parent holds the matching manifest.
func regenerableEntry(root, rel string) bool {
	parts := strings.Split(strings.TrimSuffix(rel, "/"), "/")
	for i, part := range parts {
		if i == len(parts)-1 && !strings.HasSuffix(rel, "/") {
			// The last component of a file entry is the file itself.
			break
		}
		manifest, ok := regenerableDirs[part]
		if !ok {
			continue
		}
		parent := filepath.Join(append([]string{root}, parts[:i]...)...)
		if regularFile(filepath.Join(parent, manifest)) {
			return true
		}
	}
	return false
}
