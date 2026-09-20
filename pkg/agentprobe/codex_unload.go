package agentprobe

import (
	"context"
	"encoding/json"
	"fmt"
)

// proveCodexUnloaded is the native equivalent of thread/closed: archive was
// acknowledged in this same app-server connection, its loaded inventory no
// longer contains the owned ID, and thread/read independently reports notLoaded
// with no in-progress or unrecognized turn. Process exit is never consulted.
func proveCodexUnloaded(ctx context.Context, rpc codexRPC, c *codexCapture, archived map[string]bool) error {
	loaded := map[string]bool{}
	cursor := ""
	for page := 0; page < 8; page++ {
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := rpc.call(ctx, "thread/loaded/list", params)
		if err != nil {
			return err
		}
		var response struct {
			Data       []string `json:"data"`
			NextCursor *string  `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &response) != nil || response.Data == nil {
			return fmt.Errorf("Codex unload inventory unverified")
		}
		for _, id := range response.Data {
			loaded[id] = true
		}
		if response.NextCursor == nil || *response.NextCursor == "" {
			cursor = ""
			break
		}
		cursor = *response.NextCursor
	}
	if cursor != "" {
		return fmt.Errorf("Codex unload inventory exceeds page limit")
	}
	for _, id := range codexOwnedIDs(c) {
		if c.closed[id] || c.unloaded[id] || !archived[id] || loaded[id] {
			continue
		}
		thread, err := readCodexChild(ctx, rpc, id)
		if err != nil {
			return err
		}
		if thread.Thread.Status.Type != "notLoaded" || thread.Thread.Turns == nil {
			continue
		}
		terminal := true
		for _, turn := range thread.Thread.Turns {
			switch turn.Status {
			case "completed", "failed", "interrupted":
			default:
				terminal = false
			}
		}
		if terminal {
			c.unloaded[id] = true
		}
	}
	return nil
}
