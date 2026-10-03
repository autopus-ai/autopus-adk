package loop

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// runtimeRoots are project-relative directories the harness or the test runner
// writes on every run. New untracked files there are evidence, not part of a
// fix, so the guard neither judges, stages, nor removes them.
var runtimeRoots = []string{".autopus/qa/runs/", ReportDirRel + "/", "test-results/", "playwright-report/", "blob-report/"}

func isRuntimeArtifact(rel string) bool {
	for _, root := range runtimeRoots {
		if strings.HasPrefix(rel, root) {
			return true
		}
	}
	return false
}

// runGit runs git with literal pathspecs, so a changed path is never read as
// a glob that widens a checkout or an add.
func runGit(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "GIT_LITERAL_PATHSPECS=1"), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// gitRepo runs git at the repository top so every path it reports shares one
// root, whatever subdirectory the project lives in.
type gitRepo struct {
	top string
	env []string
}

func (g gitRepo) run(args ...string) (string, error) { return runGit(g.top, g.env, args...) }

func (g gitRepo) line(args ...string) (string, error) {
	out, err := g.run(args...)
	return strings.TrimSpace(out), err
}

// paths runs a -z command and splits its NUL-separated output.
func (g gitRepo) paths(args ...string) ([]string, error) {
	out, err := g.run(args...)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func (g gitRepo) trackedDirty() ([]string, error) {
	out, err := g.run("status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines, nil
}

// currentRef returns the branch name, or the commit when HEAD is detached.
func (g gitRepo) currentRef() (ref string, detached bool, err error) {
	if name, err := g.line("symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && name != "" {
		return name, false, nil
	}
	sha, err := g.line("rev-parse", "HEAD")
	return sha, true, err
}

func (g gitRepo) untracked() (map[string]bool, error) {
	paths, err := g.paths("ls-files", "-z", "--others", "--exclude-standard")
	return setOf(paths), err
}

// changed lists the paths that differ from HEAD, staged or not, plus the
// untracked files absent from baseline. Renames are split so a moved test
// cannot hide behind its new name.
func (g gitRepo) changed(baseline map[string]bool, prefix string) ([]string, error) {
	tracked, err := g.paths("diff", "--name-only", "--no-renames", "-z", "HEAD")
	if err != nil {
		return nil, err
	}
	untracked, err := g.untracked()
	if err != nil {
		return nil, err
	}
	set := setOf(tracked)
	for p := range untracked {
		rel, inProject := strings.CutPrefix(p, prefix)
		if !baseline[p] && !(inProject && isRuntimeArtifact(rel)) {
			set[p] = true
		}
	}
	return sortedKeys(set), nil
}

// revert undoes everything changed since baseline was taken: tracked paths
// return to HEAD, files created since are removed, and untracked files that
// predate the baseline are left alone. It returns the paths it handled.
func (g gitRepo) revert(baseline map[string]bool, prefix string) ([]string, error) {
	changed, err := g.changed(baseline, prefix)
	if err != nil || len(changed) == 0 {
		return changed, err
	}
	staged, err := g.paths("diff", "--cached", "--name-only", "--no-renames", "-z", "HEAD")
	if err != nil {
		return nil, err
	}
	if len(staged) > 0 {
		if _, err := g.run(append([]string{"reset", "-q", "HEAD", "--"}, staged...)...); err != nil {
			return nil, err
		}
	}
	inHead, err := g.paths(append([]string{"ls-tree", "-r", "-z", "--name-only", "HEAD", "--"}, changed...)...)
	if err != nil {
		return nil, err
	}
	head := setOf(inHead)
	var restore []string
	for _, p := range changed {
		switch {
		case head[p]:
			restore = append(restore, p)
		case baseline[p]:
			// An untracked file that predates the agent: unstaged above, kept.
		default:
			if err := os.Remove(filepath.Join(g.top, filepath.FromSlash(p))); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}
	}
	if len(restore) > 0 {
		if _, err := g.run(append([]string{"checkout", "HEAD", "--"}, restore...)...); err != nil {
			return nil, err
		}
	}
	return changed, nil
}

// commit stages exactly paths and commits them through the project's hooks.
func (g gitRepo) commit(paths []string, message string) (string, error) {
	if _, err := g.run(append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "autopus-qaloop-commit-*.txt")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(message)
	if closeErr := file.Close(); writeErr != nil || closeErr != nil {
		return "", fmt.Errorf("write commit message: %v %v", writeErr, closeErr)
	}
	if _, err := g.run("commit", "-q", "-F", file.Name()); err != nil {
		return "", err
	}
	return g.line("rev-parse", "HEAD")
}

// ensureExcluded adds pattern to the repository's info/exclude once, so loop
// reports never show up as untracked work.
func (g gitRepo) ensureExcluded(pattern string) error {
	path, err := g.line("rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(g.top, path)
	}
	body, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	lead := ""
	if len(body) > 0 && !bytes.HasSuffix(body, []byte("\n")) {
		lead = "\n"
	}
	_, writeErr := file.WriteString(lead + "# auto qa loop reports\n" + pattern + "\n")
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr
}

func setOf(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// oneLine flattens evidence text for single-line report fields.
func oneLine(text string) string {
	flat := strings.Join(strings.Fields(text), " ")
	if len(flat) > 300 {
		return flat[:300] + "..."
	}
	return flat
}
