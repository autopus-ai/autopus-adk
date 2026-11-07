package adapter_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

type generatedOrchestrationContract struct {
	Schema    string         `json:"schema"`
	Workflow  string         `json:"workflow"`
	Semantics map[string]any `json:"semantics"`
}

func TestGeneratedOrchestration_SemanticContractsMatch(t *testing.T) {
	claudeRoot, codexRoot := generateContractSurfaces(t)
	cases := []struct {
		name           string
		claudePath     string
		codexPath      string
		requiredFields map[string]any
	}{
		{
			name:       "review",
			claudePath: ".claude/skills/auto-review/SKILL.md",
			codexPath:  ".codex/skills/codex-auto-review/SKILL.md",
			requiredFields: map[string]any{
				"risk_tiered_review":             true,
				"forward_strategy_and_providers": true,
				"degraded_requires_override":     true,
				"discovery_verification_split":   true,
				"retry_budget":                   float64(2),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claudeContract, claudeOK := contractFromFile(t, claudeRoot, tc.claudePath)
			codexContract, codexOK := contractFromFile(t, codexRoot, tc.codexPath)
			assert.True(t, claudeOK, "Claude surface must contain orchestration-contract.v1")
			assert.True(t, codexOK, "Codex surface must contain orchestration-contract.v1")
			if !claudeOK || !codexOK {
				return
			}
			assert.Equal(t, tc.name, claudeContract.Workflow)
			assert.Equal(t, tc.name, codexContract.Workflow)
			assert.Equal(t, claudeContract.Semantics, codexContract.Semantics)
			for field, want := range tc.requiredFields {
				assert.Equal(t, want, claudeContract.Semantics[field], "semantic field %q", field)
			}
		})
	}
}

func TestGeneratedOrchestration_UsesOnlyNativePlatformPrimitives(t *testing.T) {
	claudeRoot, codexRoot := generateContractSurfaces(t)
	claudeBody := readSurfaces(t, claudeRoot, []string{
		".claude/skills/auto-review/SKILL.md",
	})
	codexBody := readSurfaces(t, codexRoot, []string{
		".codex/skills/codex-auto-review/SKILL.md",
	})

	for _, foreign := range []string{"spawn_agent(", "send_input(", "close_agent("} {
		assert.Zero(t, strings.Count(claudeBody, foreign), "Claude contains Codex primitive %q", foreign)
	}
	for _, foreign := range []string{"TeamCreate(", "TeamDelete(", "SendMessage(", "Agent("} {
		assert.Zero(t, strings.Count(codexBody, foreign), "Codex contains Claude primitive %q", foreign)
	}
	assert.NotContains(t, claudeBody, "TeamCreate(")
	assert.NotContains(t, claudeBody, "TeamDelete(")
}

func TestGeneratedOrchestration_UnsupportedPlatformsContainNoClaudeTeamPrimitives(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("unsupported-team-contract")
	geminiRoot := t.TempDir()
	opencodeRoot := t.TempDir()
	_, err := antigravity.NewWithRoot(geminiRoot).Generate(context.Background(), cfg)
	require.NoError(t, err)
	_, err = opencode.NewWithRoot(opencodeRoot, opencode.WithCLIVersion("1.18.7")).Generate(context.Background(), cfg)
	require.NoError(t, err)

	for name, root := range map[string]string{"gemini": geminiRoot, "opencode": opencodeRoot} {
		body := readGeneratedTree(t, root)
		for _, foreign := range []string{"TeamCreate", "TeamDelete", "SendMessage"} {
			assert.Zero(t, strings.Count(body, foreign), "%s contains unsupported Claude primitive %q", name, foreign)
		}
		assert.NotContains(t, body, "agent-teams/SKILL.md", "%s contains a dangling team-skill reference", name)
		assert.NotContains(t, body, "skills/autopus/agent-teams.md", "%s contains a dangling team-skill reference", name)
	}
}

func generateContractSurfaces(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	cfg := config.DefaultFullConfig("contract-test")
	claudeRoot := t.TempDir()
	codexRoot := t.TempDir()
	_, err := claude.NewWithRoot(claudeRoot).Generate(ctx, cfg)
	require.NoError(t, err)
	_, err = codex.NewWithRoot(codexRoot).Generate(ctx, cfg)
	require.NoError(t, err)
	return claudeRoot, codexRoot
}

func contractFromFile(t *testing.T, root, path string) (generatedOrchestrationContract, bool) {
	t.Helper()
	body := readSurfaces(t, root, []string{path})
	remaining := body
	for {
		start := strings.Index(remaining, "```json")
		if start < 0 {
			return generatedOrchestrationContract{}, false
		}
		remaining = remaining[start+len("```json"):]
		end := strings.Index(remaining, "```")
		if end < 0 {
			return generatedOrchestrationContract{}, false
		}
		var candidate generatedOrchestrationContract
		if json.Unmarshal([]byte(strings.TrimSpace(remaining[:end])), &candidate) == nil &&
			candidate.Schema == "orchestration-contract.v1" {
			return candidate, true
		}
		remaining = remaining[end+len("```"):]
	}
}

func readSurfaces(t *testing.T, root string, paths []string) string {
	t.Helper()
	var body strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, path))
		require.NoError(t, err, "read generated surface %s", path)
		body.Write(data)
		body.WriteByte('\n')
	}
	return body.String()
}

func readGeneratedTree(t *testing.T, root string) string {
	t.Helper()
	var body strings.Builder
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		body.Write(data)
		body.WriteByte('\n')
		return nil
	}))
	return body.String()
}
