package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareFiles_UsesNativeUniqueSkillsAndV2Contracts(t *testing.T) {
	t.Parallel()
	a := NewWithRoot(t.TempDir())
	files, err := a.prepareFiles(config.DefaultFullConfig("v2-project"))
	require.NoError(t, err)

	seen := make(map[string]bool)
	for _, file := range files {
		path := filepath.ToSlash(file.TargetPath)
		assert.False(t, seen[path], "duplicate mapping: %s", path)
		seen[path] = true
		assert.False(t, strings.HasPrefix(path, ".codex/prompts/"), path)
		assert.False(t, strings.HasPrefix(path, ".codex/rules/") && strings.HasSuffix(path, ".md"), path)
		assert.False(t, strings.HasPrefix(path, ".agents/skills/"), path)
		if strings.HasPrefix(path, ".codex/skills/") {
			parts := strings.Split(path, "/")
			if len(parts) == 5 && parts[3] == "references" {
				// A reference body is a resource of the skill above it, not a
				// second entrypoint, so the codex-*/SKILL.md contract does not
				// apply to its own name.
				assert.True(t, strings.HasPrefix(parts[2], "codex-"), path)
				assert.True(t, strings.HasSuffix(parts[4], ".md"), path)
				assert.True(t, seen[".codex/skills/"+parts[2]+"/SKILL.md"] || pendingCodexEntrypoint(files, parts[2]),
					"resource %s has no skill entrypoint beside it", path)
			} else {
				require.Len(t, parts, 4, path)
				assert.Equal(t, "SKILL.md", parts[3])
				assert.True(t, strings.HasPrefix(parts[2], "codex-"), path)
				assert.Contains(t, string(file.Content), "name: "+parts[2])
				assertCodexNativeFrontmatter(t, path, string(file.Content))
			}
		}
		if path == ".agents/plugins/marketplace.json" {
			assert.Equal(t, adapter.OverwriteMerge, file.OverwritePolicy)
		}
	}

	require.True(t, seen[".codex/skills/codex-auto/SKILL.md"])
	configBody := mappingContent(t, files, codexConfigRelPath)
	assert.Contains(t, configBody, "[features.multi_agent_v2]")
	assert.Contains(t, configBody, "max_concurrent_threads_per_session = 4")
	for _, legacy := range []string{"multi_agent =", "max_threads", "max_depth", "job_max_runtime_seconds"} {
		assert.NotContains(t, configBody, legacy)
	}

	contracts := mappingContent(t, files, ".codex/skills/codex-agent-teams/SKILL.md") +
		mappingContent(t, files, ".codex/skills/codex-agent-pipeline/SKILL.md") +
		mappingContent(t, files, ".codex/skills/codex-worktree-isolation/SKILL.md")
	for _, tool := range []string{"spawn_agent", "send_message", "followup_task", "wait_agent", "interrupt_agent", "list_agents"} {
		assert.Contains(t, contracts, tool)
	}
	for _, obsolete := range []string{"send_input", "resume_agent", "close_agent", "forked workspace", "forked workspace", "auto-merges worktree", "merge worktree branch"} {
		assert.NotContains(t, contracts, obsolete)
	}
	assert.Contains(t, contracts, "shared cwd")
	assert.Contains(t, contracts, "disjoint write ownership")
}

// pendingCodexEntrypoint reports whether the mapping set contains the SKILL.md
// entrypoint for a skill directory, regardless of emission order.
func pendingCodexEntrypoint(files []adapter.FileMapping, skillDir string) bool {
	want := ".codex/skills/" + skillDir + "/SKILL.md"
	for _, file := range files {
		if filepath.ToSlash(file.TargetPath) == want {
			return true
		}
	}
	return false
}

// assertCodexNativeFrontmatter pins the projection boundary between the
// canonical catalog file and the Codex surface: the emitted entrypoint opens
// with exactly one closed frontmatter block, and that block carries native
// keys only. Catalog-authoring metadata such as triggers or level3_resources
// belongs to the source, and leaking it here would ship a second, conflicting
// description of the skill to the runtime.
func assertCodexNativeFrontmatter(t *testing.T, path, content string) {
	t.Helper()
	require.True(t, strings.HasPrefix(content, "---\n"), "%s must open with frontmatter", path)
	head, body, found := strings.Cut(strings.TrimPrefix(content, "---\n"), "\n---")
	require.True(t, found, "%s must close its frontmatter block", path)
	assert.NotContains(t, body, "\n---\nname:", "%s must not carry a second frontmatter block", path)
	for _, sourceOnly := range []string{"triggers:", "category:", "level1_metadata:", "level3_resources:"} {
		assert.NotContains(t, head, sourceOnly,
			"%s must project native frontmatter only, not canonical catalog metadata", path)
	}
}

func TestPrepareConfig_PreservesUnknownUserTOMLAndFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o755))
	existing := `# user comment
user_root = "keep"
approval_policy = "never" # old value

[features]
custom_feature = true
multi_agent = true

[mcp_servers.user]
command = "user-mcp"

[plugins."user@local"]
enabled = false

[profiles.careful]
model = "user-model"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, codexConfigRelPath), []byte(existing), 0o644))

	files, err := NewWithRoot(dir).prepareConfigFile(config.DefaultFullConfig("v2-project"))
	require.NoError(t, err)
	body := string(files[0].Content)
	for _, preserved := range []string{"# user comment", `user_root = "keep"`, "custom_feature = true", "[mcp_servers.user]", `command = "user-mcp"`, `[plugins."user@local"]`, "[profiles.careful]"} {
		assert.Contains(t, body, preserved)
	}
	assert.Contains(t, body, `approval_policy = "on-request" # old value`)
	// A key the user wrote under [features] is user content, even when the
	// harness itself has migrated to the v2 feature block. Preservation is the
	// contract; the harness only adds its own managed [features.multi_agent_v2].
	assert.Contains(t, body, "multi_agent = true")
	assert.Contains(t, body, "[features.multi_agent_v2]")

	require.NoError(t, os.WriteFile(filepath.Join(dir, codexConfigRelPath), []byte("[features\ngoals = true\n"), 0o644))
	_, err = NewWithRoot(dir).prepareConfigFile(config.DefaultFullConfig("v2-project"))
	assert.Error(t, err)
}

