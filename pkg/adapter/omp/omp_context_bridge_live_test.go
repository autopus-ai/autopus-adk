package omp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const ompContextBridgeSmokeExtension = `import autopusContextBridge from "./autopus-context.ts";

export default function bridgeSmoke(pi: any): void {
  pi.registerCommand("autopus-context-smoke", {
    description: "Exercise the generated bridge without a provider turn.",
    async handler(_args: string, context: any): Promise<void> {
	const handlers = new Map<string, Function>();
	autopusContextBridge({
	  on(eventName: string, handler: Function): void {
		handlers.set(eventName, handler);
	  },
	} as any);
	const pre = handlers.get("session_before_compact");
	if (pre === undefined) throw new Error("pre bridge handler missing");
	const preResult = await pre(undefined, context);
	if ((preResult as { cancel?: boolean } | undefined)?.cancel === true) {
	  return;
	}
	if (preResult !== undefined) throw new Error("pre bridge returned an authority claim");
	const post = handlers.get("session_compact");
	if (post === undefined) throw new Error("admitted bridge post handler missing");
	if (await post(undefined, context) !== undefined) {
	  throw new Error("post bridge handler returned an authority claim");
	}
    },
  });
}
`

func TestOMPContextBridge_InstalledRPCConfirmIsCorrelatedWithoutProvider(t *testing.T) {
	executable, err := exec.LookPath(cliBinary)
	if err != nil {
		t.Skip("installed omp binary is required for the bridge load smoke")
	}

	workspace := t.TempDir()
	cfg := optedInOMPContextBridgeConfig()
	require.NoError(t, config.Save(workspace, cfg))
	_, err = NewWithRoot(workspace).Generate(context.Background(), cfg)
	require.NoError(t, err)
	smokePath := filepath.Join(workspace, ".omp", "extensions", "autopus-context-smoke.ts")
	require.NoError(t, os.WriteFile(smokePath, []byte(ompContextBridgeSmokeExtension), 0o644))

	profile := t.TempDir()
	var providerRequests atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerRequests.Add(1)
		http.Error(w, "bridge smoke must not dispatch", http.StatusTeapot)
	}))
	t.Cleanup(provider.Close)
	writeOMPLiveModelConfig(t, profile, provider.URL)
	overlay := filepath.Join(profile, "bridge-smoke-config.yml")
	require.NoError(t, os.WriteFile(overlay, []byte("skills: {}\n"), 0o600))
	version := probeOMPLiveVersion(t, executable, profile, overlay)
	t.Logf("installed bridge smoke version: %s", version)

	baseEnv, err := isolatedOMPLiveEnv(profile, overlay)
	require.NoError(t, err)
	hashes := map[string]string{
		"binding_hash": "sha256:" + strings.Repeat("a", 64),
		"options_hash": "sha256:" + strings.Repeat("b", 64),
		"session_hash": "sha256:" + strings.Repeat("c", 64),
		"nonce_hash":   "sha256:" + strings.Repeat("d", 64),
	}
	validEnv := append(append([]string(nil), baseEnv...),
		"AUTOPUS_OMP_CONTEXT_BINDING_HASH="+hashes["binding_hash"],
		"AUTOPUS_OMP_CONTEXT_OPTIONS_HASH="+hashes["options_hash"],
		"AUTOPUS_OMP_CONTEXT_SESSION_HASH="+hashes["session_hash"],
		"AUTOPUS_OMP_CONTEXT_NONCE_HASH="+hashes["nonce_hash"],
		"AUTOPUS_OMP_CONTEXT_ACK_TIMEOUT_MS=500",
	)

	frames, stderr := runOMPContextBridgeStartup(t, executable, workspace, overlay, provider.URL, validEnv, 2)
	assert.NotContains(t, stderr, "Failed to load extension")
	assert.NotContains(t, stderr, "autopus-context.ts")
	assertOMPContextBridgeRPCEnvelopes(t, frames, hashes)
	assertNoOMPProviderActivity(t, frames)

	invalidHashEnvironments := map[string][]string{
		"missing one hash": append(append([]string(nil), baseEnv...),
			"AUTOPUS_OMP_CONTEXT_BINDING_HASH="+hashes["binding_hash"],
			"AUTOPUS_OMP_CONTEXT_OPTIONS_HASH="+hashes["options_hash"],
		),
		"uppercase hash": append(append([]string(nil), baseEnv...),
			"AUTOPUS_OMP_CONTEXT_BINDING_HASH="+hashes["binding_hash"],
			"AUTOPUS_OMP_CONTEXT_OPTIONS_HASH=sha256:"+strings.Repeat("B", 64),
			"AUTOPUS_OMP_CONTEXT_SESSION_HASH="+hashes["session_hash"],
		),
	}
	for name, invalidEnv := range invalidHashEnvironments {
		t.Run(name, func(t *testing.T) {
			invalidFrames, invalidStderr := runOMPContextBridgeStartup(
				t, executable, workspace, overlay, provider.URL, invalidEnv, 0,
			)
			assert.NotContains(t, invalidStderr, "Failed to load extension")
			for _, frame := range invalidFrames {
				_, bridgeConfirm := parseOMPContextBridgeConfirm(frame)
				assert.False(t, bridgeConfirm, printableOMPFrame(frame))
			}
			assertNoOMPProviderActivity(t, invalidFrames)
		})
	}
	assert.Zero(t, providerRequests.Load(), "startup and bridge forwarding must not dispatch to a provider")
}

