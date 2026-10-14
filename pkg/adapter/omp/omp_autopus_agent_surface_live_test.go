package omp

// TestOMPAutopusAgentSurfaceLiveSmoke runs the pinned omp over a project
// generated in the default autopus mode (SPEC-MODELUX-003) and reads the task
// tool the parent conversation is offered. It must list exactly the 16
// Autopus agents and task: project agent discovery adds the first, and the
// task.disabledAgents claim in .omp/config.yml hides every bundled agent but
// task, the five 17.2.7 bundles beside it, with the profile overlay of the
// live harness in effect. The fake provider answers once, so no child is
// spawned and no model is called.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const ompAgentSurfaceSmokeToken = "AUTOPUS_OMP_AGENT_SURFACE_SMOKE"

func TestOMPAutopusAgentSurfaceLiveSmoke(t *testing.T) {
	if os.Getenv("AUTOPUS_OMP_NATIVE_LIVE") != "1" {
		t.Skip("set AUTOPUS_OMP_NATIVE_LIVE=1 to run the isolated autopus agent surface smoke")
	}
	for _, key := range []string{
		"ANTHROPIC_API_KEY", "GITHUB_TOKEN", "OPENAI_API_KEY", "OMP_ACCESS_TOKEN", "PI_PROVIDER_TOKEN",
	} {
		t.Setenv(key, ompNativeCredential)
	}
	executable := resolveOMPNativeLiveExecutable(t)
	scratch := generateOMPOnly(t)
	for _, agent := range config.CanonicalAgentNames() {
		require.FileExists(t, scratch+"/.omp/agents/"+agent+".md")
	}
	claimed, err := os.ReadFile(scratch + "/" + configFile)
	require.NoError(t, err)
	require.Equal(t, claimSurfaceOnly, string(claimed))
	baseline := snapshotOMPNativeTree(t, scratch)

	provider := newOMPAgentSurfaceProvider(t)
	profile := t.TempDir()
	writeOMPLiveModelConfig(t, profile, provider.server.URL)
	overlay := writeOMPNativeLiveOverlay(t, profile)
	version := probeOMPLiveVersion(t, executable, profile, overlay)
	require.Equal(t, "omp/17.2.7", version, "the agent surface is pinned to the supported OMP release")

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	prompt := ompAgentSurfaceSmokeToken + ": answer without calling any tool."
	runErr := runOMPAgentSurfaceRPC(ctx, executable, scratch, profile, overlay, prompt, provider.server.URL)
	receipt := provider.receipt()
	require.NoError(t, runErr, "agent surface RPC failed: provider=%+v", receipt)
	require.Empty(t, receipt.failure)
	require.Equal(t, 1, receipt.requests)
	require.Zero(t, receipt.authHeaders)
	require.Zero(t, receipt.credentialLeaks)

	listed := ompTaskDefinitionAgents(t, receipt.taskDefinition)
	for _, agent := range config.OMPAutopusAgentNames() {
		assert.Contains(t, listed, agent, "project agent discovery or the default worker is missing")
	}
	for _, agent := range config.OMPAutopusDisabledAgents() {
		assert.NotContains(t, listed, agent, "task.disabledAgents did not hide a bundled agent")
	}
	assert.ElementsMatch(t, config.OMPAutopusAgentNames(), listed, "the task tool lists an agent beyond the Autopus set")
	require.Equal(t, baseline, snapshotOMPNativeTree(t, scratch), "the session must leave the project byte-identical")
	require.Empty(t, findOMPNativeResidualArtifacts(t, scratch, profile))

	encoded, err := json.Marshal(map[string]any{
		"version": version, "listed": len(listed), "autopus_and_task": len(config.OMPAutopusAgentNames()),
		"disabled_absent": config.OMPAutopusDisabledAgents(),
	})
	require.NoError(t, err)
	t.Logf("sanitized_autopus_agent_surface_receipt=%s", encoded)
}

// ompTaskDefinitionAgents returns the agent names the task tool lists under
// its "# Available Agents" heading, one "### <name>" line each. A heading can
// carry a note after the name, as "### scout (READ-ONLY)" does.
func ompTaskDefinitionAgents(t *testing.T, definition string) []string {
	t.Helper()
	var tool struct {
		Function struct {
			Description string `json:"description"`
		} `json:"function"`
	}
	require.NoError(t, json.Unmarshal([]byte(definition), &tool), "task tool definition missing")
	_, section, found := strings.Cut(tool.Function.Description, "\n# Available Agents\n")
	require.True(t, found, "the task tool lists no available agents")
	var agents []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "# ") {
			break
		}
		if heading, ok := strings.CutPrefix(line, "### "); ok {
			if fields := strings.Fields(heading); len(fields) > 0 {
				agents = append(agents, fields[0])
			}
		}
	}
	require.NotEmpty(t, agents)
	return agents
}

type ompAgentSurfaceReceipt struct {
	requests, authHeaders, credentialLeaks int
	taskDefinition, failure                string
}

// ompAgentSurfaceProvider is a loopback provider that records the task tool
// of the first parent request and finishes the turn without a tool call.
type ompAgentSurfaceProvider struct {
	server *httptest.Server
	mu     sync.Mutex
	state  ompAgentSurfaceReceipt
}

