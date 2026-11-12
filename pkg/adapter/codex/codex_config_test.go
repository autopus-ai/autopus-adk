package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareConfigFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, codexConfigRelPath, files[0].TargetPath)
	assert.NoFileExists(t, filepath.Join(dir, ".codex", "config.toml"))
	assert.Contains(t, string(files[0].Content), "test-project")
	assert.Contains(t, string(files[0].Content), "context7")
	rootSection := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.NotContains(t, rootSection, "\nmodel =")
	assert.NotContains(t, rootSection, "model_reasoning_effort")
}

func TestGenerateConfig_PreservesExistingCodexModelSettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte(`model = "custom-model"
model_reasoning_effort = "ultra"
model_reasoning_summary = "detailed"
model_verbosity = "high"
approval_policy = "never"
`), 0644))

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	content := string(files[0].Content)

	assert.Contains(t, content, `model = "custom-model"`)
	assert.Contains(t, content, `model_reasoning_effort = "ultra"`)
	assert.Contains(t, content, `model_reasoning_summary = "detailed"`)
	assert.Contains(t, content, `model_verbosity = "high"`)
	assert.Contains(t, content, `approval_policy = "on-request"`)
}

func TestGenerateConfig_PreservesUserModelValueLiteral(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := NewWithRoot(dir)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte("model='custom/model' # keep\nmodel_reasoning_effort='ultra' # keep\n"), 0644))

	files, err := a.prepareConfigFile(config.DefaultFullConfig("literal-project"))
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, "model = 'custom/model' # keep")
	assert.Contains(t, root, "model_reasoning_effort = 'ultra' # keep")
	assert.Contains(t, root, codexUserModelMarker)
}

func TestGenerateConfig_PreservesQuotedUserModelKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`"model" = "custom-model"
'model_reasoning_effort' = 'high'
`), 0o644))

	files, err := a.prepareConfigFile(config.DefaultFullConfig("quoted-key-project"))
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, `model = "custom-model"`)
	assert.Contains(t, root, `model_reasoning_effort = 'high'`)
	assert.Contains(t, root, codexUserModelMarker)
}

func TestGenerateConfig_IgnoresModelAndMarkerInsideMultilineString(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`notes = """
model = "spoofed-model"
# Autopus: user-owned Codex model settings
"""
approval_policy = "never"
`), 0o644))

	files, err := a.prepareConfigFile(config.DefaultFullConfig("multiline-project"))
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[features]", 2)[0]
	assert.Contains(t, root, "spoofed-model", "unknown user TOML content must be preserved")
	assert.Contains(t, root, codexUserModelMarker, "the multiline comment remains user data")
	assert.False(t, hasStandaloneCodexComment(root, codexUserModelMarker))
	assert.NotContains(t, collectCodexConfigOverrides(root), ".model")
}

func TestGenerateConfig_TOMLCommentCannotSpoofMultilineString(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`notes = "ok" # """
"model" = "custom-after-comment"
`), 0o644))

	files, err := a.prepareConfigFile(config.DefaultFullConfig("comment-project"))
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, `model = "custom-after-comment"`)
	assert.Contains(t, root, codexUserModelMarker+": model")
}

func TestGenerateConfig_PreservesUntrackedUserConfigEvenWhenValuesMatchManagedProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte("model = \"gpt-5.5\"\nmodel_reasoning_effort = \"xhigh\"\n"), 0644))

	files, err := a.prepareConfigFile(config.DefaultFullConfig("user-config"))
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, `model = "gpt-5.5"`)
	assert.Contains(t, root, `model_reasoning_effort = "xhigh"`)
	assert.Contains(t, root, codexUserModelMarker)
}

