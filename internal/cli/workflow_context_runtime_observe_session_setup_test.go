package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/pipeline"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workflowContextInheritedVersionOuterSandboxFixture = "AUTOPUS_TEST_OMP_VERSION_OUTER_SANDBOX"

func TestWorkflowContextObserveSessionPhaseModels_BindsAllCanonicalPhases(t *testing.T) {
	t.Parallel()
	const selector = "openai/gpt-5.6-sol"
	want := map[pipeline.PhaseID]string{
		pipeline.PhasePlan:         selector,
		pipeline.PhaseTestScaffold: selector,
		pipeline.PhaseImplement:    selector,
		pipeline.PhaseValidate:     selector,
		pipeline.PhaseReview:       selector,
	}

	phaseModels := workflowContextObserveSessionPhaseModels(selector)
	assert.Equal(t, want, phaseModels)
	provider, digest, err := pipelineOMPActiveModelScope(phaseModels)
	require.NoError(t, err)
	assert.Equal(t, "openai", provider)
	assert.Equal(t, "sha256:386816349ebf9b5c2a113889fb3160662121a4fd4fa4085d2bdef97660142adf", digest)
}

func TestWorkflowContextObserveSession_RuntimeBudgetsCoverSegmentWorkload(t *testing.T) {
	t.Parallel()
	// An evaluator child must outlive its whole segment: every pair costs one primary
	// turn and the optimized variant adds a compaction turn to each reused call.
	segmentTurns := workflowContextObserveSessionSegmentPairs * 2
	assert.GreaterOrEqual(t, workflowContextObserveSessionMaxTime,
		time.Duration(segmentTurns)*workflowContextObserveSessionCallBudget)
	assert.GreaterOrEqual(t, workflowContextObserveSessionTotalMaxTime,
		workflowContextObserveSessionMaxTime*workflowContextObserveSessionSegmentCount)
	assert.Equal(t, 15*time.Minute, workflowContextObserveSessionMaxTime)
	assert.Equal(t, 60*time.Minute, workflowContextObserveSessionTotalMaxTime)
}

func TestWorkflowContextObserveSessionCommand_InheritedParentSandboxIsExplicitOptIn(t *testing.T) {
	t.Parallel()
	assert.Equal(t, pipelineOMPActiveSandboxManaged, workflowContextObserveSessionSandboxMode(false))
	assert.Equal(t, pipelineOMPActiveSandboxInheritedParent, workflowContextObserveSessionSandboxMode(true))
	flag := newWorkflowContextObserveSessionCmd().Flags().Lookup("inherit-parent-sandbox")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	contextWindow := newWorkflowContextObserveSessionCmd().Flags().Lookup("model-context-window")
	require.NotNil(t, contextWindow)
	assert.Equal(t, "262144", contextWindow.DefValue)
	assert.Nil(t, newWorkflowContextObserveCallCmd().Flags().Lookup("inherit-parent-sandbox"))

	options := workflowContextObserveSessionOptions{
		Provider: "openai", Model: "gpt-5.6-sol", ModelContextWindow: 272000, SpecID: "SPEC-OMP-004",
		CredentialLocator: "AUTOPUS_OMP_CONTEXT_PROVIDER_OPENAI", TargetGitCommit: strings.Repeat("a", 40),
		Endpoint: "http://127.0.0.1:43123", Executable: "omp", WorkspaceID: "autopus-adk",
		ProducerRepository:  "insajin/omp-evals",
		ProducerWorkflowRef: "refs/heads/main@" + strings.Repeat("a", 40),
		ProducerRunID:       "123456", ProducerRunAttempt: 1, CandidateRepository: "insajin/autopus-adk",
		PolicyID: "omp-context-active-v1", OraclePolicyDigest: workflowContextRuntimeHash("oracle"),
		PromotionPolicy: promptlayer.OMPContextPromotionPolicyV1{
			Profile: "active", HistoryMode: config.OMPContextHistoryActive, MemoryMode: config.OMPContextMemoryOff,
			HistoryTargetTokens: 1000, Fallback: config.OMPContextFallbackCanonicalFull,
			CapabilityPolicy:  config.OMPContextCapabilityProbeRequired,
			RuntimeRootPolicy: config.OMPContextRuntimeIsolatedTaskOwned,
			MutationScope:     config.OMPContextMutationSessionOverlay,
		},
		EvidenceValidFor: time.Hour, SandboxMode: pipelineOMPActiveSandboxManaged,
	}
	require.NoError(t, validateWorkflowContextObserveSessionOptions(options))
}

