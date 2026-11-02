package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// postconditionGit runs the git commands of baseline capture and verification
// (SPEC-WORKERPOST-001 REQ-WPOST-READONLY-01). Every command passes a
// read-only allowlist before a process starts, so no outcome can lead the
// check to commit, push, fetch, or rewrite anything. The zero value uses the
// worker's own environment.
type postconditionGit struct {
	env     []string            // base environment; nil means os.Environ()
	timeout time.Duration       // per command; zero means postconditionTimeout
	trace   func(argv []string) // sees every argv before it starts; nil in production
}

// postconditionWaitDelay bounds how long a killed git may keep its output
// pipes open through a helper such as git-remote-http.
const postconditionWaitDelay = time.Second

// postconditionDroppedEnv are inherited variables that could point git at
// another repository, object store, or history than the work directory's.
var postconditionDroppedEnv = map[string]bool{
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_OBJECT_DIRECTORY": true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_COMMON_DIR": true, "GIT_NAMESPACE": true,
	"GIT_GRAFT_FILE": true, "GIT_REPLACE_REF_BASE": true, "GIT_SHALLOW_FILE": true,
	"GIT_CEILING_DIRECTORIES": true, "GIT_TERMINAL_PROMPT": true,
}

var urlUserinfoPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/@\s'"]+@`)

// gitMessageClip bounds a quoted git message (REQ-WPOST-REPORT-01).
const gitMessageClip = 200

type gitResult struct {
	stdout string
	stderr string // first line, credentials redacted, clipped
	code   int    // exit status; -1 when git did not finish
	err    error  // refusal, start failure, or timeout
}

// checkReadOnlyGit admits only the exact read-only forms the checks use.
// Operands may not look like options: even rev-list writes a file when handed
// --output=<path>.
func checkReadOnlyGit(args []string) error {
	if len(args) == 0 {
		return errors.New("empty git command")
	}
	allowed := false
	switch n := len(args); args[0] {
	case "rev-parse":
		allowed = (n == 4 && args[1] == "--verify" && args[2] == "-q" && gitOperand(args[3])) ||
			(n == 4 && args[1] == "--path-format=absolute" && args[2] == "--git-path" && args[3] == "info/grafts") ||
			(n == 2 && args[1] == "--is-shallow-repository")
	case "rev-list":
		allowed = n == 3 && gitOperand(args[1]) && strings.HasPrefix(args[2], "^") && gitOperand(args[2][1:])
	case "symbolic-ref":
		allowed = n == 4 && args[1] == "-q" && args[2] == "--short" && args[3] == "HEAD"
	case "remote":
		allowed = n == 3 && args[1] == "get-url" && gitOperand(args[2])
	case "merge-base":
		allowed = n == 4 && args[1] == "--is-ancestor" && gitOperand(args[2]) && gitOperand(args[3])
	case "cat-file":
		allowed = n == 3 && args[1] == "-e" && gitOperand(args[2])
	case "ls-remote":
		// The URL follows "--", so git never reads it as an option.
		allowed = (n == 4 && args[1] == "--" && strings.HasPrefix(args[3], branchRefPrefix)) ||
			(n == 4 && args[1] == "--heads" && args[2] == "--")
	}
	if !allowed {
		return fmt.Errorf("git %s is not an allowed read-only postcondition command", args[0])
	}
	return nil
}

func gitOperand(arg string) bool {
	return arg != "" && !strings.HasPrefix(arg, "-")
}

func (g postconditionGit) run(dir string, extraEnv []string, args ...string) gitResult {
	if err := checkReadOnlyGit(args); err != nil {
		return gitResult{code: -1, err: err}
	}
	timeout := g.timeout
	if timeout <= 0 {
		timeout = postconditionTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	argv := append([]string{"--no-replace-objects"}, args...)
	if g.trace != nil {
		g.trace(append([]string(nil), argv...))
	}
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Dir = dir
	cmd.Env = append(g.environ(), extraEnv...)
	cmd.WaitDelay = postconditionWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()

	res := gitResult{stdout: stdout.String(), stderr: redactGitMessage(stderr.String())}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil || errors.Is(runErr, exec.ErrWaitDelay):
		// ErrWaitDelay means git exited 0 while a helper still held a pipe.
		res.code = 0
	case ctx.Err() != nil:
		res.code, res.err = -1, fmt.Errorf("git %s timed out after %s", args[0], timeout)
	case errors.As(runErr, &exitErr) && exitErr.ExitCode() >= 0:
		res.code = exitErr.ExitCode()
	default:
		res.code, res.err = -1, fmt.Errorf("git %s: %w", args[0], runErr)
	}
	return res
}

func (g postconditionGit) environ() []string {
	base := g.env
	if base == nil {
		base = os.Environ()
	}
	env := make([]string, 0, len(base)+1)
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if postconditionDroppedEnv[strings.ToUpper(name)] {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "GIT_TERMINAL_PROMPT=0")
}