func TestGenerateConfig_UsesUltraQualityProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.Default = "ultra"
	cfg.Quality.SupervisorModelPolicy = "quality"

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	content := string(files[0].Content)

	rootSection := strings.SplitN(content, "[features]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-6-astra"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "ultra"`)
}

func TestGenerateConfig_DefaultSupervisorPolicyInheritsCodexRuntimeModel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("inherit-project")

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	rootSection := strings.SplitN(string(files[0].Content), "[features]", 2)[0]

	assert.NotContains(t, rootSection, "\nmodel =")
	assert.NotContains(t, rootSection, "model_reasoning_effort")
}

func TestPrepareConfigFile_NoDiskWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	assert.Len(t, files, 1)

	_, err = os.Stat(filepath.Join(dir, ".codex", "config.toml"))
	assert.True(t, os.IsNotExist(err))
}

func TestGenerateConfig_MCPServers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	content := string(files[0].Content)
	assert.NotContains(t, content, "[mcp_servers.autopus]")
	assert.NotContains(t, content, `args = ["mcp", "server"]`)
	assert.Contains(t, content, "[mcp_servers.context7]")
	assert.Contains(t, content, `command = "npx"`)
	assert.Contains(t, content, `args = ["-y", "--prefix=/", "@upstash/context7-mcp@4.5.1"]`)
	assert.NotContains(t, content, "@anthropic-ai/context7-mcp")
	assert.NotContains(t, strings.SplitN(content, "[features]", 2)[0], "\nmodel =")
	assert.Contains(t, content, `approval_policy = "on-request"`)
	assert.Contains(t, content, `sandbox_mode = "workspace-write"`)
	assert.Contains(t, content, `web_search = "cached"`)
	assert.Contains(t, content, "project_doc_max_bytes = 262144")
	assert.Contains(t, content, "[features]")
	assert.Contains(t, content, "goals = true")
	assert.Contains(t, content, "hooks = true")
	assert.Contains(t, content, "shell_tool = true")
	assert.Contains(t, content, "unified_exec = true")
	assert.Contains(t, content, "[features.multi_agent_v2]")
	assert.Contains(t, content, "max_concurrent_threads_per_session = 4")
	assert.NotContains(t, content, "max_threads")
	assert.NotContains(t, content, "max_depth")
	assert.NotContains(t, content, "job_max_runtime_seconds")
	assert.NotContains(t, content, "\nmulti_agent =")
}

func TestGenerateConfig_EnablesBundledBrowserPlugin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	content := string(files[0].Content)

	assert.Contains(t, content, `[plugins."browser@openai-bundled"]`)
	assert.Contains(t, content, "enabled = true")
}

func TestValidateConfig_WarnsWhenBundledBrowserPluginMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte(`model = "gpt-5.5"
model_reasoning_effort = "medium"
approval_policy = "on-request"
sandbox_mode = "workspace-write"
web_search = "cached"
project_doc_max_bytes = 262144
`), 0644))

	var errs []adapter.ValidationError
	a.validateConfig(&errs)

	found := false
	for _, e := range errs {
		if e.File == codexConfigRelPath && e.Message == "Codex bundled browser plugin이 enabled 상태가 아님" {
			found = true
		}
	}
	assert.True(t, found, "missing browser plugin enablement should warn")
}

func TestValidateConfig_WarnsWhenGoalOrMultiAgentV2FeatureMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte(`model = "gpt-5.5"
model_reasoning_effort = "medium"
approval_policy = "on-request"
sandbox_mode = "workspace-write"
web_search = "cached"
project_doc_max_bytes = 262144

[features]
shell_tool = true

[plugins."browser@openai-bundled"]
enabled = true
`), 0644))

	var errs []adapter.ValidationError
	a.validateConfig(&errs)

	messages := make(map[string]bool)
	for _, e := range errs {
		messages[e.Message] = true
	}
	assert.True(t, messages["Codex goals feature가 enabled 상태가 아님"])
	assert.True(t, messages["Codex multi_agent_v2 feature가 enabled 상태가 아님"])
	assert.True(t, messages["Codex agent concurrency 설정이 없음: 'auto update' 실행 필요"])
}
