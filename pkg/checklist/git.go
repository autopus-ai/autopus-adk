package checklist

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path"
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
		base = "HEAD"
	}
	out, err := git(root, "merge-base", base, "HEAD")
	if err != nil {
		return "", ErrBaseUnavailable
	}
	return strings.TrimSpace(string(out)), nil
}

// CollectChangeSet is every committed and uncommitted change since mb, plus
// untracked files with all their lines counted as added.
func CollectChangeSet(root, mb string) (ChangeSet, error) {
	cs := ChangeSet{Added: map[string][]string{}}
	seen := map[string]bool{}
	add := func(p string) {
		if loopState(p) {
			return
		}
		if p != "" && !seen[p] {
			seen[p] = true
			cs.Paths = append(cs.Paths, p)
		}
	}
	names, err := git(root, "diff", "--name-only", "--no-renames", mb)
	if err != nil {
		return cs, err
	}
	for _, p := range strings.Split(strings.TrimSpace(string(names)), "\n") {
		add(p)
	}
	diff, err := git(root, "diff", "-U0", "--no-color", "--no-renames", mb)
	if err != nil {
		return cs, err
	}
	parseAddedLines(diff, cs.Added)
	others, err := git(root, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return cs, err
	}
	for _, p := range strings.Split(strings.TrimSpace(string(others)), "\n") {
		if p == "" {
			continue
		}
		add(p)
		if loopState(p) {
			continue
		}
		if data, err := readFile(root, p); err == nil && len(data) <= maxUntrackedBytes {
			cs.Added[p] = strings.Split(string(data), "\n")
		}
	}
	return cs, nil
}

func parseAddedLines(diff []byte, added map[string][]string) {
	current := ""
	sc := bufio.NewScanner(bytes.NewReader(diff))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			current = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
			if current == "/dev/null" {
				current = ""
			}
		case strings.HasPrefix(line, "+") && current != "":
			added[current] = append(added[current], line[1:])
		}
	}
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
