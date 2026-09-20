package agentprobe

import "encoding/json"

// Retain responses, host requests, and native lifecycle notifications only.
// Streaming text/reasoning is not lifecycle evidence and must not consume the
// bounded pending lifecycle queue while an RPC read is awaiting its response.
func codexRetainMessage(m codexRPCMessage) (bool, error) {
	if len(m.ID) > 0 {
		return true, nil
	}
	switch m.Method {
	case "thread/started", "thread/archived", "thread/closed", "turn/started", "turn/completed", "error":
		return true, nil
	case "item/started", "item/completed":
		_, _, kind, err := codexItemEnvelope(m)
		if err != nil {
			return false, err
		}
		return kind == "subAgentActivity" || kind == "collabAgentToolCall", nil
	default:
		return false, nil
	}
}

func codexNeedsActivityRefresh(c *codexCapture, m codexRPCMessage) bool {
	var envelope struct {
		ThreadID string `json:"threadId"`
	}
	if json.Unmarshal(m.Params, &envelope) != nil {
		return false
	}
	if envelope.ThreadID != c.evidence.SupervisorID && !c.owned[envelope.ThreadID] && c.activities[envelope.ThreadID] == "" {
		return false
	}
	switch m.Method {
	case "thread/started", "turn/started", "turn/completed":
		return true
	case "item/started", "item/completed":
		_, _, kind, err := codexItemEnvelope(m)
		return err == nil && (kind == "subAgentActivity" || kind == "collabAgentToolCall")
	default:
		return false
	}
}
