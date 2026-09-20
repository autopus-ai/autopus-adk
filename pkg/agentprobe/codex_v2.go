package agentprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Current native V2 runtimes report subAgentActivity rather than a legacy
// spawnAgent collab item. The activity is only a candidate until thread/read
// independently binds the child source to the observed native parent.
func captureCodexActivity(c *codexCapture, m codexRPCMessage) (bool, error) {
	_, _, kind, err := codexItemEnvelope(m)
	if err != nil {
		return false, err
	}
	if kind != "subAgentActivity" {
		return false, nil
	}
	if m.Method != "item/completed" && m.Method != "item/started" {
		return false, nil
	}
	var params struct {
		ThreadID string `json:"threadId"`
		Item     struct {
			Type  string `json:"type"`
			Kind  string `json:"kind"`
			Child string `json:"agentThreadId"`
		} `json:"item"`
	}
	if json.Unmarshal(m.Params, &params) != nil {
		return false, fmt.Errorf("invalid Codex native activity")
	}
	if params.Item.Type != "subAgentActivity" || params.Item.Kind != "started" {
		return false, nil
	}
	if params.ThreadID != c.evidence.SupervisorID && !c.owned[params.ThreadID] {
		return false, nil
	}
	child := params.Item.Child
	if !token(child) || child == params.ThreadID || child == c.evidence.SupervisorID {
		return false, fmt.Errorf("invalid Codex activity child")
	}
	if prior, ok := c.activities[child]; ok {
		if prior != params.ThreadID {
			return false, fmt.Errorf("conflicting Codex activity parent")
		}
		return false, nil
	}
	if len(c.activities) >= MaxEvents {
		return false, fmt.Errorf("Codex activity limit exceeded")
	}
	c.activities[child] = params.ThreadID
	return true, nil
}

func verifyCodexActivity(ctx context.Context, rpc codexRPC, parent, child string) (codexReadThread, error) {
	thread, err := readCodexChild(ctx, rpc, child)
	if err != nil {
		return thread, err
	}
	var source struct {
		SubAgent struct {
			Spawn struct {
				Parent string `json:"parent_thread_id"`
			} `json:"thread_spawn"`
		} `json:"subAgent"`
	}
	if json.Unmarshal(thread.Thread.Source, &source) != nil || source.SubAgent.Spawn.Parent != parent {
		return thread, codexDiagnostic("native_activity_parent_unverified", nil)
	}
	return thread, nil
}

func observeCodexActivities(ctx context.Context, rpc codexRPC, c *codexCapture, workerCase string) error {
	c.activityReadPending = false
	ids := make([]string, 0, len(c.activities))
	for id := range c.activities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, child := range ids {
		if c.owned[child] && (child != c.children["success"] || c.successResult) {
			continue
		}
		parent := c.activities[child]
		thread, err := verifyCodexActivity(ctx, rpc, parent, child)
		if CodexFailureCode(err) == "rpc_thread_read_rejected" {
			c.activityReadPending = true
			continue
		}
		if err != nil {
			return err
		}
		if !c.owned[child] {
			if len(c.owned) >= MaxEvents {
				return fmt.Errorf("Codex native descendants exceed limit")
			}
			c.owned[child] = true
			if parent == c.evidence.SupervisorID {
				if c.children[workerCase] != "" {
					return codexDiagnostic("native_activity_extra_worker", nil)
				}
				c.children[workerCase] = child
				c.add("spawn", child, workerCase)
			}
		}
		if child != c.children["success"] || c.successResult {
			continue
		}
		for _, turn := range thread.Thread.Turns {
			if turn.Status != "completed" {
				continue
			}
			for _, item := range turn.Items {
				if item.Type == "agentMessage" {
					c.add("result", child, "success")
					code := "challenge_mismatch"
					if strings.TrimSpace(item.Text) == c.evidence.Challenge {
						code = c.evidence.Challenge
					}
					c.evidence.Events[len(c.evidence.Events)-1].ResultCode = code
					c.successResult = true
					break
				}
			}
			if c.successResult {
				break
			}
		}
	}
	return nil
}

func reconcileCodexActivityCandidates(ctx context.Context, rpc codexRPC, c *codexCapture) (bool, error) {
	added := false
	for child, parent := range c.activities {
		if c.owned[child] {
			continue
		}
		if _, err := verifyCodexActivity(ctx, rpc, parent, child); err != nil {
			return false, err
		}
		if len(c.owned) >= MaxEvents {
			return false, fmt.Errorf("Codex native descendants exceed limit")
		}
		c.owned[child] = true
		added = true
	}
	return added, nil
}
