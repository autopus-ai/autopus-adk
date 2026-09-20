package agentprobe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// RunOpenCode collects native child-session evidence from an explicitly owned
// loopback server. It never starts a server or changes global configuration.
func RunOpenCode(ctx context.Context, options OpenCodeOptions) (e Evidence, metadata OpenCodeTransport, err error) {
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return
	}
	runID := "probe_" + hex.EncodeToString(nonce[:])
	e = Evidence{Version: 1, Platform: "opencode", RuntimeVersion: options.RuntimeVersion, RunID: runID, SupervisorID: "unobserved", Challenge: runID, Events: []Event{}}
	metadata = OpenCodeTransport{Protocol: "opencode-http", Provenance: "live_endpoint_capture", ChildIDs: []string{}, Messages: []OpenCodeMessageUsage{}}
	client, clientErr := newOpenCodeClient(options, &metadata)
	if clientErr != nil {
		err = clientErr
		metadata.Failure = err.Error()
		return
	}
	defer client.client.CloseIdleConnections()
	if options.Timeout <= 0 || options.Timeout > 2*time.Minute {
		options.Timeout = 45 * time.Second
	}
	if options.CleanupTimeout <= 0 || options.CleanupTimeout > 30*time.Second {
		options.CleanupTimeout = 10 * time.Second
	}
	if options.PollInterval <= 0 || options.PollInterval > time.Second {
		options.PollInterval = 100 * time.Millisecond
	}
	probeCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	if err = client.preflight(probeCtx); err != nil {
		metadata.Failure = err.Error()
		return
	}
	e.RuntimeVersion = metadata.ObservedRuntimeVersion
	owned := []string{}
	defer func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), options.CleanupTimeout)
		defer done()
		cleanupErr := client.cleanup(cleanupCtx, &e, owned)
		if err == nil {
			err = cleanupErr
		}
		if err != nil {
			metadata.Failure = err.Error()
		}
	}()
	create := func(parent, kind string) (string, error) {
		title := runID + "-" + kind
		body := map[string]any{"title": title, "permission": []any{map[string]any{"permission": "*", "pattern": "*", "action": "deny"}}}
		if parent != "" {
			body["parentID"] = parent
		}
		var session openCodeSession
		if _, requestErr := client.request(probeCtx, "POST", "/session", body, &session); requestErr != nil {
			return "", requestErr
		}
		if !token(session.ID) || !strings.HasPrefix(session.ID, "ses") || strings.ContainsAny(session.ID, "/:.+") || session.Title != title {
			return "", fmt.Errorf("opencode created session ownership unverified")
		}
		for _, id := range owned {
			if id == session.ID {
				return "", fmt.Errorf("opencode reused owned session ID")
			}
		}
		owned = append(owned, session.ID)
		if session.ParentID != parent {
			return "", fmt.Errorf("opencode child parent binding unverified")
		}
		if len(session.Permission) != 1 || session.Permission[0].Permission != "*" || session.Permission[0].Pattern != "*" || session.Permission[0].Action != "deny" {
			return "", fmt.Errorf("opencode session deny-all permissions unverified")
		}
		return session.ID, nil
	}
	root, createErr := create("", "root")
	if createErr != nil {
		err = createErr
		return
	}
	e.SupervisorID = root
	metadata.SupervisorID = root
	success, createErr := create(root, "success")
	if createErr != nil {
		err = createErr
		return
	}
	metadata.ChildIDs = append(metadata.ChildIDs, success)
	appendOpenCodeEvent(&e, Event{Kind: "spawn", Case: "success", ChildID: success})
	cancelID, createErr := create(root, "cancel")
	if createErr != nil {
		err = createErr
		return
	}
	metadata.ChildIDs = append(metadata.ChildIDs, cancelID)
	appendOpenCodeEvent(&e, Event{Kind: "spawn", Case: "cancel", ChildID: cancelID})
	body := client.prompt("Reply with exactly this text and nothing else: " + e.Challenge)
	var message openCodeMessage
	if _, err = client.request(probeCtx, "POST", "/session/"+success+"/message", body, &message); err != nil {
		return
	}
	if err = client.captureMessage(success, message); err != nil {
		return
	}
	if message.Info.Role != "assistant" || message.Info.Error != nil || message.Info.Time.Completed == nil {
		err = fmt.Errorf("opencode success result not complete")
		return
	}
	text := ""
	for _, part := range message.Parts {
		if part.Type == "text" {
			text += part.Text
		}
	}
	result := "challenge_mismatch"
	if strings.TrimSpace(text) == e.Challenge {
		result = e.Challenge
	}
	appendOpenCodeEvent(&e, Event{Kind: "result", Case: "success", ChildID: success, ResultCode: result})
	if _, err = client.request(probeCtx, "POST", "/session/"+cancelID+"/prompt_async", client.prompt("Write a long numbered explanation of arithmetic, at least 1000 lines. Do not use any tools."), nil); err != nil {
		return
	}
	if err = client.waitFor(probeCtx, options.PollInterval, func() (bool, error) {
		state, stateErr := client.status(probeCtx, cancelID)
		if stateErr != nil {
			return false, stateErr
		}
		messages, messageErr := client.messages(probeCtx, cancelID)
		if messageErr != nil {
			return false, messageErr
		}
		for _, m := range messages {
			if m.Info.Role == "assistant" && m.Info.Error == nil && m.Info.Time.Completed == nil {
				return state == "busy", nil
			}
		}
		return false, nil
	}); err != nil {
		return
	}
	appendOpenCodeEvent(&e, Event{Kind: "cancel_requested", Case: "cancel", ChildID: cancelID})
	var acknowledged bool
	if _, err = client.request(probeCtx, "POST", "/session/"+cancelID+"/abort", nil, &acknowledged); err != nil {
		return
	}
	if !acknowledged {
		err = fmt.Errorf("opencode cancellation not acknowledged")
		return
	}
	appendOpenCodeEvent(&e, Event{Kind: "cancel_ack", Case: "cancel", ChildID: cancelID})
	err = client.waitFor(probeCtx, options.PollInterval, func() (bool, error) {
		state, stateErr := client.status(probeCtx, cancelID)
		if stateErr != nil {
			return false, stateErr
		}
		messages, messageErr := client.messages(probeCtx, cancelID)
		if messageErr != nil {
			return false, messageErr
		}
		for _, m := range messages {
			if m.Info.Role == "assistant" && m.Info.Time.Completed != nil && m.Info.Error != nil && m.Info.Error.Name == "MessageAbortedError" {
				return state == "idle", nil
			}
		}
		return false, nil
	})
	if err == nil {
		appendOpenCodeEvent(&e, Event{Kind: "terminal", Case: "cancel", ChildID: cancelID, Status: "cancelled"})
	}
	return
}

func appendOpenCodeEvent(e *Evidence, event Event) {
	event.Sequence = len(e.Events) + 1
	event.Source = "native_protocol"
	event.RunID = e.RunID
	event.ParentID = e.SupervisorID
	e.Events = append(e.Events, event)
}

func (c *openCodeClient) prompt(text string) map[string]any {
	return map[string]any{"model": map[string]string{"providerID": c.options.ProviderID, "modelID": c.options.ModelID}, "parts": []any{map[string]string{"type": "text", "text": text}}}
}
