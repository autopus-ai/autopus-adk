package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// followupFullLines is the size, in changed product code lines, from which a
// change gets the full round. Below it a behavior change still gets the light
// round: a one-line logic edit can break a written policy as surely as a
// large one, so size decides how deep verification goes, never whether it
// happens.
const followupFullLines = 40

// followupMaxRounds bounds the review and QA rounds one session runs. Pilot C
// (2026-11-07) measured a second round costing about $10 on a 360-line change
// without fixing anything the first round had not, so one round is the cap.
const followupMaxRounds = 1

type followupTier string

const (
	followupNone  followupTier = ""
	followupLight followupTier = "light"
	followupFull  followupTier = "full"
)

// followupTierFor picks the round for a passing change. Tests, comments,
// documentation, configuration and generated files get none; new product
// files or a large change get the full round.
func followupTierFor(b verifygate.Behavior) followupTier {
	switch {
	case b.None():
		return followupNone
	case b.Unmeasured || len(b.NewFiles) > 0 || b.Lines >= followupFullLines:
		return followupFull
	}
	return followupLight
}

const followupAsk = `

If a fix needs a decision only the user can make (product behavior, scope, a policy conflict), ask it with AskUserQuestion as a multiple-choice question: say what you found, give each option a description of what it changes and what it costs, and put your recommendation first. Do not ask an open question. Otherwise do not ask; proceed and state your assumption.`

// followupFullReason runs the whole verification half of the harness: user
// scenarios from the request, then review, then the hunt.
const followupFullReason = `Before finishing, verify this change the way a separate user, reviewer and tester would, without asking the user to start it:

1. Check it against the user's request first. Use the SPEC id that covers it, or save the user's original request verbatim, run ` + "`auto qa intent --request-file <file> --format json`" + `, ask its questions, and record the answers with ` + "`auto qa intent answer`" + `. The change is uncommitted, so run ` + "`auto qa go <id> --auto --no-loop`" + ` and then ` + "`auto qa exec --all`" + ` (plus ` + "`auto qa run --lane <lane>`" + ` when GUI scenarios were compiled) on the working tree; the loop without --no-loop refuses uncommitted changes.
2. Run the reviewer agent on ` + "`git diff HEAD`" + ` (and new files) against what the user asked for. Report only blockers: wrong behavior, missing cases, security or data-loss risk, each with file, line and a failing case.
3. Run the qa-explorer agent in hunt mode on the changed behavior, then ` + "`auto qa hunt verify`" + `. The hunt covers bugs, behavior that breaks a quoted product policy or project rule (policy_mismatch), and measured inefficiency.
4. Fix every failing scenario, reviewer blocker and confirmed finding, rerun the tests, and finish with a short summary of what was checked and fixed.` + followupAsk

// followupLightReason is the round for a small behavior change: no new
// scenarios are generated, but existing ones rerun and the changed behavior
// is checked against the request and the written policies.
const followupLightReason = `This is a small behavior change; before finishing, verify it without asking the user to start it:

1. Rerun the existing user scenarios on the working tree: ` + "`auto qa exec --all`" + ` (skip this step when the project has none).
2. Run the reviewer agent on ` + "`git diff HEAD`" + ` against what the user asked for. Report only blockers, each with file, line and a failing case.
3. Run the qa-explorer agent in verify mode on the changed behavior only: does it do what the user asked, and does it still match the product policies and project rules that mention it (policy_mismatch, quoted verbatim)? Judge with ` + "`auto qa hunt verify`" + `.
4. Fix every failing scenario, reviewer blocker and confirmed finding, rerun the tests, and finish with a short summary of what was checked and fixed.` + followupAsk

// followupEnabled reports whether the follow-up round runs. It is on by
// default: the harness's job is verification, and the tiers keep a change
// with no runnable behavior free. AUTOPUS_FOLLOWUP=off turns it off; the
// --followup flag and AUTOPUS_FOLLOWUP=on are kept for older settings.
func followupEnabled(_ bool) bool {
	switch strings.ToLower(os.Getenv("AUTOPUS_FOLLOWUP")) {
	case "off", "0", "false":
		return false
	}
	return true
}

// nextFollowup decides whether this tree goes on to review and QA, and how
// deep. A session runs at most followupMaxRounds rounds, so fixing the
// round's findings does not start another; a tree another session already
// sent round is not sent again, so a fresh session that only asks a question
// is not blocked by someone else's uncommitted work. When the round cannot
// be recorded the hook fails open: an unrecorded round would block every
// stop forever.
func nextFollowup(session, root, fingerprint string, tier followupTier) followupTier {
	if tier == followupNone {
		return followupNone
	}
	rounds := sessionRounds(session)
	tree := followupTreeKey(root)
	if rounds >= followupMaxRounds || (fingerprint != "" && readStopState(tree, "followup") == fingerprint) {
		return followupNone
	}
	if err := saveStopState(session, "followup", fmt.Sprintf("%d %s", rounds+1, tier)); err != nil {
		return followupNone
	}
	_ = saveStopState(tree, "followup", fingerprint)
	return tier
}

// sessionRounds is how many rounds the session has run. An empty or
// unreadable state means none.
func sessionRounds(session string) int {
	rounds := 0
	_, _ = fmt.Sscanf(readStopState(session, "followup"), "%d", &rounds)
	return rounds
}

// followupTreeKey names the per-repository record of the last tree a round
// was run on.
func followupTreeKey(root string) string {
	sum := sha256.Sum256([]byte(root))
	return "tree-" + hex.EncodeToString(sum[:8])
}

// followupRound runs after a passing build, or after a change no build
// toolchain covers (shell, Java, Swift ...), whose behavior still needs the
// user's eyes even though nothing could be compiled.
func followupRound(cmd *cobra.Command, in stopHookInput, root string, flag, built bool) stopHookOutput {
	if !followupEnabled(flag) {
		return stopHookOutput{}
	}
	if sessionRounds(in.SessionID) >= followupMaxRounds {
		// The round told this session to fix what it found, so the tree it
		// now stops on is recorded as this session's: its fixes, and any
		// work it added after the round, are not sent round again to the
		// next session; the next session that changes product code gets a
		// round over the whole uncommitted diff, which includes them.
		_ = saveStopState(followupTreeKey(root), "followup", verifygate.Fingerprint(root, verifygate.ProductPaths(cmd.Context(), root)))
		return stopHookOutput{}
	}
	b := verifygate.BehaviorChange(cmd.Context(), root)
	tier := followupTierFor(b)
	if tier == followupNone {
		return stopHookOutput{}
	}
	if tier = nextFollowup(in.SessionID, root, verifygate.Fingerprint(root, b.Paths), tier); tier != followupNone {
		return followupOutput(b, tier, built)
	}
	return stopHookOutput{}
}

func followupOutput(b verifygate.Behavior, tier followupTier, built bool) stopHookOutput {
	status := "The build and tests pass."
	if !built {
		status = "No build or test toolchain covers these files, so nothing was compiled or tested."
	}
	reason := followupLightReason
	if tier == followupFull {
		reason = followupFullReason
	}
	head := fmt.Sprintf("%d changed product code lines", b.Lines)
	if len(b.NewFiles) > 0 {
		head += fmt.Sprintf(", %d new product file(s)", len(b.NewFiles))
	}
	return stopHookOutput{Decision: "block", Reason: fmt.Sprintf("%s; %s verification round. %s %s", head, tier, status, reason)}
}
