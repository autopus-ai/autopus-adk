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
// stop on the same tree (same HEAD, same changed contents) reuses it instead
// of running the tests again, whichever session stops, and the agent can read
// it with `auto verify stop --last` instead of rerunning the suite.
type lastRun struct {
	Root string    `json:"root"`
	Tree string    `json:"tree"`
	At   time.Time `json:"at"`
	// Kind is pass, unverified (budget, baseline or hang: not run again,
	// not passed), block, or partial (owed another run, never reused).
	Kind   string `json:"kind"`
	Result string `json:"result"`
	// TestFirst marks a test-first block, which the reply's TDD-exempt
	// line answers without any edit.
	TestFirst bool `json:"test_first,omitempty"`
}

const (
	lastPass       = "pass"
	lastUnverified = "unverified"
	lastBlock      = "block"
	lastPartial    = "partial"
)

// lastHint goes under every block the gate's own run produced.
const lastHint = "\n\n`auto verify stop --last` prints this result again; read it there instead of rerunning the whole package."

// statusReused is the round status for a tree another session ran and left
// unverified.
const statusReused = "The build passes; the tests did not fully run on this tree (see the note): " +
	"run them in full (a longer -timeout, or CI) before calling the change verified."

// testFirstPrefix starts every test-first block reason (see redGate).
const testFirstPrefix = "Test-first (methodology: tdd)"

func lastRunKey(root string) string { return followupTreeKey(root) }

func readLastRun(root string) (lastRun, bool) {
	var r lastRun
	if err := json.Unmarshal([]byte(readStopState(lastRunKey(root), "last")), &r); err != nil || r.Tree == "" {
		return lastRun{}, false
	}
	return r, true
}

// reuseLastRun answers a stop on the tree the last run judged. A tree that
// was blocked is not blocked again for the same result: the stop goes
// through with the result as a note, unverified. Any edit, or a new HEAD,
// changes the tree and runs it again.
func reuseLastRun(cmd *cobra.Command, in stopHookInput, root, tree string, followup bool) (stopHookOutput, bool) {
	last, ok := readLastRun(root)
	if !ok || last.Tree != tree || (last.TestFirst && tddExempt.MatchString(in.LastAssistantMessage)) {
		return stopHookOutput{}, false
	}
	switch last.Kind {
	case lastPass:
		writeStopState(in.SessionID, "pass", tree)
		return followupRound(cmd, in, root, followup, statusPassed), true
	case lastUnverified:
		return withBudgetNote(followupRound(cmd, in, root, followup, statusReused), last.Result+" (this tree was not run again)"), true
	case lastBlock:
		return stopHookOutput{SystemMessage: "auto verify stop: this tree was blocked at an earlier stop and nothing changed since, " +
			"so this stop is not blocked again; the tree is not verified. Earlier result: " + clip(firstLine(last.Result), 300) +
			" (`auto verify stop --last` prints it in full)"}, true
	}
	return stopHookOutput{}, false
}

// recordLastRun stores what this run decided for the tree and returns out
// with the hint under a block. An interrupted run records nothing.
func recordLastRun(session, root, tree string, run verifygate.ChargedRun, out stopHookOutput) stopHookOutput {
	if slices.ContainsFunc(run.Skipped, func(s verifygate.Skip) bool { return s.Interrupted }) {
		return out
	}
	r := lastRun{Root: root, Tree: tree, At: time.Now().UTC(), Kind: lastPartial, Result: out.Reason}
	if out.Decision != "block" {
		r.Result = out.SystemMessage
	}
	switch {
	case readStopState(session, "pass") == tree:
		r.Kind = lastPass
		if r.Result == "" {
			r.Result = statusPassed
		}
	case recordedRaw(session, tree) != "":
		r.Kind, r.Result = lastUnverified, recordedRaw(session, tree)
	case out.Decision == "block" && !(run.Failure == nil && baselineOwed(run.Skipped)):
		// Tests still owed after a baseline check run at the next stop,
		// even on this tree; any other block is the tree's one block.
		r.Kind = lastBlock
		r.TestFirst = strings.HasPrefix(out.Reason, testFirstPrefix)
	}
	if data, err := json.Marshal(r); err == nil {
		_ = saveStopState(lastRunKey(root), "last", string(data))
	}
	if out.Decision == "block" {
		out.Reason += lastHint
	}
	return out
}

// recordedRaw is the note of a budget, baseline or hang record of this tree.
func recordedRaw(session, tree string) string {
	for _, kind := range []string{"budget", "baseline", "hang"} {
		recorded, note, _ := strings.Cut(readStopState(session, kind), "\n")
		if recorded == tree {
			return note
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
	_, changed, _ := verifygate.ChangedFiles(cmd.Context(), root)
	current := "the work tree has changed since"
	if verifygate.TreeHash(cmd.Context(), root, changed) == last.Tree {
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
