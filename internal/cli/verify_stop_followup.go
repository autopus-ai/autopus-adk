package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// followupMinLines is how much code must change before a passing session is
// sent on to review and QA. Smaller edits end as soon as the build is green.
const followupMinLines = 40

// followupMaxRounds bounds the review and QA rounds one session runs. Pilot C
// (2026-11-07) measured a second round costing about $10 on a 360-line change
// without fixing anything the first round had not, so one round is the cap.
const followupMaxRounds = 1

// followupReason is what the agent is told to do next once the build and
// tests pass. It runs the verification half of the harness without the user
// having to ask for it.
const followupReason = `The build and tests pass. Before finishing, verify this change the way a separate reviewer and a tester would, without asking the user to start it:

1. Run the reviewer agent on ` + "`git diff HEAD`" + ` (and new files) against what the user asked for. Report only blockers: wrong behavior, missing cases, security or data-loss risk, each with file, line and a failing case.
2. If the change has runnable behavior (a CLI, server, UI, library API), run the qa-explorer agent in hunt mode on the changed behavior, then ` + "`auto qa hunt verify`" + `.
3. Fix every reviewer blocker and every confirmed bug, rerun the tests, and finish with a short summary of what was checked and fixed.

If a fix needs a decision only the user can make (product behavior, scope), ask it with AskUserQuestion as a multiple-choice question with your recommendation first, instead of an open question. Otherwise do not ask; proceed and state your assumption.`

// followupEnabled reports whether the follow-up round is on. It is opt-in:
// on tasks the session already solves, Pilot C measured it multiplying cost
// about 14x with no change in pass rate. AUTOPUS_FOLLOWUP=on or --followup
// turns it on; AUTOPUS_FOLLOWUP=off wins over both.
func followupEnabled(flag bool) bool {
	switch strings.ToLower(os.Getenv("AUTOPUS_FOLLOWUP")) {
	case "off":
		return false
	case "on":
		return true
	}
	return flag
}

// nextFollowup decides whether this passing tree goes on to review and QA.
// It fires once per followupMinLines of change in a session, so fixing the
// review's findings does not start another round.
func nextFollowup(session string, changedLines int) bool {
	if changedLines < followupMinLines {
		return false
	}
	state := strings.Fields(readStopState(session, "followup"))
	done, rounds := 0, 0
	if len(state) == 2 {
		done, _ = strconv.Atoi(state[0])
		rounds, _ = strconv.Atoi(state[1])
	}
	if rounds >= followupMaxRounds || (done > 0 && changedLines < done+followupMinLines) {
		return false
	}
	writeStopState(session, "followup", fmt.Sprintf("%d %d", changedLines, rounds+1))
	return true
}

func followupOutput(changedLines int) stopHookOutput {
	return stopHookOutput{Decision: "block", Reason: fmt.Sprintf("%d changed lines. %s", changedLines, followupReason)}
}
