package config

// Security review P4: the loader expands ${NAME} before decoding, so a model
// candidate written as a placeholder would take its selector, thinking, or
// family from the environment. Load refuses a placeholder in any candidate of
// role_model_policy; root profile and family placeholders keep the
// SPEC-MODELUX-002 behaviour (loaded, and refused by the OMP writer).

import (
	"os"
	"path/filepath"
	"strings"
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

// Security re-check P4: the check sees the tree the expanded file will have.
// A placeholder in an unquoted flow mapping, one reached through an alias
// from outside role_model_policy, and one in a key under candidates are all
// refused, and the refusal never echoes the value. A placeholder the YAML
// cannot parse around is refused by the shape guard
// (loader_placeholder_shape_test.go).
func TestLoad_RefusesCandidatePlaceholdersInEveryShape(t *testing.T) {
	t.Setenv("OMP_PIN", "sk-proj-AbC123secret")
	t.Setenv("OMP_KEY", "thinking")
	head := "role_model_policy:\n  version: v1\n  agents:\n    executor:\n      candidates:\n"
	for name, policy := range map[string]string{
		"unquoted flow": head + "        - {selector: ${OMP_PIN}/m, thinking: high, family: anthropic}\n",
		"alias":         "x-anchor: &sel \"${OMP_PIN}/m\"\n" + head + "        - selector: *sel\n          thinking: high\n          family: anthropic\n",
		"key from env":  head + "        - selector: anthropic/claude-opus-5-5\n          ${OMP_KEY}: high\n          family: anthropic\n",
		"secret key":    head + "        - selector: anthropic/claude-opus-5-5\n          thinking: high\n          family: anthropic\n          ${OMP_PIN}: x\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadPreview(writePlaceholderPolicy(t, policy))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "role_model_policy.candidate_placeholder_unsupported")
			assert.NotContains(t, err.Error(), "AbC123secret")
		})
	}
}

// Security re-check P4 (decode): the check reads the policy as the decoder
// builds it, so a candidate a merge key brings in, at the root from a
// reserved block or inside the policy or a candidate, an anchored candidate
// list, and a candidate value tagged !!str are refused where they land, and
// so is an agent name taken from the environment. ParseConfig refuses as
// Load does. Any other tag on a placeholder is refused before
// (loader_placeholder_spell_test.go).
func TestLoad_RefusesCandidatePlaceholdersWhereTheDecoderPutsThem(t *testing.T) {
	t.Setenv("OMP_PIN", "sk-proj-AbC123secret")
	t.Setenv("OMP_KEY", "executor")
	pin := "{selector: \"${OMP_PIN}/m\", thinking: high, family: anthropic}"
	policy := "role_model_policy:\n  version: v1\n  agents:\n    executor:\n      candidates: [" + pin + "]\n"
	nested := "  " + strings.ReplaceAll(strings.TrimSuffix(policy, "\n"), "\n", "\n  ") + "\n"
	executor := "role_model_policy.agents.executor.candidates[0].selector"
	for name, row := range map[string]struct{ text, path string }{
		"root merge from future_extension":   {"future_extension: &X\n" + nested + "<<: *X\n", executor},
		"root merge from operator_extension": {"operator_extension: &X\n" + nested + "<<: *X\n", executor},
		"root merge list":                    {"future_extension: &X\n" + nested + "<<: [*X]\n", executor},
		"merge inside the policy": {"future_extension:\n  p: &P\n    agents:\n      executor:\n        candidates: [" + pin + "]\n" +
			"role_model_policy:\n  version: v1\n  <<: *P\n", executor},
		"merge into a candidate": {"future_extension:\n  base: &base " + pin + "\n" +
			"role_model_policy:\n  version: v1\n  agents:\n    executor:\n      candidates:\n        - <<: *base\n", executor},
		"anchored candidate list": {"future_extension:\n  list: &list [" + pin + "]\n" +
			"role_model_policy:\n  version: v1\n  agents:\n    executor:\n      candidates: *list\n", executor},
		"profile agent override": {"role_model_policy:\n  version: v1\n  profile: p1\n  profiles:\n    p1:\n      agents:\n" +
			"        executor:\n          candidates: [" + pin + "]\n", "role_model_policy.profiles.p1.agents.executor.candidates[0].selector"},
		"string tag": {"role_model_policy:\n  version: v1\n  agents:\n    executor:\n" +
			"      candidates: [{selector: !!str \"${OMP_PIN}/m\", thinking: high, family: anthropic}]\n", executor},
		"agent name from env": {"role_model_policy:\n  version: v1\n  agents:\n    ${OMP_KEY}:\n" +
			"      candidates: [{selector: anthropic/claude-opus-5-5, thinking: high, family: anthropic}]\n",
			"role_model_policy.agents.${OMP_KEY}"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writePlaceholderPolicy(t, row.text)
			data, err := os.ReadFile(filepath.Join(dir, configFileName))
			require.NoError(t, err)

			_, loadErr := LoadPreview(dir)
			_, parseErr := ParseConfig(data)

			for _, err := range []error{loadErr, parseErr} {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "role_model_policy.candidate_placeholder_unsupported: "+row.path)
				assert.NotContains(t, err.Error(), "AbC123secret")
			}
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
