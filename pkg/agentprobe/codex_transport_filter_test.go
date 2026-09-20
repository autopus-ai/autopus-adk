package agentprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

type codexTestInput struct{ bytes.Buffer }

func (*codexTestInput) Close() error { return nil }

func TestCodexRPCIgnoresTokenFloodDuringNativeRead(t *testing.T) {
	client := &codexRPCClient{input: &codexTestInput{}, messages: make(chan codexRPCMessage, 400), done: make(chan struct{})}
	for i := 0; i < 300; i++ {
		client.messages <- codexRPCMessage{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"threadId":"parent","delta":"token"}`)}
	}
	client.messages <- codexRPCMessage{Method: "thread/closed", Params: json.RawMessage(`{"threadId":"child"}`)}
	client.messages <- codexRPCMessage{ID: json.RawMessage(`1`), Result: json.RawMessage(`{"thread":{"id":"child"}}`)}
	if _, err := client.call(context.Background(), "thread/read", map[string]string{"threadId": "child"}); err != nil {
		t.Fatalf("irrelevant stream exhausted lifecycle queue: %v", err)
	}
	if len(client.pending) != 1 || client.pending[0].Method != "thread/closed" {
		t.Fatalf("pending=%d", len(client.pending))
	}
}

func TestCodexRefreshOnlyOnRelevantNativeLifecycle(t *testing.T) {
	c := newCodexCapture(Evidence{SupervisorID: "parent"})
	c.activities["child"] = "parent"
	cases := []struct {
		method, params string
		want           bool
	}{
		{"item/agentMessage/delta", `{"threadId":"parent","delta":"token"}`, false},
		{"item/reasoning/summaryTextDelta", `{"threadId":"parent","delta":"token"}`, false},
		{"item/completed", `{"threadId":"parent","item":{"type":"agentMessage","text":"prose"}}`, false},
		{"item/completed", `{"threadId":"parent","item":{"type":"subAgentActivity","kind":"completed","agentThreadId":"child"}}`, true},
		{"turn/started", `{"threadId":"child","turn":{"id":"turn"}}`, true},
		{"turn/completed", `{"threadId":"foreign","turn":{"id":"turn"}}`, false},
	}
	for _, tc := range cases {
		if got := codexNeedsActivityRefresh(c, codexRPCMessage{Method: tc.method, Params: json.RawMessage(tc.params)}); got != tc.want {
			t.Fatalf("%s: %v", tc.method, got)
		}
	}
}

type codexFloodActivityFixture struct {
	codexV2Fixture
	messages     []codexRPCMessage
	turns, reads int
}

func (f *codexFloodActivityFixture) next(context.Context) (codexRPCMessage, error) {
	m := f.messages[0]
	f.messages = f.messages[1:]
	return m, nil
}
func (f *codexFloodActivityFixture) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if method == "turn/start" {
		f.turns++
		if f.turns == 1 {
			f.messages = append(f.messages, codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"parent","item":{"type":"subAgentActivity","kind":"started","agentThreadId":"native-child"}}`)})
			for i := 0; i < 300; i++ {
				f.messages = append(f.messages, codexRPCMessage{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"threadId":"parent","delta":"token"}`)})
			}
			f.messages = append(f.messages, codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"parent","item":{"type":"subAgentActivity","kind":"completed","agentThreadId":"native-child"}}`)}, codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"parent","turn":{"id":"turn1","status":"completed"}}`)})
			return json.RawMessage(`{"turn":{"id":"turn1"}}`), nil
		}
		f.messages = append(f.messages, codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"parent","turn":{"id":"turn2","status":"completed"}}`)})
		return json.RawMessage(`{"turn":{"id":"turn2"}}`), nil
	}
	raw, err := f.codexV2Fixture.call(ctx, method, params)
	if method == "thread/read" {
		f.reads++
		if f.reads == 1 {
			var doc map[string]any
			_ = json.Unmarshal(raw, &doc)
			doc["thread"].(map[string]any)["turns"] = []any{map[string]any{"id": "native-turn", "status": "inProgress", "items": []any{}}}
			raw, _ = json.Marshal(doc)
		}
	}
	return raw, err
}
func TestCodexTokenFloodDoesNotAmplifyChildReads(t *testing.T) {
	e := codexTestEvidence()
	e.SupervisorID = "parent"
	capture := newCodexCapture(e)
	fake := &codexFloodActivityFixture{}
	err := runCodexCases(context.Background(), fake, capture)
	if CodexFailureCode(err) != "cancellation_worker_unobserved" {
		t.Fatalf("unexpected fixture outcome: %v", err)
	}
	if !capture.successResult || fake.reads != 2 {
		t.Fatalf("token frames amplified reads: reads=%d result=%v", fake.reads, capture.successResult)
	}
}

func TestCodexLifecycleQueueLimitRemainsBoundedAndCategorized(t *testing.T) {
	client := &codexRPCClient{input: &codexTestInput{}, messages: make(chan codexRPCMessage, 300), done: make(chan struct{})}
	for i := 0; i < 257; i++ {
		client.messages <- codexRPCMessage{Method: "thread/closed", Params: json.RawMessage(`{"threadId":"child"}`)}
	}
	_, err := client.call(context.Background(), "thread/read", map[string]string{"threadId": "child"})
	if code := CodexFailureCode(err); code != "transport_lifecycle_queue_limit" {
		t.Fatal(code)
	}
	if len(client.pending) != 256 {
		t.Fatal("lifecycle queue cap changed")
	}
	client.setError(codexDiagnostic("transport_output_limit", nil))
	client.setError(context.Canceled)
	if code := CodexFailureCode(client.err); code != "transport_output_limit" {
		t.Fatal("transport failure overwritten by cancellation")
	}
}