func TestUpdatePlan_PrunesOnlyOldManifestSurfaces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	old := adapter.NewManifest(adapterName)
	for _, path := range []string{
		".codex/skills/auto.md", ".codex/prompts/auto.md",
		".codex/rules/autopus/context7-docs.md", ".agents/skills/auto/SKILL.md",
	} {
		old.Files[path] = adapter.ManifestFile{Checksum: "old", Policy: adapter.OverwriteAlways}
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte("old"), 0o644))
	}
	userSkill := filepath.Join(dir, ".codex", "skills", "user-owned", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(userSkill), 0o755))
	require.NoError(t, os.WriteFile(userSkill, []byte("user"), 0o644))

	a := NewWithRoot(dir)
	files, err := a.prepareFilesWithManifest(config.DefaultFullConfig("v2-project"), old)
	require.NoError(t, err)
	plan, _, err := a.buildUpdateTransactionPlan(config.DefaultFullConfig("v2-project"), old, files)
	require.NoError(t, err)
	removed := make(map[string]bool)
	for _, entry := range plan.Removes {
		removed[filepath.ToSlash(entry.Path)] = true
	}
	for path := range old.Files {
		assert.True(t, removed[filepath.ToSlash(path)], path)
	}
	assert.False(t, removed[".codex/skills/user-owned/SKILL.md"])
}

func TestClean_PreservesUserSkillsHooksMarketplaceAndConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	managed := []string{
		".codex/skills/codex-auto/SKILL.md", ".codex/skills/auto.md",
		".agents/skills/auto/SKILL.md", ".codex/hooks.json",
		".agents/plugins/marketplace.json", codexConfigRelPath,
	}
	manifest := adapter.NewManifest(adapterName)
	managedContent := map[string]string{
		managed[0]: "managed",
		managed[1]: "legacy",
		managed[2]: "legacy",
	}
	for _, path := range managed {
		checksum := "managed-merge"
		if content, ok := managedContent[path]; ok {
			checksum = adapter.Checksum(content)
		}
		manifest.Files[path] = adapter.ManifestFile{Checksum: checksum, Policy: adapter.OverwriteAlways}
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755))
	}
	require.NoError(t, manifest.Save(dir))
	for path, content := range managedContent {
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644))
	}
	userSkill := filepath.Join(dir, ".codex", "skills", "user-owned", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(userSkill), 0o755))
	require.NoError(t, os.WriteFile(userSkill, []byte("user"), 0o644))
	hooks := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"user-hook"}]},{"__autopus__":true,"matcher":"","hooks":[{"type":"command","command":"auto context bootstrap","statusMessage":"Running Autopus hook"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".codex", "hooks.json"), []byte(hooks), 0o644))
	market := `{"name":"user-market","plugins":[{"name":"user","source":{"source":"local","path":"./user"},"policy":{"installation":"AVAILABLE","authentication":"NONE"},"category":"Other"},{"name":"auto","source":{"source":"local","path":"./.autopus/plugins/auto"},"policy":{"installation":"AVAILABLE","authentication":"ON_INSTALL"},"category":"Developer Tools"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".agents", "plugins", "marketplace.json"), []byte(market), 0o644))
	cfgBody := "# keep\nuser_root = true\napproval_policy = \"on-request\"\n\n[features]\ngoals = true\n\n[profiles.user]\nmodel = \"mine\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, codexConfigRelPath), []byte(cfgBody), 0o644))

	require.NoError(t, NewWithRoot(dir).Clean(context.Background()))
	assert.FileExists(t, userSkill)
	assert.NoFileExists(t, filepath.Join(dir, managed[0]))
	assert.NoFileExists(t, filepath.Join(dir, managed[1]))
	assert.NoFileExists(t, filepath.Join(dir, managed[2]))
	cleanHooks, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	require.NoError(t, err)
	assert.Contains(t, string(cleanHooks), "user-hook")
	assert.NotContains(t, string(cleanHooks), autopusHookStatusMessage)
	cleanMarket, err := os.ReadFile(filepath.Join(dir, ".agents", "plugins", "marketplace.json"))
	require.NoError(t, err)
	assert.Contains(t, string(cleanMarket), `"name": "user"`)
	assert.NotContains(t, string(cleanMarket), `"name": "auto"`)
	cleanConfig, err := os.ReadFile(filepath.Join(dir, codexConfigRelPath))
	require.NoError(t, err)
	assert.Contains(t, string(cleanConfig), "# keep")
	assert.Contains(t, string(cleanConfig), "[profiles.user]")
	assert.NotContains(t, string(cleanConfig), "approval_policy")
	assert.NoFileExists(t, filepath.Join(dir, ".autopus", "codex-manifest.json"))
}

func mappingContent(t *testing.T, files []adapter.FileMapping, target string) string {
	t.Helper()
	for _, file := range files {
		if filepath.ToSlash(file.TargetPath) == filepath.ToSlash(target) {
			return string(file.Content)
		}
	}
	t.Fatalf("mapping %s not found", target)
	return ""
}
