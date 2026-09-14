package antigravity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The published manifest schema declares `additionalProperties: false` with
// only `name` and `description`, so an extra key (including `$schema`) makes
// the bundle invalid under strict validation.
func TestPrepareAntigravityPluginJSON_MatchesPublishedManifestSchema(t *testing.T) {
	t.Parallel()

	files, err := prepareAntigravityPluginJSON()
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, ".agents/plugins/autopus/plugin.json", files[0].TargetPath)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(files[0].Content, &parsed))
	assert.Equal(t, "autopus", parsed["name"])
	assert.NotEmpty(t, parsed["description"])

	keys := make([]string, 0, len(parsed))
	for key := range parsed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{"description", "name"}, keys)
	assert.Regexp(t, `^[a-zA-Z0-9-_]+$`, parsed["name"])
}

// `agy` loads plugin components from skills/, agents/, commands/ and rules/.
// It has no notion of a workspace-level `.agents/commands` root, so a mapping
// onto that path would emit files no CLI reads.
func TestAntigravityPluginTarget_CoversLoadedComponentsOnly(t *testing.T) {
	t.Parallel()

	supported := map[string]string{
		".gemini/skills/autopus/auto-plan/SKILL.md": ".agents/plugins/autopus/skills/auto-plan/SKILL.md",
		".gemini/skills/auto/SKILL.md":              ".agents/plugins/autopus/skills/auto/SKILL.md",
		".gemini/rules/autopus/branding.md":         ".agents/plugins/autopus/rules/branding.md",
		".gemini/agents/autopus/executor.md":        ".agents/plugins/autopus/agents/executor.md",
		".gemini/commands/auto.toml":                ".agents/plugins/autopus/commands/auto.toml",
		".gemini/commands/auto/plan.toml":           ".agents/plugins/autopus/commands/auto/plan.toml",
	}
	for source, want := range supported {
		got, ok := antigravityPluginTarget(source)
		require.True(t, ok, source)
		assert.Equal(t, want, got)
	}

	for _, unsupported := range []string{
		".gemini/settings.json",
		".gemini/statusline.sh",
		"GEMINI.md",
	} {
		_, ok := antigravityPluginTarget(unsupported)
		assert.False(t, ok, unsupported)
	}
}

func TestPrepareCommandMappings_EmitsNoWorkspaceCommandsRoot(t *testing.T) {
	t.Parallel()
	a := NewWithRoot(t.TempDir())

	files, err := a.prepareCommandMappings(config.DefaultFullConfig("no-global-commands"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		assert.False(t, strings.HasPrefix(filepath.ToSlash(file.TargetPath), ".agents/commands/"),
			"%s is not an agy discovery path", file.TargetPath)
	}
}

var antigravityPluginPathRe = regexp.MustCompile(`\.agents/plugins/autopus/[A-Za-z0-9._/-]+`)

func TestGenerate_PluginBodiesReferenceInstalledFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := NewWithRoot(dir).Generate(context.Background(), config.DefaultFullConfig("plugin-refs"))
	require.NoError(t, err)

	pluginRoot := filepath.Join(dir, filepath.FromSlash(antigravityPluginDir))
	checked := 0
	require.NoError(t, filepath.WalkDir(pluginRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		assert.NotContains(t, string(body), ".agents/commands/", path)
		for _, reference := range antigravityPluginPathRe.FindAllString(string(body), -1) {
			checked++
			assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(reference)),
				"%s references a file the adapter never installs", path)
		}
		return nil
	}))
	assert.Positive(t, checked, "plugin bodies must route through installed plugin paths")
}

func TestGenerate_EmitsNoWorkspaceCommandsDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := NewWithRoot(dir).Generate(context.Background(), config.DefaultFullConfig("no-global-commands"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, ".agents", "commands"))
	assert.True(t, os.IsNotExist(err), "workspace .agents/commands is not read by agy")
}

// Generate must stay project-local: staging the workspace plugin into the
// user's global registry would publish project-rendered content to every other
// project and shadow-collide with the workspace copy.
func TestGenerate_DoesNotInvokeTheCLI(t *testing.T) {
	dir := t.TempDir()
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "agy-invoked")
	script := "#!/bin/sh\necho \"$@\" >> " + marker + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, cliBinary), []byte(script), 0o755))
	t.Setenv("PATH", binDir)

	_, err := NewWithRoot(dir).Generate(context.Background(), config.DefaultFullConfig("no-cli-side-effects"))
	require.NoError(t, err)

	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "Generate must not shell out to the Antigravity CLI")
}

