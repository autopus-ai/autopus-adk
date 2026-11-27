package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/privatedir"
)

// subagentMarkerAge is how long a sub-agent marker counts: a sub-agent
// whose SubagentStop never came (a crash, a killed session) must not defer
// the gate for good.
const subagentMarkerAge = 2 * time.Hour

// subagentHookInput is the part of Codex's SubagentStart/SubagentStop
// payload the gate reads: the parent's session and the sub-agent's id.
type subagentHookInput struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}

func subagentKind(agent string) string {
	if agent == "" {
		agent = "unknown"
	}
	return "agent-" + unsafeSessionChars.ReplaceAllString(agent, "_")
}

// recordSubagent marks a Codex sub-agent as running (start) or done
// (stop) for its parent session, and prints the empty JSON Codex expects.
func recordSubagent(stdin io.Reader, stdout io.Writer, event string) error {
	var in subagentHookInput
	_ = json.NewDecoder(stdin).Decode(&in)
	switch event {
	case "start":
		writeStopState(in.SessionID, subagentKind(in.AgentID), time.Now().UTC().Format(time.RFC3339))
	case "stop":
		_ = os.Remove(stopStateFile(in.SessionID, subagentKind(in.AgentID)))
	default:
		return fmt.Errorf("--subagent takes start or stop, not %q", event)
	}
	_, err := fmt.Fprintln(stdout, "{}")
	return err
}

// runningSubagents counts the session's sub-agents still running, removing
// markers too old to be live.
func runningSubagents(session string) int {
	if session == "" || privatedir.Ensure(stopStateDir) != nil {
		return 0
	}
	prefix := stopStateName(session, "agent-")
	entries, err := os.ReadDir(stopStateDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > subagentMarkerAge {
			if err := os.Remove(filepath.Join(stopStateDir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				continue
			}
			continue
		}
		n++
	}
	return n
}
