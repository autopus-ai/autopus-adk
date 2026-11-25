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
// round, the review alone: a one-line logic edit can break a written policy
// as surely as a large one, so size decides how deep verification goes,
// never whether it happens.
const followupFullLines = 40

// A session gets one round, with one exception: a session whose light round
// came early (an interactive session that grows a feature over several
// turns) is escalated once to the full round when its change grows by
// another followupFullLines product code lines or gains a product file.
// Pilot C (2026-11-07) measured a repeated round on the same change costing
// about $10 without fixing anything the first round had not, so fixes made
// after a round never start another one on their own.

type followupTier string

const (
	followupNone  followupTier = ""
	followupLight followupTier = "light"
	followupFull  followupTier = "full"
)

// followupTierFor picks the round for a passing change. Tests, comments,
// documentation, configuration and generated files get none; new product
// files or a large change get the full round, whose scenario QA and hunt
// still run only for a user-runnable, security or data-loss change.
func followupTierFor(b verifygate.Behavior) followupTier {
	switch {
	case b.None():
		return followupNone
	case b.Unmeasured || len(b.NewFiles) > 0 || b.Lines >= followupFullLines:
		return followupFull
	}
	return followupLight
}

// followupChecklist closes the harness lesson loop: the project's approved
// conditional checks run on the change, and a defect the earlier checks
// passed becomes a lesson without the user writing it down.
const followupChecklist = `

Also run ` + "`auto checklist run`" + ` (skip it when the project has no .autopus/checklist): an enforce check that fails is a blocker, and each "리뷰 기준" line is a criterion for the reviewer. When this round or the user found a defect that the earlier checks (tests, review, a SPEC) had passed, record it with ` + "`auto checklist record --pattern <defect> --missed-by <the check that passed> --when-path <glob> [--when-keyword <added text>]`" + ` plus ` + "`--evidence-command <repro> --evidence-before-exit <n> --evidence-after-exit 0`" + ` when a repro script failed before the fix and passes after it, then run ` + "`auto checklist propose`" + ` and ` + "`auto checklist refute --all`" + `. Approving a candidate is the user's decision; do not run approve, promote or retire yourself.`

const followupAsk = `

If a fix needs a decision only the user can make (product behavior, scope, a policy conflict), ask it with AskUserQuestion as a multiple-choice question: say what you found, give each option a description of what it changes and what it costs, and put your recommendation first. Do not ask an open question. Otherwise do not ask; proceed and state your assumption.`

// followupReview is the step every round starts with. Pilot C measured the
// independent review as the only step that changed results, and found most
// of its misses in code that models an external program; the reviewer found
// them only by running that program. Findings were lost when the agent ended
// its turn while the reviewer still ran, so the review runs in the foreground.
const followupReview = `1. Run the reviewer agent on ` + "`git diff HEAD`" + ` (and new files) against what the user asked for, in the foreground: wait for its result in this turn, and do not end the turn while it runs. Require differential checking: where the changed code models or parses an external program or format (shell grammar, CLI flags, git, file formats, protocols), the reviewer runs the real program on the same inputs and compares it with the new code. It reports only blockers reproduced by execution (wrong behavior, missing cases, security or data-loss risk), each with file, line and the command that shows it.` + followupCrossReview

// followupCrossReview adds a second review by a model from another vendor,
// with the same brief, after the reviewer agent returns. It runs only where
// the codex CLI is installed; elsewhere the step is skipped without a note.
// The review writes its answer outside the checkout so it never shows up in
// the diff, and it is bounded because a hung review would hold the turn.
const followupCrossReview = ` After the reviewer returns, if ` + "`command -v codex`" + ` finds the Codex CLI, get one more review from that other model with the same brief: run ` + "`codex exec --skip-git-repo-check -s danger-full-access -o \"${TMPDIR:-/tmp}/codex-review.md\" \"<brief>\" </dev/null 2>&1`" + ` in the foreground, where <brief> is the user's request plus the review instruction above (review ` + "`git diff HEAD`" + ` and new files, the differential check, only reproduced blockers with file, line and command) and says not to modify any file. Bound it to 15 minutes (prefix ` + "`timeout 900`" + ` where it exists, otherwise ` + "`perl -e 'alarm 900; exec @ARGV'`" + `) and give the shell call a long enough timeout, or run it in the background and wait for it in this turn; do not end the turn while it runs. If codex is not installed, skip this without mentioning it. Merge the two lists: a finding both report counts once, and a finding only one reports still counts. Confirm each finding by rerunning its command before applying it.`

const followupFix = ` Apply every confirmed finding, rerun the affected tests, and finish with a short summary of what was checked and fixed. The fixes do not get another review.`

// followupFullReason is the round for a large change: the review, and
// scenario QA plus the hunt only when the change has something a user runs.
const followupFullReason = `This is a large change. Before finishing, verify it without asking the user to start it:

` + followupReview + `
2. Only if the change touches something a user can run (a command, UI or API), a security or data-loss path, or the user asked for QA: check it against the user's request too. Use the SPEC id that covers it, or save the user's original request verbatim, run ` + "`auto qa intent --request-file <file> --format json`" + `, ask its questions, and record the answers with ` + "`auto qa intent answer`" + `. The change is uncommitted, so run ` + "`auto qa go <id> --auto --no-loop`" + ` and then ` + "`auto qa exec --all`" + ` (plus ` + "`auto qa run --lane <lane>`" + ` when GUI scenarios were compiled) on the working tree; the loop without --no-loop refuses uncommitted changes. Then run the qa-explorer agent in hunt mode on the changed behavior and ` + "`auto qa hunt verify`" + ` (bugs, policy_mismatch with a quoted policy, measured inefficiency). Otherwise skip this step.
3.` + followupFix + followupChecklist + followupAsk

