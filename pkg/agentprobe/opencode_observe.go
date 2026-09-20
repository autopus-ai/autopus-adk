package agentprobe

import (
	"context"
	"fmt"
	"time"
)

func (c *openCodeClient) status(ctx context.Context, id string) (string, error) {
	var statuses map[string]struct {
		Type string `json:"type"`
	}
	_, err := c.request(ctx, "GET", "/session/status", nil, &statuses)
	if err != nil {
		return "", err
	}
	if statuses == nil {
		return "", fmt.Errorf("opencode status inventory must be an object")
	}
	state, ok := statuses[id]
	if !ok {
		return "idle", nil
	}
	if state.Type != "busy" && state.Type != "idle" && state.Type != "retry" {
		return "", fmt.Errorf("opencode unknown native session status")
	}
	return state.Type, nil
}

func (c *openCodeClient) messages(ctx context.Context, id string) ([]openCodeMessage, error) {
	var messages []openCodeMessage
	if _, err := c.request(ctx, "GET", "/session/"+id+"/message", nil, &messages); err != nil {
		return nil, err
	}
	if messages == nil {
		return nil, fmt.Errorf("opencode message inventory must be an array")
	}
	for _, message := range messages {
		if err := c.captureMessage(id, message); err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func (c *openCodeClient) captureMessage(session string, message openCodeMessage) error {
	info := message.Info
	if info.SessionID != session || !token(info.ID) {
		return fmt.Errorf("opencode response message identity unverified")
	}
	if info.Role != "assistant" {
		return nil
	}
	value := OpenCodeMessageUsage{SessionID: session, MessageID: info.ID, ProviderID: info.ProviderID, ModelID: info.ModelID, Tokens: info.Tokens, Cost: info.Cost, Completed: info.Time.Completed != nil}
	if info.Error != nil {
		value.ErrorName = info.Error.Name
		if status := info.Error.Data.StatusCode; status != nil && *status >= 100 && *status <= 599 {
			value.ErrorStatusCode = status
		}
	}
	for index, prior := range c.metadata.Messages {
		if prior.SessionID == session && prior.MessageID == info.ID {
			c.metadata.Messages[index] = value
			return nil
		}
	}
	if len(c.metadata.Messages) >= 64 {
		return fmt.Errorf("opencode message count exceeds bound")
	}
	c.metadata.Messages = append(c.metadata.Messages, value)
	return nil
}

func (c *openCodeClient) waitFor(ctx context.Context, interval time.Duration, condition func() (bool, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("opencode lifecycle observation timed out or cancelled")
		}
		ready, err := condition()
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("opencode lifecycle observation timed out or cancelled")
		case <-timer.C:
		}
	}
}

// Cleanup uses a caller-independent deadline and only IDs returned for this run.
func (c *openCodeClient) cleanup(ctx context.Context, e *Evidence, owned []string) error {
	c.cleanupMode = true
	if len(owned) == 0 {
		return nil
	}
	root := owned[0]
	var firstErr error
	for index := len(owned) - 1; index > 0; index-- {
		id := owned[index]
		var ack bool
		if _, err := c.request(ctx, "POST", "/session/"+id+"/abort", nil, &ack); err != nil && firstErr == nil {
			firstErr = err
		}
		if _, err := c.messages(ctx, id); err != nil && firstErr == nil {
			firstErr = err
		}
		var deleted bool
		if _, err := c.request(ctx, "DELETE", "/session/"+id, nil, &deleted); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !deleted && firstErr == nil {
			firstErr = fmt.Errorf("opencode child deletion unconfirmed")
		}
		status, _ := c.request(ctx, "GET", "/session/"+id, nil, nil)
		if status != 404 && firstErr == nil {
			firstErr = fmt.Errorf("opencode child still present after deletion")
		}
	}
	var children []openCodeSession
	_, inventoryErr := c.request(ctx, "GET", "/session/"+root+"/children", nil, &children)
	if inventoryErr == nil && children != nil {
		remaining := []string{}
		for _, child := range children {
			known := false
			for _, id := range owned[1:] {
				if child.ID == id {
					known = true
					remaining = append(remaining, id)
				}
			}
			if !known {
				return fmt.Errorf("opencode unowned child prevents cascading supervisor deletion")
			}
		}
		appendOpenCodeEvent(e, Event{Kind: "inventory", OwnedIDs: remaining})
		if len(remaining) > 0 && firstErr == nil {
			firstErr = fmt.Errorf("opencode owned children remain")
		}
	} else {
		return fmt.Errorf("opencode final child inventory unverified; supervisor retained")
	}
	rootMessages, messageErr := c.messages(ctx, root)
	if messageErr == nil {
		c.metadata.SupervisorEmptyObserved = len(rootMessages) == 0
	}
	var deleted bool
	_, deleteErr := c.request(ctx, "DELETE", "/session/"+root, nil, &deleted)
	status, _ := c.request(ctx, "GET", "/session/"+root, nil, nil)
	if deleteErr == nil && deleted && status == 404 {
		appendOpenCodeEvent(e, Event{Kind: "session_closed"})
	} else if firstErr == nil {
		firstErr = fmt.Errorf("opencode supervisor cleanup unconfirmed")
	}
	c.metadata.CleanupConfirmed = firstErr == nil
	return firstErr
}
