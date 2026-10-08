package healthband

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// GitMinMajor and GitMinMinor are the oldest git band runs: git 2.44 added
// GIT_NO_LAZY_FETCH, without which a checkout of a partial clone fetches
// missing objects from its promisor remote (CD-3 M1).
const (
	GitMinMajor = 2
	GitMinMinor = 44
	// gitVersionUnknown spells an unparsable git version in its code.
	gitVersionUnknown = "unknown"
)

var gitVersionPattern = regexp.MustCompile(`^git version ([0-9]{1,9})\.([0-9]{1,9})`)

// CheckVersion is Git Execution Policy item 1's version check: "" for git
// 2.44 or later, else git_version_unsupported:<major>.<minor>, or
// git_version_unsupported:unknown when git version fails or does not parse.
// Only a done context gives an error.
func (r GitPolicyRunner) CheckVersion(ctx context.Context) (string, error) {
	out, err := r.Run(ctx, "version")
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	match := gitVersionPattern.FindStringSubmatch(string(out))
	if err != nil || match == nil {
		return GitVersionUnsupportedPrefix + gitVersionUnknown, nil
	}
	major, _ := strconv.Atoi(match[1]) // at most nine digits always parse
	minor, _ := strconv.Atoi(match[2])
	if major < GitMinMajor || major == GitMinMajor && minor < GitMinMinor {
		return fmt.Sprintf("%s%d.%d", GitVersionUnsupportedPrefix, major, minor), nil
	}
	return "", nil
}

// ResolveBase is Git Execution Policy item 4. defaultBranch is the default
// branch that SPEC-SIGMABAND-001's fetchCI resolved, or "" under --no-fetch
// or a failed lookup, when git symbolic-ref --short refs/remotes/origin/HEAD
// without its origin/ prefix names it. The name must pass git
// check-ref-format --branch unchanged (no @{-N} expansion), and the base is
// the commit of refs/remotes/origin/<default>, otherwise refs/heads/<default>.
// No fetch runs, so the base is the last state the user fetched. It returns
// the base SHA, or the code base_unavailable; only a done context gives an
// error. GIT_NO_REPLACE_OBJECTS keeps a replace ref from changing the base.
func (r GitPolicyRunner) ResolveBase(ctx context.Context, defaultBranch string) (string, string, error) {
	branch := defaultBranch
	if branch == "" {
		out, err := r.Run(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", "", ctxErr
		}
		short, found := strings.CutPrefix(strings.TrimSuffix(string(out), "\n"), "origin/")
		if err != nil || !found {
			return "", GitBaseUnavailable, nil
		}
		branch = short
	}
	if !validGitBranchName(branch) {
		return "", GitBaseUnavailable, nil
	}
	out, err := r.Run(ctx, "check-ref-format", "--branch", branch)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", "", ctxErr
	}
	if err != nil || strings.TrimSuffix(string(out), "\n") != branch {
		return "", GitBaseUnavailable, nil
	}
	for _, ref := range []string{"refs/remotes/origin/" + branch, "refs/heads/" + branch} {
		out, err := r.Run(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", "", ctxErr
		}
		if sha := strings.TrimSuffix(string(out), "\n"); err == nil && validGitOID(sha) {
			return sha, "", nil
		}
	}
	return "", GitBaseUnavailable, nil
}
