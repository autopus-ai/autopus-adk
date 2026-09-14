package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

type claudeSkillFrontmatter struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Extra       map[string]any `yaml:",inline"`
}

type claudeAgentFrontmatter struct {
	Name   string   `yaml:"name"`
	Model  string   `yaml:"model"`
	Effort string   `yaml:"effort"`
	Skills []string `yaml:"skills"`
}

func decodeClaudeFrontmatter(t *testing.T, content []byte, out any) {
	t.Helper()
	parts := strings.SplitN(string(content), "---", 3)
	require.Len(t, parts, 3)
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), out))
}

func TestPrepareFiles_ClaudeSkillsUseNativeLayoutAndResolveAgentReferences(t *testing.T) {
	t.Parallel()
	files, err := NewWithRoot(t.TempDir()).prepareFiles(config.DefaultFullConfig("native-skills"))
	require.NoError(t, err)

	skills := make(map[string]bool)
	resourceOwners := make(map[string]bool)
	for _, file := range files {
		path := filepath.ToSlash(file.TargetPath)
		if !strings.HasPrefix(path, ".claude/skills/") {
			continue
		}
		if owner, rel, ok := claudeSkillResourcePath(path); ok {
			// A reference body is a resource of the skill above it, not a second
			// entrypoint: it carries no frontmatter and owns no skill name.
			assert.True(t, strings.HasSuffix(rel, ".md"), "resource %s must be markdown", path)
			assert.NotContains(t, string(file.Content), "\ncompatibility:", "resource %s must not carry skill frontmatter", path)
			resourceOwners[owner] = true
			continue
		}
		assert.Equal(t, "SKILL.md", filepath.Base(path), path)
		var meta claudeSkillFrontmatter
		decodeClaudeFrontmatter(t, file.Content, &meta)
		dirName := filepath.Base(filepath.Dir(path))
		assert.Equal(t, dirName, meta.Name, path)
		if meta.Description == "" {
			t.Errorf("%s has empty skill description", path)
		}
		assert.Empty(t, meta.Extra, "Claude skill frontmatter must contain documented fields only: %s", path)
		skills[meta.Name] = true
	}

	for owner := range resourceOwners {
		assert.True(t, skills[owner], "resources were installed for %q but its SKILL.md entrypoint was not", owner)
	}

	for _, file := range files {
		path := filepath.ToSlash(file.TargetPath)
		if !strings.HasPrefix(path, ".claude/agents/") {
			continue
		}
		var meta claudeAgentFrontmatter
		decodeClaudeFrontmatter(t, file.Content, &meta)
		for _, name := range meta.Skills {
			assert.True(t, skills[name], "%s references unresolved skill %q", path, name)
		}
	}
}

// claudeSkillResourcePath splits `.claude/skills/<skill>/references/<file>.md`
// into its owning skill and relative file. It returns false for anything else,
// so an unexpected nested path still fails the entrypoint layout contract
// instead of being waved through as a resource.
func claudeSkillResourcePath(path string) (owner, rel string, ok bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 5 || parts[0] != ".claude" || parts[1] != "skills" || parts[3] != "references" {
		return "", "", false
	}
	return parts[2], parts[4], true
}

func TestPrepareFiles_ClaudeAgentQualityProjectsModelAndEffortTogether(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		mode       string
		agent      string
		wantModel  string
		wantEffort string
	}{
		{name: "balanced standard", mode: "balanced", agent: "tester", wantModel: config.ClaudeSonnetModel, wantEffort: "max"},
		{name: "balanced top rung", mode: "balanced", agent: "debugger", wantModel: config.ClaudeFableModel, wantEffort: "max"},
		{name: "ultra executor", mode: "ultra", agent: "executor", wantModel: "opus", wantEffort: "max"},
		{name: "ultra planner", mode: "ultra", agent: "planner", wantModel: "fable", wantEffort: "max"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.DefaultFullConfig("quality")
			cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: test.mode}
			files, err := NewWithRoot(t.TempDir()).prepareFiles(cfg)
			require.NoError(t, err)

			var got claudeAgentFrontmatter
			for _, file := range files {
				if filepath.Base(file.TargetPath) == test.agent+".md" && strings.Contains(filepath.ToSlash(file.TargetPath), "/agents/") {
					decodeClaudeFrontmatter(t, file.Content, &got)
					break
				}
			}
			assert.Equal(t, test.wantModel, got.Model)
			assert.Equal(t, test.wantEffort, got.Effort)
		})
	}
}

func TestClaudeAgentEffortUsesFourTierMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode string
		tier string
		want string
	}{
		{name: "ultra fable", mode: "ultra", tier: "fable", want: "max"},
		{name: "ultra opus", mode: "ultra", tier: "opus", want: "max"},
		{name: "ultra sonnet", mode: "ultra", tier: "sonnet", want: "high"},
		{name: "balanced fable", mode: "balanced", tier: "fable", want: "max"},
		{name: "balanced opus", mode: "balanced", tier: "opus", want: "high"},
		{name: "balanced sonnet", mode: "balanced", tier: "sonnet", want: "medium"},
		{name: "haiku omits effort", mode: "balanced", tier: "haiku", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, claudeAgentEffort(test.mode, test.tier))
		})
	}
}

func TestInstallHooks_PrunesRemovedTeamLifecyclePermissions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	settingsDir := filepath.Join(root, ".claude")
	require.NoError(t, os.MkdirAll(settingsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(settingsDir, "settings.json"),
		[]byte(`{"permissions":{"allow":["TeamCreate","TeamDelete","UserTool"]}}`),
		0o644,
	))

	err := NewWithRoot(root).InstallHooks(context.Background(), nil, &adapter.PermissionSet{
		Allow: []string{"Agent", "SendMessage"},
	})

	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(settingsDir, "settings.json"))
	require.NoError(t, err)
	var settings map[string]any
	require.NoError(t, json.Unmarshal(data, &settings))
	permissions, ok := settings["permissions"].(map[string]any)
	require.True(t, ok)
	allow, ok := permissions["allow"].([]any)
	require.True(t, ok)
	assert.ElementsMatch(t, []any{"UserTool", "Agent", "SendMessage"}, allow)
}
