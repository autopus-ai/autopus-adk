package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestNormalizeOMPRoleModelFamilyAcceptsCanonicalAndAliases(t *testing.T) {
	for _, row := range []struct{ input, expected string }{
		{"", ""},
		{"anthropic", "anthropic"},
		{"openai", "openai"},
		{"claude", "anthropic"},
		{"gpt", "openai"},
		{"  Claude  ", "anthropic"},
	} {
		normalized, err := normalizeOMPRoleModelFamily(row.input)
		require.NoError(t, err, row.input)
		assert.Equal(t, row.expected, normalized, row.input)
	}
	for _, invalid := range []string{"gemini", "openai-codex", "anthropic/claude"} {
		_, err := normalizeOMPRoleModelFamily(invalid)
		require.Error(t, err, invalid)
		assert.Contains(t, err.Error(), "family_invalid")
	}
}

func TestParseOMPProfileAgentAssignmentsAcceptsPinsAndInherit(t *testing.T) {
	assignments, err := parseOMPProfileAgentAssignments([]string{
		"debugger=anthropic/claude-fable-5-1:max", "executor=inherit",
	})
	require.NoError(t, err)
	require.Len(t, assignments, 2)
	assert.Equal(t, ompProfileAgentAssignment{
		agent: "debugger", selector: "anthropic/claude-fable-5-1", thinking: "max",
	}, assignments[0])
	assert.Equal(t, ompProfileAgentAssignment{agent: "executor", inherit: true}, assignments[1])
}

func TestParseOMPProfileAgentAssignmentsRejectsInvalidInput(t *testing.T) {
	for name, row := range map[string]struct {
		values    []string
		substring string
	}{
		"duplicate agent":  {[]string{"debugger=anthropic/claude-fable-5-1:max", "debugger=inherit"}, "agent_override_duplicate"},
		"unknown agent":    {[]string{"orchestrator=anthropic/claude-fable-5-1:max"}, "agent_override_unknown_agent"},
		"missing thinking": {[]string{"debugger=anthropic/claude-fable-5-1"}, "agent_override_malformed"},
		"missing provider": {[]string{"debugger=claude-fable-5-1:max"}, "agent_override_malformed"},
		"empty value":      {[]string{"debugger="}, "agent_override_malformed"},
		"no separator":     {[]string{"debugger"}, "agent_override_malformed"},
		"unknown thinking": {[]string{"debugger=anthropic/claude-fable-5-1:turbo"}, "agent_override_thinking_invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseOMPProfileAgentAssignments(row.values)
			require.Error(t, err)
			assert.Contains(t, err.Error(), row.substring)
		})
	}
}

// ompCLIBalancedCatalogJSON mirrors an installed OMP catalog that carries every
// model the built-in balanced matrix pins in both families.
func ompCLIBalancedCatalogJSON() []byte {
	return []byte(`{"models":[
		{"provider":"anthropic","id":"claude-fable-5-1","family":"anthropic","capabilities":["deep_reasoning","coding_tool_use","independent_dissent"],"thinking":["high","max"],"auth_enabled":true,"keyless":false,"disabled":false},
		{"provider":"anthropic","id":"claude-sonnet-5-5","family":"anthropic","capabilities":["coding_tool_use","fast_validation","vision_design","deterministic_transform","independent_dissent"],"thinking":["high","max"],"auth_enabled":true,"keyless":false,"disabled":false},
		{"provider":"openai-codex","id":"gpt-6-astra","family":"openai","capabilities":["deep_reasoning","coding_tool_use","independent_dissent"],"thinking":["high","max"],"auth_enabled":true,"keyless":false,"disabled":false},
		{"provider":"openai-codex","id":"gpt-6-luna","family":"openai","capabilities":["coding_tool_use","fast_validation","vision_design","deterministic_transform","independent_dissent"],"thinking":["high","max"],"auth_enabled":true,"keyless":false,"disabled":false}
	]}`)
}

// ompCLIProfileNativeCatalogJSON mirrors what an installed OMP actually prints: exact
// provider, id, and thinking levels, and no family or capability metadata at
// all. Strict normalization reports catalog_metadata_insufficient here, so the
// operator-attested path of the selected profile has to carry the semantics.
func ompCLIProfileNativeCatalogJSON() []byte {
	return []byte(`{"models":[
		{"provider":"anthropic","id":"claude-fable-5-1","thinking":["high","max"],"available":true},
		{"provider":"anthropic","id":"claude-sonnet-5-5","thinking":["high","max"],"available":true},
		{"provider":"openai-codex","id":"gpt-6-astra","thinking":["high","max"],"available":true},
		{"provider":"openai-codex","id":"gpt-6-luna","thinking":["high","max"],"available":true}
	]}`)
}

// ompCLIBalancedCatalogWithout drops one selector so a plan run reports a named
// unavailability instead of substituting a lower tier.
func ompCLIBalancedCatalogWithout(t *testing.T, selector string) []byte {
	t.Helper()
	var raw struct {
		Models []map[string]any `json:"models"`
	}
	require.NoError(t, json.Unmarshal(ompCLIBalancedCatalogJSON(), &raw))
	kept := make([]map[string]any, 0, len(raw.Models))
	for _, model := range raw.Models {
		if model["provider"].(string)+"/"+model["id"].(string) == selector {
			continue
		}
		kept = append(kept, model)
	}
	require.Len(t, kept, len(raw.Models)-1)
	encoded, err := json.Marshal(map[string]any{"models": kept})
	require.NoError(t, err)
	return encoded
}

// writeOMPBalancedProject creates an OMP-enabled project with no role model
// policy so a built-in selection starts from a clean root.
func writeOMPBalancedProject(t *testing.T) (string, *ompCLIFakeRunner) {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-balanced")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	return root, &ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}
}

// ompBalancedDeps never falls back to the real harness activation: a plan-only
// test that unexpectedly activates must fail instead of writing agent files.
func ompBalancedDeps(runner *ompCLIFakeRunner, activate ompProfileActivator) ompPlatformDependencies {
	if activate == nil {
		activate = func(context.Context, string, *config.HarnessConfig) error {
			return errors.New("unexpected OMP activation")
		}
	}
	return normalizeOMPPlatformDependencies(ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate:  activate,
	})
}

// readAutopusConfigTree parses autopus.yaml into a generic tree so a test can
// assert that keys outside role_model_policy were left byte-identical.
func readAutopusConfigTree(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)
	tree := map[string]any{}
	require.NoError(t, yaml.Unmarshal(data, &tree))
	return tree
}

func agentPreviewRow(
	t *testing.T,
	payload ompProfileApplyPreviewPayload,
	agent string,
) ompProfileAgentPreviewPayload {
	t.Helper()
	for _, row := range payload.Agents {
		if row.Agent == agent {
			return row
		}
	}
	t.Fatalf("agent %q missing from plan preview", agent)
	return ompProfileAgentPreviewPayload{}
}
