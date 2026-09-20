package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	ompadapter "github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	pipelineOMPActiveEndpointKey    = "AUTOPUS_OMP_CONTEXT_PROVIDER_ENDPOINT"
	pipelineOMPActiveCredentialKey  = "AUTOPUS_OMP_CONTEXT_PROVIDER_TOKEN"
	pipelineOMPActiveRPCIdentity    = "autopus.omp-pipeline-managed-rpc.v3"
	pipelineOMPActivePolicyIdentity = "manual-compact-completed-history-before-reused-call;canonical-ephemeral-readmission;correlated-ack;provider-bound-endpoint;model-image-capability=required;snapcompact-image-schema=omp-v17.2.7;auto-compaction=off;retry-off;ambient-off;sandbox=candidate-managed|producer-inherited-external-live-image-darwin-v3;tools=read,bash,edit,write,grep,glob,todo"
)

const pipelineOMPActiveDefaultContextWindow = 262144

type pipelineOMPActiveProcessConfig struct {
	backend              pipelineOMPBackendConfig
	candidate            pipelineOMPManagedActiveCandidate
	prepared             pipelineOMPManagedActivePrepared
	binding              WorkflowContextBridgeBinding
	endpoint, credential string
	sandboxMode          pipelineOMPActiveSandboxMode
	// probeMethodOrder writes the probe overlay body, whose compaction
	// methodOrder is [remote, snapcompact]. Production keeps [snapcompact].
	probeMethodOrder bool
}

func preparePipelineOMPActiveProcessConfig(
	backend pipelineOMPBackendConfig,
	candidate pipelineOMPManagedActiveCandidate,
	prepared pipelineOMPManagedActivePrepared,
) (pipelineOMPActiveProcessConfig, error) {
	endpointRaw, endpointFound := pipelineOMPEnvironmentValue(backend.Environment, pipelineOMPActiveEndpointKey)
	credential, credentialFound := pipelineOMPEnvironmentValue(backend.Environment, pipelineOMPActiveCredentialKey)
	endpoint, err := validatePipelineOMPActiveEndpoint(endpointRaw)
	if !endpointFound || !credentialFound || err != nil {
		return pipelineOMPActiveProcessConfig{}, errors.New("pipeline: managed active broker authority is unavailable")
	}
	nonce, err := newWorkflowContextRunNonceHash()
	if err != nil {
		return pipelineOMPActiveProcessConfig{}, err
	}
	implementation := pipelineOMPActiveImplementationDigest()
	providerAuthority, err := pipelineOMPActiveProviderAuthorityDigest(
		prepared.Binding.PolicyDigest, implementation, candidate.ModelScopeDigest,
		backend.ModelContextWindow, endpoint, credential,
	)
	if err != nil {
		return pipelineOMPActiveProcessConfig{}, err
	}
	return pipelineOMPActiveProcessConfig{
		backend: backend, candidate: candidate, prepared: prepared, endpoint: endpoint, credential: credential,
		binding: WorkflowContextBridgeBinding{
			SchemaVersion: workflowContextBridgeSchemaVersion,
			BindingHash: workflowContextRuntimeHash(strings.Join([]string{
				prepared.Binding.GrantDigest, prepared.Binding.WorkspaceID, prepared.Binding.SpecID,
				prepared.Binding.GitCommitHash, prepared.Binding.AutoSourceCommit, prepared.Binding.AutoSourceTree,
				candidate.ScopeProvider, candidate.ModelScopeDigest, providerAuthority,
			}, "\x00")),
			OptionsHash: providerAuthority,
			SessionHash: workflowContextRuntimeHash(prepared.Binding.WorkspaceID + "\x00" + prepared.Binding.SpecID + "\x00" + prepared.Binding.GitCommitHash),
			NonceHash:   nonce,
		},
	}, nil
}