func runOMPContextBridgeStartup(
	t *testing.T,
	executable, workspace, overlay, allowedEndpoint string,
	env []string,
	expectedBridgeEvents int,
) ([][]byte, string) {
	t.Helper()
	// Backstop only. omp enforces its own --max-time below; a Go-side deadline
	// close to it preempts the tool under load and reports a false rpc_timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable,
		"--mode", "rpc",
		"--no-session",
		"--cwd", workspace,
		"--model", "s7dummy/"+ompLiveModel,
		"--config", overlay,
		"--no-tools",
		"--no-skills",
		"--no-rules",
		"--no-lsp",
		"--no-pty",
		"--max-time", "10s",
	)
	cmd.Dir = workspace
	cmd.Env = env
	cmd.WaitDelay = ompRPCWaitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, configureOMPRPCNetworkSandbox(cmd, allowedEndpoint))
	require.NoError(t, configureOMPRPCProcessGroup(cmd))
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	waited := false
	defer func() {
		_ = stdin.Close()
		if !waited {
			_ = terminateOMPRPCProcessGroup(cmd)
			_ = cmd.Wait()
		}
		reapOMPRPCProcessGroup(cmd)
	}()

	stream, done := scanOMPRPCFrames(stdout)
	frames := make([][]byte, 0, 16)
	ready := false
	commandAvailable := false
	commandSent := false
	commandCompleted := false
	bridgeEvents := 0
	fenceSent := false
	fenceCompleted := false
	encoder := json.NewEncoder(stdin)
	for !ready || !commandSent || !commandCompleted || bridgeEvents < expectedBridgeEvents || !fenceCompleted {
		frame, readErr := nextOMPRPCFrame(ctx, stream, done)
		if readErr != nil {
			t.Fatalf("installed bridge RPC failed: %v stderr=%q frames=%s",
				readErr, stderr.String(), summarizeOMPFrames(frames))
		}
		frames = append(frames, frame)
		ready = ready || rpcFrameType(frame) == "ready"
		commandAvailable = commandAvailable || ompRPCFrameHasCommand(frame, "autopus-context-smoke")
		if ready && commandAvailable && !commandSent {
			require.NoError(t, encoder.Encode(map[string]any{
				"id": "bridge-smoke", "type": "prompt", "message": "/autopus-context-smoke",
			}))
			commandSent = true
		}
		if request, isBridgeConfirm := parseOMPContextBridgeConfirm(frame); isBridgeConfirm {
			bridgeEvents++
			require.NoError(t, encoder.Encode(map[string]any{
				"type": "extension_ui_response", "id": "wrong-" + request.ID, "confirmed": false,
			}))
			require.NoError(t, encoder.Encode(map[string]any{
				"type": "extension_ui_response", "id": request.ID, "confirmed": true,
			}))
			require.NoError(t, encoder.Encode(map[string]any{
				"type": "extension_ui_response", "id": request.ID, "confirmed": false,
			}))
		}
		commandCompleted = commandCompleted || ompRPCFrameIsSuccessfulResponse(frame, "bridge-smoke")
		fenceCompleted = fenceCompleted || ompRPCFrameIsSuccessfulResponse(frame, "bridge-fence")
		if commandCompleted && bridgeEvents >= expectedBridgeEvents && !fenceSent {
			require.NoError(t, encoder.Encode(map[string]any{"id": "bridge-fence", "type": "get_state"}))
			fenceSent = true
		}
	}
	require.NoError(t, stdin.Close())
	require.NoError(t, cmd.Wait(), "stderr: %s", stderr.String())
	waited = true
	return frames, stderr.String()
}
