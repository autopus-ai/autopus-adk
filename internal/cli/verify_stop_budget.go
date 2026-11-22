package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// budgetSkipped handles a run whose only skips are budget skips: the suite
// is longer than the budget, its tests were making progress, and nothing had
// failed or hung when time ran out. Rerunning the same tree at every stop
// would spend the budget again for the same answer, so the tree is recorded
// with the note; any edit changes the fingerprint and runs it again. The
// agent is never told the tests passed.
func budgetSkipped(cmd *cobra.Command, in stopHookInput, root string, changed []string, fingerprint string,
	skipped []verifygate.Skip, left time.Duration, followup bool) stopHookOutput {
	unfinished := describeSkips(skipped)
	note := "auto verify stop: " + unfinished + "; nothing failed within the budget, so the stop is " +
		"not blocked. Run the rest with a longer -timeout (or leave it to CI) before calling the change verified"
	status := "The build passes; tests did not finish within the budget: " + unfinished +
		". Nothing had failed when time ran out; run them in full (a longer -timeout, or CI) before calling the change verified."
	// Test-first and the verification round still apply: a missing test is
	// known without running anything, and the round is the agent's.
	record := true
	if out := redGate(cmd.Context(), in, root, changed, left); out != nil {
		if out.Decision == "block" {
			return withBudgetNote(*out, note)
		}
		note = joinWarnings(note, out.SystemMessage)
		// Test-first was not checked: the next stop checks this tree again.
		record = false
	}
	if record {
		resetStopBlocks(in.SessionID)
		writeStopState(in.SessionID, "budget", fingerprint+"\n"+note)
	}
	return withBudgetNote(followupRound(cmd, in, root, followup, status), note)
}

// withBudgetNote carries the note where the reader of the output sees it:
// in the reason the agent gets when the stop is blocked, as a system message
// otherwise.
func withBudgetNote(out stopHookOutput, note string) stopHookOutput {
	if out.Decision == "block" {
		out.Reason += "\n\n" + note
		return out
	}
	return withSecretWarning(out, note)
}

// recordedBudgetNote returns the note recorded with a budget skip of this
// exact tree, so a later stop repeats it instead of reporting nothing.
func recordedBudgetNote(session, fingerprint string) (string, bool) {
	recorded, note, _ := strings.Cut(readStopState(session, "budget"), "\n")
	if recorded == "" || recorded != fingerprint {
		return "", false
	}
	return note + " (this tree was not run again)", true
}

func onlyBudgetSkips(skipped []verifygate.Skip) bool {
	for _, s := range skipped {
		if !s.Budget {
			return false
		}
	}
	return true
}

func describeSkips(skipped []verifygate.Skip) string {
	parts := make([]string, 0, len(skipped))
	for _, s := range skipped {
		parts = append(parts, fmt.Sprintf("`%s` (%s)", s.Name, s.Reason))
	}
	return strings.Join(parts, ", ")
}
