package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestPlatformOMPProfileInitPlanWritesNothingAndHasSixCapabilities(t *testing.T) {
	root := t.TempDir()
	sentinelPath := filepath.Join(root, "autopus.yaml")
	sentinel := []byte("sentinel: unchanged\n")
	require.NoError(t, os.WriteFile(sentinelPath, sentinel, 0o640))
	runner := &ompCLIFakeRunner{catalog: ompCLIReadyCatalogJSON()}
	deps := normalizeOMPPlatformDependencies(ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
	})

	encoded := executeOMPSubcommand(
		t, newPlatformOMPProfileInitCmd(&root, deps), "--name", "balanced", "--plan", "--json",
	)
	var envelope ompCLIJSONEnvelope
	require.NoError(t, json.Unmarshal([]byte(encoded), &envelope))
	var payload ompProfilePlanPayload
	require.NoError(t, json.Unmarshal(envelope.Data, &payload))
	assert.Equal(t, "plan", payload.Mode)
	assert.Empty(t, payload.Writes)
	assert.Len(t, payload.Capabilities, 6)
	after, err := os.ReadFile(sentinelPath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, after)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestPlatformOMPProfileApplyPersistsAndRollsBackAtomically(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-apply")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	configPath := filepath.Join(root, "autopus.yaml")
	original, err := os.ReadFile(configPath)
	require.NoError(t, err)
	original = append(original, []byte("operator_extension:\n  credential_ref: ${OMP_SECRET}\n")...)
	require.NoError(t, os.WriteFile(configPath, original, 0o640))
	require.NoError(t, os.Chmod(configPath, 0o640))
	runner := &ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}
	activated := false
	deps := normalizeOMPPlatformDependencies(ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate: func(_ context.Context, _ string, _ *config.HarnessConfig) error {
			activated = true
			return nil
		},
	})
	dir := root
	executeOMPSubcommand(t, newPlatformOMPProfileApplyCmd(&dir, deps), "balanced")
	assert.True(t, activated)
	loaded, err := config.LoadPreview(root)
	require.NoError(t, err)
	name, _, selected := loaded.RoleModelPolicy.SelectedRoleModelProfileForQuality(loaded.Quality)
	assert.True(t, selected)
	assert.Equal(t, "balanced", name)
	persisted, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(persisted), "credential_ref: ${OMP_SECRET}")
	assert.Equal(t, os.FileMode(0o640), mustFileMode(t, configPath))

	before, err := os.ReadFile(configPath)
	require.NoError(t, err)
	_, err = applyOMPProfile(
		context.Background(), root, ompProfileApplyOptions{name: "second"}, runner,
		func(context.Context, string, *config.HarnessConfig) error { return errors.New("activation failed") },
	)
	require.Error(t, err)
	after, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

func TestPlatformOMPExplainShowsDisabledUnauthorizedAndMissingFallbacks(t *testing.T) {
	root, runner, profile := writeSelectedOMPProfile(t)
	// The collapse leaves no bundled row on coding_tool_use, so the visible
	// fallback chain has to be attached to a capability a bundled agent routes
	// through: fast_validation is `scout`'s.
	route := profile.Capabilities[config.CapabilityFastValidation]
	route.Candidates = []config.RoleModelCandidateConf{
		{Selector: "openai/disabled-coder", Thinking: "high", Family: "openai"},
		{Selector: "openai/unauthorized-coder", Thinking: "high", Family: "openai"},
		{Selector: "openai/missing-coder", Thinking: "high", Family: "openai"},
		{Selector: "openai/beta-coder", Thinking: "high", Family: "openai"},
	}
	profile.Capabilities[config.CapabilityFastValidation] = route
	cfg, err := config.LoadPreview(root)
	require.NoError(t, err)
	cfg.RoleModelPolicy.Profiles["balanced"] = profile
	require.NoError(t, config.Save(root, cfg))
	deps := normalizeOMPPlatformDependencies(ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
	})
	dir := root

	text := executeOMPSubcommand(t, newPlatformOMPExplainCmd(&dir, deps))
	for _, reason := range []string{"disabled", "unauthorized", "model_unknown", "selected"} {
		assert.Contains(t, text, "reason="+reason)
	}
	encoded := executeOMPSubcommand(t, newPlatformOMPExplainCmd(&dir, deps), "--json")
	var explainEnvelope struct {
		Data struct {
			Models ompModelOperatorProjection `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(encoded), &explainEnvelope))
	reasons := map[string]bool{}
	for _, model := range explainEnvelope.Data.Models.Models {
		for _, attempt := range model.FallbackAttempts {
			reasons[attempt.Reason] = true
		}
	}
	for _, reason := range []string{"disabled", "unauthorized", "model_unknown"} {
		assert.True(t, reasons[reason], reason)
	}
	assert.NotContains(t, strings.ToLower(encoded), "api_key")
}

func TestOMPProfileProposalRejectsInvalidMissingCapability(t *testing.T) {
	catalog := mustOMPCatalog(t, ompCLIReadyCatalogJSON())
	filtered := catalog.Models[:0]
	for _, model := range catalog.Models {
		if !containsOMPString(model.Capabilities, config.CapabilityVisionDesign) {
			filtered = append(filtered, model)
		}
	}
	catalog.Models = filtered
	_, err := buildOMPProfileProposal("balanced", catalog)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability_unavailable:vision_design")
}

func TestOMPProfileInvalidNameFailsClosedInTextAndJSON(t *testing.T) {
	root := t.TempDir()
	runner := &ompCLIFakeRunner{catalog: ompCLIReadyCatalogJSON()}
	deps := normalizeOMPPlatformDependencies(ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
	})
	textCmd := newPlatformOMPProfileInitCmd(&root, deps)
	textCmd.SilenceErrors = true
	textCmd.SilenceUsage = true
	textCmd.SetArgs([]string{"--name", "invalid/name"})
	err := textCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "profile_name_invalid")
	assert.Empty(t, runner.calls)

	jsonCmd := newPlatformOMPProfileInitCmd(&root, deps)
	var out bytes.Buffer
	jsonCmd.SetOut(&out)
	jsonCmd.SetErr(&out)
	jsonCmd.SilenceErrors = true
	jsonCmd.SilenceUsage = true
	jsonCmd.SetArgs([]string{"--name", "invalid/name", "--json"})
	err = jsonCmd.Execute()
	require.Error(t, err)
	var envelope struct {
		Status jsonEnvelopeStatus `json:"status"`
		Error  jsonErrorPayload   `json:"error"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &envelope))
	assert.Equal(t, jsonStatusError, envelope.Status)
	assert.Equal(t, "omp_profile_invalid", envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "profile_name_invalid")
	assert.Empty(t, runner.calls)
}
