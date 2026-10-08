package healthband

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Parsers of the git output that the Cleanup Rules and recovery read. Every
// format is the -z form, so no path byte can forge a field.

// gitWorktree is one entry of git worktree list --porcelain -z.
type gitWorktree struct {
	path   string
	head   string
	locked bool
}

// parseWorktreeList reads NUL-separated attribute lines; an empty field ends
// an entry.
func parseWorktreeList(out []byte) []gitWorktree {
	var list []gitWorktree
	current := -1
	for _, field := range strings.Split(string(out), "\x00") {
		switch {
		case field == "":
			current = -1
		case strings.HasPrefix(field, "worktree "):
			list = append(list, gitWorktree{path: strings.TrimPrefix(field, "worktree ")})
			current = len(list) - 1
		case current < 0:
		case strings.HasPrefix(field, "HEAD "):
			list[current].head = strings.TrimPrefix(field, "HEAD ")
		case field == "locked" || strings.HasPrefix(field, "locked "):
			list[current].locked = true
		}
	}
	return list
}

// findWorktree returns the entry whose path equals path.
func findWorktree(list []gitWorktree, path string) (gitWorktree, bool) {
	for _, worktree := range list {
		if filepath.Clean(worktree.path) == filepath.Clean(path) {
			return worktree, true
		}
	}
	return gitWorktree{}, false
}

// gitdirFileBytes bounds an admin directory's gitdir file read.
const gitdirFileBytes = 4096

// adminDirFor returns the admin directory under <common>/worktrees/ whose
// gitdir file names <worktree>/.git, absolute or relative to that directory.
func adminDirFor(commonDir, worktree string) (string, bool) {
	parent := filepath.Join(commonDir, "worktrees")
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", false
	}
	want := filepath.Join(worktree, ".git")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		admin := filepath.Join(parent, entry.Name())
		file, err := OpenRegular(filepath.Join(admin, "gitdir"))
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(file, gitdirFileBytes))
		_ = file.Close()
		target := strings.TrimRight(string(data), "\r\n")
		if err != nil || target == "" {
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(admin, target)
		}
		if filepath.Clean(target) == want {
			return admin, true
		}
	}
	return "", false
}

// exists reports an entry at path without following a final symlink.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// gitCommit is the part of a commit object that identifies a claim commit.
type gitCommit struct {
	tree    string
	parents []string
	message []byte
}

// parseCommit reads git cat-file commit output: header lines up to the first
// blank line, then the message bytes. Continuation lines start with a space.
func parseCommit(data []byte) (gitCommit, bool) {
	header, message, found := bytes.Cut(data, []byte("\n\n"))
	if !found {
		return gitCommit{}, false
	}
	commit := gitCommit{message: message}
	for _, line := range strings.Split(string(header), "\n") {
		if tree, ok := strings.CutPrefix(line, "tree "); ok {
			commit.tree = tree
		} else if parent, ok := strings.CutPrefix(line, "parent "); ok {
			commit.parents = append(commit.parents, parent)
		}
	}
	return commit, validGitOID(commit.tree)
}

// zFields splits -z output into its non-empty fields.
func zFields(out []byte) []string {
	var fields []string
	for _, field := range strings.Split(string(out), "\x00") {
		if field != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

// indexFlagged reports an assume-unchanged (lowercase tag) or skip-worktree
// (S) entry in git ls-files -v -z output (CD-3 F-007).
func indexFlagged(out []byte) bool {
	for _, field := range zFields(out) {
		if tag := field[0]; tag >= 'a' && tag <= 'z' || tag == 'S' {
			return true
		}
	}
	return false
}

// gitlinkPaths returns the mode-160000 paths of git ls-files -s -z output:
// "<mode> <object> <stage>\t<path>".
func gitlinkPaths(out []byte) []string {
	var paths []string
	for _, field := range zFields(out) {
		meta, path, found := strings.Cut(field, "\t")
		if found && strings.HasPrefix(meta, "160000 ") {
			paths = append(paths, path)
		}
	}
	return paths
}

// onlyStaged reports git status --porcelain -z output whose every entry is
// a staged modification or addition ("M " or "A ") with nothing unstaged,
// untracked, or ignored.
func onlyStaged(out []byte) bool {
	fields := zFields(out)
	for _, field := range fields {
		if len(field) < 4 || field[2] != ' ' || field[1] != ' ' || field[0] != 'M' && field[0] != 'A' {
			return false
		}
	}
	return len(fields) > 0
}
