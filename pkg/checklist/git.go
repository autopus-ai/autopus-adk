package checklist

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

// maxUntrackedBytes bounds how much of an untracked file is read as added lines.
const maxUntrackedBytes = 1 << 20

// ErrBaseUnavailable means the base commit is not in this checkout.
var ErrBaseUnavailable = errors.New("base commit not available: fetch it, or run without checks")

func git(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// MergeBase resolves the commit whose tree holds the checks for a change
// against base. A change cannot alter the checks that judge it.
func MergeBase(root, base string) (string, error) {
	if base == "" {
		base = DefaultBase(root)
	}
	out, err := git(root, "merge-base", base, "HEAD")
	if err != nil {
		return "", ErrBaseUnavailable
	}
	return strings.TrimSpace(string(out)), nil
}

// diffArgs pin the output format against user and repository config: no
// quoting of non-ASCII paths and fixed a/ b/ prefixes.
var diffArgs = []string{
	"-c", "core.quotePath=false", "-c", "diff.mnemonicPrefix=false", "-c", "diff.noprefix=false",
}

// DefaultBase is the remote default branch when the checkout knows it, so a
// branch is judged by the checks on main, not by ones its own commits
// changed; otherwise HEAD, which covers only uncommitted changes.
func DefaultBase(root string) string {
	if _, err := git(root, "rev-parse", "--verify", "--quiet", "origin/HEAD"); err == nil {
		return "origin/HEAD"
	}
	return "HEAD"
}

// ProjectRoot is the top of the git work tree holding dir, or dir outside git.
func ProjectRoot(dir string) string {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return dir
	}
	return strings.TrimSpace(string(out))
}

// gitDiff runs git diff with the attributes of the base tree (git 2.40+), so
// a change cannot add a -diff or filter attribute that hides its own added
// lines; binary files stay binary and add no lines. Older git falls back to
// the working tree's attributes.
func gitDiff(root, mb string, args ...string) ([]byte, error) {
	base := append(append([]string(nil), diffArgs...), "--attr-source="+mb, "diff")
	out, err := git(root, append(base, args...)...)
	if err != nil && strings.Contains(err.Error(), "attr-source") {
		return git(root, append(append(append([]string(nil), diffArgs...), "diff"), args...)...)
	}
	return out, err
}

// CollectChangeSet is every committed and uncommitted change since mb, plus
// untracked files with all their lines counted as added.
func CollectChangeSet(root, mb string) (ChangeSet, error) {
	cs := ChangeSet{Added: map[string][]string{}}
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || loopState(p) || seen[p] {
			return
		}
		seen[p] = true
		cs.Paths = append(cs.Paths, p)
	}
	names, err := gitDiff(root, mb, "--name-only", "-z", "--no-renames", "--no-ext-diff", mb)
	if err != nil {
		return cs, err
	}
	for _, p := range strings.Split(string(names), "\x00") {
		add(p)
	}
	diff, err := gitDiff(root, mb, "-U0", "--no-color", "--no-renames", "--no-ext-diff",
		"--src-prefix=a/", "--dst-prefix=b/", mb)
	if err != nil {
		return cs, err
	}
	if err := parseAddedLines(diff, cs.Added); err != nil {
		return cs, err
	}
	others, err := git(root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return cs, err
	}
	for _, p := range strings.Split(string(others), "\x00") {
		if p == "" || loopState(p) {
			continue
		}
		add(p)
		if data, ok := readSmallFile(root, p, maxUntrackedBytes); ok {
			cs.Added[p] = strings.Split(string(data), "\n")
		}
	}
	return cs, nil
}

// parseAddedLines collects the added lines per file. A "+++ " line is a
// file header only right after its "--- " line; inside a hunk it is an added
// line that starts with "++".
func parseAddedLines(diff []byte, added map[string][]string) error {
	current, prevOld := "", false
	r := bufio.NewReader(bytes.NewReader(diff))
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			switch {
			case prevOld && strings.HasPrefix(line, "+++ "):
				current = headerPath(strings.TrimPrefix(line, "+++ "))
			case strings.HasPrefix(line, "+") && current != "":
				added[current] = append(added[current], line[1:])
			}
			prevOld = strings.HasPrefix(line, "--- ")
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// headerPath turns a "+++ b/<path>" header value into the path: git appends
// a tab to a path with spaces and C-quotes one with special characters.
func headerPath(v string) string {
	v = strings.TrimSuffix(v, "\t")
	if strings.HasPrefix(v, `"`) {
		if u, err := strconv.Unquote(v); err == nil {
			v = u
		}
	}
	if v == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(v, "b/")
}

// BaseChecks loads the checks stored in commit mb. Files that do not parse
// are returned in invalid with their reason.
func BaseChecks(root, mb string) (checks []Check, invalid map[string]string, err error) {
	invalid = map[string]string{}
	out, err := git(root, "ls-tree", "--name-only", mb, ChecksDir+"/")
	if err != nil {
		return nil, invalid, nil
	}
	for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if path.Ext(p) != ".md" || path.Dir(p) != ChecksDir {
			continue
		}
		data, err := git(root, "show", mb+":"+p)
		if err != nil {
			invalid[p] = err.Error()
			continue
		}
		c, err := ParseCheck(data)
		if err != nil {
			invalid[p] = err.Error()
			continue
		}
		checks = append(checks, c)
	}
	return checks, invalid, nil
}

// ChangedChecks lists check files the change itself adds, modifies, or
// deletes; they are not applied to the change that carries them.
func ChangedChecks(root string, cs ChangeSet, base []Check) map[string]string {
	inBase := map[string]bool{}
	for _, c := range base {
		inBase[path.Join(ChecksDir, c.ID+".md")] = true
	}
	changed := map[string]string{}
	for _, p := range cs.Paths {
		if path.Dir(p) != ChecksDir || path.Ext(p) != ".md" {
			continue
		}
		_, readErr := readFile(root, p)
		switch {
		case inBase[p] && readErr != nil:
			changed[p] = "deleted"
		case inBase[p]:
			changed[p] = "modified"
		default:
			changed[p] = "added"
		}
	}
	return changed
}

// loopState reports the loop's own files, which are not part of the change
// a check judges: candidates and the hit log change on every run.
func loopState(p string) bool {
	return strings.HasPrefix(p, CandidatesDir+"/") || strings.HasPrefix(p, ".autopus/runtime/")
}
