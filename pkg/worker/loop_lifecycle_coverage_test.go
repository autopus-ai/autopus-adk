package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memCredStore is an in-memory setup.CredentialStore used to enable JWT refresh.
type memCredStore struct{ val string }

func (m *memCredStore) Save(service, value string) error { m.val = value; return nil }
func (m *memCredStore) Load(service string) (string, error) {
	return m.val, nil
}
func (m *memCredStore) Delete(service string) error { m.val = ""; return nil }

func lifecycleBackend(t *testing.T, seen *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			seen.Store(r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Guards the audit path fallback: an empty AuditLogPath must resolve under WorkDir,
// and API-key mode must not start the JWT refresher even with a credential store.
func TestStartServices_APIKeyModeSkipsRefresherAndUsesWorkDirAudit(t *testing.T) {
	srv := lifecycleBackend(t, nil)
	workDir := t.TempDir()

	wl := NewWorkerLoop(LoopConfig{
		BackendURL:      srv.URL,
		WorkDir:         workDir,
		AuthToken:       "acos_worker_abc",
		CredentialStore: &memCredStore{},
	})

	wl.startServices(context.Background())
	defer wl.stopServices()

	_, err := os.Stat(filepath.Join(workDir, ".autopus", "audit.jsonl"))
	require.NoError(t, err, "audit writer must fall back to {WorkDir}/.autopus/audit.jsonl")
	assert.Nil(t, wl.authRefresher, "API key mode must not refresh tokens")
	assert.Nil(t, wl.authReconnector)
	assert.Nil(t, wl.memorySearcher, "memory searcher requires a workspace ID")
	assert.Nil(t, wl.schedulerDisp, "scheduler requires a workspace ID")
	assert.NotNil(t, wl.netMonitor, "net monitor starts in every auth mode")
}

// Guards refresher gating: JWT + credential store is the only combination that
// produces a refresher/reconnector, and an explicit audit path wins over the default.
func TestStartServices_JWTWithStoreStartsRefresherAndHonorsAuditPath(t *testing.T) {
	srv := lifecycleBackend(t, nil)
	workDir := t.TempDir()
	auditPath := filepath.Join(t.TempDir(), "nested", "custom-audit.jsonl")

	wl := NewWorkerLoop(LoopConfig{
		BackendURL:      srv.URL,
		WorkDir:         workDir,
		AuthToken:       "jwt-token",
		AuditLogPath:    auditPath,
		CredentialStore: &memCredStore{val: `{"access_token":"jwt-token"}`},
	})

	wl.startServices(context.Background())
	defer wl.stopServices()

	_, err := os.Stat(auditPath)
	require.NoError(t, err, "explicit AuditLogPath must take precedence")
	_, err = os.Stat(filepath.Join(workDir, ".autopus", "audit.jsonl"))
	assert.True(t, os.IsNotExist(err), "default audit path must not be created when overridden")
	assert.NotNil(t, wl.authRefresher)
	assert.NotNil(t, wl.authReconnector)
}

// Guards the refresher precondition: JWT mode without a credential store keeps the
// initial bearer token instead of starting a refresher.
func TestStartServices_JWTWithoutStoreHasNoRefresher(t *testing.T) {
	srv := lifecycleBackend(t, nil)

	wl := NewWorkerLoop(LoopConfig{
		BackendURL: srv.URL,
		WorkDir:    t.TempDir(),
		AuthToken:  "jwt-token",
	})

	wl.startServices(context.Background())
	defer wl.stopServices()

	assert.Nil(t, wl.authRefresher)
	assert.Nil(t, wl.authReconnector)
}

// Guards service gating on workspace/knowledge flags: knowledge search needs both
// KnowledgeSync and WorkspaceID, while memory and scheduler need only WorkspaceID.
func TestStartServices_ServiceGatingByWorkspaceAndKnowledgeFlag(t *testing.T) {
	srv := lifecycleBackend(t, nil)

	withWorkspace := NewWorkerLoop(LoopConfig{
		BackendURL:  srv.URL,
		WorkDir:     t.TempDir(),
		WorkspaceID: "ws-1",
		AuthToken:   "jwt-token",
	})
	withWorkspace.startServices(context.Background())
	defer withWorkspace.stopServices()

	assert.Nil(t, withWorkspace.knowledgeSearcher, "knowledge search stays off without KnowledgeSync")
	assert.NotNil(t, withWorkspace.memorySearcher)
	assert.NotNil(t, withWorkspace.schedulerDisp)

	syncOnly := NewWorkerLoop(LoopConfig{
		BackendURL:    srv.URL,
		WorkDir:       t.TempDir(),
		KnowledgeSync: true,
		AuthToken:     "jwt-token",
	})
	syncOnly.startServices(context.Background())
	defer syncOnly.stopServices()

	assert.Nil(t, syncOnly.knowledgeSearcher, "knowledge search stays off without a workspace ID")
	assert.Nil(t, syncOnly.memorySearcher)

	both := NewWorkerLoop(LoopConfig{
		BackendURL:    srv.URL,
		WorkDir:       t.TempDir(),
		KnowledgeSync: true,
		WorkspaceID:   "ws-1",
		AuthToken:     "jwt-token",
	})
	both.startServices(context.Background())
	defer both.stopServices()

	assert.NotNil(t, both.knowledgeSearcher, "knowledge search needs both flags")
}

// Guards stopServices: the lifecycle context must be cancelled so every started
// service observes shutdown.
func TestStopServices_CancelsLifecycleContext(t *testing.T) {
	srv := lifecycleBackend(t, nil)
	wl := NewWorkerLoop(LoopConfig{
		BackendURL: srv.URL,
		WorkDir:    t.TempDir(),
		AuthToken:  "jwt-token",
	})

	wl.startServices(context.Background())
	require.NoError(t, wl.lifecycleCtx.Err(), "lifecycle context must be live after start")

	wl.stopServices()
	assert.ErrorIs(t, wl.lifecycleCtx.Err(), context.Canceled)
}

// Guards parent-context propagation: cancelling the caller's context must cancel
// the derived lifecycle context without an explicit stopServices call.
func TestStartServices_ParentCancelPropagates(t *testing.T) {
	srv := lifecycleBackend(t, nil)
	ctx, cancel := context.WithCancel(context.Background())

	wl := NewWorkerLoop(LoopConfig{
		BackendURL: srv.URL,
		WorkDir:    t.TempDir(),
		AuthToken:  "jwt-token",
	})
	wl.startServices(ctx)
	cancel()

	<-wl.lifecycleCtx.Done()
	assert.ErrorIs(t, wl.lifecycleCtx.Err(), context.Canceled)
	wl.stopServices()
}

// Guards token rotation: an empty token is ignored, and a real token reaches the
// outbound Authorization header of every token-bearing service.
func TestUpdateAuthToken_IgnoresEmptyAndPropagatesToSearchers(t *testing.T) {
	var seen atomic.Value
	srv := lifecycleBackend(t, &seen)

	wl := NewWorkerLoop(LoopConfig{
		BackendURL:    srv.URL,
		WorkDir:       t.TempDir(),
		WorkspaceID:   "ws-1",
		KnowledgeSync: true,
		AuthToken:     "old-token",
		MemoryAgentID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
	})
	wl.startServices(context.Background())
	defer wl.stopServices()

	wl.updateAuthToken("")
	assert.Equal(t, "old-token", wl.config.AuthToken, "empty token must not clear existing auth")

	wl.updateAuthToken("new-token")
	assert.Equal(t, "new-token", wl.config.AuthToken)

	_, err := wl.memorySearcher.GetContext(
		context.Background(),
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		"describe",
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer new-token", seen.Load(), "rotated token must be used for memory requests")

	_, err = wl.knowledgeSearcher.Search(context.Background(), "query")
	require.NoError(t, err)
	assert.Equal(t, "Bearer new-token", seen.Load(), "rotated token must be used for knowledge search")
}

// Guards degraded-mode reporting: engaging the REST fallback must surface a
// runtime_degraded host event to observers.
func TestActivateFallbackPoller_EmitsRuntimeDegradedEvent(t *testing.T) {
	wl := NewWorkerLoop(LoopConfig{WorkDir: t.TempDir()})

	events := make(chan HostEvent, 4)
	wl.AddHostObserver(HostObserverFunc(func(e HostEvent) { events <- e }))

	wl.activateFallbackPoller()

	select {
	case got := <-events:
		assert.Equal(t, HostEventRuntimeDegraded, got.Type)
		assert.Contains(t, got.Message, "fallback poller")
	default:
		t.Fatal("expected a host event for fallback activation")
	}
}
