package worker

import (
	"fmt"
	"strings"
)

// The checks of SPEC-WORKERPOST-001. Each walks its decision table in
// spec.md top to bottom and the first matching row decides, so overlapping
// conditions have one result; a lookup that failed is never read as absence.

func withStatus(check postconditionCheck, status, detail string) postconditionCheck {
	check.Status, check.Detail = status, detail
	return check
}

// commitCheck judges the commit action on HEAD (REQ-WPOST-COMMIT-01, -02).
func (g postconditionGit) commitCheck(workDir string, level requirementLevel, base executionBaseline) postconditionCheck {
	check := postconditionCheck{Name: actionCommit, Requirement: string(level)}
	if level == levelNone {
		return withStatus(check, statusNotRequired, "the task does not require a commit")
	}
	head := g.head(workDir)
	check.Observed = head.sha
	switch {
	case head.state == presenceUnknown:
		return withStatus(check, statusUnknown, "cannot read HEAD: "+head.err)
	case base.Head == presenceUnknown:
		return withStatus(check, statusUnknown, "the baseline HEAD could not be read: "+base.HeadErr)
	}
	if level == levelForbidden {
		return g.forbiddenCommit(check, workDir, head, base)
	}
	switch {
	case base.Head == presenceAbsent:
		return withStatus(check, statusUnknown, "no baseline commit was captured before execution")
	case head.state == presenceAbsent:
		return withStatus(check, statusUnknown, "HEAD does not resolve to a commit after execution")
	case head.sha == base.HeadSHA:
		return withStatus(check, statusUnsatisfied, "HEAD did not advance after execution")
	}
	switch answer, why := g.isAncestor(workDir, base.HeadSHA, head.sha); answer {
	case ancestryYes:
		return withStatus(check, statusSatisfied, "HEAD advanced to "+head.sha)
	case ancestryNo:
		return withStatus(check, statusUnsatisfied, movedAwayDetail(head.sha, base.HeadSHA))
	default:
		return withStatus(check, statusUnknown, why)
	}
}

func (g postconditionGit) forbiddenCommit(check postconditionCheck, workDir string, head headInfo, base executionBaseline) postconditionCheck {
	switch {
	case base.Head == presenceAbsent && head.state == presenceAbsent:
		return withStatus(check, statusSatisfied, "no commit exists before or after execution")
	case base.Head == presenceAbsent:
		return withStatus(check, statusUnsatisfied, "a HEAD commit "+head.sha+" appeared in a work directory that had none")
	case head.state == presenceAbsent:
		return withStatus(check, statusUnknown, "HEAD does not resolve to a commit after execution")
	case head.sha == base.HeadSHA:
		return withStatus(check, statusSatisfied, "HEAD unchanged at "+head.sha)
	}
	switch answer, why := g.isAncestor(workDir, base.HeadSHA, head.sha); answer {
	case ancestryYes:
		return withStatus(check, statusUnsatisfied,
			fmt.Sprintf("HEAD advanced from %s to %s although commit is forbidden", base.HeadSHA, head.sha))
	case ancestryNo:
		return withStatus(check, statusUnknown, movedAwayDetail(head.sha, base.HeadSHA))
	default:
		return withStatus(check, statusUnknown, why)
	}
}

func movedAwayDetail(head, baseline string) string {
	return fmt.Sprintf("HEAD moved to %s, which does not descend from the baseline %s", head, baseline)
}

// branchChecks judges the branch action (REQ-WPOST-BRANCH-01, -02, LEGACY-02).
func (g postconditionGit) branchChecks(workDir string, plan taskPostconditions, base executionBaseline) []postconditionCheck {
	check := postconditionCheck{Name: actionBranch, Requirement: string(plan.Branch)}
	if plan.Branch == levelNone {
		return []postconditionCheck{withStatus(check, statusNotRequired, "the task does not require a branch")}
	}
	if plan.Source == sourceLegacy {
		checks := make([]postconditionCheck, 0, len(plan.Branches))
		for _, branch := range plan.Branches {
			checks = append(checks, g.localBranchCheck(check, workDir, branchRefPrefix+branch, ""))
		}
		return checks
	}
	if plan.Ref == "" {
		return []postconditionCheck{withStatus(check, statusUnknown, "the contract names no ref")}
	}
	if plan.Branch == levelRequired {
		return []postconditionCheck{g.localBranchCheck(check, workDir, plan.Ref, plan.ExpectedCommit)}
	}
	return []postconditionCheck{g.forbiddenBranch(check, workDir, plan.Ref, base.RefState)}
}

func (g postconditionGit) forbiddenBranch(check postconditionCheck, workDir, ref string, before presence) postconditionCheck {
	check.Ref = ref
	name := strings.TrimPrefix(ref, branchRefPrefix)
	sha, state, why := g.resolveRef(workDir, ref)
	check.Observed = sha
	switch {
	case state == presenceUnknown:
		return withStatus(check, statusUnknown, "cannot read the local branch: "+why)
	case state == presenceAbsent:
		return withStatus(check, statusSatisfied, fmt.Sprintf("local branch %q does not exist", name))
	case before == presencePresent:
		return withStatus(check, statusSatisfied, fmt.Sprintf("local branch %q existed before execution", name))
	case before == presenceUnknown:
		return withStatus(check, statusUnknown, fmt.Sprintf("cannot tell whether local branch %q existed before execution", name))
	default:
		return withStatus(check, statusUnsatisfied,
			fmt.Sprintf("local branch %q was created during execution although branch is forbidden", name))
	}
}

func (g postconditionGit) localBranchCheck(check postconditionCheck, workDir, ref, expected string) postconditionCheck {
	check.Ref, check.Expected = ref, expected
	name := strings.TrimPrefix(ref, branchRefPrefix)
	sha, state, why := g.resolveRef(workDir, ref)
	check.Observed = sha
	switch {
	case state == presenceUnknown:
		return withStatus(check, statusUnknown, "cannot read the local branch: "+why)
	case state == presenceAbsent:
		return withStatus(check, statusUnsatisfied, fmt.Sprintf("local branch %q not found", name))
	case expected != "" && sha != expected:
		return withStatus(check, statusUnsatisfied, fmt.Sprintf("local branch %q is at %s, expected %s", name, sha, expected))
	default:
		return withStatus(check, statusSatisfied, fmt.Sprintf("local branch %q exists", name))
	}
}

type ancestryAnswer int

const (
	ancestryUnknown ancestryAnswer = iota
	ancestryYes
	ancestryNo
)

// isAncestor answers from trusted local history only (historyTrust): a
// negative answer from a shallow repository is unknown.
func (g postconditionGit) isAncestor(workDir, ancestor, descendant string) (ancestryAnswer, string) {
	shallow, untrusted := g.historyTrust(workDir)
	if untrusted != "" {
		return ancestryUnknown, untrusted
	}
	res := g.run(workDir, nil, "merge-base", "--is-ancestor", ancestor, descendant)
	switch {
	case res.err != nil:
		return ancestryUnknown, res.err.Error()
	case res.code == 0:
		return ancestryYes, ""
	case res.code != 1:
		return ancestryUnknown, fmt.Sprintf("git merge-base exited %d: %s", res.code, res.stderr)
	case shallow:
		return ancestryUnknown, "the repository is shallow, so a missing ancestor may lie beyond its boundary"
	default:
		return ancestryNo, ""
	}
}
