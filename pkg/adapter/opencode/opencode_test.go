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
	a := NewWithRoot("/some/path")
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
	a := NewWithRoot(t.TempDir())
	_, err := a.Detect(context.Background())
	assert.NoError(t, err)
}

func TestAdapter_Generate_CreatesOpenCodeFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("demo")

	pf, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, pf.Files)

	assert.FileExists(t, filepath.Join(dir, "AGENTS.md"))
	assert.FileExists(t, filepath.Join(dir, "opencode.json"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "commands", "auto.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "commands", "auto-setup.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "commands", "auto-plan.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "agents", "planner.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "plugins", "autopus-hooks.js"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "planning", "SKILL.md"))
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

	autoIdeaSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-idea", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoIdeaSkill), "auto orchestra brainstorm")
	assert.Contains(t, string(autoIdeaSkill), "Clarification Ledger")
	assert.Contains(t, string(autoIdeaSkill), "Current understanding")
	assert.Contains(t, string(autoIdeaSkill), "Blocked decision")
	assert.Contains(t, string(autoIdeaSkill), "Recommended answer")
	assert.Contains(t, string(autoIdeaSkill), "Question")
	assert.Contains(t, string(autoIdeaSkill), "Question Audit")
	assert.Contains(t, string(autoIdeaSkill), "question_transport")
	assert.NotContains(t, string(autoIdeaSkill), "request_user_input")
	assert.Contains(t, string(autoIdeaSkill), "scope_boundary")
	assert.Contains(t, string(autoIdeaSkill), "brownfield_impact")
	assert.Contains(t, string(autoIdeaSkill), "Debate focus")
	assert.Contains(t, string(autoIdeaSkill), "Visual Brief")
	assert.Contains(t, string(autoIdeaSkill), "wireframe")
	assert.Contains(t, string(autoIdeaSkill), "## OpenCode Invocation")
	assert.Contains(t, string(autoIdeaSkill), "## Autopus Branding")
	autoSetupSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-setup", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoSetupSkill), "ARCHITECTURE.md")
	assert.Contains(t, string(autoSetupSkill), "explorer")
	assert.Contains(t, string(autoSetupSkill), "## OpenCode Invocation")
	agentPipelineSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "agent-pipeline", "SKILL.md"))
	require.NoError(t, err)
	// The entrypoint owns the dispatch-authenticity evidence and routes to its
	// detail; documentation fetch and the review retry loop live in resources.
	assert.Contains(t, string(agentPipelineSkill), "subagent_dispatch_count")
	assert.Contains(t, string(agentPipelineSkill), "degraded_mode")
	assert.Contains(t, string(agentPipelineSkill), "do not report main-session work")
	pipelineResources := readOpenCodeSkillResources(t, dir, "agent-pipeline")
	for _, token := range []string{
		"web search",   // documentation fetch keeps its fallback
		"retry budget", // the repair loop stays bounded
		"same invocation",
	} {
		assert.Contains(t, pipelineResources, token,
			"the pipeline resources lost %q", token)
	}
	autoPlanSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-plan", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoPlanSkill), "Clarification Ledger")
	assert.Contains(t, string(autoPlanSkill), "Plan Intent Ledger")
	assert.Contains(t, string(autoPlanSkill), "Question Audit")
	assert.Contains(t, string(autoPlanSkill), "answered")
	assert.Contains(t, string(autoPlanSkill), "assumed")
	assert.Contains(t, string(autoPlanSkill), "deferred")
	assert.Contains(t, string(autoPlanSkill), "Semantic Invariant Inventory")
	assert.Contains(t, string(autoPlanSkill), "oracle acceptance")
	assert.Contains(t, string(autoPlanSkill), "structural-only")
	assert.Contains(t, string(autoPlanSkill), "untrusted prompt input")
	assert.Contains(t, string(autoPlanSkill), "never as instructions")
	assert.Contains(t, string(autoPlanSkill), "redact")
	assert.Contains(t, string(autoPlanSkill), "multi-line raw user text")
	assert.Contains(t, string(autoPlanSkill), "Visual Planning Brief")
	assert.Contains(t, string(autoPlanSkill), "Mermaid `flowchart`")
	autoGoSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-go", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoGoSkill), "SPEC Path Resolution")
	assert.Contains(t, string(autoGoSkill), "{SPEC_PATH}")
	assert.Contains(t, string(autoGoSkill), "WORKING_DIR")
	assert.Contains(t, string(autoGoSkill), "autopus.yaml")
	assert.Contains(t, string(autoGoSkill), "spec.review_gate.enabled")
	assert.Contains(t, string(autoGoSkill), "max_revisions")
	assert.Contains(t, string(autoGoSkill), "approved")
	assert.Contains(t, string(autoGoSkill), "재귀 auto-chain")
	assert.Contains(t, string(autoGoSkill), "--model <provider/model>")
	assert.Contains(t, string(autoGoSkill), "--variant <value>")
	assert.Contains(t, string(autoGoSkill), "Autonomous Review Loop Contract")
	assert.Contains(t, string(autoGoSkill), "review retry budget이 남아 있는 동안에는 사용자에게 수동 수정")
	assert.Contains(t, string(autoGoSkill), "terminal handoff는 `/auto sync {SPEC-ID}` 까지입니다")
	assert.Contains(t, string(autoGoSkill), "subagent_dispatch_count")
	assert.Contains(t, string(autoGoSkill), "workflow authenticity blocker")
	assert.Contains(t, string(autoGoSkill), "degraded-mode")
	assert.Contains(t, string(autoGoSkill), "SPEC Markdown under .autopus/specs/** is exempt")
	assert.Contains(t, string(autoGoSkill), "Sync Readiness Gate")
	assert.Contains(t, string(autoGoSkill), "completion_verdict_preview")
	assert.Contains(t, string(autoGoSkill), "spec_status_after_go")
	autoReviewSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-review", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoReviewSkill), "repair loop 입력입니다")
	assert.Contains(t, string(autoReviewSkill), "standalone `/auto review`")
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
	assert.Contains(t, instructions, ".opencode/rules/autopus/branding.md")

	context7Rule, err := os.ReadFile(filepath.Join(dir, ".opencode", "rules", "autopus", "context7-docs.md"))
	require.NoError(t, err)
	assert.Contains(t, string(context7Rule), "web search")
	assert.Contains(t, string(context7Rule), "official docs")

	techstackRule, err := os.ReadFile(filepath.Join(dir, ".opencode", "rules", "autopus", "techstack-freshness.md"))
	require.NoError(t, err)
	assert.Contains(t, string(techstackRule), "Technology Stack Decision")
	assert.Contains(t, string(techstackRule), "greenfield")
}

func TestAdapter_Generate_AutoRouterUsesThinOpenCodeContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)

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
	assert.Contains(t, string(autoSkill), "/auto-canary")
	assert.NotContains(t, string(autoSkill), "Codex용 canonical router surface")
}

func TestAdapter_Generate_NilConfig(t *testing.T) {
	t.Parallel()
	a := NewWithRoot(t.TempDir())
	_, err := a.Generate(context.Background(), nil)
	assert.Error(t, err)
}

func TestAdapter_Update_PreservesMergedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
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
	assert.Contains(t, jsonStringSlice(configDoc["instructions"]), ".opencode/rules/autopus/branding.md")
}

func TestAdapter_Validate_AfterGenerate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs)
}

func TestAdapter_Clean_RemovesGeneratedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
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
