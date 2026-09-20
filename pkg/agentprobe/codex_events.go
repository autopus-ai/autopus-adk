package agentprobe

import (
	"encoding/json"
	"fmt"
	"strings"
)

type codexCapture struct {
	rootTurn            string
	rootTurnStatus      string
	activityReadPending bool
	evidence            Evidence
	owned               map[string]bool
	children            map[string]string
	seen                map[string]bool
	archived            map[string]bool
	closed              map[string]bool
	unloaded            map[string]bool
	activities          map[string]string
	successResult       bool
	rootTurnDone        bool
	cancelTerminal      bool
	cancelTurn          string
}

func newCodexCapture(e Evidence) *codexCapture {
	return &codexCapture{evidence: e, owned: map[string]bool{}, children: map[string]string{}, seen: map[string]bool{}, archived: map[string]bool{}, closed: map[string]bool{}, unloaded: map[string]bool{}, activities: map[string]string{}}
}
func (c *codexCapture) add(kind, child, workerCase string) {
	c.evidence.Events = append(c.evidence.Events, Event{Sequence: len(c.evidence.Events) + 1, Kind: kind, Source: "native_protocol", RunID: c.evidence.RunID, ParentID: c.evidence.SupervisorID, ChildID: child, Case: workerCase})
}
func (c *codexCapture) observe(m codexRPCMessage, workerCase string) error {
	switch m.Method {
	case "thread/archived", "thread/closed", "turn/completed":
	case "item/started", "item/completed":
		_, _, kind, err := codexItemEnvelope(m)
		if err != nil {
			return err
		}
		if kind == "subAgentActivity" {
			_, err := captureCodexActivity(c, m)
			return err
		}
		if kind != "collabAgentToolCall" {
			return nil
		}
	default:
		return nil
	}
	var params struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
		Item struct {
			ID        string   `json:"id"`
			Type      string   `json:"type"`
			Tool      string   `json:"tool"`
			Status    string   `json:"status"`
			Sender    string   `json:"senderThreadId"`
			Receivers []string `json:"receiverThreadIds"`
			States    map[string]struct {
				Status  string  `json:"status"`
				Message *string `json:"message"`
			} `json:"agentsStates"`
		} `json:"item"`
	}
	if json.Unmarshal(m.Params, &params) != nil {
		return codexDiagnostic("native_event_parameters_invalid", nil)
	}
	if m.Method == "thread/archived" || m.Method == "thread/closed" {
		if c.owned[params.ThreadID] || params.ThreadID == c.evidence.SupervisorID {
			c.archived[params.ThreadID] = true
			if m.Method == "thread/closed" {
				c.closed[params.ThreadID] = true
			}
		}
		return nil
	}
	if m.Method == "turn/completed" {
		if params.ThreadID == c.evidence.SupervisorID && params.Turn.ID == c.rootTurn {
			c.rootTurnDone = true
			c.rootTurnStatus = params.Turn.Status
		}
		if params.ThreadID == c.children["cancel"] && c.cancelTurn != "" && params.Turn.ID == c.cancelTurn && params.Turn.Status == "interrupted" {
			c.cancelTerminal = true
		}
		return nil
	}
	if m.Method == "item/completed" && params.Item.Sender != c.evidence.SupervisorID {
		if _, err := captureCodexOwnedSpawn(c, m); err != nil {
			return err
		}
	}
	if _, err := captureCodexActivity(c, m); err != nil {
		return err
	}
	item := params.Item
	if m.Method != "item/completed" || params.ThreadID != c.evidence.SupervisorID || item.Sender != c.evidence.SupervisorID || item.Type != "collabAgentToolCall" || item.Status != "completed" {
		return nil
	}
	if !token(item.ID) {
		return fmt.Errorf("invalid Codex collaboration item ID")
	}
	if c.seen[item.ID] {
		return nil
	}
	c.seen[item.ID] = true
	if item.Tool == "spawnAgent" {
		if len(item.Receivers) != 1 || !token(item.Receivers[0]) || item.Receivers[0] == c.evidence.SupervisorID {
			return fmt.Errorf("invalid Codex child spawn")
		}
		child := item.Receivers[0]
		c.owned[child] = true
		if c.children[workerCase] != "" {
			return fmt.Errorf("unexpected additional Codex worker")
		}
		for _, prior := range c.children {
			if prior == child {
				return fmt.Errorf("Codex worker ID reused")
			}
		}
		c.children[workerCase] = child
		c.add("spawn", child, workerCase)
	}
	child := c.children["success"]
	if state, ok := item.States[child]; ok && state.Status == "completed" && state.Message != nil && !c.successResult {
		c.add("result", child, "success")
		code := "challenge_mismatch"
		if strings.TrimSpace(*state.Message) == c.evidence.Challenge {
			code = c.evidence.Challenge
		}
		c.evidence.Events[len(c.evidence.Events)-1].ResultCode = code
		c.successResult = true
	}
	return nil
}