// output returns the trimmed stdout of a command that must exit 0.
func (g postconditionGit) output(dir string, args ...string) (string, error) {
	res := g.run(dir, nil, args...)
	if res.err != nil {
		return "", res.err
	}
	if res.code != 0 {
		return "", fmt.Errorf("git %s exited %d: %s", args[0], res.code, res.stderr)
	}
	return strings.TrimSpace(res.stdout), nil
}

// lsRemote queries a URL pinned before execution from a fresh empty directory
// under GIT_CEILING_DIRECTORIES, so no repository configuration the task may
// have written (a URL rewrite, a credential helper, an ssh command) applies.
func (g postconditionGit) lsRemote(args ...string) gitResult {
	dir, err := os.MkdirTemp("", "autopus-postcondition-")
	if err != nil {
		return gitResult{code: -1, err: fmt.Errorf("neutral directory: %w", err)}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	ceiling := []string{"GIT_CEILING_DIRECTORIES=" + filepath.Dir(dir)}
	return g.run(dir, ceiling, append([]string{"ls-remote"}, args...)...)
}

// redactGitMessage keeps the first line of a git message, replaces URL
// credentials with <redacted>, and clips it.
func redactGitMessage(raw string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(raw), "\n")
	line = urlUserinfoPattern.ReplaceAllString(strings.TrimSpace(line), "${1}<redacted>@")
	if len(line) > gitMessageClip {
		cut := gitMessageClip
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		line = line[:cut]
	}
	return line
}

// executionBaseline is the state captured before the provider runs.
type executionBaseline struct {
	GitRepo   bool     // HEAD resolved to a commit
	HeadSHA   string   // that commit
	RemoteURL string   // fetch URL of the plan's remote; "" when not configured
	RefState  refState // the local branch of the plan's ref, when it names one
	// RemoteHeads snapshots the remote's branch heads when push is forbidden,
	// so a head that already pointed at a commit is not blamed on the task;
	// nil when the snapshot was not needed or the lookup failed.
	RemoteHeads map[string]string
}

func (g postconditionGit) captureBaseline(workDir string, plan taskPostconditions) executionBaseline {
	var base executionBaseline
	if head := g.head(workDir); head.sha != "" {
		base.GitRepo, base.HeadSHA = true, head.sha
	}
	remote := plan.Remote
	if remote == "" {
		remote = defaultPostconditionRemote
	}
	base.RemoteURL, _ = g.output(workDir, "remote", "get-url", remote)
	if plan.Ref != "" {
		_, base.RefState, _ = g.resolveRef(workDir, plan.Ref)
	}
	if plan.Push == levelForbidden && base.GitRepo && base.RemoteURL != "" {
		if heads := g.lsRemote("--heads", "--", base.RemoteURL); lookupFailure(heads) == "" {
			base.RemoteHeads = parseRemoteHeads(heads.stdout)
		}
	}
	return base
}

// parseRemoteHeads reads `ls-remote --heads` output into ref -> lowercase SHA.
func parseRemoteHeads(stdout string) map[string]string {
	heads := make(map[string]string)
	for _, line := range strings.Split(stdout, "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && strings.HasPrefix(fields[1], branchRefPrefix) {
			heads[fields[1]] = strings.ToLower(fields[0])
		}
	}
	return heads
}

// headInfo is HEAD after (or before) execution.
type headInfo struct {
	sha    string // "" when HEAD does not resolve to a commit
	branch string // "" when detached or unresolved
	err    string // set only when git could not run to completion
}

func (g postconditionGit) head(workDir string) headInfo {
	res := g.run(workDir, nil, "rev-parse", "--verify", "-q", "HEAD^{commit}")
	if res.err != nil {
		return headInfo{err: res.err.Error()}
	}
	if res.code != 0 {
		return headInfo{}
	}
	info := headInfo{sha: strings.TrimSpace(res.stdout)}
	branch := g.run(workDir, nil, "symbolic-ref", "-q", "--short", "HEAD")
	if branch.err != nil {
		return headInfo{err: branch.err.Error()}
	}
	if branch.code == 0 {
		info.branch = strings.TrimSpace(branch.stdout)
	}
	return info
}

type refState int

const (
	refUnknown refState = iota
	refAbsent
	refPresent
)

// resolveRef reads the commit a local ref points at.
func (g postconditionGit) resolveRef(workDir, ref string) (string, refState, string) {
	res := g.run(workDir, nil, "rev-parse", "--verify", "-q", ref+"^{commit}")
	switch {
	case res.err != nil:
		return "", refUnknown, res.err.Error()
	case res.code == 0:
		return strings.TrimSpace(res.stdout), refPresent, ""
	case res.code == 1:
		return "", refAbsent, ""
	default:
		return "", refUnknown, fmt.Sprintf("git rev-parse exited %d: %s", res.code, res.stderr)
	}
}
