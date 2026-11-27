package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// agyStopInput is the Antigravity CLI Stop payload: camelCase, with the
// workspace in workspacePaths and no final reply.
type agyStopInput struct {
	ConversationID    string   `json:"conversationId"`
	Cwd               string   `json:"cwd"`
	WorkspacePaths    []string `json:"workspacePaths"`
	TerminationReason string   `json:"terminationReason"`
	Error             string   `json:"error"`
	// FullyIdle is false while background commands or async work still run.
	FullyIdle *bool `json:"fullyIdle"`
}

// agyStopOutput is the Antigravity Stop reply: "continue" sends the agent
// back with reason as a system message; any other decision lets it stop.
type agyStopOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// agyFinished reports a stop the model chose: agy 1.5 sends NO_TOOL_CALL
// (its docs say "model_stop"). MAX_INVOCATIONS, ERROR and the cancellations
// end a run the agent did not finish, which the gate does not judge.
func agyFinished(reason string) bool {
	switch reason {
	case "", "NO_TOOL_CALL", "model_stop":
		return true
	}
	return false
}

// runVerifyStopAgy runs the gate on an Antigravity Stop payload. It waits
// while work still runs, skips a stop the model did not choose (an error, a
// step limit), and maps a block to "continue". The payload has no final
// reply, so reply markers (TDD-exempt, version-pin) are not read, and no
// transcript is handed on: it is not a Claude Code transcript.
func runVerifyStopAgy(cmd *cobra.Command, budget time.Duration, followup bool) error {
	var in agyStopInput
	_ = json.NewDecoder(cmd.InOrStdin()).Decode(&in)
	out := agyStopOutput{Decision: "stop"}
	if (in.FullyIdle == nil || *in.FullyIdle) && in.Error == "" && agyFinished(in.TerminationReason) {
		mapped := stopHookInput{SessionID: in.ConversationID, Cwd: in.Cwd}
		if mapped.Cwd == "" && len(in.WorkspacePaths) > 0 {
			mapped.Cwd = in.WorkspacePaths[0]
		}
		data, err := json.Marshal(mapped)
		if err != nil {
			return err
		}
		if gate := runVerifyStop(cmd, bytes.NewReader(data), budget, followup); gate.Decision == "block" {
			out = agyStopOutput{Decision: "continue", Reason: gate.Reason}
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return err
}
