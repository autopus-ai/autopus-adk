package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func observeCallTestDriver(t *testing.T) *WorkflowContextManagedRPCDriver {
	t.Helper()
	base := t.TempDir()
	require.NoError(t, os.Chmod(base, 0o700))
	workspace := filepath.Join(base, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o700))
	installWorkflowContextManagedLiveBridge(t, workspace)
	runtimeRoot := filepath.Join(base, "omp-runtime")
	sessionDir := filepath.Join(runtimeRoot, "sessions")
	require.NoError(t, os.MkdirAll(sessionDir, 0o700))
	configPath := filepath.Join(runtimeRoot, "config.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("compaction: {}\n"), 0o600))
	driver, err := NewWorkflowContextManagedRPCDriver(WorkflowContextManagedRPCOptions{
		Executable: "/usr/bin/true", Workspace: workspace, RuntimeBase: base,
		RuntimeRoot: runtimeRoot, SessionDir: sessionDir, ConfigPath: configPath,
		Model: "fixture/model", AllowedEndpoint: "http://127.0.0.1:43127",
		Environment: []string{"PI_CODING_AGENT_DIR=" + runtimeRoot, "PI_CONFIG_FILES=" + configPath},
	})
	require.NoError(t, err)
	return driver
}

// Each row is a different missing piece of provider evidence; accepting any of
// them would let the canary publish token deltas it never observed.
func TestValidateWorkflowContextObserveOutput_RejectsIncompleteProviderEvidence(t *testing.T) {
	t.Parallel()
	complete := WorkflowContextProviderUsage{PrimaryInputTokens: 10, PrimaryOutputTokens: 3, TotalTokens: 13}
	require.NoError(t, validateWorkflowContextObserveOutput("answer", complete))

	missingOutput := complete
	require.Error(t, validateWorkflowContextObserveOutput("", missingOutput))
	noInput := complete
	noInput.PrimaryInputTokens = 0
	require.ErrorContains(t, validateWorkflowContextObserveOutput("answer", noInput), "result is incomplete")
	noPrimary := complete
	noPrimary.PrimaryOutputTokens = 0
	require.Error(t, validateWorkflowContextObserveOutput("answer", noPrimary))
	noTotal := complete
	noTotal.TotalTokens = 0
	require.Error(t, validateWorkflowContextObserveOutput("answer", noTotal))
}

func TestBuildWorkflowContextObserveAdmission_RefusesIncompleteCanonicalLayer(t *testing.T) {
	t.Parallel()
	ephemeral := promptlayer.OMPContextEphemeral{OriginalTask: "/auto go SPEC", DecisionDelta: "delta"}
	for _, layer := range []promptlayer.Layer{
		{SourceRef: "", Content: "body"},
		{SourceRef: "AGENTS.md", Content: ""},
	} {
		delivery := promptlayer.ContextDeliveryResult{Prompt: "canonical", Layers: []promptlayer.Layer{layer}}
		_, err := buildWorkflowContextObserveAdmission("full", delivery, ephemeral)
		require.ErrorContains(t, err, "canonical document is incomplete")
	}
}

func TestBuildWorkflowContextObserveAdmission_VariantSelectsDispatchModeAndCarriesDocuments(t *testing.T) {
	t.Parallel()
	delivery := promptlayer.ContextDeliveryResult{
		Prompt: "CANONICAL-PROMPT",
		Layers: []promptlayer.Layer{
			{SourceRef: "AGENTS.md", Content: "AGENT-BODY"},
			{SourceRef: "spec.md", Content: "SPEC-BODY"},
		},
	}
	ephemeral := promptlayer.OMPContextEphemeral{OriginalTask: "/auto go SPEC-OMP-004", DecisionDelta: "DELTA"}

	for variant, mode := range map[string]string{
		"full":      WorkflowContextDispatchCanonicalFull,
		"optimized": WorkflowContextDispatchOptimized,
	} {
		encoded, err := buildWorkflowContextObserveAdmission(variant, delivery, ephemeral)
		require.NoError(t, err)
		var admission workflowContextManagedAdmission
		require.NoError(t, json.Unmarshal([]byte(encoded), &admission))
		assert.Equal(t, workflowContextManagedAdmissionSchemaVersion, admission.SchemaVersion)
		assert.Equal(t, mode, admission.Mode)
		assert.Equal(t, "CANONICAL-PROMPT", admission.CanonicalPrompt)
		assert.Equal(t, "/auto go SPEC-OMP-004", admission.OriginalTask)
		assert.Equal(t, "DELTA", admission.DecisionDelta)
		require.Len(t, admission.Documents, 2)
		assert.Equal(t, "AGENT-BODY", admission.Documents[0].Body)
		assert.Empty(t, admission.MemoryInjections)
		assert.Empty(t, admission.DocumentOmissions)
	}
}