func TestPopulateWorkflowContextObserveSessionPolicy_DoesNotInspectOrMutateUserOMPState(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("evidence-workspace")
	cfg.OMPContextPolicy = config.OMPContextPolicyConf{
		Profile: "active",
		Profiles: map[string]config.OMPContextProfileConf{"active": {
			HistoryMode: config.OMPContextHistoryActive, MemoryMode: config.OMPContextMemoryOff,
			HistoryTargetTokens: 1000, Fallback: config.OMPContextFallbackCanonicalFull,
			CapabilityPolicy:  config.OMPContextCapabilityProbeRequired,
			RuntimeRootPolicy: config.OMPContextRuntimeIsolatedTaskOwned,
			MutationScope:     config.OMPContextMutationSessionOverlay,
		}},
	}
	require.NoError(t, config.Save(root, cfg))
	userOMPPath := filepath.Join(root, ".omp", "config.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(userOMPPath), 0o700))
	require.NoError(t, os.WriteFile(userOMPPath, []byte("USER-OWNED-OMP-CONFIG"), 0o600))

	options := workflowContextObserveSessionOptions{ProjectDir: root}
	require.NoError(t, populateWorkflowContextObserveSessionPolicy(&options))
	assert.Equal(t, "evidence-workspace", options.WorkspaceID)
	assert.Equal(t, config.OMPContextHistoryActive, options.PromotionPolicy.HistoryMode)
	assert.Equal(t, config.OMPContextMemoryOff, options.PromotionPolicy.MemoryMode)
	body, err := os.ReadFile(userOMPPath)
	require.NoError(t, err)
	assert.Equal(t, "USER-OWNED-OMP-CONFIG", string(body))
}

func TestWorkflowContextObserveVersion_InheritedModeUsesVerifiedPathWithoutInnerWrapper(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("inherited parent sandbox is Darwin-only")
	}
	options := WorkflowContextManagedRPCOptions{
		Executable: os.Args[0], Workspace: t.TempDir(), Environment: os.Environ(),
		AllowedEndpoint: "http://127.0.0.1:43123",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command, _, err := newWorkflowContextObserveInheritedVersionCommand(ctx, options)
	require.NoError(t, err)
	executable, _, err := canonicalPipelineOMPExecutable(os.Args[0])
	require.NoError(t, err)
	assert.True(t, command.inheritedDarwinPath)
	assert.False(t, command.inheritedDarwinPrivate)
	assert.Equal(t, executable, command.cmd.Path)
	assert.Equal(t, executable, command.cmd.Args[0])
	assert.Empty(t, command.cmd.ExtraFiles)
	assert.False(t, pipelineOMPVerifiedExecUsesDarwinPtrace(command))
	require.NoError(t, command.Close())

	got, err := probeWorkflowContextObserveVersion(ctx, options, pipelineOMPActiveSandboxInheritedParent)
	require.NoError(t, err)
	assert.Equal(t, "omp/17.2.7", got)
}

func TestWorkflowContextObserveVersion_InheritedModeStripsSecretsInsideDenyDefaultSandbox(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("inherited parent sandbox is Darwin-only")
	}
	if os.Getenv(workflowContextInheritedVersionOuterSandboxFixture) == "" {
		const profile = `(version 1)
(deny default)
(allow file-read*)
(allow file-write* (literal "/dev/null"))
(allow process*)
(allow mach*)
(allow sysctl-read)
`
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := sandboxCoverageFixtureCommand(t, ctx, profile,
			"TestWorkflowContextObserveVersion_InheritedModeStripsSecretsInsideDenyDefaultSandbox")
		cmd.Env = append(cmd.Env, workflowContextInheritedVersionOuterSandboxFixture+"=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, strings.TrimSpace(string(output)))
		return
	}

	options := WorkflowContextManagedRPCOptions{
		Executable: os.Args[0], Workspace: filepath.Dir(os.Args[0]),
		Environment: []string{
			"PATH=/sentinel/bin", pipelineOMPActiveCredentialKey + "=pipeline-sentinel",
			"OPENAI_API_KEY=provider-sentinel", "UNRELATED_SECRET=unrelated-sentinel",
		},
		AllowedEndpoint: "http://127.0.0.1:43123",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command, _, err := newWorkflowContextObserveInheritedVersionCommand(ctx, options)
	require.NoError(t, err)
	if command.cmd.Env == nil || len(command.cmd.Env) != 0 {
		t.Fatalf("inherited version environment is not empty: entries=%d", len(command.cmd.Env))
	}
	require.NoError(t, command.Close())

	got, err := probeWorkflowContextObserveVersion(ctx, options, pipelineOMPActiveSandboxInheritedParent)
	require.NoError(t, err)
	assert.Equal(t, "omp/17.2.7", got)
}
