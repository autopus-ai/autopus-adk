package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/privatedir"
	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// verifyStopMaxBlocks caps how often one session is sent back to work, so a
// failure the agent cannot fix ends the session with a warning instead of a loop.
const verifyStopMaxBlocks = 3

// stopHookInput is the part of the Claude Code Stop hook payload the gate reads.
type stopHookInput struct {
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	LastAssistantMessage string `json:"last_assistant_message"`
	// BackgroundTasks lists tasks still in flight when the session stops.
	BackgroundTasks []struct {
		Type string `json:"type"`
	} `json:"background_tasks"`
}

// treeWriters are the background task types that may still be editing the
// work tree: verifying it now would judge a change that is not finished. A
// long-running shell or monitor (a dev server, a log tail) is not one, or it
// would turn the gate off for the whole session.
var treeWriters = []string{"subagent", "workflow", "teammate"}

func backgroundWriters(in stopHookInput) int {
	n := 0
	for _, task := range in.BackgroundTasks {
		if slices.Contains(treeWriters, task.Type) {
			n++
		}
	}
	return n
}

// stopHookOutput is the Stop hook decision. An empty object lets the session end.
type stopHookOutput struct {
	Decision      string `json:"decision,omitempty"`
	Reason        string `json:"reason,omitempty"`
	SystemMessage string `json:"systemMessage,omitempty"`
}

func newVerifyStopCmd() *cobra.Command {
	var (
		budget   time.Duration
		followup bool
		last     bool
	)
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop hook: build and test the changed files before the session ends",
		Long: "Reads the Claude Code Stop hook payload on stdin. When the work tree has changes, runs the " +
			"build and tests that cover them and blocks the stop with the failure output. Go runs the tests the change " +
			"edited first (with at most half the budget), then the changed packages whole; a suite that outlasts the " +
			"budget with tests finishing and nothing failed does not block, and that tree is not run again (its note is repeated); " +
			"a test still running after half of its step's window is treated as hung and blocks. Failed Go tests are run again " +
			"on HEAD (at most 50; a whole-package step with a failure ends at two thirds of its window to leave time for it): " +
			"those that fail there too, and that the change did not edit, are reported but do not block; when HEAD cannot " +
			"be checked the failure blocks and the note says why. HEAD's answer for each failed test is cached per HEAD commit and " +
			"Go environment (a \"fails on HEAD\" answer only for the session that asked, for an hour), and a step whose failures a " +
			"recent HEAD shared is not stopped early. A tree (HEAD, every git status entry with its mode and contents, the Go " +
			"environment and the budget; ignored files are not compared) is blocked at most once, except a test-first block, which " +
			"stands until a TDD-exempt line or the block limit: a later stop on the same tree reuses the last result without " +
			"running the tests (a pass or an unverified result only within an hour). --last prints that result. An interrupted run " +
			"(Ctrl-C, SIGTERM) records nothing and does not block. Under methodology: tdd with " +
			"enforce: true, the changed tests must also fail against the code before the change (HEAD); a source change " +
			"without a test change is blocked unless the reply has a `TDD-exempt: <reason>` line. Set " +
			"AUTOPUS_VERIFY_STOP=off to disable. Before the build, added lines that look like credentials block the stop " +
			"once, naming file:line and rule but never the value (AUTOPUS_SECRET_STOP=off or hooks.secret_stop: false " +
			"turns that off). Dependency, toolchain and image pins this turn adds or bumps that are a full release line behind " +
			"the latest stable block the stop once, unless the reply has a `version-pin: <name> <reason>` line " +
			"(AUTOPUS_VERSION_STOP=off or hooks.version_stop: false turns that off). Once the build passes (or when no build toolchain covers the changed files), a change that alters product code " +
			"in any language the file-size limit counts is sent on to verification, once per session " +
			"(AUTOPUS_FOLLOWUP=off turns it off). Tests, comments, docs, configuration and generated files alone get no round. " +
			"Fewer than 40 changed product code lines get the light round: one foreground reviewer that checks by executing. " +
			"40 or more lines or a new product file get the full round: the same review, plus auto qa intent, auto qa go --no-loop, " +
			"auto qa exec --all and the qa-explorer hunt when it has a user-runnable, security or data-loss surface or the user asks.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if last {
				dir, _ := os.Getwd()
				return printLastRun(cmd, cmd.OutOrStdout(), dir)
			}
			out := runVerifyStop(cmd, cmd.InOrStdin(), budget, followup)
			data, err := json.Marshal(out)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().DurationVar(&budget, "budget", 8*time.Minute, "Total time for all verification commands")
	cmd.Flags().BoolVar(&last, "last", false, "Print the gate's last result for this repository (no tests run)")
	cmd.Flags().BoolVar(&followup, "followup", false, "Kept for older settings; the verification round is on by default (AUTOPUS_FOLLOWUP=off turns it off)")
	return cmd
}

