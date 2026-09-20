package agentprobe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenCodeUsagePreservesMissingFields(t *testing.T) {
	var tokens OpenCodeTokens
	if err := json.Unmarshal([]byte(`{"input":0,"cache":{"read":0}}`), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.Input == nil || *tokens.Input != 0 || tokens.Cache.Read == nil || *tokens.Cache.Read != 0 {
		t.Fatal("explicit zero usage was lost")
	}
	if tokens.Output != nil || tokens.Reasoning != nil || tokens.Cache.Write != nil || tokens.Total != nil {
		t.Fatal("missing usage was invented")
	}
}

func TestOpenCodeErrorPreservesOnlySafeStatus(t *testing.T) {
	for _, status := range []int{0, 99, 401, 599, 600} {
		raw, err := json.Marshal(map[string]any{"info": map[string]any{
			"id": "msg_error", "sessionID": "ses_child", "role": "assistant",
			"error": map[string]any{"name": "APIError", "data": map[string]any{
				"statusCode": status, "message": "secret-credential", "responseHeaders": map[string]string{"Authorization": "secret-header"}, "responseBody": "secret-payload",
			}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		var message openCodeMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
		metadata := OpenCodeTransport{}
		client := &openCodeClient{metadata: &metadata}
		if err := client.captureMessage("ses_child", message); err != nil {
			t.Fatal(err)
		}
		usage := metadata.Messages[0]
		if usage.ErrorName != "APIError" {
			t.Fatal(usage.ErrorName)
		}
		if status >= 100 && status <= 599 {
			if usage.ErrorStatusCode == nil || *usage.ErrorStatusCode != status {
				t.Fatal("safe code missing")
			}
		} else if usage.ErrorStatusCode != nil {
			t.Fatal("invalid code retained")
		}
		encoded, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "secret-") || strings.Contains(string(encoded), "Authorization") {
			t.Fatal("raw provider data leaked")
		}
	}
}
