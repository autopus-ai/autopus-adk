package worker

import (
	"fmt"
	"sort"
	"strings"
)

// pushCheck judges a required or ambiguous push by the SHA of the exact ref
// at the pinned URL (REQ-WPOST-PUSH-01..03), and a forbidden one by the
// remote heads (REQ-WPOST-PUSH-04).
func (g postconditionGit) pushCheck(workDir string, plan taskPostconditions, base executionBaseline) postconditionCheck {
	check := postconditionCheck{Name: actionPush, Requirement: string(plan.Push), Remote: plan.Remote}
	switch plan.Push {
	case levelNone:
		return withStatus(check, statusNotRequired, "the task does not require a push")
	case levelForbidden:
		return g.forbiddenPush(check, workDir, base)
	}
	head := g.head(workDir)
	if head.err != "" {
		return withStatus(check, statusUnknown, "cannot read HEAD: "+head.err)
	}
	check.Ref, check.Match = plan.Ref, plan.Match
	if plan.Source == sourceLegacy {
		check.Ref = fallbackPushRef(plan, head.branch)
	}
	switch {
	case check.Ref == "" && plan.Source == sourceLegacy:
		return withStatus(check, statusUnknown,
			"cannot determine the pushed branch: HEAD is detached or on a worker- branch, or the prompt names several branches")
	case check.Ref == "":
		return withStatus(check, statusUnknown, "the contract names no ref")
	case base.RemoteURL == "":
		return withStatus(check, statusUnknown, fmt.Sprintf("remote %q had no URL before execution", plan.Remote))
	}
	check.Expected = plan.ExpectedCommit
	if check.Expected == "" {
		if head.sha == "" || head.branch == "" {
			return withStatus(check, statusUnknown, "HEAD is detached or unresolved and no expected_commit was given")
		}
		check.Expected = head.sha
	}

	observed, found, why := g.remoteRefSHA(base.RemoteURL, check.Ref)
	switch {
	case why != "":
		return withStatus(check, statusUnknown, why)
	case !found:
		return withStatus(check, statusUnsatisfied, fmt.Sprintf("remote branch %q not found on %s",
			strings.TrimPrefix(check.Ref, branchRefPrefix), plan.Remote))
	}
	check.Observed = observed
	if observed == check.Expected {
		return withStatus(check, statusSatisfied, fmt.Sprintf("%s on %s is at the expected commit", check.Ref, plan.Remote))
	}
	if plan.Match != matchContains {
		return withStatus(check, statusUnsatisfied,
			fmt.Sprintf("%s on %s is at %s, expected %s", check.Ref, plan.Remote, observed, check.Expected))
	}
	return g.containsCheck(check, workDir)
}

// containsCheck accepts a remote tip that descends from the expected commit,
// judged only from local objects with trusted ancestry.
func (g postconditionGit) containsCheck(check postconditionCheck, workDir string) postconditionCheck {
	present := g.run(workDir, nil, "cat-file", "-e", check.Observed+"^{commit}")
	switch {
	case present.err != nil:
		return withStatus(check, statusUnknown, present.err.Error())
	case present.code != 0:
		return withStatus(check, statusUnknown, fmt.Sprintf(
			"remote tip %s is not available locally, so ancestry cannot be verified without fetching", check.Observed))
	}
	switch answer, why := g.isAncestor(workDir, check.Expected, check.Observed); answer {
	case ancestryYes:
		return withStatus(check, statusSatisfied, fmt.Sprintf("remote tip %s contains the expected commit", check.Observed))
	case ancestryNo:
		return withStatus(check, statusUnsatisfied,
			fmt.Sprintf("remote tip %s does not contain the expected commit %s", check.Observed, check.Expected))
	default:
		return withStatus(check, statusUnknown, why)
	}
}

