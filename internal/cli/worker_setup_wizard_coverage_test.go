package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/insajin/autopus-adk/pkg/worker/setup"
)

// isolatedSetupEnv pins credentials, config, and progress files to a temp HOME and
// keeps credential writes out of the real OS keychain.
func isolatedSetupEnv(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	_ = keyring.Delete("autopus-worker", "credentials")
	t.Cleanup(func() { _ = keyring.Delete("autopus-worker", "credentials") })
	t.Setenv("HOME", t.TempDir())
}

func setupCmd() (*cobra.Command, *bytes.Buffer) {
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "setup"}
	cmd.SetOut(&out)
	return cmd, &out
}

// Guards the legacy API-key branch: the key is persisted as api_key, the backend is
// never asked to resolve the workspace, and the ID is used verbatim. The run then
// stops at MCP generation because that config requires a bearer token.
func TestRunWorkerSetup_APIKeyModeUsesWorkspaceIDWithoutBackendLookup(t *testing.T) {
	isolatedSetupEnv(t)

	var backendCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalls.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cmd, out := setupCmd()
	err := runWorkerSetup(cmd, srv.URL, "", "ws-api", "acos_worker_secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "설정 저장")
	assert.Contains(t, err.Error(), "MCP config")

	assert.Zero(t, backendCalls.Load(), "API key mode must not call the backend")
	assert.Contains(t, out.String(), "레거시 Worker API Key 저장 완료")
	assert.Contains(t, out.String(), "워크스페이스 ID: ws-api")
	assert.NotContains(t, out.String(), "Setup complete!")

	key, loadErr := setup.LoadAPIKey()
	require.NoError(t, loadErr)
	assert.Equal(t, "acos_worker_secret", key)
}

// Guards the backend default: an empty --backend must be replaced by the production
// endpoint in the stored credentials.
func TestRunWorkerSetup_EmptyBackendFallsBackToDefault(t *testing.T) {
	isolatedSetupEnv(t)

	cmd, _ := setupCmd()
	_ = runWorkerSetup(cmd, "", "", "ws-api", "acos_worker_secret")

	raw, err := keyring.Get("autopus-worker", "credentials")
	require.NoError(t, err)

	var creds map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &creds))
	assert.Equal(t, defaultBackendURL, creds["backend_url"])
	assert.Equal(t, "api_key", creds["auth_type"])
}

// Guards the JWT non-interactive branch: the token is persisted, the workspace ID is
// validated against the backend, and the resolved workspace name is reported.
func TestRunWorkerSetup_TokenModeResolvesWorkspaceAndMemoryAgent(t *testing.T) {
	isolatedSetupEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspaces":
			assert.Equal(t, "Bearer jwt-1", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": "ws-other", "name": "Other"},
					{"id": "ws-1", "name": "Primary"},
				},
			})
		case "/api/v1/workspaces/ws-1/agents":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": "agent-1", "type": "dev_worker", "tier": "worker", "status": "active"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd, out := setupCmd()
	require.NoError(t, runWorkerSetup(cmd, srv.URL, "jwt-1", "ws-1", ""))

	assert.Contains(t, out.String(), "워크스페이스: Primary (ws-1)")
	assert.Contains(t, out.String(), "Setup complete!")

	token, err := setup.LoadAuthToken()
	require.NoError(t, err)
	assert.Equal(t, "jwt-1", token)

	cfg, err := setup.LoadWorkerConfig()
	require.NoError(t, err)
	assert.Equal(t, "ws-1", cfg.WorkspaceID)
	assert.Equal(t, "agent-1", cfg.MemoryAgentID)
}

// Guards workspace validation: an ID the backend does not know must abort setup with
// the lookup context and must not leave a worker config behind.
func TestRunWorkerSetup_UnknownWorkspaceIDFails(t *testing.T) {
	isolatedSetupEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    []map[string]any{{"id": "ws-other", "name": "Other"}},
		})
	}))
	defer srv.Close()

	cmd, _ := setupCmd()
	err := runWorkerSetup(cmd, srv.URL, "jwt-1", "ws-missing", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "워크스페이스 조회")

	_, loadErr := setup.LoadWorkerConfig()
	assert.Error(t, loadErr, "failed setup must not write a worker config")
}

// Guards the interactive workspace step when the backend rejects the token: the
// error is labelled as a selection failure and carries the HTTP status.
func TestRunWorkerSetup_WorkspaceFetchErrorIsLabelled(t *testing.T) {
	isolatedSetupEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "token expired", http.StatusUnauthorized)
	}))
	defer srv.Close()

	cmd, _ := setupCmd()
	err := runWorkerSetup(cmd, srv.URL, "jwt-1", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "워크스페이스 선택")
}

// Guards single-workspace auto-selection: no prompt is needed and the only workspace
// is adopted.
func TestStepSelectWorkspace_AutoSelectsSingleWorkspace(t *testing.T) {
	isolatedSetupEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    []map[string]any{{"id": "ws-solo", "name": "Solo"}},
		})
	}))
	defer srv.Close()

	cmd, out := setupCmd()
	ws, err := stepSelectWorkspace(cmd, srv.URL, "jwt-1")
	require.NoError(t, err)
	assert.Equal(t, "ws-solo", ws.ID)
	assert.Contains(t, out.String(), "워크스페이스: Solo (ws-solo)")

	progress, err := setup.LoadProgress()
	require.NoError(t, err)
	require.NotNil(t, progress)
	assert.Equal(t, 2, progress.Step, "workspace step must record resume progress")
}

// Guards the empty-account case: zero workspaces is an explicit error, not a nil
// workspace handed to the config writer.
func TestStepSelectWorkspace_NoWorkspacesFails(t *testing.T) {
	isolatedSetupEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{}})
	}))
	defer srv.Close()

	cmd, _ := setupCmd()
	ws, err := stepSelectWorkspace(cmd, srv.URL, "jwt-1")
	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "no workspaces available")
}

// Guards device-auth failure reporting: a backend that cannot issue a device code
// must surface the request stage.
func TestStepDeviceAuth_DeviceCodeRequestFailure(t *testing.T) {
	isolatedSetupEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cmd, _ := setupCmd()
	token, err := stepDeviceAuth(cmd, srv.URL)
	require.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "request device code")

	progress, err := setup.LoadProgress()
	require.NoError(t, err)
	require.NotNil(t, progress)
	assert.Equal(t, 1, progress.Step)
}

// Guards the malformed-verification-URI guard: setup must abort before waiting for a
// browser authorization that can never happen.
func TestStepDeviceAuth_EmptyVerificationURIAborts(t *testing.T) {
	isolatedSetupEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"device_code": "dc-1",
				"user_code":   "ABCD-EFGH",
				"expires_in":  600,
				"interval":    5,
			},
		})
	}))
	defer srv.Close()

	cmd, out := setupCmd()
	token, err := stepDeviceAuth(cmd, srv.URL)
	require.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "empty verification URI")
	assert.Contains(t, out.String(), "Code: ABCD-EFGH")
}

// Guards the error wrapper contract: the Korean stage label is kept and the
// technical cause is translated rather than dropped.
func TestUserError_KeepsStageLabelAndCause(t *testing.T) {
	err := userError("인증", errors.New("connection refused"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "인증: ")
	assert.Contains(t, err.Error(), setup.HumanError(errors.New("connection refused")))
}
