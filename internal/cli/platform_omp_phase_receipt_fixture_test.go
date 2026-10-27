package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// A project last updated under the removed agent_source: native still holds a
// receipt keyed by the bundled agents. Once the key is deleted, each phase
// runs on its own agent, which that receipt does not route, so the loader
// fails closed until auto update rewrites it; with the key still there, the
// config load names the removal instead.
func TestPipelineOMPPhaseModels_RejectABundledAgentKeyedReceipt(t *testing.T) {
	projectDir := t.TempDir()
	cfg := config.DefaultFullConfig("omp-native-era-receipt")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(projectDir, cfg))
	writeOMPPhaseReceipt(t, projectDir, map[string]string{
		"scout": "provider/scout-model", "reviewer": "provider/review-model",
		"security-reviewer": "provider/guard-model", "task": "provider/task-model",
		"sonic": "provider/fast-model",
	})
	executable := writePipelineOMPVersionFixture(t, "omp/17.1.8")

	models, err := loadPipelineOMPPhaseModels(projectDir, executable)

	require.ErrorContains(t, err, "OMP model receipt has no route for agent")
	assert.Nil(t, models)

	path := filepath.Join(projectDir, "autopus.yaml")
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.NoError(t, os.WriteFile(path, append(data, "role_model_policy:\n  agent_source: native\n"...), 0o600))
	_, err = loadPipelineOMPPhaseModels(projectDir, executable)
	require.ErrorContains(t, err, "role_model_policy.agent_source_removed")
	assert.Equal(t, map[pipeline.PhaseID]string(nil), models)
}

func writeOMPPhaseReceipt(t *testing.T, root string, selectors map[string]string) {
	t.Helper()
	hash := "sha256:" + strings.Repeat("b", 64)
	// The receipt is bound to the overlay it activated, so the fixture has to
	// write that file too or the binding check fails before any route lookup.
	overlay := []byte("task:\n  agentModelOverrides: {}\n")
	overlayPath := filepath.Join(root, filepath.FromSlash(omp.DefaultOMPModelOverlayPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(overlayPath), 0o700))
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))
	receipt := omp.OMPModelResolutionReceipt{
		OMPVersion: "omp/17.1.8", CatalogFingerprint: hash, Profile: "balanced", ConfigSource: "overlay",
		Activation: omp.OMPModelActivationReceipt{
			Argv: []string{"omp"}, ConfigHash: omp.OMPModelSHA256(overlay), ReadbackHash: hash,
		},
		GeneratedAt: time.Now().UTC(),
	}
	for _, agent := range orderedOMPPhaseReceiptAgents(selectors) {
		provider, model, _ := strings.Cut(selectors[agent], "/")
		receipt.Roles = append(receipt.Roles, omp.OMPModelRoleReceipt{
			Agent: agent, Profile: "balanced", ConfigSource: "overlay",
			RequestedRole: "autopus_planner", EffectiveRole: "autopus_planner",
			Capability: config.CapabilityDeepReasoning,
			Provider:   provider, Model: model, Selector: selectors[agent], Thinking: "high",
			FamilyDiversity: omp.OMPModelFamilyDiversityReceipt{Status: "not_applicable"},
			SafetySource:    "user_effective",
		})
	}
	_, err := omp.WriteOMPModelResolutionReceipt(omp.OMPModelReceiptWriteInput{
		WorkspaceRoot: root, Receipt: receipt,
	})
	require.NoError(t, err)
}

// orderedOMPPhaseReceiptAgents keeps a receipt deterministic: bundled agents
// first in registry order, then any other key the fixture declares.
func orderedOMPPhaseReceiptAgents(selectors map[string]string) []string {
	ordered := make([]string, 0, len(selectors))
	for _, native := range config.OMPNativeAgentNames() {
		if _, ok := selectors[native]; ok {
			ordered = append(ordered, native)
		}
	}
	rest := make([]string, 0, len(selectors))
	for agent := range selectors {
		if _, err := config.OMPNativeAgentRepresentative(agent); err != nil {
			rest = append(rest, agent)
		}
	}
	sort.Strings(rest)
	return append(ordered, rest...)
}
