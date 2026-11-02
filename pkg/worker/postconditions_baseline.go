package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// presence is the result of one lookup (SPEC-WORKERPOST-001 Definitions,
// Lookup result). Only an explicit answer from git is absent; anything that
// kept git from answering is unknown, never absent.
type presence int

const (
	presenceUnknown presence = iota
	presenceAbsent
	presencePresent
)

// refFormat makes for-each-ref print exactly the ref it was asked for, so a
// lookup never resolves a different ref the way rev-parse's DWIM rules can.
const refFormat = "--format=%(objectname) %(refname)"

// lookupPresence classifies a finished lookup: exit 0 is present, the
// command's documented not-found exit is absent, and so is a directory git
// reports as not a repository; every other result is unknown.
func lookupPresence(res gitResult, absentCode int) (presence, string) {
	switch {
	case res.err != nil:
		return presenceUnknown, res.err.Error()
	case res.code == 0:
		return presencePresent, ""
	case res.code == absentCode:
		return presenceAbsent, ""
	case res.code == 128 && strings.Contains(res.stderr, "not a git repository"):
		return presenceAbsent, ""
	default:
		return presenceUnknown, fmt.Sprintf("git exited %d: %s", res.code, res.stderr)
	}
}

// executionBaseline is the state captured right before the provider runs.
type executionBaseline struct {
	Head      presence // HEAD's commit
	HeadSHA   string
	HeadErr   string
	Remote    presence // the plan's remote
	RemoteURL string   // its pinned URL; a relative local path is made absolute
	RemoteErr string
	RefState  presence // the local branch of the plan's ref, when it names one
	// RemoteHeads snapshots the remote's branch heads when push is forbidden,
	// so a head that already pointed at a commit is not blamed on the task;
	// nil when the snapshot was not needed or the lookup failed.
	RemoteHeads map[string]string
}

func (g postconditionGit) captureBaseline(workDir string, plan taskPostconditions) executionBaseline {
	head := g.head(workDir)
	base := executionBaseline{Head: head.state, HeadSHA: head.sha, HeadErr: head.err}
	remote := plan.Remote
	if remote == "" {
		remote = defaultPostconditionRemote
	}
	base.Remote, base.RemoteURL, base.RemoteErr = g.remoteURL(workDir, remote)
	if plan.Ref != "" {
		_, base.RefState, _ = g.resolveRef(workDir, plan.Ref)
	}
	if plan.Push == levelForbidden && base.Head == presencePresent && base.Remote == presencePresent {
		if heads := g.lsRemote("--heads", "--", base.RemoteURL); lookupFailure(heads) == "" {
			base.RemoteHeads = parseRemoteHeads(heads.stdout)
		}
	}
	return base
}

// headInfo is HEAD at the time of a lookup.
type headInfo struct {
	state  presence
	sha    string
	branch string // "" when detached
	err    string
}

func (g postconditionGit) head(workDir string) headInfo {
	res := g.run(workDir, nil, "rev-parse", "--verify", "-q", "HEAD")
	if state, why := lookupPresence(res, 1); state != presencePresent {
		return headInfo{state: state, err: why}
	}
	info := headInfo{state: presencePresent, sha: strings.ToLower(strings.TrimSpace(res.stdout))}
	branch := g.run(workDir, nil, "symbolic-ref", "-q", "--short", "HEAD")
	switch state, why := lookupPresence(branch, 1); state {
	case presencePresent:
		info.branch = strings.TrimSpace(branch.stdout)
	case presenceUnknown:
		return headInfo{state: presenceUnknown, err: "cannot read HEAD's branch: " + why}
	}
	return info
}

// remoteURL pins a remote's fetch URL. git resolves a relative local path
// against the work tree's top level, so the pinned URL is made absolute the
// same way; the lookup itself later runs in another directory.
func (g postconditionGit) remoteURL(workDir, remote string) (presence, string, string) {
	res := g.run(workDir, nil, "remote", "get-url", remote)
	if state, why := lookupPresence(res, 2); state != presencePresent {
		return state, "", why
	}
	url := strings.TrimSpace(res.stdout)
	if !isRelativeLocalPath(url) {
		return presencePresent, url, ""
	}
	top, err := g.output(workDir, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return presenceUnknown, "", fmt.Sprintf("cannot resolve the relative URL of remote %q against the work tree", remote)
	}
	return presencePresent, filepath.Clean(filepath.Join(top, url)), ""
}

// isRelativeLocalPath applies git's url_is_local_not_ssh rule: without
// "scheme://", a URL whose first ':' does not come before its first '/' is a
// local path rather than scp-style ssh.
func isRelativeLocalPath(url string) bool {
	if strings.Contains(url, "://") {
		return false
	}
	colon, slash := strings.IndexByte(url, ':'), strings.IndexByte(url, '/')
	local := colon < 0 || (slash >= 0 && slash < colon) || filepath.VolumeName(url) != ""
	return local && !filepath.IsAbs(url)
}

// resolveRef reads the commit an exact local ref points at.
func (g postconditionGit) resolveRef(workDir, ref string) (string, presence, string) {
	res := g.run(workDir, nil, "for-each-ref", refFormat, ref)
	if state, why := lookupPresence(res, -1); state != presencePresent {
		return "", state, why
	}
	for _, line := range strings.Split(res.stdout, "\n") {
		if sha, name, ok := strings.Cut(strings.TrimSpace(line), " "); ok && name == ref {
			return strings.ToLower(sha), presencePresent, ""
		}
	}
	return "", presenceAbsent, ""
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

// historyTrust tells whether local ancestry can be trusted. Replace refs and
// the commit-graph are already ignored by every command; a graft file still
// rewrites parents, and a shallow boundary hides ancestors, so there only a
// positive ancestry answer holds.
func (g postconditionGit) historyTrust(workDir string) (shallow bool, untrusted string) {
	graft, err := g.output(workDir, "rev-parse", "--path-format=absolute", "--git-path", "info/grafts")
	if err != nil {
		return false, "cannot locate the graft file: " + err.Error()
	}
	if _, statErr := os.Lstat(graft); statErr == nil {
		return false, "the repository has a graft file (info/grafts), so its history cannot be trusted"
	} else if !os.IsNotExist(statErr) {
		return false, "cannot inspect the graft file: " + statErr.Error()
	}
	flag, err := g.output(workDir, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return false, "cannot tell whether the repository is shallow: " + err.Error()
	}
	return flag != "false", ""
}
