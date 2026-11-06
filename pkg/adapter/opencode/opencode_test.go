package opencode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestNew_DefaultRoot(t *testing.T) {
	t.Parallel()
	a := New()
	assert.Equal(t, ".", a.root)
}

func TestNewWithRoot(t *testing.T) {
	t.Parallel()
	a := NewWithRoot("/some/path", WithCLIVersion("1.18.7"))
	assert.Equal(t, "/some/path", a.root)
}

func TestAdapter_Accessors(t *testing.T) {
	t.Parallel()
	a := New()
	assert.Equal(t, "opencode", a.Name())
	assert.Equal(t, "1.0.0", a.Version())
	assert.Equal(t, "opencode", a.CLIBinary())
	assert.True(t, a.SupportsHooks())
}

func TestAdapter_Detect_NoError(t *testing.T) {
	t.Parallel()
	a := NewWithRoot(t.TempDir(), WithCLIVersion("1.18.7"))
	_, err := a.Detect(context.Background())
	assert.NoError(t, err)
}

func TestAdapter_Generate_CreatesOpenCodeFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	cfg := config.DefaultFullConfig("demo")

	pf, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, pf.Files)

	assert.FileExists(t, filepath.Join(dir, "AGENTS.md"))
	assert.FileExists(t, filepath.Join(dir, "opencode.json"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "commands", "auto.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "commands", "auto-review.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "commands", "auto-qa.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "agents", "reviewer.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".opencode", "commands", "auto-plan.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".opencode", "agents", "planner.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "plugins", "autopus-hooks.js"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "verification", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".autopus", "opencode-manifest.json"))

	autoSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoSkill), "얇은 라우터")
	assert.Contains(t, string(autoSkill), "상세 스킬")
	assert.Contains(t, string(autoSkill), "## Router Contract")
	assert.Contains(t, string(autoSkill), "## Context Load")
	assert.Contains(t, string(autoSkill), "## SPEC Path Resolution")
	assert.Contains(t, string(autoSkill), "## Autopus Branding")
	assert.Contains(t, string(autoSkill), "🐙 Autopus ─────────────────────────")
	assert.NotContains(t, string(autoSkill), "mode =")

	autoReviewSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-review", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoReviewSkill), "수정 뒤 같은 범위로 `/auto review`를 다시 실행")
	assert.NotContains(t, string(autoReviewSkill), "/auto go")
	autoCommand, err := os.ReadFile(filepath.Join(dir, ".opencode", "commands", "auto.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoCommand), "Immediately load skill `auto`")
	assert.Contains(t, string(autoCommand), "Preserve `--model <provider/model>`")
	assert.Contains(t, string(autoCommand), "Do not restate or expand the arguments")

	agentsData, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(agentsData), markerBegin)
	assert.Contains(t, string(agentsData), "플랫폼")
	assert.NotContains(t, string(agentsData), "Codex Rules: .codex/rules/autopus/")

	configDoc := readConfigJSON(t, filepath.Join(dir, "opencode.json"))
	instructions := jsonStringSlice(configDoc["instructions"])
	assert.NotEmpty(t, instructions)
	assert.Contains(t, instructions, ".opencode/rules/autopus/language-policy.md")

}

func TestAdapter_Generate_AutoRouterUsesThinOpenCodeContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	autoCommand, readErr := os.ReadFile(filepath.Join(dir, ".opencode", "commands", "auto.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(autoCommand), "Immediately load skill `auto`")
	assert.NotContains(t, string(autoCommand), "## SPEC Path Resolution")
	assert.NotContains(t, string(autoCommand), "Codex용 canonical router surface")

	autoSkill, readErr := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(autoSkill), "## Router Contract")
	assert.Contains(t, string(autoSkill), "## Context Load")
	assert.Contains(t, string(autoSkill), "## SPEC Path Resolution")
	assert.Contains(t, string(autoSkill), "지원 서브커맨드")
	assert.Contains(t, string(autoSkill), "`update`")
	assert.Contains(t, string(autoSkill), "`auto-update`")
	assert.Contains(t, string(autoSkill), "/auto-qa")
	assert.NotContains(t, string(autoSkill), "Codex용 canonical router surface")
}

func TestAdapter_Generate_NilConfig(t *testing.T) {
	t.Parallel()
	a := NewWithRoot(t.TempDir(), WithCLIVersion("1.18.7"))
	_, err := a.Generate(context.Background(), nil)
	assert.Error(t, err)
}

func TestAdapter_Update_PreservesMergedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	cfg := config.DefaultFullConfig("demo")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Custom Header\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"share":"manual"}`), 0644))

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	updated, err := a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, updated.Files)

	agentsData, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(agentsData), "# Custom Header")
	assert.Contains(t, string(agentsData), markerBegin)

	configDoc := readConfigJSON(t, filepath.Join(dir, "opencode.json"))
	assert.Equal(t, "manual", configDoc["share"])
	assert.Contains(t, jsonStringSlice(configDoc["instructions"]), ".opencode/rules/autopus/language-policy.md")
}

func TestAdapter_Validate_AfterGenerate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs)
}

func TestAdapter_Clean_RemovesGeneratedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Custom Header\n"), 0644))
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	err = a.Clean(context.Background())
	require.NoError(t, err)

	assert.NoDirExists(t, filepath.Join(dir, ".opencode"))
	assert.NoFileExists(t, filepath.Join(dir, "opencode.json"))
	agentsData, readErr := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(agentsData), "# Custom Header")
	assert.NotContains(t, string(agentsData), markerBegin)
}

func readConfigJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc
}

// readOpenCodeSkillResources concatenates the retrievable resources emitted
// beside a generated skill. A slim entrypoint is only correct if its detail is
// actually installed next to it, so an empty directory is a failure.
func readOpenCodeSkillResources(t *testing.T, root, skill string) string {
	t.Helper()
	dir := filepath.Join(root, ".agents", "skills", skill, "references")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "skill %s must emit its retrievable resources", skill)
	require.NotEmpty(t, entries)
	var combined string
	for _, entry := range entries {
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, readErr)
		combined += string(data)
	}
	return combined
}
