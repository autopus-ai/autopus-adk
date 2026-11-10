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
	)
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop hook: build and test the changed files before the session ends",
		Long: "Reads the Claude Code Stop hook payload on stdin. When the work tree has changes, runs the " +
			"build and tests that cover them and blocks the stop with the failure output. Under methodology: tdd with " +
			"enforce: true, the changed tests must also fail against the code before the change (HEAD); a source change " +
			"without a test change is blocked unless the reply has a `TDD-exempt: <reason>` line. Set " +
			"AUTOPUS_VERIFY_STOP=off to disable. Before the build, added lines that look like credentials block the stop " +
			"once, naming file:line and rule but never the value (AUTOPUS_SECRET_STOP=off or hooks.secret_stop: false " +
			"turns that off). Opt-in with AUTOPUS_FOLLOWUP=on or --followup: once the build passes, " +
			"a change of 40 or more lines is sent on to the reviewer and qa-explorer agents, once per session.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
	cmd.Flags().BoolVar(&followup, "followup", false, "After a passing build, send a large enough change on to review and QA hunt, once per session (also AUTOPUS_FOLLOWUP=on)")
	return cmd
}

func runVerifyStop(cmd *cobra.Command, stdin io.Reader, budget time.Duration, followup bool) stopHookOutput {
	if strings.EqualFold(os.Getenv("AUTOPUS_VERIFY_STOP"), "off") {
		return stopHookOutput{}
	}
	var in stopHookInput
	_ = json.NewDecoder(stdin).Decode(&in)
	dir := in.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}
	root, changed, ok := verifygate.ChangedFiles(cmd.Context(), dir)
	if !ok || len(changed) == 0 {
		return stopHookOutput{}
	}
	block, warning := secretGate(cmd.Context(), in.SessionID, root)
	if block != nil {
		return *block
	}
	return withSecretWarning(verifyChanged(cmd, in, root, changed, budget, followup), warning)
}

// verifyChanged builds and tests the changed files, then applies the
// test-first and follow-up rounds.
func verifyChanged(cmd *cobra.Command, in stopHookInput, root string, changed []string, budget time.Duration, followup bool) stopHookOutput {
	steps := verifygate.Plan(root, changed)
	if len(steps) == 0 {
		return stopHookOutput{}
	}
	if n := backgroundWriters(in); n > 0 {
		return stopHookOutput{SystemMessage: fmt.Sprintf(
			"auto verify stop: %d background agent task(s) may still be changing the work tree; verification runs at the next stop", n)}
	}
	fingerprint := verifygate.Fingerprint(root, changed)
	if readStopState(in.SessionID, "pass") == fingerprint {
		return stopHookOutput{}
	}
	started := time.Now()
	failure, skipped := verifygate.Run(cmd.Context(), steps, budget)
	if failure == nil && len(skipped) > 0 {
		// Nothing failed, but not everything ran: that is the machine, not
		// the change, so the stop is not blocked. The tree is not recorded as
		// passing, so it is checked again once the environment is fixed.
		return stopHookOutput{SystemMessage: "auto verify stop: could not run " + describeSkips(skipped) +
			"; this is an environment problem, not a verdict on the change"}
	}
	if failure == nil {
		if out := redGate(cmd.Context(), in, root, changed, budget-time.Since(started)); out != nil {
			return *out
		}
		resetStopBlocks(in.SessionID)
		writeStopState(in.SessionID, "pass", fingerprint)
		if followupEnabled(followup) {
			if lines := verifygate.ChangedLines(cmd.Context(), root, changed); nextFollowup(in.SessionID, lines) {
				return followupOutput(lines)
			}
		}
		return stopHookOutput{}
	}
	summary := fmt.Sprintf("Verification failed: `%s` (%s).\n%s", strings.Join(failure.Step.Argv, " "), failure.Reason, failure.Output)
	if countStopBlock(in.SessionID) > verifyStopMaxBlocks {
		return stopHookOutput{SystemMessage: fmt.Sprintf("auto verify stop: still failing after %d attempts; ending without a passing build. %s",
			verifyStopMaxBlocks, firstLine(summary))}
	}
	return stopHookOutput{Decision: "block", Reason: summary + "\n\nFix the failure, or explain why it is not caused by this change, before finishing."}
}

var unsafeSessionChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// stopStateFile is per-session gate state: the block count, and the
// fingerprint of the last tree that passed.
func stopStateFile(session, kind string) string {
	if session == "" {
		session = "unknown"
	}
	return filepath.Join(os.TempDir(), "autopus-verify-stop", unsafeSessionChars.ReplaceAllString(session, "_")+"."+kind)
}

func readStopState(session, kind string) string {
	data, _ := os.ReadFile(stopStateFile(session, kind))
	return strings.TrimSpace(string(data))
}

func writeStopState(session, kind, value string) {
	path := stopStateFile(session, kind)
	if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		_ = os.WriteFile(path, []byte(value), 0o600)
	}
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
}

func describeSkips(skipped []verifygate.Skip) string {
	parts := make([]string, 0, len(skipped))
	for _, s := range skipped {
		parts = append(parts, fmt.Sprintf("`%s` (%s)", s.Name, s.Reason))
	}
	return strings.Join(parts, ", ")
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
