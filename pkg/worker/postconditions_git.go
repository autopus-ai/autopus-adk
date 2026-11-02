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
	env           []string            // base environment; nil means os.Environ()
	remoteTimeout time.Duration       // per ls-remote; zero means postconditionRemoteTimeout
	trace         func(argv []string) // sees every argv before it starts; nil in production
}

// postconditionGlobalArgs precede every subcommand: history is read from the
// commit objects alone, never from replace refs or a commit-graph file the
// task could have written (REQ-WPOST-READONLY-01, Trusted history).
var postconditionGlobalArgs = []string{"--no-replace-objects", "-c", "core.commitGraph=false"}

// postconditionFixedEnv keeps every command from prompting, taking optional
// locks, fetching missing objects lazily (GIT_NO_LAZY_FETCH, git 2.44+), or
// translating the messages the lookups classify.
var postconditionFixedEnv = []string{"GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C"}

// localTransportBlock denies every transport to commands that only read the
// local repository, so a promisor remote cannot be fetched from even where
// GIT_NO_LAZY_FETCH is unknown; "none" names no real protocol.
const localTransportBlock = "GIT_ALLOW_PROTOCOL=none"

// postconditionWaitDelay bounds how long a killed git may keep its output
// pipes open through a helper such as git-remote-http.
const postconditionWaitDelay = time.Second

// postconditionDroppedEnv are inherited variables that could point git at
// another repository, object store, or history than the work directory's, or
// that the fixed environment replaces.
var postconditionDroppedEnv = map[string]bool{
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_OBJECT_DIRECTORY": true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_COMMON_DIR": true, "GIT_NAMESPACE": true,
	"GIT_GRAFT_FILE": true, "GIT_REPLACE_REF_BASE": true, "GIT_SHALLOW_FILE": true,
	"GIT_CEILING_DIRECTORIES": true, "GIT_TERMINAL_PROMPT": true, "GIT_NO_LAZY_FETCH": true,
	"GIT_OPTIONAL_LOCKS": true, "LC_ALL": true, "LANGUAGE": true,
}

var (
	// urlUserinfoPattern matches the credentials of scheme://user:secret@host.
	urlUserinfoPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/@\s'"]+@`)
	// bareUserinfoPattern matches user:secret@ written without a scheme.
	bareUserinfoPattern = regexp.MustCompile(`(^|[\s'"(])[^\s/@'":]+:[^\s/@'"]+@`)
)

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
			(n == 2 && (args[1] == "--is-shallow-repository" || args[1] == "--show-toplevel"))
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
	case "for-each-ref":
		allowed = n == 3 && args[1] == refFormat && strings.HasPrefix(args[2], branchRefPrefix)
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
	timeout := postconditionLocalTimeout
	if args[0] == "ls-remote" {
		timeout = postconditionRemoteTimeout
		if g.remoteTimeout > 0 {
			timeout = g.remoteTimeout
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	argv := append(append([]string(nil), postconditionGlobalArgs...), args...)
	if g.trace != nil {
		g.trace(append([]string(nil), argv...))
	}
	env := append(g.environ(), extraEnv...)
	if args[0] != "ls-remote" {
		env = append(env, localTransportBlock)
	}
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Dir = dir
	cmd.Env = env
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
	env := make([]string, 0, len(base)+len(postconditionFixedEnv))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if postconditionDroppedEnv[strings.ToUpper(name)] {
			continue
		}
		env = append(env, entry)
	}
	return append(env, postconditionFixedEnv...)
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

// redactCredentials replaces the user and password of every URL, with or
// without a scheme, by <redacted> (REQ-WPOST-REPORT-01).
func redactCredentials(text string) string {
	text = urlUserinfoPattern.ReplaceAllString(text, "${1}<redacted>@")
	return bareUserinfoPattern.ReplaceAllString(text, "${1}<redacted>@")
}

// redactGitMessage keeps the first line of a git message, removes URL
// credentials, and clips it.
func redactGitMessage(raw string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(raw), "\n")
	return clipRunes(redactCredentials(strings.TrimSpace(line)), gitMessageClip)
}

// clipRunes cuts text to at most limit bytes on a rune boundary.
func clipRunes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}
