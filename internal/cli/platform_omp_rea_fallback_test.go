package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

const ompReaFallbackLine = "qa-explorer: openai-codex/gpt-6.1-sol not in catalog → session model"

// ompCLICatalogWithout drops one selector from an OMP catalog fixture.
func ompCLICatalogWithout(t *testing.T, catalog []byte, selector string) []byte {
	t.Helper()
	var raw struct {
		Models []map[string]any `json:"models"`
	}
	require.NoError(t, json.Unmarshal(catalog, &raw))
	kept := raw.Models[:0]
	for _, model := range raw.Models {
		if model["provider"].(string)+"/"+model["id"].(string) != selector {
			kept = append(kept, model)
		}
	}
	raw.Models = kept
	data, err := json.Marshal(raw)
	require.NoError(t, err)
	return data
}

func ompReaFallbackRow(t *testing.T, payload ompProfileApplyPreviewPayload) ompProfileAgentPreviewPayload {
	t.Helper()
	for _, row := range payload.Agents {
		if row.Agent == "qa-explorer" {
			return row
		}
	}
	t.Fatal("the plan has no qa-explorer row")
	return ompProfileAgentPreviewPayload{}
}

// S11 K1: a catalog serving the analysis rung routes its generated agent,
// qa-explorer, and prints no fallback line.
func TestOMPProfilePlan_ServedAnalysisRungRoutesQAExplorer(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = ompCLIProfileNativeCatalogJSON()

	payload := planOMPProfileJSON(t, root, runner, "balanced", "--family", "gpt")

	assert.Empty(t, payload.Blockers)
	assert.Empty(t, payload.Warnings)
	row := ompReaFallbackRow(t, payload)
	assert.Equal(t, "openai-codex/gpt-6.1-sol", row.EffectiveSelector)
	assert.Equal(t, "xhigh", row.EffectiveThinking)
}

// useComposeLanguage pins the run's language for one test.
func useComposeLanguage(t *testing.T, lang string) {
	t.Helper()
	original := composeLanguage
	t.Cleanup(func() { composeLanguage = original })
	composeLanguage = func() string { return lang }
}

// S11 K2 and F-002: a missing analysis model, or one without the rung's
// thinking level, prints the fallback line once instead of a blocker.
func TestOMPProfilePlan_UnservedAnalysisRungFallsBackToTheSessionModel(t *testing.T) {
	useComposeLanguage(t, "en")
	for name, catalog := range map[string][]byte{
		"model absent": ompCLICatalogWithout(t, ompCLIProfileNativeCatalogJSON(), "openai-codex/gpt-6.1-sol"),
		"thinking unsupported": bytes.Replace(ompCLIProfileNativeCatalogJSON(),
			[]byte(`"id":"gpt-6.1-sol","thinking":["xhigh","max"]`), []byte(`"id":"gpt-6.1-sol","thinking":["max"]`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			root, runner := writeOMPBalancedProject(t)
			runner.catalog = catalog

			payload := planOMPProfileJSON(t, root, runner, "balanced", "--family", "gpt")

			assert.Empty(t, payload.Blockers)
			assert.Equal(t, []string{ompReaFallbackLine}, payload.Warnings)
			row := ompReaFallbackRow(t, payload)
			assert.Equal(t, ompProfileStatusSessionModel, row.Status)
			assert.Empty(t, row.EffectiveSelector)

			dir := root
			text := executeOMPSubcommand(t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
				"balanced", "--family", "gpt", "--plan")
			assert.Equal(t, 1, strings.Count(text, ompReaFallbackLine), text)
			assert.Contains(t, text, "Blockers: none\n")
		})
	}
}

// S11 K3 and K4: an existing rung model keeps the named blocker, and the
// analysis rung prints no fallback line next to it.
func TestOMPProfilePlan_ExistingRungKeepsTheBlocker(t *testing.T) {
	native := ompCLIProfileNativeCatalogJSON()
	withoutLuna := ompCLICatalogWithout(t, native, "openai-codex/gpt-6-luna")
	for name, catalog := range map[string][]byte{
		"K3": withoutLuna,
		"K4": ompCLICatalogWithout(t, withoutLuna, "openai-codex/gpt-6.1-sol"),
	} {
		t.Run(name, func(t *testing.T) {
			root, runner := writeOMPBalancedProject(t)
			runner.catalog = catalog
			dir := root

			text, err := executeOMPSubcommandExpectingError(t,
				newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)), "balanced", "--family", "gpt", "--plan")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "omp_profile_candidate_unavailable")
			assert.Contains(t, text, "openai-codex/gpt-6-luna")
			assert.NotContains(t, text, ompReaFallbackLine)
		})
	}
}

// REQ-REA-18: the fallback line follows the run's language.
func TestOMPProfilePlan_FallbackLineFollowsTheLanguage(t *testing.T) {
	useComposeLanguage(t, "ko")
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = ompCLICatalogWithout(t, ompCLIProfileNativeCatalogJSON(), "openai-codex/gpt-6.1-sol")

	payload := planOMPProfileJSON(t, root, runner, "balanced", "--family", "gpt")

	assert.Equal(t, []string{"qa-explorer: openai-codex/gpt-6.1-sol이 OMP catalog에 없어 세션 모델을 씁니다"},
		payload.Warnings)
}

// Doctor keeps the route generation drops and reads it as the session-model
// fallback, not as a blocked role (SPEC-REA-001 S11).
func TestOMPDoctorRouting_ReadsTheAnalysisRungFallbackAsSessionModel(t *testing.T) {
	policy := config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai"}
	_, profile, ok := policy.SelectedRoleModelProfileForQuality(config.DefaultFullConfig("rea-doctor").Quality)
	require.True(t, ok)
	runner := &ompCLIFakeRunner{catalog: ompCLICatalogWithout(t, ompCLIProfileNativeCatalogJSON(), "openai-codex/gpt-6.1-sol")}
	probe, err := probeInstalledOMPCatalogForProfile(context.Background(), runner, profile)
	require.NoError(t, err)

	statuses := map[string]string{}
	for _, resolution := range compileOMPModelDoctorRouting(profile, probe.Catalog).Resolutions {
		statuses[resolution.Agent] = resolution.Status
	}
	assert.Equal(t, omp.OMPReaSessionModelStatus, statuses["qa-explorer"])
	assert.Equal(t, "selected", statuses["reviewer"])
	assert.Equal(t, "selected", statuses["task"])
}
