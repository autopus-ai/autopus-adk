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
		// The file holds one line per stop the marker deferred.
		writeStopState(in.SessionID, subagentKind(in.AgentID), "")
	case "stop":
		_ = os.Remove(stopStateFile(in.SessionID, subagentKind(in.AgentID)))
	default:
		return fmt.Errorf("--subagent takes start or stop, not %q", event)
	}
	_, err := fmt.Fprintln(stdout, "{}")
	return err
}

// subagentMaxDefers is how many stops one sub-agent marker may defer. Codex
// sends no SubagentStop for a sub-agent turn that was interrupted or ended
// on an error, so a marker can outlive its sub-agent; past this many stops
// it is taken as gone and the tree is verified.
const subagentMaxDefers = 2

// runningSubagents counts the session's sub-agents still taken as running,
// and charges each one stop it defers. A marker older than
// subagentMarkerAge, or that already deferred subagentMaxDefers stops, is
// removed instead.
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
		path := filepath.Join(stopStateDir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		data, _ := os.ReadFile(path)
		defers := strings.Count(string(data), "\n")
		if time.Since(info.ModTime()) > subagentMarkerAge || defers >= subagentMaxDefers {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				continue
			}
			continue
		}
		_ = privatedir.WriteFile(stopStateDir, e.Name(), append(data, '\n'))
		n++
	}
	return n
}
