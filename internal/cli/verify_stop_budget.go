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
	skipped []verifygate.Skip, preexisting []string, left time.Duration, followup bool) stopHookOutput {
	unfinished := describeSkips(skipped)
	failed := "nothing failed within the budget"
	statusFailed := "Nothing had failed when time ran out"
	if baselineOwed(skipped) {
		return baselineOwedRun(cmd, in, root, changed, unfinished, preexisting, left, followup)
	}
	if len(preexisting) > 0 {
		// The step was stopped early to check its failures against HEAD.
		failed = fmt.Sprintf("%d test(s) failed as they do on HEAD (not charged to this change) and the tests after them did not run",
			len(preexisting))
		statusFailed = fmt.Sprintf("%d test(s) failed as they do on HEAD and were not charged to this change: %s; the tests after them did not run",
			len(preexisting), strings.Join(preexisting, ", "))
	}
	note := "auto verify stop: " + unfinished + "; " + failed + ", so the stop is " +
		"not blocked. Run the rest with a longer -timeout (or leave it to CI) before calling the change verified"
	status := "The build passes; tests did not finish within the budget: " + unfinished +
		". " + statusFailed + "; run them in full (a longer -timeout, or CI) before calling the change verified."
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

// stopBudget is the --budget flag when it was given. Without it,
// hooks.verify_stop_budget may only lower the default: whatever started the
// gate bare (a hook line written before the key, a runner that passes no
// flag) has a kill timer sized for the default, and a larger budget would be
// killed with no verdict. The generators pass --budget whenever the key is
// set, so budget and timer move together.
func stopBudget(cmd *cobra.Command, root string, flag time.Duration) time.Duration {
	if cmd.Flags().Changed("budget") {
		return flag
	}
	if cfg := nearestConfig(root); cfg != nil {
		if d, err := cfg.Hooks.VerifyStopBudgetDuration(); err == nil {
			return min(d, flag)
		}
	}
	return flag
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

// recordedNote returns the note recorded with a budget skip or a suspected
// hang (kind) of this exact tree, so a later stop repeats it instead of
// reporting nothing; neither is a pass.
func recordedNote(session, kind, fingerprint string) (string, bool) {
	recorded, note, _ := strings.Cut(readStopState(session, kind), "\n")
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

// baselineOwedRun handles a run that stopped a step early only to check its
// failures on HEAD, all of which fail there too. That is not the suite
// outlasting the budget: nothing is charged, the stop is not blocked for it,
// and the tree is not recorded, so the next stop runs the tests after them
// (HEAD's answers are cached, so that run is not stopped early again).
func baselineOwedRun(cmd *cobra.Command, in stopHookInput, root string, changed []string, unfinished string,
	preexisting []string, left time.Duration, followup bool) stopHookOutput {
	note := fmt.Sprintf("auto verify stop: %s; %d test(s) fail as they do on HEAD and were not charged to this change, "+
		"so the stop is not blocked; the tests after them run at the next stop", unfinished, len(preexisting))
	status := fmt.Sprintf("The build passes; %d test(s) fail as they do on HEAD and were not charged to this change: %s; "+
		"the tests after them were not run yet and run at the next stop.", len(preexisting), strings.Join(preexisting, ", "))
	if out := redGate(cmd.Context(), in, root, changed, left); out != nil {
		if out.Decision == "block" {
			return withBudgetNote(*out, note)
		}
		note = joinWarnings(note, out.SystemMessage)
	}
	return withBudgetNote(followupRound(cmd, in, root, followup, status), note)
}