// @AX:WARN [AUTO]: managed active process startup contains 18 if branches.
// @AX:REASON [AUTO]: runtime ownership, executable identity, overlay, sandbox, process group, pipes, and readiness gates converge before admission.
func startPipelineOMPActiveProcess(
	ctx context.Context,
	active pipelineOMPActiveProcessConfig,
) (result *pipelineOMPProcess, resultErr error) {
	runtimeRoot, err := os.MkdirTemp(active.backend.RuntimeBase, "pipeline-active-")
	if err != nil {
		return nil, fmt.Errorf("create managed active runtime: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(runtimeRoot) }
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		cleanup()
		return nil, err
	}
	runtimeInfo, err := os.Lstat(runtimeRoot)
	if err != nil {
		cleanup()
		return nil, err
	}
	sessionDir := filepath.Join(runtimeRoot, "sessions")
	if err := os.Mkdir(sessionDir, 0o700); err != nil {
		cleanup()
		return nil, err
	}
	privateExecutable, err := materializePipelineOMPExecutable(
		active.backend.Executable, active.backend.executableID, runtimeRoot,
	)
	if err != nil {
		cleanup()
		return nil, err
	}
	bridgePath, err := materializePipelineOMPActiveBridge(runtimeRoot)
	if err != nil {
		cleanup()
		return nil, err
	}
	overlay := newWorkflowContextManagedManualCompactionOverlay
	if active.probeMethodOrder {
		overlay = newWorkflowContextManagedProbeCompactionOverlay
	}
	configPath, err := overlay(runtimeRoot, config.OMPContextMemoryOff)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := writePipelineOMPActiveModels(runtimeRoot, active); err != nil {
		cleanup()
		return nil, err
	}
	environment, err := pipelineOMPActiveEnvironment(runtimeRoot, configPath, active)
	if err != nil {
		cleanup()
		return nil, err
	}
	args := []string{
		"--mode", "rpc", "--no-session", "--no-extensions", "-e", bridgePath,
		"--session-dir", sessionDir,
		"--cwd", active.backend.ProjectDir, "--model", active.candidate.Provider + "/" + active.candidate.Model,
		"--config", configPath, "--tools", "read,bash,edit,write,grep,glob,todo",
		"--no-skills", "--no-rules", "--no-lsp", "--no-pty", "--no-title",
		"--max-time", pipelineOMPMaxTimeSeconds(active.backend.MaxTime),
	}
	privateExecutable, privateIdentity, err := canonicalPipelineOMPExecutable(privateExecutable)
	if err != nil || privateIdentity.digest != active.backend.executableID.digest {
		cleanup()
		return nil, errors.New("managed active OMP child executable identity is invalid")
	}
	verifiedCommand, err := newPipelineOMPVerifiedExecCommandWithGate(ctx, privateExecutable, privateIdentity, args...)
	if err != nil {
		cleanup()
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, verifiedCommand.Close()) }()
	cmd := verifiedCommand.cmd
	// The child's stderr used to go to io.Discard, which made a startup failure
	// unexplainable: the Darwin ptrace gate reports "context deadline exceeded"
	// and the only text that could say why was thrown away. Bounded so a chatty
	// child cannot grow this without limit, and only used on the failure paths.
	childStderr := &boundedOutput{limit: pipelineOMPActiveStderrTailBytes}
	cmd.Dir, cmd.Env, cmd.Stderr = active.backend.ProjectDir,
		workflowContextManagedRPCEnvironment(environment, active.binding), childStderr
	cmd.WaitDelay = 500 * time.Millisecond
	if err := configurePipelineOMPActiveSandbox(cmd, active.endpoint, active.sandboxMode); err != nil {
		cleanup()
		return nil, err
	}
	if err := configurePipelineOMPVerifiedExecSandboxMode(verifiedCommand, active.sandboxMode, true); err != nil {
		cleanup()
		return nil, err
	}
	if err := configureWorkflowContextManagedRPCProcessGroup(cmd); err != nil {
		cleanup()
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cleanup()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		cleanup()
		return nil, err
	}
	if err := verifiedCommand.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		cleanup()
		return nil, fmt.Errorf("start managed active OMP RPC: %w%s", err,
			pipelineOMPActiveStderrDetail(childStderr))
	}
	frameCtx, stopFrames := context.WithCancel(context.Background())
	frames, done := readPipelineOMPFrames(frameCtx, stdout)
	process := &pipelineOMPProcess{
		cmd: cmd, stdin: stdin, frames: frames, done: done,
		runtimeRoot: runtimeRoot, runtimeInfo: runtimeInfo, stopFrames: stopFrames,
	}
	readyCtx, cancel := context.WithTimeout(ctx, active.backend.MaxTime)
	defer cancel()
	frame, err := process.next(readyCtx)
	if err != nil || frame.Type != "ready" {
		detail := pipelineOMPActiveStderrDetail(childStderr)
		_ = process.Close()
		return nil, fmt.Errorf("managed active OMP RPC readiness was not observed%s", detail)
	}
	return process, nil
}

