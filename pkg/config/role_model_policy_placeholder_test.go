package config

// Security review P4: the loader expands ${NAME} before decoding, so a model
// candidate written as a placeholder would take its selector, thinking, or
// family from the environment. Load refuses a placeholder in any candidate of
// role_model_policy; root profile and family placeholders keep the
// SPEC-MODELUX-002 behaviour (loaded, and refused by the OMP writer).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePlaceholderPolicy(t *testing.T, policy string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := DefaultFullConfig("placeholder")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, Save(dir, cfg))
	path := filepath.Join(dir, configFileName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, []byte(policy)...), 0o644))
	return dir
}

func TestLoad_RefusesAPlaceholderInARoleModelCandidate(t *testing.T) {
	t.Setenv("OMP_PIN", "openai-codex/gpt-6-astra")
	for name, row := range map[string]struct{ policy, path string }{
		"pin selector": {"role_model_policy:\n  version: v1\n  agents:\n    executor:\n      candidates:\n        - selector: ${OMP_PIN}\n          thinking: max\n          family: openai\n",
			"role_model_policy.agents.executor.candidates[0].selector"},
		"pin family": {"role_model_policy:\n  version: v1\n  agents:\n    executor:\n      candidates:\n        - selector: openai-codex/gpt-6-astra\n          thinking: max\n          family: ${OMP_FAMILY}\n",
			"role_model_policy.agents.executor.candidates[0].family"},
		"profile route": {"role_model_policy:\n  version: v1\n  profile: p1\n  profiles:\n    p1:\n      capabilities:\n        deep_reasoning:\n          candidates:\n            - {selector: \"${OMP_PIN}\", thinking: max, family: openai}\n",
			"role_model_policy.profiles.p1.capabilities.deep_reasoning.candidates[0].selector"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadPreview(writePlaceholderPolicy(t, row.policy))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "role_model_policy.candidate_placeholder_unsupported: "+row.path)
		})
	}
}

func TestLoad_KeepsRootRoleModelPolicyPlaceholders(t *testing.T) {
	t.Setenv("OMP_FAMILY", "openai")
	dir := writePlaceholderPolicy(t, "role_model_policy:\n  version: v1\n  profile: balanced\n  family: ${OMP_FAMILY}\n")

	cfg, err := LoadPreview(dir)

	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.RoleModelPolicy.Family)
}