func runVerifyStop(cmd *cobra.Command, stdin io.Reader, budget time.Duration, followup bool) stopHookOutput {
	var in stopHookInput
	_ = json.NewDecoder(stdin).Decode(&in)
	dir := in.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}
	// The automatic disk reclaim rides the per-turn Stop hook: rate limited
	// and detached, it never adds a build or a walk to the turn.
	maybeStartAutoReclaim(dir)
	if strings.EqualFold(os.Getenv("AUTOPUS_VERIFY_STOP"), "off") {
		return stopHookOutput{}
	}
	root, changed, ok := verifygate.ChangedFiles(cmd.Context(), dir)
	if !ok || len(changed) == 0 {
		return stopHookOutput{}
	}
	block, warning := secretGate(cmd.Context(), in.SessionID, root)
	if block != nil {
		return *block
	}
	versionBlock, versionWarn := versionGate(cmd.Context(), in, root)
	if versionBlock != nil {
		return withSecretWarning(*versionBlock, warning)
	}
	warning = joinWarnings(warning, versionWarn)
	budget = stopBudget(cmd, root, budget)
	return withSecretWarning(verifyChanged(cmd, in, root, changed, budget, followup), warning)
}

// verifyChanged builds and tests the changed files, then applies the
// test-first and follow-up rounds.
func verifyChanged(cmd *cobra.Command, in stopHookInput, root string, changed []string, budget time.Duration, followup bool) stopHookOutput {
	steps := verifygate.Plan(root, changed)
	if n := backgroundWriters(in); n > 0 {
		if len(steps) == 0 {
			return stopHookOutput{}
		}
		return stopHookOutput{SystemMessage: fmt.Sprintf(
			"auto verify stop: %d background agent task(s) may still be changing the work tree; verification runs at the next stop", n)}
	}
	if len(steps) == 0 {
		// Nothing to build, but a changed shell script or Java class is
		// still behavior the user will run.
		return followupRound(cmd, in, root, followup, statusUnbuilt)
	}
	fingerprint := verifygate.TreeHash(cmd.Context(), root, budget.String())
	if readStopState(in.SessionID, "pass") == fingerprint {
		return stopHookOutput{}
	}
	if note, ok := recordedNote(in.SessionID, "budget", fingerprint); ok {
		// Not run again, but not passed either: the note is repeated.
		return stopHookOutput{SystemMessage: note}
	}
	if note, ok := recordedNote(in.SessionID, "baseline", fingerprint); ok {
		// Passed except for failures HEAD has too: not run again, not passed.
		return stopHookOutput{SystemMessage: note}
	}
	if note, ok := recordedNote(in.SessionID, "hang", fingerprint); ok {
		// The stop that lets a hang-suspected tree through: not run again,
		// not passed, and the verification round the block withheld is due
		// now (once per session, like every round).
		return withBudgetNote(followupRound(cmd, in, root, followup, statusHangSuspected), note)
	}
	if out, ok := reuseLastRun(cmd, in, root, changed, fingerprint, budget, followup); ok {
		return out
	}
	started := time.Now()
	verifygate.BaselineCacheDir, verifygate.BaselineCacheSession = stopStateDir, in.SessionID
	// Go test failures that HEAD has too are not charged to the change; the
	// note says which, or why they could not be checked.
	run := verifygate.RunCharged(cmd.Context(), root, changed, steps, budget)
	out := judgeRun(cmd, in, root, changed, fingerprint, run, started, budget, followup)
	if note := run.Note(); note != "" {
		out = withBudgetNote(out, note)
	}
	// A tree edited while it ran is not the tree that was judged.
	same := verifygate.TreeHash(cmd.Context(), root, budget.String()) == fingerprint
	return recordLastRun(in.SessionID, root, fingerprint, budget, run, out, same)
}

