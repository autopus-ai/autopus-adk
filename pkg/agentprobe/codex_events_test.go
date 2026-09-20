package agentprobe

import (
	"encoding/json"
	"testing"
)

func TestCodexNativeCollabOnly(t *testing.T) {
	e := Evidence{Version: 1, Platform: "codex", RuntimeVersion: "0.155.1", RunID: "run", SupervisorID: "parent", Challenge: "nonce"}
	state := newCodexCapture(e)
	prose := codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"parent","item":{"type":"agentMessage","text":"spawned nonce"}}`)}
	if err := state.observe(prose, "success"); err != nil || len(state.evidence.Events) != 0 {
		t.Fatal("prose accepted")
	}
	spawn := codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"parent","item":{"id":"spawn1","type":"collabAgentToolCall","tool":"spawnAgent","status":"completed","senderThreadId":"parent","receiverThreadIds":["child"],"agentsStates":{"child":{"status":"running"}}}}`)}
	if err := state.observe(spawn, "success"); err != nil {
		t.Fatal(err)
	}
	result := codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"parent","item":{"id":"wait1","type":"collabAgentToolCall","tool":"wait","status":"completed","senderThreadId":"parent","receiverThreadIds":["child"],"agentsStates":{"child":{"status":"completed","message":"nonce"}}}}`)}
	if err := state.observe(result, "success"); err != nil {
		t.Fatal(err)
	}
	if len(state.evidence.Events) != 2 || state.evidence.Events[1].ResultCode != "nonce" {
		t.Fatalf("%+v", state.evidence.Events)
	}
}

func TestCodexForeignParentCannotCreateOwnership(t *testing.T) {
	state := newCodexCapture(Evidence{SupervisorID: "parent"})
	m := codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"foreign","item":{"id":"x","type":"collabAgentToolCall","tool":"spawnAgent","status":"completed","senderThreadId":"foreign","receiverThreadIds":["child"]}}`)}
	if err := state.observe(m, "success"); err != nil {
		t.Fatal(err)
	}
	if len(state.owned) != 0 {
		t.Fatal("foreign owned")
	}
}

func TestCodexNestedOwnershipRequiresProvenSender(t *testing.T) {
	c := newCodexCapture(Evidence{SupervisorID: "parent"})
	raw := func(sender string) codexRPCMessage {
		return codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"` + sender + `","item":{"type":"collabAgentToolCall","tool":"spawnAgent","status":"completed","senderThreadId":"` + sender + `","receiverThreadIds":["grandchild"]}}`)}
	}
	added, err := captureCodexOwnedSpawn(c, raw("foreign"))
	if err != nil || added || len(c.owned) != 0 {
		t.Fatal("unproven sender granted ownership")
	}
	c.owned["child"] = true
	added, err = captureCodexOwnedSpawn(c, raw("child"))
	if err != nil || !added || !c.owned["grandchild"] {
		t.Fatalf("native ancestry rejected: %v", err)
	}
}

func TestCodexIgnoresOpaqueUnrelatedNotifications(t *testing.T) {
	c := newCodexCapture(Evidence{SupervisorID: "parent"})
	for _, message := range []codexRPCMessage{
		{Method: "experimental/opaque", Params: nil},
		{Method: "experimental/progress", Params: json.RawMessage(`{"turn":"opaque","item":{"status":{"type":"running"}}}`)},
		{Method: "item/completed", Params: json.RawMessage(`{"threadId":"parent","item":{"type":"futureNativeTool","status":{"type":"running"},"tool":{"name":"unrelated"}}}`)},
	} {
		if err := c.observe(message, "success"); err != nil {
			t.Fatalf("unrelated observation interrupted native lifecycle: %v", err)
		}
	}
	if len(c.evidence.Events) != 0 {
		t.Fatal("opaque notification created proof")
	}
}
