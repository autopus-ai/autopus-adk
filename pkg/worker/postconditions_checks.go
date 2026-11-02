package worker

import (
	"fmt"
	"os"
	"strings"
)

// The checks of SPEC-WORKERPOST-001. Each returns exactly one of the four
// states; a git failure or a missing fact is `unknown`, never `satisfied`.

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
	if head.err != "" {
		return withStatus(check, statusUnknown, "cannot read HEAD: "+head.err)
	}
	check.Observed = head.sha
	if level == levelForbidden {
		return g.forbiddenCommit(check, workDir, head, base)
	}
	switch {
	case !base.GitRepo:
		return withStatus(check, statusUnknown, "no baseline commit was captured before execution")
	case head.sha == "":
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
	case !base.GitRepo && head.sha == "":
		return withStatus(check, statusSatisfied, "no commit exists before or after execution")
	case !base.GitRepo:
		return withStatus(check, statusUnsatisfied, "a HEAD commit "+head.sha+" appeared in a work directory that had none")
	case head.sha == "":
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

// branchChecks judges the branch action (REQ-WPOST-BRANCH-01, LEGACY-02).
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

func (g postconditionGit) forbiddenBranch(check postconditionCheck, workDir, ref string, before refState) postconditionCheck {
	check.Ref = ref
	name := strings.TrimPrefix(ref, branchRefPrefix)
	sha, state, why := g.resolveRef(workDir, ref)
	check.Observed = sha
	switch {
	case state == refUnknown:
		return withStatus(check, statusUnknown, why)
	case state == refAbsent:
		return withStatus(check, statusSatisfied, fmt.Sprintf("local branch %q does not exist", name))
	case before == refPresent:
		return withStatus(check, statusSatisfied, fmt.Sprintf("local branch %q existed before execution", name))
	case before == refAbsent:
		return withStatus(check, statusUnsatisfied,
			fmt.Sprintf("local branch %q was created during execution although branch is forbidden", name))
	default:
		return withStatus(check, statusUnknown, fmt.Sprintf("cannot tell whether local branch %q existed before execution", name))
	}
}

func (g postconditionGit) localBranchCheck(check postconditionCheck, workDir, ref, expected string) postconditionCheck {
	check.Ref, check.Expected = ref, expected
	name := strings.TrimPrefix(ref, branchRefPrefix)
	sha, state, why := g.resolveRef(workDir, ref)
	check.Observed = sha
	switch {
	case state == refUnknown:
		return withStatus(check, statusUnknown, why)
	case state == refAbsent:
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

// isAncestor answers from local objects only. Replace refs are ignored by the
// runner; a graft file makes any answer untrustworthy, and a shallow boundary
// makes a negative answer untrustworthy.
func (g postconditionGit) isAncestor(workDir, ancestor, descendant string) (ancestryAnswer, string) {
	graft, err := g.output(workDir, "rev-parse", "--path-format=absolute", "--git-path", "info/grafts")
	if err != nil {
		return ancestryUnknown, "cannot locate the graft file: " + err.Error()
	}
	if _, statErr := os.Lstat(graft); statErr == nil {
		return ancestryUnknown, "the repository has a graft file (info/grafts), so ancestry cannot be trusted"
	} else if !os.IsNotExist(statErr) {
		return ancestryUnknown, "cannot inspect the graft file: " + statErr.Error()
	}
	res := g.run(workDir, nil, "merge-base", "--is-ancestor", ancestor, descendant)
	switch {
	case res.err != nil:
		return ancestryUnknown, res.err.Error()
	case res.code == 0:
		return ancestryYes, ""
	case res.code != 1:
		return ancestryUnknown, fmt.Sprintf("git merge-base exited %d: %s", res.code, res.stderr)
	}
	if shallow, err := g.output(workDir, "rev-parse", "--is-shallow-repository"); err != nil || shallow != "false" {
		return ancestryUnknown, "the repository may be shallow, so a missing ancestor may lie beyond its boundary"
	}
	return ancestryNo, ""
}
