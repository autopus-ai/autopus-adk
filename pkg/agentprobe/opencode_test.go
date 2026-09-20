package agentprobe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type openCodeFixture struct {
	mu                            sync.Mutex
	sessions                      map[string]map[string]any
	deleted                       []string
	count                         int
	aborted, ackOnly, denyMissing bool
}

func (f *openCodeFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	path := r.URL.Path
	switch {
	case path == "/global/health":
		write(map[string]any{"healthy": true, "version": "1.18.7"})
	case path == "/doc":
		write(map[string]any{"paths": map[string]any{
			"/session":                          map[string]any{"post": map[string]any{"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"properties": map[string]any{"parentID": map[string]any{}, "permission": map[string]any{}}}}}}}},
			"/session/{sessionID}/message":      map[string]any{"get": map[string]any{}, "post": map[string]any{}},
			"/session/{sessionID}/prompt_async": map[string]any{"post": map[string]any{}},
			"/session/{sessionID}/abort":        map[string]any{"post": map[string]any{}},
			"/session/{sessionID}/children":     map[string]any{"get": map[string]any{}},
			"/session/{sessionID}":              map[string]any{"get": map[string]any{}, "delete": map[string]any{}},
			"/session/status":                   map[string]any{"get": map[string]any{}},
		}})
	case path == "/session" && r.Method == "POST":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.count++
		id := []string{"ses_root", "ses_success", "ses_cancel"}[f.count-1]
		body["id"] = id
		if f.denyMissing {
			delete(body, "permission")
		}
		f.sessions[id] = body
		write(body)
	case path == "/session/status":
		state := "busy"
		if f.aborted {
			state = "idle"
		}
		write(map[string]any{"ses_cancel": map[string]any{"type": state}})
	case path == "/session/ses_success/message" && r.Method == "POST":
		var body struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		challenge := strings.TrimPrefix(body.Parts[0].Text, "Reply with exactly this text and nothing else: ")
		write(map[string]any{"info": map[string]any{"id": "msg_success", "role": "assistant", "sessionID": "ses_success", "time": map[string]any{"completed": 1}}, "parts": []any{map[string]any{"type": "text", "text": challenge}}})
	case strings.HasSuffix(path, "/prompt_async"):
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(path, "/abort"):
		f.aborted = true
		write(true)
	case path == "/session/ses_cancel/message":
		info := map[string]any{"id": "msg_cancel", "role": "assistant", "sessionID": "ses_cancel"}
		if f.aborted && !f.ackOnly {
			info["error"] = map[string]any{"name": "MessageAbortedError"}
			info["time"] = map[string]any{"completed": 1}
		}
		write([]any{map[string]any{"info": info, "parts": []any{}}})
	case path == "/session/ses_success/message" || path == "/session/ses_root/message":
		write([]any{})
	case strings.HasSuffix(path, "/children"):
		children := []any{}
		for _, s := range f.sessions {
			if s["parentID"] == "ses_root" {
				children = append(children, s)
			}
		}
		write(children)
	case r.Method == "DELETE":
		id := strings.TrimPrefix(path, "/session/")
		f.deleted = append(f.deleted, id)
		delete(f.sessions, id)
		write(true)
	case r.Method == "GET" && strings.HasPrefix(path, "/session/"):
		s, ok := f.sessions[strings.TrimPrefix(path, "/session/")]
		if !ok {
			w.WriteHeader(404)
			write(map[string]any{})
		} else {
			write(s)
		}
	default:
		w.WriteHeader(404)
	}
}

func TestOpenCodeNativeLifecycle(t *testing.T) {
	f := &openCodeFixture{sessions: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	defer server.Close()
	evidence, metadata, err := RunOpenCode(context.Background(), OpenCodeOptions{Endpoint: server.URL, RuntimeVersion: "1.18.7", ProviderID: "test", ModelID: "model", Timeout: time.Second, PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	report, err := Evaluate(evidence)
	if err != nil || report.Overall != "pass" {
		t.Fatalf("%+v %v", report, err)
	}
	if !metadata.CleanupConfirmed || len(f.deleted) != 3 {
		t.Fatalf("cleanup %+v %v", metadata, f.deleted)
	}
}

func TestOpenCodeAbortAckAloneIsUnknown(t *testing.T) {
	f := &openCodeFixture{sessions: map[string]map[string]any{}, ackOnly: true}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	defer server.Close()
	evidence, metadata, err := RunOpenCode(context.Background(), OpenCodeOptions{Endpoint: server.URL, RuntimeVersion: "1.18.7", ProviderID: "test", ModelID: "model", Timeout: 120 * time.Millisecond, PollInterval: time.Millisecond})
	if err == nil {
		t.Fatal("ack only accepted")
	}
	report, parseErr := Evaluate(evidence)
	if parseErr != nil || report.Overall == "pass" || !metadata.CleanupConfirmed {
		t.Fatalf("%+v %+v %v", report, metadata, parseErr)
	}
}