func TestBuildWorkflowContextObserveAdmission_RefusesOversizedAdmissionFrame(t *testing.T) {
	t.Parallel()
	oversized := make([]byte, workflowContextManagedRPCMaxInputFrameBytes)
	for index := range oversized {
		oversized[index] = 'x'
	}
	delivery := promptlayer.ContextDeliveryResult{
		Prompt: "prompt",
		Layers: []promptlayer.Layer{{SourceRef: "big.md", Content: string(oversized)}},
	}
	_, err := buildWorkflowContextObserveAdmission("full", delivery, promptlayer.OMPContextEphemeral{})
	require.ErrorContains(t, err, "canonical admission is unavailable")
}

func TestObserveLifecycleFacts_ReportMirrorsObservedCountersAndTerminalState(t *testing.T) {
	t.Parallel()
	observation := WorkflowContextManagedRPCObservation{
		PreACKs: 2, PostACKs: 1, NativeStarts: 3, NativeEnds: 4, ProviderTurns: 5,
		SameProcess: true, SameSession: true, Sandboxed: true,
	}
	facts := observeLifecycleFacts(observation, false)
	assert.Equal(t, 2, facts.PreCompactionEvents)
	assert.Equal(t, 1, facts.PostCompactionEvents)
	assert.Equal(t, 3, facts.NativeStarts)
	assert.Equal(t, 4, facts.NativeEnds)
	assert.Equal(t, 5, facts.ProviderTurns)
	// A failed run must never be reported as terminally idle.
	assert.False(t, facts.TerminalIdle)
	assert.True(t, observeLifecycleFacts(observation, true).TerminalIdle)
}

func TestBindWorkflowContextObserveDriver_DerivesTaskScopedHashesWithFreshNonce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	first := observeCallTestDriver(t)
	second := observeCallTestDriver(t)
	require.NoError(t, bindWorkflowContextObserveDriver(ctx, first, "task-01"))
	require.NoError(t, bindWorkflowContextObserveDriver(ctx, second, "task-01"))

	assert.Equal(t, workflowContextBridgeSchemaVersion, first.binding.SchemaVersion)
	assert.Equal(t, first.binding.BindingHash, second.binding.BindingHash)
	assert.Equal(t, first.binding.SessionHash, second.binding.SessionHash)
	assert.NotEqual(t, first.binding.BindingHash, first.binding.SessionHash)
	// Replay protection: the run nonce must never repeat across bindings.
	assert.NotEqual(t, first.binding.NonceHash, second.binding.NonceHash)
	require.ErrorContains(t, bindWorkflowContextObserveDriver(ctx, first, "task-01"), "not replaceable")
}

func TestObserveRunCanonicalPrimary_RefusesUnboundAndClosedDrivers(t *testing.T) {
	t.Parallel()
	unbound := observeCallTestDriver(t)
	_, _, err := unbound.runCanonicalPrimary(context.Background(), "prompt")
	require.ErrorContains(t, err, "driver is not ready")
	assert.False(t, unbound.Observation().ProviderObserved)

	closed := observeCallTestDriver(t)
	require.NoError(t, bindWorkflowContextObserveDriver(context.Background(), closed, "task-02"))
	require.NoError(t, closed.Cleanup(context.Background()))
	_, usage, err := closed.runCanonicalPrimary(context.TODO(), "prompt")
	require.ErrorContains(t, err, "driver is not ready")
	assert.Zero(t, usage.TotalTokens)
}

