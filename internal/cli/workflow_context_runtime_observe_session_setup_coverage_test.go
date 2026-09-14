package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func observeSessionValidOptions() workflowContextObserveSessionOptions {
	commit := strings.Repeat("a", 40)
	return workflowContextObserveSessionOptions{
		Provider: "openai", Model: "gpt-5.6-sol", ModelContextWindow: 262144, SpecID: "SPEC-OMP-004",
		CredentialLocator: "AUTOPUS_OMP_CONTEXT_PROVIDER_OPENAI", TargetGitCommit: commit,
		Endpoint: "http://127.0.0.1:43123", Executable: "omp", WorkspaceID: "autopus-adk",
		ProducerRepository: "insajin/omp-evals", ProducerWorkflowRef: "refs/heads/main@" + commit,
		ProducerRunID: "123456", ProducerRunAttempt: 1, CandidateRepository: "insajin/autopus-adk",
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
}

// Every case names a distinct admission rule. Accepting any of them would let
// the promotion lane publish evidence produced under unpinned coordinates.
func TestValidateWorkflowContextObserveSessionOptions_RejectsEachUnpinnedCoordinate(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateWorkflowContextObserveSessionOptions(observeSessionValidOptions()))

	cases := map[string]func(*workflowContextObserveSessionOptions){
		"memory must be off":       func(o *workflowContextObserveSessionOptions) { o.PromotionPolicy.MemoryMode = "on" },
		"history must be active":   func(o *workflowContextObserveSessionOptions) { o.PromotionPolicy.HistoryMode = "" },
		"evidence must not expire": func(o *workflowContextObserveSessionOptions) { o.EvidenceValidFor = 0 },
		"evidence bounded to a day": func(o *workflowContextObserveSessionOptions) {
			o.EvidenceValidFor = 25 * time.Hour
		},
		"context window floor": func(o *workflowContextObserveSessionOptions) { o.ModelContextWindow = 4096 },
		"context window ceil":  func(o *workflowContextObserveSessionOptions) { o.ModelContextWindow = 1 << 31 },
		"sandbox mode pinned": func(o *workflowContextObserveSessionOptions) {
			o.SandboxMode = pipelineOMPActiveSandboxInheritedParent + 1
		},
		"provider pattern":      func(o *workflowContextObserveSessionOptions) { o.Provider = "open ai" },
		"model pattern":         func(o *workflowContextObserveSessionOptions) { o.Model = "" },
		"spec pattern":          func(o *workflowContextObserveSessionOptions) { o.SpecID = "spec omp" },
		"credential pattern":    func(o *workflowContextObserveSessionOptions) { o.CredentialLocator = "lower_case" },
		"run id pattern":        func(o *workflowContextObserveSessionOptions) { o.ProducerRunID = "run/1" },
		"run attempt positive":  func(o *workflowContextObserveSessionOptions) { o.ProducerRunAttempt = 0 },
		"oracle digest hashed":  func(o *workflowContextObserveSessionOptions) { o.OraclePolicyDigest = "not-a-hash" },
		"metadata not blank":    func(o *workflowContextObserveSessionOptions) { o.WorkspaceID = "  " },
		"metadata not padded":   func(o *workflowContextObserveSessionOptions) { o.PolicyID = " policy " },
		"metadata bounded":      func(o *workflowContextObserveSessionOptions) { o.CandidateRepository = strings.Repeat("r", 257) },
		"endpoint present":      func(o *workflowContextObserveSessionOptions) { o.Endpoint = " " },
		"executable present":    func(o *workflowContextObserveSessionOptions) { o.Executable = "" },
		"probe dir absolute":    func(o *workflowContextObserveSessionOptions) { o.ProbeDir = "relative/probe" },
		"promotion policy sane": func(o *workflowContextObserveSessionOptions) { o.PromotionPolicy.Profile = "" },
	}
	for name, mutate := range cases {
		options := observeSessionValidOptions()
		mutate(&options)
		require.ErrorContains(t, validateWorkflowContextObserveSessionOptions(options),
			"observe-session coordinates are invalid", "case %q must be refused", name)
	}
}

func TestPrepareWorkflowContextObserveSession_FailsClosedOnUnprovenCoordinates(t *testing.T) {
	project := t.TempDir()
	writeWorkflowContextObserveCanonicalDocuments(t, project, "SPEC-OMP-004")
	base := observeSessionValidOptions()
	base.ProjectDir = project
	base.CredentialLocator = "AUTOPUS_TEST_OBSERVE_SESSION_TOKEN"
	base.Executable = os.Args[0]
	ctx := context.Background()

	// No credential in the environment: the session must refuse before it spawns.
	_, err := prepareWorkflowContextObserveSession(ctx, base, workflowContextRuntimeHash("grant"))
	require.ErrorContains(t, err, "credential is unavailable")

	t.Setenv(base.CredentialLocator, "session-secret-token")
	remote := base
	remote.Endpoint = "http://10.1.2.3:43123"
	_, err = prepareWorkflowContextObserveSession(ctx, remote, workflowContextRuntimeHash("grant"))
	require.ErrorContains(t, err, "endpoint is invalid")
	assert.NotContains(t, err.Error(), "session-secret-token")

	badProject := base
	badProject.ProjectDir = filepath.Join(project, "AGENTS.md")
	_, err = prepareWorkflowContextObserveSession(ctx, badProject, workflowContextRuntimeHash("grant"))
	require.ErrorContains(t, err, "project directory is invalid")

	missingExecutable := base
	missingExecutable.Executable = filepath.Join(project, "absent-omp")
	_, err = prepareWorkflowContextObserveSession(ctx, missingExecutable, workflowContextRuntimeHash("grant"))
	require.ErrorContains(t, err, "OMP executable is unavailable")

	// The candidate commit must match the running build, or no evidence may be produced.
	_, err = prepareWorkflowContextObserveSession(ctx, base, workflowContextRuntimeHash("grant"))
	require.ErrorContains(t, err, "candidate build provenance is unavailable")
}

func TestObserveSessionSetup_SegmentLifecycleRefusesInvalidTransitions(t *testing.T) {
	t.Parallel()
	var absent *workflowContextObserveSessionSetup
	require.NoError(t, absent.close())
	require.NoError(t, absent.closePair())
	require.ErrorContains(t, absent.rotate(context.Background()), "segment rotation is invalid")

	// Rotation requires a live pair; a setup that never started must not rotate.
	setup := &workflowContextObserveSessionSetup{}
	require.ErrorContains(t, setup.rotate(context.Background()), "segment rotation is invalid")
	require.NoError(t, setup.closePair())

	exhausted := &workflowContextObserveSessionSetup{segmentsStarted: workflowContextObserveSessionSegmentCount}
	require.ErrorContains(t, exhausted.start(context.Background()), "segment startup is invalid")
}

func TestObserveSessionSetup_CloseRemovesOwnedRootExactlyOnce(t *testing.T) {
	t.Parallel()
	taskRoot := filepath.Join(t.TempDir(), "task-root")
	require.NoError(t, os.Mkdir(taskRoot, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(taskRoot, "evidence"), []byte("x"), 0o600))
	setup := &workflowContextObserveSessionSetup{taskRoot: taskRoot}

	require.NoError(t, setup.close())
	_, statErr := os.Lstat(taskRoot)
	assert.True(t, os.IsNotExist(statErr))
	// The ownership record is released, so a second close is a no-op, not a retry.
	assert.Empty(t, setup.taskRoot)
	require.NoError(t, setup.close())
}
