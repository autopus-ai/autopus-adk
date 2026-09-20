package agentprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
)

type codexFakeRPC struct {
	messages           []codexRPCMessage
	turn               int
	interrupted        bool
	methods            []string
	archivedIDs        []string
	skipArchives       bool
	rootLoaded         bool
	skipRootClosed     bool
	nested             bool
	historyUnavailable bool
	nativeUnloaded     bool
	residual           bool
	foreign            bool
	missingTerminal    bool
}

func (f *codexFakeRPC) notify(string, any) error { return nil }
func (f *codexFakeRPC) next(context.Context) (codexRPCMessage, error) {
	if len(f.messages) == 0 {
		return codexRPCMessage{}, io.EOF
	}
	m := f.messages[0]
	f.messages = f.messages[1:]
	return m, nil
}
func (f *codexFakeRPC) push(method string, params any) {
	raw, _ := json.Marshal(params)
	f.messages = append(f.messages, codexRPCMessage{Method: method, Params: raw})
}
func (f *codexFakeRPC) call(_ context.Context, method string, params any) (json.RawMessage, error) {
	f.methods = append(f.methods, method)
	switch method {
	case "initialize":
		return json.RawMessage(`{}`), nil
	case "thread/start":
		return json.RawMessage(`{"thread":{"id":"parent"}}`), nil
	case "turn/start":
		f.turn++
		child := fmt.Sprintf("child%d", f.turn)
		item := map[string]any{"id": fmt.Sprintf("spawn%d", f.turn), "type": "collabAgentToolCall", "tool": "spawnAgent", "status": "completed", "senderThreadId": "parent", "receiverThreadIds": []string{child}, "agentsStates": map[string]any{child: map[string]string{"status": "running"}}}
		f.push("item/completed", map[string]any{"threadId": "parent", "item": item})
		if f.turn == 1 {
			item = map[string]any{"id": "wait1", "type": "collabAgentToolCall", "tool": "wait", "status": "completed", "senderThreadId": "parent", "receiverThreadIds": []string{child}, "agentsStates": map[string]any{child: map[string]string{"status": "completed", "message": "nonce"}}}
			f.push("item/completed", map[string]any{"threadId": "parent", "item": item})
			f.push("turn/completed", map[string]any{"threadId": "parent", "turn": map[string]string{"id": "turn1", "status": "completed"}})
		}
		return json.RawMessage(fmt.Sprintf(`{"turn":{"id":"turn%d"}}`, f.turn)), nil
	case "thread/read":
		p := params.(map[string]any)
		readID := p["threadId"].(string)
		statusType := ""
		if f.nativeUnloaded {
			statusType = "notLoaded"
		}
		if readID != "child2" || f.interrupted {
			if f.historyUnavailable && readID == "parent" {
				return nil, errors.New("history unavailable")
			}
			if f.nested && readID == "child1" {
				return json.RawMessage(`{"thread":{"id":"child1","turns":[{"id":"done","status":"completed","items":[{"id":"nested-spawn","type":"collabAgentToolCall","tool":"spawnAgent","status":"completed","senderThreadId":"child1","receiverThreadIds":["nested"]}]}]}}`), nil
			}
			if readID != "child2" {
				return json.RawMessage(fmt.Sprintf(`{"thread":{"id":%q,"status":{"type":%q},"turns":[]}}`, readID, statusType)), nil
			}
		}
		if f.missingTerminal && f.interrupted {
			return nil, errors.New("native read unavailable")
		}
		state := "inProgress"
		if f.interrupted {
			state = "interrupted"
		}
		id := "child2"
		if f.foreign {
			id = "foreign"
		}
		return json.RawMessage(fmt.Sprintf(`{"thread":{"id":%q,"status":{"type":%q},"turns":[{"id":"childturn","status":%q,"items":[]}]}}`, id, statusType, state)), nil
	case "turn/interrupt":
		f.interrupted = true
		return json.RawMessage(`{}`), nil
	case "thread/archive":
		p := params.(map[string]string)
		f.archivedIDs = append(f.archivedIDs, p["threadId"])
		if p["threadId"] != "parent" && p["threadId"] != "child1" && p["threadId"] != "child2" && p["threadId"] != "nested" {
			return nil, errors.New("foreign cleanup")
		}
		if !f.skipArchives {
			f.push("thread/archived", map[string]string{"threadId": p["threadId"]})
			if !f.skipRootClosed {
				f.push("thread/closed", map[string]string{"threadId": p["threadId"]})
			}
		}
		return json.RawMessage(`{}`), nil
	case "thread/loaded/list":
		if f.rootLoaded {
			return json.RawMessage(`{"data":["parent"]}`), nil
		}
		if f.residual {
			return json.RawMessage(`{"data":["child2"]}`), nil
		}
		return json.RawMessage(`{"data":[]}`), nil
	}
	return nil, fmt.Errorf("unexpected %s", method)
}
func codexTestEvidence() Evidence {
	return Evidence{Version: 1, Platform: "codex", RuntimeVersion: "0.155.1", RunID: "run", SupervisorID: "unobserved", Challenge: "nonce", Events: []Event{}}
}