// followupLightReason is the round for a small behavior change: the review
// alone.
const followupLightReason = `This is a small behavior change; before finishing, review it without asking the user to start it:

` + followupReview + `
2.` + followupFix + followupChecklist + followupAsk

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
// deep. Fixing a round's findings does not start another; a tree another
// session already sent round is not sent again, so a fresh session that only
// asks a question is not blocked by someone else's uncommitted work. When
// the round cannot be recorded the hook fails open: an unrecorded round
// would block every stop forever.
func nextFollowup(session, root, fingerprint string, b verifygate.Behavior) followupTier {
	tier := followupTierFor(b)
	if tier == followupNone {
		return followupNone
	}
	prev := readSessionRound(session)
	switch {
	case prev.tier == followupFull:
		return followupNone
	case prev.tier == followupLight && !prev.escalates(b):
		return followupNone
	case fingerprint != "" && readStopState(followupTreeKey(root), "followup") == fingerprint:
		return followupNone
	}
	if prev.tier == followupLight {
		tier = followupFull
	}
	round := sessionRound{tier: tier, lines: b.Lines, newFiles: len(b.NewFiles)}
	if err := saveStopState(session, "followup", round.String()); err != nil {
		return followupNone
	}
	_ = saveStopState(followupTreeKey(root), "followup", fingerprint)
	return tier
}

// sessionRound is the round a session last ran and the size it was run on.
type sessionRound struct {
	tier     followupTier
	lines    int
	newFiles int
}

func (r sessionRound) String() string {
	return fmt.Sprintf("%s %d %d", r.tier, r.lines, r.newFiles)
}

// escalates reports whether a change that already had the light round has
// grown enough since to need the full one: another followupFullLines code
// lines, or a product file it did not have.
func (r sessionRound) escalates(b verifygate.Behavior) bool {
	return followupTierFor(b) == followupFull &&
		(b.Unmeasured || b.Lines >= r.lines+followupFullLines || len(b.NewFiles) > r.newFiles)
}

// readSessionRound reads the session's last round. A record from the
// one-round format ("1 light") or one it cannot read counts as a full round
// already run, so an upgrade never sends a session round twice.
func readSessionRound(session string) sessionRound {
	f := strings.Fields(readStopState(session, "followup"))
	if len(f) == 0 {
		return sessionRound{}
	}
	if len(f) == 3 && followupTier(f[0]) == followupLight {
		r := sessionRound{tier: followupLight}
		_, errL := fmt.Sscanf(f[1], "%d", &r.lines)
		_, errN := fmt.Sscanf(f[2], "%d", &r.newFiles)
		if errL == nil && errN == nil {
			return r
		}
	}
	return sessionRound{tier: followupFull}
}

// followupTreeKey names the per-repository record of the last tree a round
// was run on.
func followupTreeKey(root string) string {
	sum := sha256.Sum256([]byte(root))
	return "tree-" + hex.EncodeToString(sum[:8])
}

// recordSessionTree marks the tree a session stops on after its round as
// that session's: its fixes, and work it added after the round, are not sent
// round again to the next session. The next session that changes product
// code gets a round over the whole uncommitted diff, which includes them.
func recordSessionTree(root string, paths []string) {
	_ = saveStopState(followupTreeKey(root), "followup", verifygate.Fingerprint(root, paths))
}

// followupRound runs after a passing build, or after a change no build
// toolchain covers (shell, Java, Swift ...), whose behavior still needs the
// user's eyes even though nothing could be compiled. status is the sentence
// the round tells the agent about what the gate ran: statusPassed,
// statusUnbuilt, or what did not finish within the budget.
func followupRound(cmd *cobra.Command, in stopHookInput, root string, flag bool, status string) stopHookOutput {
	if !followupEnabled(flag) {
		return stopHookOutput{}
	}
	prev := readSessionRound(in.SessionID)
	if prev.tier == followupFull {
		recordSessionTree(root, verifygate.ProductPaths(cmd.Context(), root))
		return stopHookOutput{}
	}
	b := verifygate.BehaviorChange(cmd.Context(), root)
	if tier := nextFollowup(in.SessionID, root, verifygate.Fingerprint(root, b.Paths), b); tier != followupNone {
		return followupOutput(b, tier, status, prev.tier == followupLight)
	}
	if prev.tier != followupNone {
		recordSessionTree(root, b.Paths)
	}
	return stopHookOutput{}
}

// The status sentences of a verification round.
const (
	statusPassed  = "The build and tests pass."
	statusUnbuilt = "No build or test toolchain covers these files, so nothing was compiled or tested."
	// statusHangSuspected is the round after a suspected hang was let through.
	statusHangSuspected = "The build passes; the tests were suspected hung when the budget ran out and are not verified: " +
		"run them in full (a longer -timeout, or CI) before calling the change verified."
)

func followupOutput(b verifygate.Behavior, tier followupTier, status string, escalated bool) stopHookOutput {
	reason := followupLightReason
	if tier == followupFull {
		reason = followupFullReason
	}
	head := fmt.Sprintf("%d changed product code lines", b.Lines)
	if len(b.NewFiles) > 0 {
		head += fmt.Sprintf(", %d new product file(s)", len(b.NewFiles))
	}
	if escalated {
		head += " (grown since this session's light round)"
	}
	return stopHookOutput{Decision: "block", Reason: fmt.Sprintf("%s; %s verification round. %s %s", head, tier, status, reason)}
}
