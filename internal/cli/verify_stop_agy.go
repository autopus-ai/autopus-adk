package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	// LastUserInput is empty in a sub-agent's conversation: agy sends a
	// Stop for every invoke_subagent run, with fullyIdle covering only that
	// sub-agent's own work.
	// A pointer: only a field that is there and empty marks a sub-agent, so
	// a later agy that drops the field does not turn the gate off.
	LastUserInput  *string `json:"lastUserInput"`
	TranscriptPath string  `json:"transcriptPath"`
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
// step limit) and a sub-agent's stop (the parent stops again once its
// sub-agents are done), and maps a block to "continue". The final reply,
// for the TDD-exempt and version-pin lines, comes from agy's transcript; no
// transcript is handed on to the harness feedback loop, which reads Claude
// Code's. agy's reply has no field for a note, so a gate note that does not
// block (unverified, given up after three blocks) is not shown there.
func runVerifyStopAgy(cmd *cobra.Command, budget time.Duration, followup bool) error {
	var in agyStopInput
	_ = json.NewDecoder(cmd.InOrStdin()).Decode(&in)
	out := agyStopOutput{Decision: "stop"}
	if (in.FullyIdle == nil || *in.FullyIdle) && in.Error == "" && agyFinished(in.TerminationReason) &&
		(in.LastUserInput == nil || *in.LastUserInput != "") {
		mapped := stopHookInput{SessionID: in.ConversationID, Cwd: in.Cwd,
			LastAssistantMessage: agyLastReply(in.TranscriptPath)}
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

// agyTranscriptTail bounds the read of agy's transcript for the final reply.
const agyTranscriptTail = 512 << 10

// agyLastReply is the model's last reply of the current turn in agy's
// transcript_full.jsonl: the last PLANNER_RESPONSE after the last
// USER_INPUT, or "" when there is none or the file cannot be read.
func agyLastReply(path string) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	offset := max(info.Size()-agyTranscriptTail, 0)
	data, err := io.ReadAll(io.NewSectionReader(f, offset, info.Size()-offset))
	if err != nil {
		return ""
	}
	lines := bytes.Split(data, []byte("\n"))
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:] // cut by the offset
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var step struct {
			Type    string `json:"type"`
			Source  string `json:"source"`
			Content string `json:"content"`
		}
		if json.Unmarshal(lines[i], &step) != nil {
			continue
		}
		switch {
		case step.Type == "USER_INPUT":
			return ""
		case step.Type == "PLANNER_RESPONSE" && step.Source == "MODEL" && step.Content != "":
			return step.Content
		}
	}
	return ""
}
