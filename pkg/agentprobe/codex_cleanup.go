package agentprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type codexReadThread struct {
	Thread struct {
		ID     string          `json:"id"`
		Source json.RawMessage `json:"source"`
		Status struct {
			Type string `json:"type"`
		} `json:"status"`
		Turns []struct {
			ID     string          `json:"id"`
			Status string          `json:"status"`
			Items  []codexReadItem `json:"items"`
		} `json:"turns"`
	} `json:"thread"`
}

func readCodexChild(ctx context.Context, rpc codexRPC, child string) (codexReadThread, error) {
	var thread codexReadThread
	raw, err := rpc.call(ctx, "thread/read", map[string]any{"threadId": child, "includeTurns": true})
	if err != nil {
		return thread, codexDiagnostic("native_child_read_failed", err)
	}
	if json.Unmarshal(raw, &thread) != nil {
		return thread, codexDiagnostic("native_child_read_schema_invalid", nil)
	}
	if thread.Thread.ID != child {
		return thread, codexDiagnostic("native_child_identity_mismatch", nil)
	}
	return thread, nil
}

func interruptCodexChild(ctx context.Context, rpc codexRPC, c *codexCapture) error {
	child := c.children["cancel"]
	var thread codexReadThread
	var err error
	for c.cancelTurn == "" {
		thread, err = readCodexChild(ctx, rpc, child)
		if err != nil && CodexFailureCode(err) != "rpc_thread_read_rejected" {
			return err
		}
		terminal := false
		if err == nil {
			for _, turn := range thread.Thread.Turns {
				if turn.Status == "inProgress" {
					if c.cancelTurn != "" {
						return codexDiagnostic("native_child_multiple_active_turns", nil)
					}
					c.cancelTurn = turn.ID
				} else if turn.Status == "completed" || turn.Status == "interrupted" || turn.Status == "failed" {
					terminal = true
				}
			}
		}
		if c.cancelTurn != "" {
			break
		}
		if terminal {
			return codexDiagnostic("native_cancel_child_not_running", nil)
		}
		if err := codexReadRetryPause(ctx); err != nil {
			return err
		}
	}
	if !token(c.cancelTurn) {
		return fmt.Errorf("invalid Codex child turn ID")
	}
	c.add("cancel_requested", child, "cancel")
	if _, err := rpc.call(ctx, "turn/interrupt", map[string]any{"threadId": child, "turnId": c.cancelTurn}); err != nil {
		return err
	}
	c.add("cancel_ack", child, "cancel")
	// thread/read is native state evidence even when this connection does not
	// subscribe to the child thread's turn notifications.
	for {
		thread, err = readCodexChild(ctx, rpc, child)
		if CodexFailureCode(err) == "rpc_thread_read_rejected" {
			if err := codexReadRetryPause(ctx); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		for _, turn := range thread.Thread.Turns {
			if turn.ID == c.cancelTurn && turn.Status != "inProgress" {
				status := "failed"
				if turn.Status == "interrupted" {
					status = "cancelled"
				}
				if turn.Status == "completed" {
					status = "completed"
				}
				c.add("terminal", child, "cancel")
				c.evidence.Events[len(c.evidence.Events)-1].Status = status
				return nil
			}
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func cleanupCodexProbe(ctx context.Context, rpc codexRPC, c *codexCapture) error {
	requested := map[string]bool{}
	// Closing each proven ancestor before reading its full persisted history
	// establishes a quiescent ownership boundary. Archive alone is insufficient.
	for rounds := 0; rounds < MaxEvents; rounds++ {
		if err := closeCodexOwned(ctx, rpc, c, requested); err != nil {
			return err
		}
		added, err := reconcileCodexAncestry(ctx, rpc, c)
		if err != nil {
			return err
		}
		if !added {
			return finishCodexInventory(ctx, rpc, c)
		}
	}
	return fmt.Errorf("Codex ancestry reconciliation exceeds limit")
}

func closeCodexOwned(ctx context.Context, rpc codexRPC, c *codexCapture, requested map[string]bool) error {
	archive := func() error {
		for _, id := range codexOwnedIDs(c) {
			if requested[id] {
				continue
			}
			requested[id] = true
			if _, err := rpc.call(ctx, "thread/archive", map[string]string{"threadId": id}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := archive(); err != nil {
		return err
	}
	if err := proveCodexUnloaded(ctx, rpc, c, requested); err != nil {
		return err
	}
	for !allCodexClosed(c) {
		m, err := rpc.next(ctx)
		if err != nil {
			return err
		}
		if m.Method == "item/completed" || m.Method == "item/started" {
			if _, err := captureCodexActivity(c, m); err != nil {
				return err
			}
			if _, err := reconcileCodexActivityCandidates(ctx, rpc, c); err != nil {
				return err
			}
			if _, err := captureCodexOwnedSpawn(c, m); err != nil {
				return err
			}
			if err := archive(); err != nil {
				return err
			}
			if err := proveCodexUnloaded(ctx, rpc, c, requested); err != nil {
				return err
			}
		}
		if m.Method == "thread/archived" || m.Method == "thread/closed" {
			if err := c.observe(m, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func finishCodexInventory(ctx context.Context, rpc codexRPC, c *codexCapture) error {
	ownedLoaded := []string{}
	rootLoaded := false
	cursor := ""
	for pages := 0; pages < 8; pages++ {
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := rpc.call(ctx, "thread/loaded/list", params)
		if err != nil {
			return err
		}
		var listing struct {
			Data       []string `json:"data"`
			NextCursor *string  `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &listing) != nil || listing.Data == nil {
			return fmt.Errorf("invalid Codex loaded thread inventory")
		}
		for _, id := range listing.Data {
			if id == c.evidence.SupervisorID {
				rootLoaded = true
			}
			if c.owned[id] {
				ownedLoaded = append(ownedLoaded, id)
			}
		}
		if listing.NextCursor == nil || *listing.NextCursor == "" {
			cursor = ""
			break
		}
		cursor = *listing.NextCursor
	}
	if cursor != "" {
		return fmt.Errorf("Codex thread inventory exceeds page limit")
	}
	// The inventory response is a protocol barrier. Any earlier buffered spawn
	// must belong to the already reconciled, closed ancestry; otherwise fail closed.
	for _, m := range rpc.buffered() {
		if _, err := captureCodexActivity(c, m); err != nil {
			return err
		}
		activityAdded, err := reconcileCodexActivityCandidates(ctx, rpc, c)
		if err != nil {
			return err
		}
		if activityAdded {
			return fmt.Errorf("late Codex V2 child after closed ancestry barrier")
		}
		added, err := captureCodexOwnedSpawn(c, m)
		if err != nil {
			return err
		}
		if added {
			return fmt.Errorf("late Codex child after closed ancestry barrier")
		}
	}
	c.add("inventory", "", "")
	sort.Strings(ownedLoaded)
	c.evidence.Events[len(c.evidence.Events)-1].OwnedIDs = ownedLoaded
	if rootLoaded {
		return fmt.Errorf("Codex supervisor remains loaded after closure")
	}
	if !allCodexClosed(c) {
		return fmt.Errorf("Codex native thread closure unverified")
	}
	c.add("session_closed", "", "")
	return nil
}

func codexOwnedIDs(c *codexCapture) []string {
	ids := make([]string, 0, len(c.owned)+1)
	for id := range c.owned {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return append(ids, c.evidence.SupervisorID)
}
func allCodexClosed(c *codexCapture) bool {
	for _, id := range codexOwnedIDs(c) {
		if !c.closed[id] && !c.unloaded[id] {
			return false
		}
	}
	return true
}