func newOMPAgentSurfaceProvider(t *testing.T) *ompAgentSurfaceProvider {
	t.Helper()
	provider := &ompAgentSurfaceProvider{}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *ompAgentSurfaceProvider) handle(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.requests++
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		p.reject(w, "unexpected_endpoint")
		return
	}
	if ompNativeRequestHasAuth(r.Header) {
		p.state.authHeaders++
		p.reject(w, "unexpected_auth_header")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		p.reject(w, "request_oversized")
		return
	}
	if strings.Contains(string(body), ompNativeCredential) {
		p.state.credentialLeaks++
		p.reject(w, "credential_leak")
		return
	}
	var request ompNativeChatRequest
	if json.Unmarshal(body, &request) != nil || request.Model != ompLiveModel || !request.Stream {
		p.reject(w, "invalid_request_shape")
		return
	}
	if !strings.Contains(string(body), ompAgentSurfaceSmokeToken) || p.state.taskDefinition != "" {
		p.reject(w, "unexpected_request")
		return
	}
	p.state.taskDefinition = ompNativeToolDefinition(request.Tools, "task")
	writeOMPNativeFinalSSE(w, 1, "agent surface observed")
}

// reject records the first failure; the caller holds p.mu.
func (p *ompAgentSurfaceProvider) reject(w http.ResponseWriter, reason string) {
	if p.state.failure == "" {
		p.state.failure = reason
	}
	http.Error(w, "agent surface fixture rejected request", http.StatusBadRequest)
}

func (p *ompAgentSurfaceProvider) receipt() ompAgentSurfaceReceipt {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// runOMPAgentSurfaceRPC runs one RPC prompt in the loopback-only sandbox the
// native smoke uses and returns once the assistant turn stops.
func runOMPAgentSurfaceRPC(ctx context.Context, executable, scratch, profile, overlay, prompt, endpoint string) error {
	cmd := exec.CommandContext(ctx, executable,
		"--mode", "rpc", "--no-session", "--cwd", scratch,
		"--model", "s7dummy/"+ompLiveModel, "--config", overlay, "--tools", "task,hub",
		"--auto-approve", "--no-extensions", "--no-rules", "--no-lsp", "--no-pty", "--max-time", "30s",
	)
	cmd.Dir = scratch
	env, err := isolatedOMPLiveEnv(profile, overlay)
	if err != nil {
		return fmt.Errorf("agent_surface_rpc_isolation_invalid")
	}
	cmd.Env = env
	cmd.Stderr = io.Discard
	cmd.WaitDelay = ompRPCWaitDelay
	if err := configureOMPRPCNetworkSandbox(cmd, endpoint); err != nil {
		return fmt.Errorf("agent_surface_rpc_network_sandbox_unavailable: %w", err)
	}
	if err := configureOMPRPCProcessGroup(cmd); err != nil {
		return fmt.Errorf("agent_surface_rpc_process_group_unsupported")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("agent_surface_rpc_stdin_pipe")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("agent_surface_rpc_stdout_pipe")
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("agent_surface_rpc_start")
	}
	waited, completed := false, false
	defer func() {
		_ = stdin.Close()
		if !completed {
			_ = terminateOMPRPCProcessGroup(cmd)
		}
		if !waited {
			_ = cmd.Wait()
		}
	}()
	frames, scanDone := scanOMPRPCFrames(stdout)
	for ready := false; !ready; {
		frame, err := nextOMPRPCFrame(ctx, frames, scanDone)
		if err != nil {
			return err
		}
		ready = rpcFrameType(frame) == "ready"
	}
	encoder := json.NewEncoder(stdin)
	for _, command := range []map[string]any{
		{"id": "disable-retry", "type": "set_auto_retry", "enabled": false},
		{"id": "disable-compaction", "type": "set_auto_compaction", "enabled": false},
		{"id": "agent-surface-prompt", "type": "prompt", "message": prompt},
	} {
		if encoder.Encode(command) != nil {
			return fmt.Errorf("agent_surface_rpc_write_command")
		}
	}
	if err := awaitOMPAgentSurfaceStop(ctx, frames, scanDone); err != nil {
		return err
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("agent_surface_rpc_close_stdin")
	}
	waitErr := cmd.Wait()
	waited = true
	if waitErr != nil {
		return fmt.Errorf("agent_surface_rpc_exit_nonzero")
	}
	completed = true
	return nil
}

func awaitOMPAgentSurfaceStop(ctx context.Context, frames <-chan []byte, scanDone <-chan error) error {
	for {
		frame, err := nextOMPRPCFrame(ctx, frames, scanDone)
		if err != nil {
			return err
		}
		var event struct {
			Type    string `json:"type"`
			Success *bool  `json:"success"`
			Message struct {
				Role       string `json:"role"`
				StopReason string `json:"stopReason"`
			} `json:"message"`
		}
		if json.Unmarshal(frame, &event) != nil {
			continue
		}
		if event.Type == "response" && event.Success != nil && !*event.Success {
			return fmt.Errorf("agent_surface_rpc_response_failed")
		}
		if event.Type == "message_end" && event.Message.Role == "assistant" {
			if event.Message.StopReason != "stop" {
				return fmt.Errorf("agent_surface_rpc_stopped:%s", event.Message.StopReason)
			}
			return nil
		}
	}
}
