package cli

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// tddExempt is the line a reply ends with when a source change has no
// behavior to test, such as a pure refactor or a generated file.
var tddExempt = regexp.MustCompile(`(?m)^\s*TDD-exempt:\s*\S`)

// minRedBudget is the least time worth giving the before-the-change run.
const minRedBudget = 30 * time.Second

// tddEnforced reports whether the project asks for test-first work:
// methodology.mode tdd with enforce: true, unless AUTOPUS_TDD_STOP=off.
func tddEnforced(root string) bool {
	if strings.EqualFold(os.Getenv("AUTOPUS_TDD_STOP"), "off") {
		return false
	}
	cfg := nearestConfig(root)
	return cfg != nil && cfg.Methodology.Enforce && content.NormalizeMethodologyMode(cfg.Methodology.Mode) == "tdd"
}

// redGate runs after the tests pass on the changed tree. It returns nil when
// the tree may be recorded as passing; otherwise the output to return, and
// the tree is checked again at the next stop.
func redGate(ctx context.Context, in stopHookInput, root string, changed []string, budget time.Duration) *stopHookOutput {
	if !tddEnforced(root) || tddExempt.MatchString(in.LastAssistantMessage) {
		return nil
	}
	if budget < minRedBudget {
		return &stopHookOutput{SystemMessage: "auto verify stop: no time left to run the changed tests against the code before the change; test-first was not checked"}
	}
	res := verifygate.CheckRed(ctx, root, changed, budget)
	var reason string
	switch res.Verdict {
	case verifygate.RedNotApplicable, verifygate.RedProven:
		return nil
	case verifygate.RedUnavailable:
		return &stopHookOutput{SystemMessage: "auto verify stop: could not run the changed tests against the code before the change (" +
			res.Detail + "); test-first was not checked"}
	case verifygate.RedNoTest:
		reason = fmt.Sprintf("Test-first (methodology: tdd): product source changed (%s) but no test changed. "+
			"Add a test that fails without this change and passes with it.", strings.Join(res.Sources, ", "))
	case verifygate.RedPassesBefore:
		reason = fmt.Sprintf("Test-first (methodology: tdd): the changed tests (%s) also pass against the code before this change, "+
			"so they do not test it. Add an assertion that fails without the change to %s.",
			strings.Join(res.Tests, ", "), strings.Join(res.Sources, ", "))
	}
	if countStopBlock(in.SessionID) > verifyStopMaxBlocks {
		return &stopHookOutput{SystemMessage: "auto verify stop: test-first still not shown after several attempts; ending anyway. " + reason}
	}
	return &stopHookOutput{Decision: "block", Reason: reason +
		"\n\nIf the change has no behavior to test (a pure refactor, a rename, generated output), end your reply with a line " +
		"`TDD-exempt: <reason>` instead."}
}