func TestClean_PreservesSharedSkillsAndOwnedPluginRemoval(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("clean-preserve"))
	require.NoError(t, err)

	foreignSkill := filepath.Join(dir, ".agents", "skills", "opencode-owned", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(foreignSkill), 0o755))
	require.NoError(t, os.WriteFile(foreignSkill, []byte("---\nname: opencode-owned\n---\n"), 0o644))

	require.NoError(t, a.Clean(context.Background()))

	assert.FileExists(t, foreignSkill, "shared .agents/skills belongs to other adapters and the user")
	assert.NoDirExists(t, filepath.Join(dir, filepath.FromSlash(antigravityPluginDir)))
}

// Clean withdraws what the adapter installed and nothing else. Dropping the
// permission block would silently relax a fail-closed posture the user chose.
func TestRemoveManagedSettingsKeys_WithdrawsOnlyOwnedEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".gemini", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(settingsPath), 0o755))
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("settings-clean")
	original, err := a.renderAuthoredSettings(cfg)
	require.NoError(t, err)
	applyAntigravityHooksAndPermissions(original, a.configuredLegacyGeminiHooks(cfg), nil)
	original["theme"] = "user-choice"
	original["hooks"].(map[string]any)["UserPreStart"] = []any{map[string]any{"matcher": "*"}}
	original["mcpServers"].(map[string]any)["my-server"] = map[string]any{"command": "./mine"}
	original["permissions"] = map[string]any{"deny": []any{"command(rm -rf)"}}
	body, err := json.MarshalIndent(original, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settingsPath, append(body, '\n'), 0o644))

	require.NoError(t, NewWithRoot(dir).removeManagedSettingsKeys())

	remaining := readAntigravitySettings(settingsPath)
	assert.Equal(t, "user-choice", remaining["theme"])
	assert.Equal(t, map[string]any{"deny": []any{"command(rm -rf)"}}, remaining["permissions"])
	hooks, ok := remaining["hooks"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, hooks, "BeforeTool")
	assert.NotContains(t, hooks, "AfterTool")
	assert.NotContains(t, hooks, "AfterAgent")
	assert.Contains(t, hooks, "UserPreStart")
	servers, ok := remaining["mcpServers"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, servers, "context7")
	assert.Contains(t, servers, "my-server")
}

func TestRemoveManagedSettingsKeys_RemovesFileWithNothingLeft(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".gemini", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(settingsPath), 0o755))
	authored, err := NewWithRoot(dir).renderAuthoredSettings(config.DefaultFullConfig("settings-clean"))
	require.NoError(t, err)
	raw, err := json.Marshal(authored)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settingsPath, raw, 0o644))

	require.NoError(t, NewWithRoot(dir).removeManagedSettingsKeys())
	assert.NoFileExists(t, settingsPath)
}

func TestUpdate_PrunesObsoleteOwnedPathsButKeepsUserEdits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("prune-legacy")
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	untouched := ".agents/commands/auto.toml"
	edited := ".agents/plugins/autopus/commands/legacy.toml"
	writeLegacyAntigravityArtifact(t, dir, untouched, "generated body\n")
	writeLegacyAntigravityArtifact(t, dir, edited, "generated body\n")

	manifest, err := adapter.LoadManifest(dir, adapterName)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	manifest.Files[untouched] = adapter.ManifestFile{
		Checksum: checksum("generated body\n"),
		Policy:   adapter.OverwriteAlways,
	}
	manifest.Files[edited] = adapter.ManifestFile{
		Checksum: checksum("generated body\n"),
		Policy:   adapter.OverwriteAlways,
	}
	require.NoError(t, manifest.Save(dir))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, filepath.FromSlash(edited)), []byte("hand edited\n"), 0o644))

	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(untouched)),
		"an unmodified obsolete artifact this adapter recorded must be pruned")
	assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(edited)),
		"a user-edited artifact must survive the prune")
}

func writeLegacyAntigravityArtifact(t *testing.T, root, relative, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
