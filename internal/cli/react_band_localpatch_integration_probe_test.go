//go:build unix

package cli

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Probes of the integration world: snapshots of the user checkout and of
// the bare origin, the git wrapper's ordered call record, and the trace2
// events of every git process that band started, its children included.

// lpitSnapshot is the user checkout and the bare origin at one moment.
type lpitSnapshot struct {
	files, gitDir, bare                map[string]string // SHA-256 by slash path
	status, head, stash, refs, objects string
}

func (w *lpitWorld) snapshot() lpitSnapshot {
	w.t.Helper()
	s := lpitSnapshot{files: map[string]string{}, gitDir: map[string]string{}, bare: lpitHashTree(w.t, w.bare)}
	for rel, sum := range lpitHashTree(w.t, w.repo) {
		if inner, ok := strings.CutPrefix(rel, ".git/"); ok {
			s.gitDir[inner] = sum
			continue
		}
		s.files[rel] = sum
	}
	s.status = w.inspect("status", "--porcelain=v1", "-z", "--untracked-files=all")
	s.head = w.inspect("rev-parse", "HEAD")
	s.stash = w.inspect("stash", "list")
	s.refs = w.inspect("for-each-ref", "--format=%(refname) %(objectname) %(symref)")
	s.objects = w.inspect("count-objects", "-v")
	return s
}

// lpitHashTree hashes every file below root: a regular file by its bytes,
// a symlink by its link text.
func lpitHashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	hashes := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		var data []byte
		if entry.Type()&fs.ModeSymlink != 0 {
			link, linkErr := os.Readlink(path)
			data, err = []byte("symlink:"+link), linkErr
		} else {
			data, err = os.ReadFile(path)
		}
		sum := sha256.Sum256(data)
		rel, _ := filepath.Rel(root, path)
		hashes[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return err
	}))
	return hashes
}

// lpitCall is one entry of the wrapper's ordered record: a git call with
// its cwd, argv, and GIT_* variables, or a fake claude call.
type lpitCall struct {
	claude int
	dir    string
	argv   []string
	env    []string
}

// lpitPolicyFlag opens the argv of every Git Execution Policy command.
var lpitPolicyFlag = []string{"-c", "core.hooksPath=/dev/null"}

// policy splits a Git Execution Policy command into its -c values and the
// arguments after them; ok is false for any other git call.
func (c lpitCall) policy() (config, args []string, ok bool) {
	if c.claude != 0 || len(c.argv) < 2 || !slices.Equal(c.argv[:2], lpitPolicyFlag) {
		return nil, nil, false
	}
	args = c.argv
	for len(args) >= 2 && args[0] == "-c" {
		config, args = append(config, args[1]), args[2:]
	}
	return config, args, true
}

// sub is the subcommand of a policy command, "" for any other call.
func (c lpitCall) sub() string {
	if _, args, ok := c.policy(); ok && len(args) > 0 {
		return args[0]
	}
	return ""
}

func (w *lpitWorld) calls() []lpitCall {
	w.t.Helper()
	entries, err := os.ReadDir(w.gitrec)
	require.NoError(w.t, err)
	var calls []lpitCall
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "call.") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(w.gitrec, entry.Name()))
		require.NoError(w.t, err)
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		if n, ok := strings.CutPrefix(lines[0], "CLAUDE "); ok {
			number, err := strconv.Atoi(n)
			require.NoError(w.t, err)
			calls = append(calls, lpitCall{claude: number})
			continue
		}
		require.GreaterOrEqual(w.t, len(lines), 3, entry.Name())
		argv := strings.Split(strings.TrimSuffix(lines[2], "\x1f"), "\x1f")
		calls = append(calls, lpitCall{dir: lines[1], argv: argv, env: lines[3:]})
	}
	return calls
}

// policyCalls are the Git Execution Policy commands whose subcommand is sub.
func (w *lpitWorld) policyCalls(sub string) []lpitCall {
	var calls []lpitCall
	for _, call := range w.calls() {
		if call.sub() == sub {
			calls = append(calls, call)
		}
	}
	return calls
}

// lpitTrace is the part of a trace2 event that the oracles read.
type lpitTrace struct {
	Event      string   `json:"event"`
	SID        string   `json:"sid"`
	Argv       []string `json:"argv"`
	ChildClass string   `json:"child_class"`
}