// judgeRun turns the outcome of the run into the hook's output.
func judgeRun(cmd *cobra.Command, in stopHookInput, root string, changed []string, fingerprint string,
	run verifygate.ChargedRun, started time.Time, budget time.Duration, followup bool) stopHookOutput {
	failure, skipped := run.Failure, run.Skipped
	if failure == nil && slices.ContainsFunc(skipped, func(s verifygate.Skip) bool { return s.Interrupted }) {
		// A signal is not a verdict: no block, no block count, no record.
		return stopHookOutput{SystemMessage: "auto verify stop: interrupted before verification finished; " +
			"nothing was recorded, so the next stop checks this tree again"}
	}
	if failure == nil && len(skipped) > 0 && onlyBudgetSkips(skipped) {
		return budgetSkipped(cmd, in, root, changed, fingerprint, skipped, run.Preexisting, budget-time.Since(started), followup)
	}
	if failure == nil && len(skipped) > 0 {
		// Nothing failed, but not everything ran: that is the machine, not
		// the change, so the stop is not blocked. The tree is not recorded as
		// passing, so it is checked again once the environment is fixed.
		return stopHookOutput{SystemMessage: "auto verify stop: could not run " + describeSkips(skipped) +
			"; this is an environment problem, not a verdict on the change"}
	}
	if failure == nil && len(run.Preexisting) > 0 {
		return baselinePassed(cmd, in, root, changed, fingerprint, run, budget-time.Since(started), followup)
	}
	if failure == nil {
		if out := redGate(cmd.Context(), in, root, changed, budget-time.Since(started)); out != nil {
			return *out
		}
		resetStopBlocks(in.SessionID)
		writeStopState(in.SessionID, "pass", fingerprint)
		return followupRound(cmd, in, root, followup, statusPassed)
	}
	summary := fmt.Sprintf("Verification failed: `%s` (%s).\n%s", shortCommand(failure.Step.Argv), failure.Reason, failure.Output)
	if failure.Hung {
		return suspectedHang(in.SessionID, fingerprint, summary, failure.Window, budget)
	}
	if countStopBlock(in.SessionID) > verifyStopMaxBlocks {
		return stopHookOutput{SystemMessage: fmt.Sprintf("auto verify stop: still failing after %d attempts; ending without a passing build. %s",
			verifyStopMaxBlocks, firstLine(summary))}
	}
	return stopHookOutput{Decision: "block", Reason: summary + "\n\nFix the failure, or explain why it is not caused by this change, before finishing."}
}

// suspectedHang blocks once per tree on a suspected hang. No threshold over
// the events seen at kill time tells a slow test from a stuck one, so the
// agent is told once; the tree is recorded as unverified (kind "hang"), and
// the next stop on it repeats the note without blocking or running again.
// Any edit changes the fingerprint and re-arms the block.
func suspectedHang(session, fingerprint, summary string, window, budget time.Duration) stopHookOutput {
	note := "auto verify stop: a suspected hang was reported for this tree and it was not run again; it is not verified. " +
		firstLine(summary)
	writeStopState(session, "hang", fingerprint+"\n"+note)
	hint := fmt.Sprintf("\n\nThe step had %s of the %s budget; when the slow test is one this change edited, the changed "+
		"tests run first and the final step gets only about half the budget. If this is a legitimately slow test, stopping "+
		"again will not block this tree again (it stays unverified). To give it more time, set `hooks.verify_stop_budget` in "+
		"autopus.yaml (for example \"15m\", at most %s) and run `auto update`; Claude Code reads hooks when a session starts, "+
		"so the new budget applies from the next session on.", window, budget, config.MaxVerifyStopBudget)
	return stopHookOutput{Decision: "block", Reason: summary + hint +
		"\n\nIf it is stuck, fix it, or explain why it is not caused by this change, before finishing."}
}

var unsafeSessionChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// stopStateDir holds per-session gate state in a private per-user
// directory: the block count, and the fingerprint of the last tree that
// passed.
var stopStateDir = privatedir.Path("verify-stop")

// stopStateName is the file name of one kind of state for a session.
func stopStateName(session, kind string) string {
	if session == "" {
		session = "unknown"
	}
	return unsafeSessionChars.ReplaceAllString(session, "_") + "." + kind
}

func stopStateFile(session, kind string) string {
	return filepath.Join(stopStateDir, stopStateName(session, kind))
}

func readStopState(session, kind string) string {
	data, _ := privatedir.ReadFile(stopStateDir, stopStateName(session, kind))
	return strings.TrimSpace(string(data))
}

func writeStopState(session, kind, value string) {
	_ = saveStopState(session, kind, value)
}

// saveStopState is writeStopState for a caller that must know the write
// landed, such as a round that would otherwise repeat on every stop.
func saveStopState(session, kind, value string) error {
	return privatedir.WriteFile(stopStateDir, stopStateName(session, kind), []byte(value))
}

// countStopBlock records one more block for the session and returns the total.
func countStopBlock(session string) int {
	n, _ := strconv.Atoi(readStopState(session, "blocks"))
	n++
	writeStopState(session, "blocks", strconv.Itoa(n))
	return n
}

func resetStopBlocks(session string) { _ = os.Remove(stopStateFile(session, "blocks")) }

func resetStopState(session string) {
	resetStopBlocks(session)
	_ = os.Remove(stopStateFile(session, "pass"))
	_ = os.Remove(stopStateFile(session, "secrets"))
	_ = os.Remove(stopStateFile(session, "versions"))
	_ = os.Remove(stopStateFile(session, "versions-warn"))
	_ = os.Remove(stopStateFile(session, "budget"))
	_ = os.Remove(stopStateFile(session, "hang"))
	_ = os.Remove(stopStateFile(session, "baseline"))
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// shortCommand renders argv for a message, eliding a long middle such as a
// -run pattern of many test names.
func shortCommand(argv []string) string {
	const max = 300
	line := strings.Join(argv, " ")
	if len(line) <= max {
		return line
	}
	return strings.ToValidUTF8(line[:max/2], "") + " … " + strings.ToValidUTF8(line[len(line)-max/2:], "")
}
