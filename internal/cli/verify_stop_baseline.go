package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// baselinePassed handles a run whose only failures are tests that fail the
// same way on HEAD. That is not a pass: the tree is recorded under its own
// kind ("baseline") with the note, never as "pass", so a later stop on the
// same tree repeats the note instead of reporting nothing, and the
// verification round is told which tests still fail. Any edit changes the
// fingerprint and runs it again.
func baselinePassed(cmd *cobra.Command, in stopHookInput, root string, changed []string, fingerprint string,
	run verifygate.ChargedRun, left time.Duration, followup bool) stopHookOutput {
	note := run.Note()
	status := fmt.Sprintf("The build passes; %d failing test(s) fail as they do on HEAD and were not charged to this change: %s.",
		len(run.Preexisting), strings.Join(run.Preexisting, ", "))
	if out := redGate(cmd.Context(), in, root, changed, left); out != nil {
		// Blocked on test-first, or test-first was not checked: either way
		// the next stop checks this tree again.
		return *out
	}
	resetStopBlocks(in.SessionID)
	writeStopState(in.SessionID, "baseline", fingerprint+"\n"+note)
	return followupRound(cmd, in, root, followup, status)
}