func (w *lpitWorld) traceEvents() []lpitTrace {
	w.t.Helper()
	var events []lpitTrace
	entries, err := os.ReadDir(w.trace)
	require.NoError(w.t, err)
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(w.trace, entry.Name()))
		require.NoError(w.t, err)
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(nil, 4<<20)
		for scanner.Scan() {
			var event lpitTrace
			require.NoError(w.t, json.Unmarshal(scanner.Bytes(), &event), scanner.Text())
			events = append(events, event)
		}
		require.NoError(w.t, scanner.Err())
	}
	return events
}

// assertNoHookNorNetwork is the trace2 oracle of S4, S6, and S12: no hook
// ran, and no process started git-lfs, maintenance, gc, a network command,
// or a remote helper.
func (w *lpitWorld) assertNoHookNorNetwork(t *testing.T) {
	t.Helper()
	events := w.traceEvents()
	starts := 0
	forbidden := []string{"git-lfs", "maintenance", "gc", "fetch", "push", "ls-remote", "upload-pack", "receive-pack"}
	for _, event := range events {
		if event.Event == "start" {
			starts++
		}
		if event.Event != "child_start" {
			continue
		}
		assert.NotEqual(t, "hook", event.ChildClass, "a hook ran: %v", event.Argv)
		for _, arg := range event.Argv {
			name := filepath.Base(arg)
			assert.False(t, slices.Contains(forbidden, name) || strings.HasPrefix(name, "git-remote-") ||
				strings.HasPrefix(name, "git-lfs") || strings.HasPrefix(name, "git-upload-pack"), "child %v", event.Argv)
		}
	}
	assert.Greater(t, starts, 10, "trace2 logged band's git processes, so the oracle is not vacuous")
	markers, err := os.ReadDir(w.markers)
	require.NoError(t, err)
	assert.Empty(t, markers, "no marker file: no hook or filter ran")
}

// assertRepoUnchanged compares the user's status, HEAD, stash list, and
// every ref but the band branch with before.
func (w *lpitWorld) assertRepoUnchanged(t *testing.T, before lpitSnapshot, branch string) {
	t.Helper()
	after := w.snapshot()
	assert.Equal(t, before.status, after.status, "status")
	assert.Equal(t, before.head, after.head, "HEAD")
	assert.Equal(t, before.stash, after.stash, "stash list")
	assert.Equal(t, before.gitDir["index"], after.gitDir["index"], "index file hash")
	var refs []string
	for _, line := range strings.Split(after.refs, "\n") {
		if !strings.HasPrefix(line, "refs/heads/"+branch+" ") || branch == "" {
			refs = append(refs, line)
		}
	}
	assert.Equal(t, before.refs, strings.Join(refs, "\n"), "every ref but the band branch")
	assert.Equal(t, before.bare, after.bare, "the bare origin's refs and objects")
}

// assertNoRemoteCall is the S12 recorder oracle: band's own git commands
// hold no network subcommand, 001's git calls are its two read-only forms,
// and gh made no pr call and no api call other than the default-branch GET.
func (w *lpitWorld) assertNoRemoteCall(t *testing.T) {
	t.Helper()
	tracked := []string{"-c", "core.fsmonitor=false", "ls-files", "-z", "--"}
	for _, call := range w.calls() {
		if call.claude != 0 {
			continue
		}
		if _, args, ok := call.policy(); ok {
			assert.NotContains(t, []string{"push", "fetch", "ls-remote", "remote", "pull", "clone"}, args[0], "%v", args)
			continue
		}
		readOnly := slices.Equal(call.argv, []string{"remote", "get-url", "origin"}) ||
			len(call.argv) > len(tracked) && slices.Equal(call.argv[:len(tracked)], tracked)
		assert.True(t, readOnly, "a git call outside the policy and 001's two forms: %v", call.argv)
	}
	data, err := os.ReadFile(filepath.Join(w.gh, "calls"))
	require.NoError(t, err)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		assert.False(t, strings.HasPrefix(line, "pr "), line)
		if strings.HasPrefix(line, "api ") {
			assert.Equal(t, "api repos/acme/app --hostname github.com --jq .default_branch", line)
		}
	}
}
