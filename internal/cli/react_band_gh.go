package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// gh Invocation Table of SPEC-SIGMABAND-001, verified against gh 2.98.0 help:
// only `run list` and `run view` take -R; `auth status` and `api` take
// --hostname. Commands for pull requests belong to SPEC-SIGMABAND-002.
const (
	bandGitHubHost    = "github.com"
	bandGHCallTimeout = 30 * time.Second // each gh call and the origin lookup
	bandGHLogTimeout  = 60 * time.Second // each failed-step log
	bandDefaultLimit  = 200
	bandMaxLimit      = 1000
	bandRunListFields = "databaseId,attempt,conclusion,status,headBranch,event,workflowName,createdAt"
	// bandTextOutputCap bounds the origin URL and default-branch outputs, and
	// bandRunListCap the run list JSON (1,000 runs stay far below it).
	bandTextOutputCap = 64 << 10
	bandRunListCap    = 16 << 20
	// bandWaitDelay bounds how long Wait lingers on a killed process.
	bandWaitDelay = time.Second
)

var (
	errBandCommandNotAllowed = errors.New("react band: command is outside the gh Invocation Table")
	errBandOutputTooLarge    = errors.New("react band: command output exceeds its bound")

	bandHostPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,252}$`)
	bandSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.-]{0,99}$`)
	bandIDPattern      = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
)

// bandCommand is one subprocess call of auto react band. Env is the full
// environment (nil inherits); a nil Stdout discards the output.
type bandCommand struct {
	Name   string
	Args   []string
	Dir    string
	Env    []string
	Stdout io.Writer
}

// bandRunner is the command-runner seam: every git and gh subprocess of band
// goes through it, so tests drive both with fakes and never start real gh.
type bandRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, command bandCommand) error
}

// execBandRunner runs commands for real, and only those of the table.
type execBandRunner struct{}

func (execBandRunner) LookPath(file string) (string, error) { return exec.LookPath(file) }

// Run executes an allowlisted command directly, never through a shell, with
// stdin and stderr on the null device (gh never prompts and its untrusted
// stderr is never read); ctx ends it. The command starts in its own process
// group, so a timeout kills gh together with any helper it spawned.
func (execBandRunner) Run(ctx context.Context, command bandCommand) error {
	if err := checkBandCommand(command); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, command.Name, command.Args...) //nolint:gosec // argv is allowlisted above
	cmd.Dir = command.Dir
	cmd.Env = command.Env
	cmd.Stdout = command.Stdout
	cmd.WaitDelay = bandWaitDelay
	killReadinessProcessGroupOnCancel(cmd)
	return cmd.Run()
}

// checkBandCommand admits the read-only origin lookup and the four rows of
// the gh Invocation Table with well-formed values, nothing else: no git
// mutation, no gh pr, no gh api method or field flag (so every api call is a
// GET), and no run list status filter.
func checkBandCommand(command bandCommand) error {
	a := command.Args
	allowed := false
	switch {
	case command.Name == "git":
		allowed = slices.Equal(a, []string{"remote", "get-url", "origin"})
	case command.Name != "gh":
	case len(a) == 4 && a[0] == "auth" && a[1] == "status" && a[2] == "--hostname":
		allowed = validBandHost(a[3])
	case len(a) == 6 && a[0] == "api" && a[2] == "--hostname" && a[4] == "--jq" && a[5] == ".default_branch":
		slug, isRepoPath := strings.CutPrefix(a[1], "repos/")
		allowed = isRepoPath && validBandSlug(slug) && validBandHost(a[3])
	case len(a) == 8 && a[0] == "run" && a[1] == "list" && a[2] == "-R" && a[4] == "--limit" && a[6] == "--json":
		limit, err := strconv.Atoi(a[5])
		allowed = validBandSlug(a[3]) && err == nil && strconv.Itoa(limit) == a[5] &&
			validateBandLimit(limit) == nil && a[7] == bandRunListFields
	case len(a) == 8 && a[0] == "run" && a[1] == "view" && a[3] == "-R" && a[5] == "--attempt" && a[7] == "--log-failed":
		allowed = bandIDPattern.MatchString(a[2]) && validBandSlug(a[4]) && bandIDPattern.MatchString(a[6])
	}
	if !allowed {
		return fmt.Errorf("%w: %s", errBandCommandNotAllowed, command.Name)
	}
	return nil
}

// validateBandLimit is REQ-19: --limit takes 1 to 1000 runs.
func validateBandLimit(limit int) error {
	if limit < 1 || limit > bandMaxLimit {
		return fmt.Errorf("--limit must be between 1 and %d, got %d", bandMaxLimit, limit)
	}
	return nil
}

// bandGHTarget is the repository the origin remote names.
type bandGHTarget struct {
	Host  string
	Owner string
	Repo  string
}

// Slug is <owner>/<repo>, the form of -R and GH_REPO.
func (t bandGHTarget) Slug() string { return t.Owner + "/" + t.Repo }

func validBandSegment(segment string) bool {
	return bandSegmentPattern.MatchString(segment) && segment != "." && segment != ".."
}

func validBandSlug(slug string) bool {
	owner, repo, found := strings.Cut(slug, "/")
	return found && validBandSegment(owner) && validBandSegment(repo)
}