func TestCodexNativeLifecycleDriver(t *testing.T) {
	fake := &codexFakeRPC{}
	e, err := runCodexLifecycle(context.Background(), context.Background(), fake, codexTestEvidence(), CodexOptions{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r, err := Evaluate(e)
	if err != nil || r.Overall != "pass" {
		t.Fatalf("%+v %v events=%+v", r, err, e.Events)
	}
	if !fake.interrupted {
		t.Fatal("native interrupt absent")
	}
}
func TestCodexCleanupRequiresNativeConfirmation(t *testing.T) {
	for _, fake := range []*codexFakeRPC{{skipArchives: true}, {residual: true}} {
		e, _ := runCodexLifecycle(context.Background(), context.Background(), fake, codexTestEvidence(), CodexOptions{}, t.TempDir())
		r, err := Evaluate(e)
		if err != nil {
			t.Fatal(err)
		}
		if r.Gates[3].Status == "pass" {
			t.Fatal("cleanup unproved")
		}
	}
}
func TestCodexDoesNotInterruptForeignOrInventTerminal(t *testing.T) {
	for _, fake := range []*codexFakeRPC{{foreign: true}, {missingTerminal: true}} {
		e, err := runCodexLifecycle(context.Background(), context.Background(), fake, codexTestEvidence(), CodexOptions{}, t.TempDir())
		if err == nil {
			t.Fatal("missing native condition accepted")
		}
		r, err := Evaluate(e)
		if err != nil {
			t.Fatal(err)
		}
		if r.Gates[2].Status != "unknown" {
			t.Fatal(r)
		}
		if fake.foreign && fake.interrupted {
			t.Fatal("foreign child interrupted")
		}
	}
}

func TestCodexRootArchiveDoesNotProveUnloadedSession(t *testing.T) {
	for _, fake := range []*codexFakeRPC{{rootLoaded: true, skipRootClosed: true}, {rootLoaded: true}, {skipRootClosed: true}} {
		e, _ := runCodexLifecycle(context.Background(), context.Background(), fake, codexTestEvidence(), CodexOptions{}, t.TempDir())
		r, err := Evaluate(e)
		if err != nil {
			t.Fatal(err)
		}
		if r.Gates[3].Status == "pass" {
			t.Fatal("archive acknowledgement mistaken for unloaded supervisor")
		}
	}
}

func TestCodexCleanupReconcilesNativeNestedAncestry(t *testing.T) {
	fake := &codexFakeRPC{nested: true}
	e, err := runCodexLifecycle(context.Background(), context.Background(), fake, codexTestEvidence(), CodexOptions{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The fake rejects cleanup of any identity not created by fixture ancestry.
	found := false
	for _, id := range fake.archivedIDs {
		if id == "nested" {
			found = true
		}
	}
	if !found {
		t.Fatal("nested native child escaped cleanup")
	}
	report, err := Evaluate(e)
	if err != nil || report.Gates[3].Status != "pass" {
		t.Fatalf("%+v %v", report, err)
	}
}
func TestCodexCleanupCannotAssumeUnobservedDescendantsAbsent(t *testing.T) {
	fake := &codexFakeRPC{historyUnavailable: true}
	e, _ := runCodexLifecycle(context.Background(), context.Background(), fake, codexTestEvidence(), CodexOptions{}, t.TempDir())
	report, err := Evaluate(e)
	if err != nil {
		t.Fatal(err)
	}
	if report.Gates[3].Status == "pass" {
		t.Fatal("unverified ancestry closure passed")
	}
}

func (f *codexFakeRPC) buffered() []codexRPCMessage {
	messages := f.messages
	f.messages = nil
	return messages
}

func TestCodexNativeUnloadProofWithoutClosedNotification(t *testing.T) {
	fake := &codexFakeRPC{nativeUnloaded: true, skipRootClosed: true}
	e, err := runCodexLifecycle(context.Background(), context.Background(), fake, codexTestEvidence(), CodexOptions{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	report, err := Evaluate(e)
	if err != nil || report.Gates[3].Status != "pass" {
		t.Fatalf("%+v %v", report, err)
	}
	fake = &codexFakeRPC{nativeUnloaded: true, skipRootClosed: true, rootLoaded: true}
	e, _ = runCodexLifecycle(context.Background(), context.Background(), fake, codexTestEvidence(), CodexOptions{}, t.TempDir())
	report, err = Evaluate(e)
	if err != nil {
		t.Fatal(err)
	}
	if report.Gates[3].Status == "pass" {
		t.Fatal("loaded root contradicts unload proof")
	}
}
