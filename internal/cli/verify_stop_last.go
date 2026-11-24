package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// lastRun is the result of the last tree the gate ran, per repository: a
// stop on the same tree (see verifygate.TreeHash) reuses it instead of
// running the tests again, and the agent can read it with
// `auto verify stop --last` instead of rerunning the suite.
type lastRun struct {
	Tree string    `json:"tree"`
	At   time.Time `json:"at"`
	// Budget is the hook's budget, part of the tree hash, so --last can
	// tell whether the tree is still the one judged.
	Budget string `json:"budget"`
	// Session is the session that ran it: only that session may turn a
	// recorded run into a pass.
	Session string `json:"session"`
	// Kind is pass, unverified (budget, baseline or hang: not run again,
	// not passed), block, testfirst (tests ran, test-first was not shown),
	// or partial (owed another run, never reused).
	Kind   string `json:"kind"`
	Result string `json:"result"`
	// Run is the run behind a testfirst block, judged again without
	// running anything once the reply carries a TDD-exempt line.
	Run *verifygate.ChargedRun `json:"run,omitempty"`
}

const (
	lastPass       = "pass"
	lastUnverified = "unverified"
	lastBlock      = "block"
	lastTestFirst  = "testfirst"
	lastPartial    = "partial"
)

// lastHint goes under every block the gate's own run produced.
const lastHint = "\n\n`auto verify stop --last` prints this result again; read it there instead of rerunning the whole package."

// testFirstPrefix starts every test-first block reason (see redGate).
const testFirstPrefix = "Test-first (methodology: tdd)"

func readLastRun(root string) (lastRun, bool) {
	var r lastRun
	if err := json.Unmarshal([]byte(readStopState(followupTreeKey(root), "last")), &r); err != nil || r.Tree == "" {
		return lastRun{}, false
	}
	return r, true
}

// reuseLastRun answers a stop on the tree the last run blocked. The block
// is not repeated for the same result, from any session: the stop goes
// through with the result as a note, unverified (failing closed: the tree
// never passes this way). A test-first block stands until the reply carries
// a TDD-exempt line, which the session that ran the tree judges on the
// recorded run. Passes and unverified results are reused only by the session
// that recorded them (its own state, checked before this): the tree hash
// cannot see ignored files or environment variables, so another session
// runs the tree again. Any edit, a new HEAD or another Go environment
// changes the tree.
func reuseLastRun(cmd *cobra.Command, in stopHookInput, root string, changed []string, tree string,
	budget time.Duration, followup bool) (stopHookOutput, bool) {
	last, ok := readLastRun(root)
	if !ok || last.Tree != tree {
		return stopHookOutput{}, false
	}
	switch {
	case last.Kind == lastBlock:
		return stopHookOutput{SystemMessage: "auto verify stop: this tree was blocked at an earlier stop and no tracked or untracked " +
			"file changed since (ignored files are not compared), so this stop is not blocked again; the tree is not verified. " +
			"Earlier result: " + clip(firstLine(last.Result), 300) + " (`auto verify stop --last` prints it in full)"}, true
	case last.Kind == lastTestFirst && last.Run != nil &&
		!(tddExempt.MatchString(in.LastAssistantMessage) && last.Session != in.SessionID):
		return reuseTestFirst(cmd, in, root, changed, tree, last, budget, followup), true
	}
	return stopHookOutput{}, false
}

// reuseTestFirst repeats a test-first block on an unchanged tree without
// running the tests again, up to the block limit; a TDD-exempt line judges
// the recorded run as if it had just finished.
func reuseTestFirst(cmd *cobra.Command, in stopHookInput, root string, changed []string, tree string,
	last lastRun, budget time.Duration, followup bool) stopHookOutput {
	if tddExempt.MatchString(in.LastAssistantMessage) {
		out := judgeRun(cmd, in, root, changed, tree, *last.Run, time.Now(), budget, followup)
		if note := last.Run.Note(); note != "" {
			out = withBudgetNote(out, note)
		}
		return recordLastRun(in.SessionID, root, tree, budget, *last.Run, out, true)
	}
	if countStopBlock(in.SessionID) > verifyStopMaxBlocks {
		return stopHookOutput{SystemMessage: "auto verify stop: test-first still not shown after several attempts; ending anyway. " +
			firstLine(last.Result)}
	}
	return stopHookOutput{Decision: "block", Reason: last.Result + lastHint}
}

// recordLastRun stores what this run decided for the tree and returns out
// with the hint under a block. An interrupted run, or one whose tree changed
// while it ran (same is false), records nothing.
func recordLastRun(session, root, tree string, budget time.Duration, run verifygate.ChargedRun, out stopHookOutput, same bool) stopHookOutput {
	if same && !slices.ContainsFunc(run.Skipped, func(s verifygate.Skip) bool { return s.Interrupted }) {
		saveLastRun(session, root, tree, budget, run, out)
	}
	if out.Decision == "block" {
		out.Reason += lastHint
	}
	return out
}

func saveLastRun(session, root, tree string, budget time.Duration, run verifygate.ChargedRun, out stopHookOutput) {
	r := lastRun{Tree: tree, At: time.Now().UTC(), Budget: budget.String(), Session: session, Kind: lastPartial, Result: out.Reason}
	if out.Decision != "block" {
		r.Result = out.SystemMessage
	}
	if note := recordedRaw(session, tree); readStopState(session, "pass") == tree {
		r.Kind = lastPass
		if r.Result == "" {
			r.Result = statusPassed
		}
	} else if note != "" {
		r.Kind, r.Result = lastUnverified, note
	} else if out.Decision == "block" && strings.HasPrefix(out.Reason, testFirstPrefix) {
		r.Kind, r.Run = lastTestFirst, &run
	} else if out.Decision == "block" && !(run.Failure == nil && baselineOwed(run.Skipped)) {
		// Tests still owed after a baseline check run at the next stop,
		// even on this tree; any other block is the tree's one block.
		r.Kind = lastBlock
	}
	if data, err := json.Marshal(r); err == nil {
		_ = saveStopState(followupTreeKey(root), "last", string(data))
	}
}

// recordedRaw is the note of a budget, baseline or hang record of this tree.
func recordedRaw(session, tree string) string {
	for _, kind := range []string{"budget", "baseline", "hang"} {
		if note, ok := recordedNote(session, kind, tree); ok {
			return strings.TrimSuffix(note, recordedSuffix)
		}
	}
	return ""
}

// baselineOwed reports a run whose only unfinished work is the tests a
// baseline check stopped short of.
func baselineOwed(skipped []verifygate.Skip) bool {
	return len(skipped) > 0 && !slices.ContainsFunc(skipped, func(s verifygate.Skip) bool { return !s.Baseline })
}

// printLastRun prints the last result of the repository holding dir.
func printLastRun(cmd *cobra.Command, w io.Writer, dir string) error {
	root, _, ok := verifygate.ChangedFiles(cmd.Context(), dir)
	if !ok {
		return fmt.Errorf("%s is not inside a git work tree", dir)
	}
	last, found := readLastRun(root)
	if !found {
		fmt.Fprintf(w, "auto verify stop: no result recorded for %s yet\n", root)
		return nil
	}
	current := "the work tree has changed since"
	if verifygate.TreeHash(cmd.Context(), root, last.Budget) == last.Tree {
		current = "the work tree is unchanged since"
	}
	fmt.Fprintf(w, "auto verify stop, %s at %s (%s):\n\n%s\n", last.Kind, last.At.Local().Format(time.RFC3339), current, last.Result)
	return nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + " …"
}
