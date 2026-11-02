package worker

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// hermeticGitEnv isolates git from the developer's configuration
// (SPEC-WORKERPOST-001 REQ-WPOST-VERIFY-01): no global or system config, no
// askpass helper, no proxy, and a throwaway HOME.
func hermeticGitEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "GIT_") || strings.HasSuffix(upper, "_PROXY") || strings.HasPrefix(upper, "LC_") ||
			upper == "HOME" || upper == "XDG_CONFIG_HOME" || upper == "SSH_ASKPASS" || upper == "LANGUAGE" {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.invalid",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.invalid",
		"GIT_TERMINAL_PROMPT=0",
		"NO_PROXY=*",
		"LC_ALL=C", // git messages the tests match are untranslated
	)
}

// gitFixture is a work repository on `main` with one empty base commit and a
// bare `origin` that holds `main`.
type gitFixture struct {
	t    *testing.T
	env  []string
	repo string
	bare string
	base string // SHA of the base commit A0
	git  postconditionGit
}

func newGitFixture(t *testing.T) *gitFixture {
	t.Helper()
	f := &gitFixture{t: t, env: hermeticGitEnv(t), repo: t.TempDir(), bare: t.TempDir()}
	f.git = postconditionGit{env: f.env}
	f.run(f.repo, "init", "-q", "-b", "main")
	f.base = f.commit("A0")
	f.run(f.bare, "init", "-q", "--bare")
	f.run(f.repo, "remote", "add", "origin", f.bare)
	f.run(f.repo, "push", "-q", "origin", "main")
	return f
}

func (f *gitFixture) run(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	require.NoError(f.t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// commit adds an empty commit on HEAD and returns its SHA.
func (f *gitFixture) commit(message string) string {
	f.t.Helper()
	f.run(f.repo, "commit", "-q", "--allow-empty", "-m", message)
	return f.run(f.repo, "rev-parse", "HEAD")
}

// orphan creates a parentless commit unrelated to the base, without moving HEAD.
func (f *gitFixture) orphan(message string) string {
	f.t.Helper()
	return f.run(f.repo, "commit-tree", "HEAD^{tree}", "-m", message)
}

// bareRemote creates another bare repository and returns its path.
func (f *gitFixture) bareRemote() string {
	f.t.Helper()
	dir := f.t.TempDir()
	f.run(dir, "init", "-q", "--bare")
	return dir
}

func (f *gitFixture) refs(dir string) string {
	f.t.Helper()
	return f.run(dir, "for-each-ref")
}

func (f *gitFixture) plan(contract *completionContractMessage, prompt string) taskPostconditions {
	f.t.Helper()
	plan := planTaskPostconditions(contract, prompt)
	require.Empty(f.t, plan.Problems)
	return plan
}

func (f *gitFixture) baseline(plan taskPostconditions) executionBaseline {
	return f.git.captureBaseline(f.repo, plan)
}

// verify judges the plan and decodes the report; an empty report means the
// verifier emitted no artifact.
func (f *gitFixture) verify(plan taskPostconditions, base executionBaseline) (postconditionReport, error) {
	f.t.Helper()
	artifact, err := f.git.verify(f.repo, plan, base)
	var report postconditionReport
	if artifact.Name != "" {
		require.Equal(f.t, "postconditions.json", artifact.Name)
		require.NoError(f.t, json.Unmarshal([]byte(artifact.Data), &report))
	}
	return report, err
}

func decodeReport(t *testing.T, data string) postconditionReport {
	t.Helper()
	var report postconditionReport
	require.NoError(t, json.Unmarshal([]byte(data), &report))
	return report
}

func findCheck(t *testing.T, report postconditionReport, name string) postconditionCheck {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	require.Failf(t, "check not found", "no %q check in %+v", name, report.Checks)
	return postconditionCheck{}
}

// commandLog records every argv the verifier starts.
type commandLog struct {
	mu    sync.Mutex
	argvs [][]string
}

func (l *commandLog) record(argv []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.argvs = append(l.argvs, argv)
}

func (l *commandLog) ran(subcommand string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, argv := range l.argvs {
		if len(argv) > 1 && argv[1] == subcommand {
			return true
		}
	}
	return false
}

// traced returns a copy of the fixture's verifier that records commands.
func (f *gitFixture) traced() (postconditionGit, *commandLog) {
	log := &commandLog{}
	g := f.git
	g.trace = log.record
	return g, log
}

func contractOf(raw string) *completionContractMessage {
	var c completionContractMessage
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		panic(err)
	}
	return &c
}
