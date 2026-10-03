package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// signOff is the last line of every commit the loop makes.
const signOff = "🐙 Autopus <noreply@autopus.co>"

// fixOrder is the class priority. The product comes first because a product
// fix can make the test failures behind it disappear on the next run.
var fixOrder = []triage.Class{triage.ClassProductDefect, triage.ClassTestDefect, triage.ClassTestDrift}

// fix makes one agent call for the highest-priority fixable class, guards its
// diff, and commits or reverts it. It returns nil only after a commit.
func (r *runner) fix(ctx context.Context, it *Iteration) *stopState {
	class, verdicts := pickClass(it.Failures)
	head, err := r.git.line("rev-parse", "HEAD")
	if err != nil {
		return gitStop(err)
	}
	dirty, err := r.git.trackedDirty()
	if err != nil {
		return gitStop(err)
	}
	if len(dirty) > 0 {
		return &stopState{StopBlockedEnv, CodeBlockedEnv, "the lane run changed tracked files: " + strings.Join(dirty, "; ")}
	}
	base, err := r.git.snapshot()
	if err != nil {
		return gitStop(err)
	}
	started := time.Now()
	resp, agentErr := r.deps.Agent(ctx, agentexec.Request{
		Target: r.opts.Agent, Mode: agentexec.ModeEdit, Prompt: r.prompt(it.N, class, verdicts),
		WorkDir: r.projectDir, Timeout: r.opts.AgentTimeout,
	})
	duration := resp.Duration
	if duration <= 0 {
		duration = time.Since(started)
	}
	it.Agent = &AgentRecord{Class: class, Journeys: journeyIDs(verdicts), ExitCode: resp.ExitCode, Duration: duration.Round(time.Millisecond).String()}
	if agentErr != nil {
		it.Agent.Error = agentErr.Error()
		var failure *agentexec.AgentError
		if errors.As(agentErr, &failure) {
			it.Agent.ExitCode = failure.ExitCode
		}
		reverted, revertErr := r.restore(head, base)
		it.ChangedPaths = reverted
		return withRevert(agentStop(agentErr), revertErr)
	}
	if moved := r.headMoved(head); moved != "" {
		reverted, revertErr := r.restore(head, base)
		it.ChangedPaths = reverted
		it.Guard = &GuardVerdict{Reason: moved}
		return withRevert(&stopState{StopGuardRejected, CodeGuardRejected, moved}, revertErr)
	}
	changed, err := r.git.changed(base, r.prefix)
	if err != nil {
		return gitStop(err)
	}
	judged := r.projectRel(changed)
	it.ChangedPaths = changed
	if base.excludeChanged() {
		// No diff shows info/exclude, so the guard is told about it directly.
		judged = append([]string{gitExcludeRel}, judged...)
		it.ChangedPaths = append([]string{gitExcludeRel}, changed...)
	}
	if len(judged) == 0 {
		it.Guard = &GuardVerdict{Reason: "agent changed nothing"}
		return &stopState{StopNoProgress, CodeNoProgress, fmt.Sprintf("the %s agent changed nothing", class)}
	}
	verdict := Guard(GuardInput{Class: class, Paths: judged, TestDir: r.testDir, Before: r.headContent, After: r.treeContent})
	it.Guard = &verdict
	if !verdict.Accepted {
		_, revertErr := r.restore(head, base)
		return withRevert(&stopState{StopGuardRejected, CodeGuardRejected, verdict.Path + ": " + verdict.Reason}, revertErr)
	}
	stage := changed
	if class == triage.ClassTestDrift {
		if stage, err = r.recompile(changed, base); err != nil {
			_, revertErr := r.restore(head, base)
			it.Guard = &GuardVerdict{Reason: "recompile after the heal failed: " + err.Error()}
			return withRevert(&stopState{StopGuardRejected, CodeGuardRejected, it.Guard.Reason}, revertErr)
		}
		it.ChangedPaths = stage
	}
	sha, err := r.git.commit(stage, commitMessage(it.N, r.report.RunID, class, verdicts))
	if err != nil {
		_, revertErr := r.restore(head, base)
		return withRevert(&stopState{StopCommitRejected, CodeCommitRejected, err.Error()}, revertErr)
	}
	it.Commit = sha
	return nil
}

func pickClass(verdicts []triage.Verdict) (triage.Class, []triage.Verdict) {
	for _, class := range fixOrder {
		var picked []triage.Verdict
		for _, v := range verdicts {
			if v.Class == class {
				picked = append(picked, v)
			}
		}
		if len(picked) > 0 {
			return class, picked
		}
	}
	return triage.ClassUnknown, nil
}

