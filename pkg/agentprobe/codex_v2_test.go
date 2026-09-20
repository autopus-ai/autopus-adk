package agentprobe

import (
	"context"
	"encoding/json"
	"testing"
)

type codexV2Fixture struct {
	codexFakeRPC
	foreignParent bool
}

func (f *codexV2Fixture) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if method == "thread/read" {
		parent := "parent"
		if f.foreignParent {
			parent = "foreign"
		}
		raw, _ := json.Marshal(map[string]any{"thread": map[string]any{"id": "native-child", "source": map[string]any{"subAgent": map[string]any{"thread_spawn": map[string]any{"parent_thread_id": parent}}}, "turns": []any{map[string]any{"id": "native-turn", "status": "completed", "items": []any{map[string]string{"type": "agentMessage", "text": "nonce"}}}}}})
		return raw, nil
	}
	return f.codexFakeRPC.call(ctx, method, params)
}
func TestCodexV2NativeActivityBindsChildAndResult(t *testing.T) {
	e := codexTestEvidence()
	e.SupervisorID = "parent"
	capture := newCodexCapture(e)
	activity := codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"parent","item":{"id":"activity1","type":"subAgentActivity","kind":"started","agentThreadId":"native-child","agentPath":"/root/worker"}}`)}
	if err := capture.observe(activity, "success"); err != nil {
		t.Fatal(err)
	}
	if len(capture.evidence.Events) != 0 {
		t.Fatal("activity cannot grant unverified ownership")
	}
	if err := observeCodexActivities(context.Background(), &codexV2Fixture{}, capture, "success"); err != nil {
		t.Fatal(err)
	}
	if len(capture.evidence.Events) != 2 || capture.evidence.Events[1].ResultCode != "nonce" {
		t.Fatalf("native V2 missing: %+v", capture.evidence.Events)
	}
}
func TestCodexV2ActivityRejectsForeignSourceBinding(t *testing.T) {
	e := codexTestEvidence()
	e.SupervisorID = "parent"
	capture := newCodexCapture(e)
	activity := codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"parent","item":{"id":"activity1","type":"subAgentActivity","kind":"started","agentThreadId":"native-child"}}`)}
	if err := capture.observe(activity, "success"); err != nil {
		t.Fatal(err)
	}
	if err := observeCodexActivities(context.Background(), &codexV2Fixture{foreignParent: true}, capture, "success"); err == nil {
		t.Fatal("foreign source accepted")
	}
	if len(capture.owned) != 0 || len(capture.evidence.Events) != 0 {
		t.Fatal("foreign source acquired ownership")
	}
}

type codexTransientActivityFixture struct {
	codexV2Fixture
	calls          int
	afterAdmission bool
}

func (f *codexTransientActivityFixture) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if method == "thread/read" {
		f.calls++
		if (!f.afterAdmission && f.calls == 1) || (f.afterAdmission && f.calls == 2) {
			return nil, codexDiagnostic("rpc_thread_read_rejected", nil)
		}
		raw, err := f.codexV2Fixture.call(ctx, method, params)
		if f.afterAdmission && f.calls == 1 {
			var doc map[string]any
			_ = json.Unmarshal(raw, &doc)
			thread := doc["thread"].(map[string]any)
			thread["turns"] = []any{map[string]any{"id": "native-turn", "status": "inProgress", "items": []any{}}}
			raw, _ = json.Marshal(doc)
		}
		return raw, err
	}
	return f.codexV2Fixture.call(ctx, method, params)
}
func TestCodexActivityWaitsForNativeReadVisibility(t *testing.T) {
	for _, after := range []bool{false, true} {
		e := codexTestEvidence()
		e.SupervisorID = "parent"
		capture := newCodexCapture(e)
		capture.activities["native-child"] = "parent"
		fake := &codexTransientActivityFixture{afterAdmission: after}
		if err := observeCodexActivities(context.Background(), fake, capture, "success"); err != nil {
			t.Fatalf("startup visibility race aborted: %v", err)
		}
		if !after && (len(capture.owned) != 0 || len(capture.evidence.Events) != 0) {
			t.Fatal("unverified read admitted ownership")
		}
		if err := observeCodexActivities(context.Background(), fake, capture, "success"); err != nil {
			t.Fatalf("postspawn visibility race aborted: %v", err)
		}
		if after {
			if err := observeCodexActivities(context.Background(), fake, capture, "success"); err != nil {
				t.Fatal(err)
			}
		}
		if len(capture.evidence.Events) != 2 || capture.evidence.Events[1].ResultCode != "nonce" {
			t.Fatal("deferred native result missing")
		}
	}
}

func TestCodexReadItemIgnoresUnrelatedVariantPayload(t *testing.T) {
	var item codexReadItem
	if err := json.Unmarshal([]byte(`{"type":"reasoning","text":[{"text":"opaque"}],"status":{"type":"running"}}`), &item); err != nil {
		t.Fatal(err)
	}
	if item.Type != "reasoning" || item.Text != "" {
		t.Fatal("opaque reasoning became result")
	}
	if err := json.Unmarshal([]byte(`{"type":"agentMessage","text":["not-a-native-message"]}`), &item); err == nil {
		t.Fatal("malformed actual result accepted")
	}
}