func materializePipelineOMPActiveBridge(runtimeRoot string) (string, error) {
	identity := ompadapter.ExpectedOMPContextBridgeSourceIdentity()
	source := ompadapter.ExpectedOMPContextBridgeSource()
	if int64(len(source)) != identity.Size || pipelineOMPActiveHash(source) != "sha256:"+identity.SHA256 {
		return "", errors.New("pipeline: embedded managed active bridge identity is invalid")
	}
	extensions := filepath.Join(runtimeRoot, "extensions")
	if err := os.Mkdir(extensions, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(extensions, "autopus-context.ts")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		return "", err
	}
	actual, err := captureWorkflowContextManagedSourceIdentity(path)
	if err != nil || actual.size != identity.Size || actual.sha256 != identity.SHA256 || actual.mode.Perm() != 0o600 {
		return "", errors.New("pipeline: private managed active bridge identity is invalid")
	}
	return path, nil
}

func pipelineOMPActiveEnvironment(
	runtimeRoot, configPath string,
	active pipelineOMPActiveProcessConfig,
) ([]string, error) {
	paths := map[string]string{
		"HOME": filepath.Join(runtimeRoot, "home"), "TMPDIR": filepath.Join(runtimeRoot, "tmp"),
		"XDG_CACHE_HOME": filepath.Join(runtimeRoot, "cache"), "XDG_CONFIG_HOME": filepath.Join(runtimeRoot, "config"),
		"XDG_DATA_HOME": filepath.Join(runtimeRoot, "data"), "XDG_STATE_HOME": filepath.Join(runtimeRoot, "state"),
	}
	result := []string{"PI_CODING_AGENT_DIR=" + runtimeRoot, "PI_CONFIG_FILES=" + configPath,
		pipelineOMPActiveCredentialKey + "=" + active.credential}
	for key, path := range paths {
		if err := os.Mkdir(path, 0o700); err != nil {
			return nil, err
		}
		result = append(result, key+"="+path)
	}
	if pathValue, found := pipelineOMPEnvironmentValue(active.backend.Environment, "PATH"); found {
		result = append(result, "PATH="+pathValue)
	}
	return result, nil
}

func writePipelineOMPActiveModels(runtimeRoot string, active pipelineOMPActiveProcessConfig) error {
	models := make(map[string]map[string]bool)
	for _, selector := range active.backend.PhaseModels {
		provider, model, ok := strings.Cut(selector, "/")
		if !ok || !safePipelineOMPToken(provider) || !safePipelineOMPToken(model) {
			return errors.New("pipeline: managed active model catalog is invalid")
		}
		if models[provider] == nil {
			models[provider] = make(map[string]bool)
		}
		models[provider][model] = true
	}
	providers := make([]string, 0, len(models))
	for provider := range models {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	var body strings.Builder
	body.WriteString("providers:\n")
	for _, provider := range providers {
		fmt.Fprintf(&body, "  %s:\n    baseUrl: %s/v1\n    apiKey: %s\n    authHeader: true\n    api: openai-completions\n    models:\n",
			provider, active.endpoint, pipelineOMPActiveCredentialKey)
		ids := make([]string, 0, len(models[provider]))
		for id := range models[provider] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			// Preserve capabilities from the pinned OMP catalog for known models; OMP defaults
			// unknown custom models to text-only, which the managed startup gate rejects.
			fmt.Fprintf(&body, "      - id: %s\n        name: Managed Active\n        reasoning: true\n        contextWindow: %d\n        maxTokens: 32768\n",
				id, active.backend.ModelContextWindow)
		}
	}
	path := filepath.Join(runtimeRoot, "models.yml")
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
