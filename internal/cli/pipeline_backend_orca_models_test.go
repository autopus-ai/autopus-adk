package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ompadapter "github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/orcarun"
	"github.com/insajin/autopus-adk/pkg/pipeline"
	"github.com/stretchr/testify/require"
)

func orcaModelRole(agent, provider, model, thinking string) ompadapter.OMPModelRoleReceipt {
	return ompadapter.OMPModelRoleReceipt{
		Agent: agent, Profile: "balanced", ConfigSource: "overlay", RequestedRole: "task",
		EffectiveRole: "task", Capability: "pipeline", Provider: provider, Model: model,
		Selector: provider + "/" + model, Thinking: thinking,
		FamilyDiversity: ompadapter.OMPModelFamilyDiversityReceipt{Status: "not_applicable"},
		SafetySource:    "autopus_profile",
	}
}

func writePipelineOrcaModelReceipt(t *testing.T, root string, roles []ompadapter.OMPModelRoleReceipt) {
	t.Helper()
	overlay := []byte("models:\n  pipeline: provider/model\n")
	overlayPath := filepath.Join(root, filepath.FromSlash(ompadapter.DefaultOMPModelOverlayPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(overlayPath), 0o700))
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))
	_, err := ompadapter.WriteOMPModelResolutionReceipt(ompadapter.OMPModelReceiptWriteInput{
		WorkspaceRoot: root,
		Receipt: ompadapter.OMPModelResolutionReceipt{
			OMPVersion: "omp/17.1.8", CatalogFingerprint: "sha256:" + strings.Repeat("a", 64),
			Profile: "balanced", ConfigSource: "overlay", GeneratedAt: time.Now().UTC(), Roles: roles,
			Activation: ompadapter.OMPModelActivationReceipt{
				Argv: []string{"omp"}, ConfigHash: ompadapter.OMPModelSHA256(overlay),
				ReadbackHash: "sha256:" + strings.Repeat("c", 64),
			},
			Safety: ompadapter.OMPModelSafetyReceipt{
				ApprovalMode: "write", IsolationMode: "auto", Source: "autopus_profile",
			},
		},
	})
	require.NoError(t, err)
}

// orcaCanonicalModelRoles keys the receipt by bundled agent. Four phases share
// `task`, so they legitimately launch the same agent, model, and effort.
func orcaCanonicalModelRoles() []ompadapter.OMPModelRoleReceipt {
	return []ompadapter.OMPModelRoleReceipt{
		orcaModelRole("task", "anthropic", "claude-opus-5", "xhigh"),
		orcaModelRole("reviewer", "google", "gemini-3.5-pro", "low"),
	}
}

// TestLoadPipelineOrcaPhaseLaunch_UsesModelAndEffortNotSelector covers REQ-107:
// the joined provider/model selector stays inside the policy plane while the
// opaque model id and effort cross to orca.
func TestLoadPipelineOrcaPhaseLaunch_UsesModelAndEffortNotSelector(t *testing.T) {
	root := t.TempDir()
	writePipelineOrcaModelReceipt(t, root, orcaCanonicalModelRoles())

	launches, err := loadPipelineOrcaPhaseLaunch(root)
	require.NoError(t, err)
	require.Equal(t, map[pipeline.PhaseID]orcarun.Launch{
		pipeline.PhasePlan:         {Agent: "claude", Model: "claude-opus-5", Effort: "xhigh"},
		pipeline.PhaseTestScaffold: {Agent: "claude", Model: "claude-opus-5", Effort: "xhigh"},
		pipeline.PhaseImplement:    {Agent: "claude", Model: "claude-opus-5", Effort: "xhigh"},
		pipeline.PhaseValidate:     {Agent: "claude", Model: "claude-opus-5", Effort: "xhigh"},
		pipeline.PhaseReview:       {Agent: "gemini", Model: "gemini-3.5-pro", Effort: "low"},
	}, launches)
	for _, launch := range launches {
		require.NotContains(t, launch.Model, "/", "the joined selector must not reach orca")
	}
}

// TestLoadPipelineOrcaPhaseLaunch_FailsClosed covers the two ways a receipt can
// leave a phase unroutable. Guessing an agent or skipping a phase would run
// work under an unverified contract.
func TestLoadPipelineOrcaPhaseLaunch_FailsClosed(t *testing.T) {
	t.Run("unknown provider", func(t *testing.T) {
		root := t.TempDir()
		roles := orcaCanonicalModelRoles()
		roles[0] = orcaModelRole("task", "mystery-vendor", "mystery-1", "high")
		writePipelineOrcaModelReceipt(t, root, roles)

		_, err := loadPipelineOrcaPhaseLaunch(root)
		require.ErrorContains(t, err, "mystery-vendor")
	})

	t.Run("incomplete phase coverage", func(t *testing.T) {
		root := t.TempDir()
		writePipelineOrcaModelReceipt(t, root, orcaCanonicalModelRoles()[:1])

		_, err := loadPipelineOrcaPhaseLaunch(root)
		require.ErrorContains(t, err, "no route for native agent reviewer")
	})
}

// TestNewPipelineOrcaBackendForRun_ChecksOrcaBeforeAnythingElse covers REQ-108:
// a missing process plane is a configuration error the caller can recognise,
// and it is detected before any receipt is read.
func TestNewPipelineOrcaBackendForRun_ChecksOrcaBeforeAnythingElse(t *testing.T) {
	root := t.TempDir()
	writePipelineOrcaModelReceipt(t, root, orcaCanonicalModelRoles())
	resolved := resolvedPipelineSpec{Dir: root, Path: filepath.Join(root, "spec.md"), SnapshotHash: "snap"}

	t.Run("orca missing", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		backend, err := newPipelineOrcaBackendForRun(root, "SPEC-EXECPLANE-002", resolved, "abc1234")
		require.Nil(t, backend)
		require.True(t, errors.Is(err, orcarun.ErrOrcaUnavailable), "got %v", err)
	})

	t.Run("orca present", func(t *testing.T) {
		binDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "orca"), []byte("#!/bin/sh\nexit 0\n"), 0o700))
		t.Setenv("PATH", binDir)

		backend, err := newPipelineOrcaBackendForRun(root, "SPEC-EXECPLANE-002", resolved, "abc1234")
		require.NoError(t, err)
		require.Equal(t, "claude", backend.config.PhaseLaunch[pipeline.PhasePlan].Agent)
		require.Equal(t, pipelineOrcaDefaultPhaseTimeout, backend.config.PhaseTimeout)
		require.Equal(t, pipelineOrcaDefaultReadLimit, backend.config.ReadLimit)

		// Construction must not write run state.
		_, statErr := os.Stat(filepath.Join(root, ".autopus", "pipeline-state"))
		require.True(t, os.IsNotExist(statErr), "construction must not persist run state")
	})

	t.Run("unresolved run binding", func(t *testing.T) {
		binDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "orca"), []byte("#!/bin/sh\nexit 0\n"), 0o700))
		t.Setenv("PATH", binDir)

		_, err := newPipelineOrcaBackendForRun(root, "SPEC-EXECPLANE-002", resolved, "")
		require.ErrorContains(t, err, "git commit hash")
	})
}
