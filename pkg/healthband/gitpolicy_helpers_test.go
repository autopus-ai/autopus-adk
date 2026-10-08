//go:build unix

package healthband

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// gpFixture is the temp world of a Git Execution Policy test: a setup HOME
// with a plain global configuration for building repositories, a band HOME
// whose global configuration the test controls and whose trace2.eventTarget
// only band commands write (band strips every GIT_* variable, so
// GIT_TRACE2_EVENT could not reach them), a bin directory of marker scripts
// in front of PATH, and a marker directory.
type gpFixture struct {
	t        *testing.T
	root     string
	bin      string
	markers  string
	setupEnv []string
}

const gpBaseGlobal = "[user]\n\tname = Setup\n\temail = setup@example.invalid\n" +
	"[init]\n\tdefaultBranch = main\n[protocol \"file\"]\n\tallow = always\n"

func newGPFixture(t *testing.T) *gpFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	f := &gpFixture{t: t, root: root, bin: filepath.Join(root, "bin"), markers: filepath.Join(root, "markers")}
	require.NoError(t, os.MkdirAll(f.bin, 0o700))
	require.NoError(t, os.MkdirAll(f.markers, 0o700))
	// Setup git starts no background maintenance that could race a test.
	f.setupEnv = f.home("setup-home", "[maintenance]\n\tauto = false\n[gc]\n\tauto = 0\n", "")
	return f
}

// home writes a HOME whose .gitconfig is the base configuration plus extra
// and, when trace is set, a trace2 event target; it returns its environment.
func (f *gpFixture) home(name, extra, trace string) []string {
	home := filepath.Join(f.root, name)
	require.NoError(f.t, os.MkdirAll(filepath.Join(home, ".config"), 0o700))
	config := gpBaseGlobal + extra
	if trace != "" {
		config += "[trace2]\n\teventTarget = " + trace + "\n"
	}
	require.NoError(f.t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0o600))
	return []string{
		"PATH=" + f.bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "TMPDIR=" + os.TempDir(),
	}
}

// script writes an executable shell script into the bin directory.
func (f *gpFixture) script(name, body string) string {
	path := filepath.Join(f.bin, name)
	require.NoError(f.t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700))
	return path
}

// marker returns a script line that records name in the marker directory;
// name may use the script's arguments, such as lfs-$1.
func (f *gpFixture) marker(name string) string {
	return `touch "` + filepath.Join(f.markers, name) + "\"\n"
}

func (f *gpFixture) markerNames() []string {
	entries, err := os.ReadDir(f.markers)
	require.NoError(f.t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// git runs raw git (no policy) for setup and controls and returns stdout.
func (f *gpFixture) git(env []string, dir string, args ...string) string {
	f.t.Helper()
	out, err := f.gitErr(env, dir, args...)
	require.NoError(f.t, err, "git %s", strings.Join(args, " "))
	return out
}

func (f *gpFixture) gitErr(env []string, dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, env
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.String(), err
}

// repo initializes name with files (a value starting with "->" is a symlink
// target, "+x " marks an executable) and commits them; it returns the path
// and the commit.
func (f *gpFixture) repo(name string, files map[string]string) (string, string) {
	dir := filepath.Join(f.root, name)
	f.git(f.setupEnv, f.root, "init", "-q", dir)
	f.writeFiles(dir, files)
	f.git(f.setupEnv, dir, "add", "-A")
	f.git(f.setupEnv, dir, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "base")
	return dir, strings.TrimSpace(f.git(f.setupEnv, dir, "rev-parse", "HEAD"))
}

func (f *gpFixture) writeFiles(dir string, files map[string]string) {
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
		if target, link := strings.CutPrefix(content, "->"); link {
			require.NoError(f.t, os.Symlink(target, path))
			continue
		}
		mode := os.FileMode(0o644)
		if body, exec := strings.CutPrefix(content, "+x "); exec {
			content, mode = body, 0o755
		}
		require.NoError(f.t, os.WriteFile(path, []byte(content), mode))
	}
}

// runner is a policy runner in dir over env.
func (f *gpFixture) runner(env []string, dir string) GitPolicyRunner {
	return GitPolicyRunner{Dir: dir, Environ: func() []string { return slices.Clone(env) }}
}

// gpChild is one trace2 child_start event.
type gpChild struct {
	Class string   `json:"child_class"`
	Argv  []string `json:"argv"`
}

// gpChildren reads the child_start events of a trace2 event file.
func gpChildren(t *testing.T, path string) []gpChild {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var children []gpChild
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<24)
	for scanner.Scan() {
		var event struct {
			Event string `json:"event"`
			gpChild
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Event == "child_start" {
			children = append(children, event.gpChild)
		}
	}
	require.NoError(t, scanner.Err())
	return children
}

// gpNetwork are the subcommands that S12 counts as network children.
var gpNetwork = []string{"fetch", "push", "ls-remote", "upload-pack", "receive-pack"}

// gpCount counts children of class (any class when "") with an argv word
// equal to one of words or containing one of parts; with neither, every
// child of the class counts.
func gpCount(children []gpChild, class string, words []string, parts ...string) int {
	count := 0
	for _, child := range children {
		if class != "" && child.Class != class {
			continue
		}
		if len(words)+len(parts) == 0 || slices.ContainsFunc(child.Argv, func(word string) bool {
			return slices.Contains(words, word) ||
				slices.ContainsFunc(parts, func(part string) bool { return strings.Contains(word, part) })
		}) {
			count++
		}
	}
	return count
}

func gpContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func gpTrim(out []byte) string { return strings.TrimSuffix(string(out), "\n") }
