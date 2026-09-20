package agentprobe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeRejectsUnverifiedEndpoints(t *testing.T) {
	for _, endpoint := range []string{"https://127.0.0.1:1", "http://example.com:1", "http://localhost:1", "http://127.0.0.1:1/path", "http://user:pass@127.0.0.1:1", "http://127.0.0.1:1?secret=1"} {
		_, _, err := RunOpenCode(context.Background(), OpenCodeOptions{Endpoint: endpoint, ProviderID: "p", ModelID: "m"})
		if err == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
}

func TestOpenCodeRefusesMissingDenyPermission(t *testing.T) {
	f := &openCodeFixture{sessions: map[string]map[string]any{}, denyMissing: true}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	defer server.Close()
	_, metadata, err := RunOpenCode(context.Background(), OpenCodeOptions{Endpoint: server.URL, ProviderID: "p", ModelID: "m", Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "deny-all") || len(f.deleted) != 1 {
		t.Fatalf("%+v %v deleted %v", metadata, err, f.deleted)
	}
}

func TestOpenCodeRuntimeMismatchDoesNotCreateSessions(t *testing.T) {
	f := &openCodeFixture{sessions: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	defer server.Close()
	_, _, err := RunOpenCode(context.Background(), OpenCodeOptions{Endpoint: server.URL, RuntimeVersion: "wrong", ProviderID: "p", ModelID: "m"})
	if err == nil || f.count != 0 {
		t.Fatalf("error %v created %d", err, f.count)
	}
}

func TestOpenCodeDoesNotDeleteUnknownChildrenThroughParent(t *testing.T) {
	f := &openCodeFixture{sessions: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/children") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "ses_foreign", "parentID": "ses_root"}})
			return
		}
		f.serve(w, r)
	}))
	defer server.Close()
	_, metadata, err := RunOpenCode(context.Background(), OpenCodeOptions{Endpoint: server.URL, ProviderID: "p", ModelID: "m", Timeout: time.Second})
	if err == nil || metadata.CleanupConfirmed {
		t.Fatalf("unowned children ignored %+v %v", metadata, err)
	}
	for _, id := range f.deleted {
		if id == "ses_root" || id == "ses_foreign" {
			t.Fatalf("cascading cleanup: %v", f.deleted)
		}
	}
}