// remoteRefSHA reads one exact ref. found is false when the remote answered
// without that ref; why is set when the lookup itself failed.
func (g postconditionGit) remoteRefSHA(url, ref string) (sha string, found bool, why string) {
	res := g.lsRemote("--", url, ref)
	if failure := lookupFailure(res); failure != "" {
		return "", false, failure
	}
	for _, line := range strings.Split(res.stdout, "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[1] == ref {
			return strings.ToLower(fields[0]), true, ""
		}
	}
	return "", false, ""
}

func lookupFailure(res gitResult) string {
	if res.err != nil {
		return "remote lookup failed: " + res.err.Error()
	}
	if res.code != 0 {
		return fmt.Sprintf("remote lookup failed (git exit %d): %s", res.code, res.stderr)
	}
	return ""
}

func (g postconditionGit) forbiddenPush(check postconditionCheck, workDir string, base executionBaseline) postconditionCheck {
	head := g.head(workDir)
	switch {
	case head.err != "":
		return withStatus(check, statusUnknown, "cannot read HEAD: "+head.err)
	case !base.GitRepo && head.sha == "":
		return withStatus(check, statusSatisfied, "no commit exists to publish")
	case !base.GitRepo:
		return withStatus(check, statusUnknown, "a HEAD commit appeared in a work directory that had none")
	case head.sha == "":
		return withStatus(check, statusUnknown, "HEAD does not resolve to a commit after execution")
	case head.sha == base.HeadSHA:
		return withStatus(check, statusSatisfied, "HEAD unchanged; no new commit to publish")
	}
	if base.RemoteURL == "" {
		if _, err := g.output(workDir, "remote", "get-url", check.Remote); err == nil {
			return withStatus(check, statusUnknown, fmt.Sprintf("remote %q gained a URL during execution", check.Remote))
		}
		return withStatus(check, statusSatisfied, fmt.Sprintf("remote %q is not configured", check.Remote))
	}
	created, err := g.output(workDir, "rev-list", head.sha, "^"+base.HeadSHA)
	if err != nil {
		return withStatus(check, statusUnknown, "cannot list the commits created after the baseline: "+err.Error())
	}
	newCommits := wordSet(strings.Fields(created)...)
	heads := g.lsRemote("--heads", "--", base.RemoteURL)
	if failure := lookupFailure(heads); failure != "" {
		return withStatus(check, statusUnknown, failure)
	}
	return judgeRemoteHeads(check, parseRemoteHeads(heads.stdout), base.RemoteHeads, newCommits)
}

// judgeRemoteHeads blames the task for a remote head that points at a commit
// HEAD gained and that moved there during execution. A head already there at
// the baseline was pulled, not pushed; without a baseline snapshot the move
// cannot be attributed, so the push is unknown.
func judgeRemoteHeads(check postconditionCheck, now, before map[string]string, newCommits map[string]bool) postconditionCheck {
	refs := make([]string, 0, len(now))
	for ref := range now {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	unattributed := ""
	for _, ref := range refs {
		sha := now[ref]
		if !newCommits[sha] || (before != nil && before[ref] == sha) {
			continue
		}
		if before == nil {
			if unattributed == "" {
				unattributed = ref
			}
			continue
		}
		check.Ref, check.Observed = ref, sha
		return withStatus(check, statusUnsatisfied, fmt.Sprintf(
			"%s on %s moved to %s, a commit HEAD gained during execution, although push is forbidden", ref, check.Remote, sha))
	}
	if unattributed != "" {
		check.Ref, check.Observed = unattributed, now[unattributed]
		return withStatus(check, statusUnknown, fmt.Sprintf(
			"%s on %s points at %s, a commit HEAD gained during execution, and no baseline snapshot of the remote heads exists",
			unattributed, check.Remote, now[unattributed]))
	}
	return withStatus(check, statusSatisfied,
		fmt.Sprintf("no branch on %s moved to the %d commit(s) HEAD gained after the baseline", check.Remote, len(newCommits)))
}