// parseBandOrigin reads host and owner/repo from an origin URL in URL form
// (https, http, ssh, git) or scp form ([user@]host:owner/repo). The host is
// lowercased and a subdomain of github.com becomes github.com, as gh
// normalizes it. User info is never kept. Anything else is not a GitHub
// repository: local paths, other schemes, and paths that are not exactly
// owner/repo.
func parseBandOrigin(raw string) (bandGHTarget, bool) {
	raw = strings.TrimSpace(raw)
	var host, path string
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || !slices.Contains([]string{"https", "http", "ssh", "git", "git+ssh", "ssh+git"}, parsed.Scheme) {
			return bandGHTarget{}, false
		}
		host, path = parsed.Hostname(), parsed.Path
	} else {
		authority, rest, found := strings.Cut(raw, ":")
		if !found || strings.Contains(authority, "/") {
			return bandGHTarget{}, false
		}
		host, path = authority[strings.LastIndexByte(authority, '@')+1:], rest
	}
	host = strings.ToLower(host)
	if strings.HasSuffix(host, "."+bandGitHubHost) {
		host = bandGitHubHost
	}
	owner, repo, _ := strings.Cut(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/")
	target := bandGHTarget{Host: host, Owner: owner, Repo: repo}
	if !validBandHost(host) || !validBandSlug(target.Slug()) {
		return bandGHTarget{}, false
	}
	return target, true
}

// validBandHost accepts a lowercase DNS name and refuses localhost and IP
// literals, which no GitHub host is: a token must never reach a loopback,
// link-local, or metadata address that an origin URL names. A last label
// that starts with a digit is an IPv4 literal or a numeric form a resolver
// may read as one (2130706433, 0x7f000001); IPv6 literals fail the pattern.
func validBandHost(host string) bool {
	if !bandHostPattern.MatchString(host) || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	last := host[strings.LastIndexByte(host, '.')+1:]
	return last != "" && (last[0] < '0' || last[0] > '9')
}

// bandGHClient runs the gh Invocation Table through the runner seam. Every
// call runs under its own timeout, so a hung gh ends at its budget, and each
// failed-step log keeps only its last logCapture bytes.
type bandGHClient struct {
	runner      bandRunner
	environ     func() []string
	callTimeout time.Duration
	logTimeout  time.Duration
	logCapture  int
}

func newBandGHClient(runner bandRunner) bandGHClient {
	return bandGHClient{
		runner: runner, environ: os.Environ, callTimeout: bandGHCallTimeout,
		logTimeout: bandGHLogTimeout, logCapture: healthband.CILogCaptureBytes,
	}
}

// gh builds a gh command for a host that already passed gh auth status: its
// environment replaces any inherited GH_REPO and GH_HOST with the resolved
// repository, so a stray value cannot redirect a call.
func (c bandGHClient) gh(target bandGHTarget, dir string, args ...string) bandCommand {
	return bandCommand{Name: "gh", Args: args, Dir: dir, Env: c.ghEnv(target, true)}
}

// ghAuthStatus builds the host check. It injects no GH_HOST, because gh
// counts GH_HOST as a configured host and would then check, over the
// network, an environment token (GH_ENTERPRISE_TOKEN) against whatever host
// origin names. Without it gh answers only for a host in its hosts config,
// or for an inherited GH_HOST that already names this host.
func (c bandGHClient) ghAuthStatus(target bandGHTarget, dir string) bandCommand {
	args := []string{"auth", "status", "--hostname", target.Host}
	return bandCommand{Name: "gh", Args: args, Dir: dir, Env: c.ghEnv(target, false)}
}

// ghEnv is the inherited environment without the keys that redirect gh or
// make it prompt, page, or color (GH_FORCE_TTY, CLICOLOR_FORCE), plus
// GH_REPO, the plain-output settings, and GH_HOST when injectHost is set.
func (c bandGHClient) ghEnv(target bandGHTarget, injectHost bool) []string {
	base := c.environ()
	env := make([]string, 0, len(base)+5)
	for _, entry := range base {
		key, value, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GH_REPO", "GH_PROMPT_DISABLED", "GH_PAGER", "NO_COLOR", "GH_FORCE_TTY", "CLICOLOR_FORCE":
			continue
		case "GH_HOST":
			if injectHost || !strings.EqualFold(strings.TrimSpace(value), target.Host) {
				continue
			}
		}
		env = append(env, entry)
	}
	env = append(env, "GH_REPO="+target.Slug(), "GH_PROMPT_DISABLED=1", "GH_PAGER=cat", "NO_COLOR=1")
	if injectHost {
		env = append(env, "GH_HOST="+target.Host)
	}
	return env
}

// capture runs command into out under timeout. A run that outlives its
// timeout fails even if the process reported success.
func (c bandGHClient) capture(ctx context.Context, timeout time.Duration, command bandCommand, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command.Stdout = out
	if err := c.runner.Run(ctx, command); err != nil {
		return err
	}
	return ctx.Err()
}

// output runs command and returns its stdout, which must fit limit bytes:
// a cut JSON document or branch name is unusable.
func (c bandGHClient) output(ctx context.Context, command bandCommand, limit int) (string, error) {
	out := healthband.NewHeadBuffer(limit)
	if err := c.capture(ctx, c.callTimeout, command, out); err != nil {
		return "", err
	}
	text, dropped := out.Captured()
	if dropped {
		return "", errBandOutputTooLarge
	}
	return text, nil
}