func journeyIDs(verdicts []triage.Verdict) []string {
	set := map[string]bool{}
	for _, v := range verdicts {
		set[v.JourneyID] = true
	}
	ids := sortedKeys(set)
	sort.Strings(ids)
	return ids
}

// restore returns the work tree to head plus the baseline untracked files. It
// refuses once the agent has left the loop branch: resetting another branch
// could destroy work that is not the loop's.
func (r *runner) restore(head string, base baseline) ([]string, error) {
	branch, detached, err := r.git.currentRef()
	if err != nil {
		return nil, err
	}
	if detached || branch != r.report.Branch {
		return nil, fmt.Errorf("agent left the loop branch for %s; nothing was reverted", branch)
	}
	if now, err := r.git.line("rev-parse", "HEAD"); err == nil && now != head {
		if _, err := r.git.run("reset", "-q", "--mixed", head); err != nil {
			return nil, err
		}
	}
	return r.git.revert(base, r.prefix)
}

// headMoved reports an agent that committed or switched branches itself; only
// the loop commits, after the guard.
func (r *runner) headMoved(head string) string {
	branch, detached, err := r.git.currentRef()
	switch {
	case err != nil:
		return "cannot read HEAD after the agent run: " + err.Error()
	case detached || branch != r.report.Branch:
		return "agent left the loop branch for " + branch
	}
	if now, err := r.git.line("rev-parse", "HEAD"); err != nil || now != head {
		return "agent moved HEAD; only the loop may commit"
	}
	return ""
}

// recompile regenerates specs after an accepted heal and returns every path
// to stage. Compiler output must stay under the Playwright testDir.
func (r *runner) recompile(guarded []string, base baseline) ([]string, error) {
	if err := r.deps.Compile(r.projectDir); err != nil {
		return nil, err
	}
	all, err := r.git.changed(base, r.prefix)
	if err != nil {
		return nil, err
	}
	root := r.prefix
	if dir := strings.Trim(path.Clean(r.testDir), "/"); dir != "" && dir != "." {
		root += dir + "/"
	}
	allowed := setOf(guarded)
	for _, p := range all {
		if !allowed[p] && !strings.HasPrefix(p, root) {
			return nil, fmt.Errorf("the compiler changed %s outside %s", p, root)
		}
	}
	return all, nil
}

// projectRel maps repository paths to project paths. A path outside the
// project keeps a "../" marker so the guard rejects it.
func (r *runner) projectRel(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if rel, ok := strings.CutPrefix(p, r.prefix); ok {
			out = append(out, rel)
		} else {
			out = append(out, "../"+p)
		}
	}
	return out
}

func (r *runner) headContent(rel string) ([]byte, bool) {
	out, err := r.git.run("show", "HEAD:"+r.prefix+rel)
	return []byte(out), err == nil
}

func (r *runner) treeContent(rel string) ([]byte, bool) {
	body, err := os.ReadFile(filepath.Join(r.projectDir, filepath.FromSlash(rel)))
	return body, err == nil
}

func commitMessage(n int, runID string, class triage.Class, verdicts []triage.Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "fix(qa): %s %s\n\n", class, strings.Join(journeyIDs(verdicts), " "))
	fmt.Fprintf(&b, "The QA loop agent repaired this %s failure in iteration %d, and the\ndiff guard accepted the change. Triage evidence:\n\n", class, n)
	for _, v := range verdicts {
		fmt.Fprintf(&b, "- %s: %s\n", v.JourneyID, firstNonEmpty(oneLine(v.Signal), "no signal"))
	}
	fmt.Fprintf(&b, "\nConstraint: QA loop iteration %d\nConfidence: medium\nRelated: %s\n\n%s\n", n, runID, signOff)
	return b.String()
}

func agentStop(err error) *stopState {
	var gap *agentexec.SetupGapError
	if errors.As(err, &gap) {
		return &stopState{StopBlockedEnv, firstNonEmpty(gap.Code, agentexec.CodeCLIMissing), err.Error()}
	}
	return &stopState{StopAgentFailed, firstNonEmpty(agentexec.ErrorCode(err), CodeAgentFailed), err.Error()}
}

func withRevert(stop *stopState, revertErr error) *stopState {
	if revertErr != nil {
		stop.detail += "; revert failed: " + revertErr.Error()
	}
	return stop
}

func gitStop(err error) *stopState {
	return &stopState{StopBlockedEnv, CodeGitFailed, err.Error()}
}
