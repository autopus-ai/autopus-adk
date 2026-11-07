package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

const ompReaFallbackLineKO = "  qa-explorer: openai-codex/gpt-6.1-sol이 OMP catalog에 없어 세션 모델을 씁니다\n"

// updateOMPBalancedOpenAI runs `auto update --yes` on an OMP project with the
// built-in balanced openai profile, OMP answering catalog, and returns the
// output, the overrides OMP reads back, the project, and the error.
func updateOMPBalancedOpenAI(t *testing.T, catalog []byte) (string, map[string]string, string, error) {
	t.Helper()
	newComposeUpdateRun(t, false, "")
	generation := &ompModelDoctorE2ERunner{catalog: catalog}
	usePlatformOMPModelRunner(t, generation)
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("rea-s11")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
	}
	require.NoError(t, config.Save(dir, cfg))

	out, err := runComposeUpdate(t, strings.NewReader(""), "--dir", dir, "--yes")
	if err != nil {
		return out, nil, dir, err
	}
	overlay := filepath.Join(dir, filepath.FromSlash(omp.DefaultOMPModelOverlayPath))
	body, readErr := generation.Run(context.Background(), "omp", "--config", overlay, "config", "get",
		config.OMPNativeAgentModelOverridesKey, "--json")
	require.NoError(t, readErr)
	var readback struct {
		Value map[string]string `json:"value"`
	}
	require.NoError(t, json.Unmarshal(body, &readback))
	return out, readback.Value, dir, nil
}

// S11 K1 and K2 through a real update: a served analysis rung routes
// qa-explorer; a missing one leaves only qa-explorer on the session
// model with one fallback line, and every other override stays the same.
func TestUpdateOMP_AnalysisRungFallbackKeepsEveryOtherRoute(t *testing.T) {
	native := ompCLIProfileNativeCatalogJSON()
	out, served, _, err := updateOMPBalancedOpenAI(t, native)
	require.NoError(t, err, out)
	assert.Len(t, served, len(config.OMPAutopusAgentNames()))
	assert.Equal(t, "openai-codex/gpt-6.1-sol:xhigh", served["qa-explorer"])
	assert.NotContains(t, out, ompReaFallbackLineKO)

	out, fallback, dir, err := updateOMPBalancedOpenAI(t, ompCLICatalogWithout(t, native, "openai-codex/gpt-6.1-sol"))
	require.NoError(t, err, out)
	assert.Equal(t, 1, strings.Count(out, ompReaFallbackLineKO), out)
	assert.NotContains(t, fallback, "qa-explorer")
	delete(served, "qa-explorer")
	assert.Equal(t, served, fallback)
	definition, readErr := os.ReadFile(filepath.Join(dir, ".omp", "agents", "qa-explorer.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(definition), "\nthinkingLevel: high\n")
	assert.NotContains(t, string(definition), "\nmodel:")
}

// S11 K3 and K4 through a real update: a missing existing-rung model stops the
// run with the named blocker and prints no fallback line.
func TestUpdateOMP_ExistingRungModelMissingStopsWithTheNamedBlocker(t *testing.T) {
	withoutLuna := ompCLICatalogWithout(t, ompCLIProfileNativeCatalogJSON(), "openai-codex/gpt-6-luna")
	for name, catalog := range map[string][]byte{
		"K3": withoutLuna,
		"K4": ompCLICatalogWithout(t, withoutLuna, "openai-codex/gpt-6.1-sol"),
	} {
		t.Run(name, func(t *testing.T) {
			out, _, dir, err := updateOMPBalancedOpenAI(t, catalog)
			require.Error(t, err, out)
			assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(omp.DefaultOMPModelOverlayPath)),
				"a blocked run writes no overlay")
			assert.Contains(t, out+err.Error(), "omp_profile_candidate_unavailable")
			assert.Contains(t, out+err.Error(), "openai-codex/gpt-6-luna")
			assert.NotContains(t, out, ompReaFallbackLineKO)
		})
	}
}
