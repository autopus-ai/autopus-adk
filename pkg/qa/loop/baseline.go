package loop

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// gitExcludeRel names the repository's info/exclude file to the guard. No
// diff shows it, because it lives inside .git, so the loop compares it itself.
const gitExcludeRel = ".git/info/exclude"

// baseline is what the work tree held outside HEAD before an agent ran: every
// untracked file, ignored or not, plus the info/exclude rules. A path in it
// predates the agent, so the loop never commits or removes it, whatever the
// agent does to the ignore rules.
type baseline struct {
	files map[string]bool
	// ignoredDirs are directories an ignore rule matches ("node_modules/").
	// Git reports each once, so a large ignored tree is never enumerated.
	ignoredDirs []string
	excludePath string
	exclude     []byte
	hadExclude  bool
}

// has reports whether p, a repository-relative path, predates the agent.
func (b baseline) has(p string) bool {
	if b.files[p] {
		return true
	}
	for _, dir := range b.ignoredDirs {
		if strings.HasPrefix(p, dir) {
			return true
		}
	}
	return false
}

// snapshot records the baseline. --ignored=matching reports a directory an
// ignore rule matches as one entry, but lists each ignored file of a directory
// no rule matches, so a file the agent adds beside those is still its own.
func (g gitRepo) snapshot() (baseline, error) {
	out, err := g.run("--no-optional-locks", "status", "--porcelain=v1", "-z",
		"--ignored=matching", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		return baseline{}, err
	}
	b := baseline{files: map[string]bool{}}
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		code, p := entry[:2], entry[3:]
		switch {
		case code == "!!" && strings.HasSuffix(p, "/"):
			b.ignoredDirs = append(b.ignoredDirs, p)
		case code == "!!" || code == "??":
			b.files[p] = true
		case code[0] == 'R' || code[0] == 'C':
			i++ // a rename or copy carries its source path as the next field
		}
	}
	if b.excludePath, err = g.excludePath(); err != nil {
		return baseline{}, err
	}
	b.exclude, err = os.ReadFile(b.excludePath)
	switch {
	case err == nil:
		b.hadExclude = true
	case os.IsNotExist(err):
		err = nil
	}
	return b, err
}

// excludeChanged reports an info/exclude that differs from the baseline.
func (b baseline) excludeChanged() bool {
	if b.excludePath == "" {
		return false
	}
	now, err := os.ReadFile(b.excludePath)
	if err != nil {
		return b.hadExclude || !os.IsNotExist(err)
	}
	return !b.hadExclude || !bytes.Equal(now, b.exclude)
}

// restoreExclude puts info/exclude back the way the baseline found it. It
// removes whatever the agent left there first, so a planted symlink is
// replaced rather than written through.
func (b baseline) restoreExclude() error {
	if !b.excludeChanged() {
		return nil
	}
	if err := os.Remove(b.excludePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if !b.hadExclude {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(b.excludePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(b.excludePath, b.exclude, 0o644)
}
