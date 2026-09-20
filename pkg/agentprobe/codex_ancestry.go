package agentprobe

import (
	"context"
	"encoding/json"
	"fmt"
)

// Only a completed native spawn from a proven ancestor can extend ownership.
// Arbitrary loaded session IDs and ordinary collaboration receivers cannot.
func captureCodexOwnedSpawn(c *codexCapture, m codexRPCMessage) (bool, error) {
	_, _, kind, err := codexItemEnvelope(m)
	if err != nil {
		return false, err
	}
	if kind != "collabAgentToolCall" {
		return false, nil
	}
	if m.Method != "item/completed" {
		return false, nil
	}
	var params struct {
		ThreadID string `json:"threadId"`
		Item     struct {
			Type      string   `json:"type"`
			Tool      string   `json:"tool"`
			Status    string   `json:"status"`
			Sender    string   `json:"senderThreadId"`
			Receivers []string `json:"receiverThreadIds"`
		} `json:"item"`
	}
	if json.Unmarshal(m.Params, &params) != nil {
		return false, fmt.Errorf("invalid Codex ancestry event")
	}
	item := params.Item
	if item.Type != "collabAgentToolCall" || item.Tool != "spawnAgent" || item.Status != "completed" || params.ThreadID != item.Sender {
		return false, nil
	}
	if item.Sender != c.evidence.SupervisorID && !c.owned[item.Sender] {
		return false, nil
	}
	if len(item.Receivers) != 1 || !token(item.Receivers[0]) || item.Receivers[0] == c.evidence.SupervisorID || item.Receivers[0] == item.Sender {
		return false, fmt.Errorf("invalid native descendant identity")
	}
	child := item.Receivers[0]
	if c.owned[child] {
		return false, nil
	}
	if len(c.owned) >= MaxEvents {
		return false, fmt.Errorf("Codex native descendants exceed limit")
	}
	c.owned[child] = true
	return true, nil
}

func reconcileCodexAncestry(ctx context.Context, rpc codexRPC, c *codexCapture) (bool, error) {
	added := false
	for _, id := range codexOwnedIDs(c) {
		raw, err := rpc.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": true})
		if err != nil {
			return false, err
		}
		var response struct {
			Thread struct {
				ID    string `json:"id"`
				Turns []struct {
					Items []json.RawMessage `json:"items"`
				} `json:"turns"`
			} `json:"thread"`
		}
		if json.Unmarshal(raw, &response) != nil || response.Thread.ID != id || response.Thread.Turns == nil {
			return false, fmt.Errorf("Codex owned ancestry history unverified")
		}
		for _, turn := range response.Thread.Turns {
			// Actual turns expose their items. A missing field would make the native
			// spawn inventory incomplete; an empty list is a valid complete inventory.
			if turn.Items == nil {
				return false, fmt.Errorf("Codex owned turn item inventory missing")
			}
			for _, item := range turn.Items {
				params, _ := json.Marshal(map[string]any{"threadId": id, "item": item})
				message := codexRPCMessage{Method: "item/completed", Params: params}
				if _, err := captureCodexActivity(c, message); err != nil {
					return false, err
				}
				activityAdded, err := reconcileCodexActivityCandidates(ctx, rpc, c)
				if err != nil {
					return false, err
				}
				added = added || activityAdded
				fresh, err := captureCodexOwnedSpawn(c, message)
				if err != nil {
					return false, err
				}
				added = added || fresh
			}
		}
	}
	return added, nil
}
