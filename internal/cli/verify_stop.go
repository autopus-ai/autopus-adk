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
			"turns that off). Dependency, toolchain and image pins this turn adds or bumps that are a full release line behind " +
			"the latest stable block the stop once, unless the reply has a `version-pin: <name> <reason>` line " +
			"(AUTOPUS_VERSION_STOP=off or hooks.version_stop: false turns that off). Once the build passes (or when no build toolchain covers the changed files), a change that alters product code " +
			"in any language the file-size limit counts is sent on to verification, once per session " +
			"(AUTOPUS_FOLLOWUP=off turns it off). Tests, comments, docs, configuration and generated files alone get no round. " +
			"Fewer than 40 changed product code lines get the light round (existing scenarios via auto qa exec --all, the reviewer, " +
			"and qa-explorer in verify mode); 40 or more lines or a new product file get the full round (auto qa intent, " +
			"auto qa go --no-loop, auto qa exec --all, the reviewer, and the qa-explorer hunt).",
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
		return followupRound(cmd, in, root, followup, false)
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
		return followupRound(cmd, in, root, followup, true)
	}
	summary := fmt.Sprintf("Verification failed: `%s` (%s).\n%s", strings.Join(failure.Step.Argv, " "), failure.Reason, failure.Output)
	if countStopBlock(in.SessionID) > verifyStopMaxBlocks {
		return stopHookOutput{SystemMessage: fmt.Sprintf("auto verify stop: still failing after %d attempts; ending without a passing build. %s",
			verifyStopMaxBlocks, firstLine(summary))}
	}
	return stopHookOutput{Decision: "block", Reason: summary + "\n\nFix the failure, or explain why it is not caused by this change, before finishing."}
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
