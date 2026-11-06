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

// followupReason is what the agent is told to do next once the build and
// tests pass. It runs the verification half of the harness without the user
// having to ask for it.
const followupReason = `The build and tests pass. Before finishing, verify this change the way a separate reviewer and a tester would, without asking the user to start it:

1. Run the reviewer agent on ` + "`git diff HEAD`" + ` (and new files) against what the user asked for. Report only blockers: wrong behavior, missing cases, security or data-loss risk, each with file, line and a failing case.
2. If the change has runnable behavior (a CLI, server, UI, library API), run the qa-explorer agent in hunt mode on the changed behavior, then ` + "`auto qa hunt verify`" + `.
3. Fix every reviewer blocker and every confirmed bug, rerun the tests, and finish with a short summary of what was checked and fixed.

If a fix needs a decision only the user can make (product behavior, scope), ask it with AskUserQuestion as a multiple-choice question with your recommendation first, instead of an open question. Otherwise do not ask; proceed and state your assumption.`

// nextFollowup decides whether this passing tree goes on to review and QA.
// It fires once per followupMinLines of change in a session, so fixing the
// review's findings does not start another round.
func nextFollowup(session string, changedLines int) bool {
	if strings.EqualFold(os.Getenv("AUTOPUS_FOLLOWUP"), "off") || changedLines < followupMinLines {
		return false
	}
	done, _ := strconv.Atoi(readStopState(session, "followup"))
	if done > 0 && changedLines < done+followupMinLines {
		return false
	}
	writeStopState(session, "followup", strconv.Itoa(changedLines))
	return true
}

func followupOutput(changedLines int) stopHookOutput {
	return stopHookOutput{Decision: "block", Reason: fmt.Sprintf("%d changed lines. %s", changedLines, followupReason)}
}