func TestRunWorkflowContextObserveCall_FailsClosedBeforeAnyProviderTurn(t *testing.T) {
	project := t.TempDir()
	writeWorkflowContextObserveCanonicalDocuments(t, project, "SPEC-OMP-004")
	base := workflowContextObserveCallOptions{
		ProjectDir: project, SpecID: "SPEC-OMP-004", Provider: "openai", Model: "gpt-5.6-sol",
		Endpoint: "http://127.0.0.1:43123", CredentialLocator: "AUTOPUS_TEST_OBSERVE_CALL_TOKEN",
		Executable: os.Args[0],
	}
	request := workflowContextObserveCallRequest{
		SchemaVersion: workflowContextObserveCallRequestSchema, Sequence: 1, PairSequence: 1,
		TaskID: "task-01", Prompt: "decision delta", Variant: "full",
	}

	// Credential absent: the canary must refuse before it creates a task root.
	result, err := RunWorkflowContextObserveCall(context.Background(), request, base)
	require.ErrorContains(t, err, "credential is unavailable")
	assert.Empty(t, result.SchemaVersion)
	assert.Zero(t, result.CleanupFacts.OwnedRootsCreated)

	t.Setenv("AUTOPUS_TEST_OBSERVE_CALL_TOKEN", "observe-secret-token")
	remote := base
	remote.Endpoint = "http://10.1.2.3:43123"
	_, err = RunWorkflowContextObserveCall(context.Background(), request, remote)
	require.ErrorContains(t, err, "task endpoint is invalid")
	assert.NotContains(t, err.Error(), "observe-secret-token")

	missingExecutable := base
	missingExecutable.Executable = filepath.Join(project, "absent-omp")
	_, err = RunWorkflowContextObserveCall(context.Background(), request, missingExecutable)
	require.ErrorContains(t, err, "OMP executable is unavailable")

	badProject := base
	badProject.ProjectDir = filepath.Join(project, "AGENTS.md")
	_, err = RunWorkflowContextObserveCall(context.Background(), request, badProject)
	require.ErrorContains(t, err, "project directory is invalid")
}

// Neither variant may reach a provider once its driver has been cleaned up:
// the observe lane must report the bind failure instead of dispatching blind.
func TestExecuteWorkflowContextObserveVariant_RefusesBothVariantsAfterCleanup(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	writeWorkflowContextObserveCanonicalDocuments(t, project, "SPEC-OMP-004")
	deliveryOptions := promptlayer.ContextDeliveryOptions{
		Root: project, Command: "go",
		SpecDir: filepath.ToSlash(filepath.Join(".autopus", "specs", "SPEC-OMP-004")),
	}
	delivery, err := promptlayer.BuildContextDelivery(deliveryOptions)
	require.NoError(t, err)
	setup := workflowContextObserveCallSetup{projectDir: project, delivery: delivery}
	ephemeral := promptlayer.OMPContextEphemeral{OriginalTask: "/auto go SPEC-OMP-004", DecisionDelta: "delta"}
	ctx := context.Background()

	for _, variant := range []string{"full", "optimized"} {
		driver := observeCallTestDriver(t)
		require.NoError(t, driver.Cleanup(ctx))
		request := workflowContextObserveCallRequest{
			Sequence: 1, PairSequence: 1, TaskID: "task-03", Prompt: "delta", Variant: variant,
		}
		output, usage, lifecycle, runErr := executeWorkflowContextObserveVariant(
			ctx, driver, request, setup, ephemeral, "admission",
		)
		require.Error(t, runErr, "variant %q must fail closed", variant)
		assert.Empty(t, output)
		assert.Zero(t, usage.TotalTokens)
		assert.Zero(t, lifecycle.ProviderTurns)
		assert.False(t, lifecycle.TerminalIdle)
		assert.False(t, driver.Observation().ProviderObserved)
	}
}
