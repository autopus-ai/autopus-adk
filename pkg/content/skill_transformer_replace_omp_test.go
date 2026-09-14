package content_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/content"
)

// ompFlatSkillPathRe matches the stage-1-only skill path form. REQ-013 emits
// .agents/skills/<name>/SKILL.md, so this form names a file that never exists.
var ompFlatSkillPathRe = regexp.MustCompile(`\.agents/skills/[a-z0-9-]+\.md`)

// ompDoubledRuleNamespaceRe matches the doubled namespace produced when a source
// already carries the autopus namespace and the prefix map prepends the
// `autopus-` filename prefix again. omp reads .omp/rules non-recursively, so the
// only valid form is .omp/rules/autopus-<name>.md.
var ompDoubledRuleNamespaceRe = regexp.MustCompile(`\.omp/rules/autopus-autopus[-/]`)

var ompLegacyCoordinationTokens = []string{
	"Agent(", "subagent_type", "prompt =", "prompt=", "task tool", "task(...)", "spawn_agent", "multi_agent",
	"send_input", "wait_agent", "close_agent",
	"TodoWrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
	"TeamCreate", "TeamDelete", "SendMessage", "ToolSearch",
	"AskUserQuestion", "request_user_input", "auto pipeline worktree",
}

var ompForeignSurfaceTokens = []string{
	".claude/", ".codex/", ".opencode/", ".gemini/",
}

var ompRootGlobInventoryTestRe = regexp.MustCompile(
	`\.(codex|claude|gemini|opencode)/\*\*([^/A-Za-z0-9_-]|$)`,
)

func stripOMPTestRootGlobInventory(body string) string {
	return ompRootGlobInventoryTestRe.ReplaceAllString(body, "${2}")
}

func ompContentBodies(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries, err := fs.ReadDir(contentfs.FS, dir)
	require.NoError(t, err)

	bodies := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, readErr := fs.ReadFile(contentfs.FS, dir+"/"+entry.Name())
		require.NoError(t, readErr)
		bodies[dir+"/"+entry.Name()] = string(raw)
	}
	require.NotEmpty(t, bodies)
	return bodies
}

// TestReplacePlatformReferencesOMP_S12_NotANoOp pins the REQ-015 regression this
// SPEC exists for: before omp keys were added, ReplacePlatformReferences
// returned the body untouched.
func TestReplacePlatformReferencesOMP_S12_NotANoOp(t *testing.T) {
	t.Parallel()

	raw, err := fs.ReadFile(contentfs.FS, "rules/doc-storage.md")
	require.NoError(t, err)
	source := string(raw)
	require.Contains(t, source, ".claude/", "fixture precondition: the source names .claude/")

	out := content.ReplacePlatformReferences(source, "omp")

	assert.NotEqual(t, source, out, "omp normalization must not be a no-op")
	assert.Contains(t, out, ".omp/", "the catch-all .claude/ prefix maps to the omp native root")
	assert.NotContains(t, out, ".claude/", "no Claude-native path may survive")
}

// TestReplacePlatformReferencesOMP_S12_PathPrefixes pins each REQ-015 stage-1
// mapping, including the already-namespaced rule path that must not double.
func TestReplacePlatformReferencesOMP_S12_PathPrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"rules bare", "See `.claude/rules/branding.md`.", "`.omp/rules/autopus-branding.md`"},
		{"rules already namespaced", "See `.claude/rules/autopus/branding.md`.", "`.omp/rules/autopus-branding.md`"},
		{"commands", "See `.claude/commands/auto.md`.", "`.omp/commands/auto.md`"},
		{"agents", "See `.claude/agents/autopus/executor.md`.", "`.omp/agents/autopus/executor.md`"},
		{"generic claude dir", "Config lives in `.claude/settings.json`.", "`.omp/settings.json`"},
		{"codex skill root", "See `.codex/skills/autopus/verification.md`.", "`.omp/skills/verification/SKILL.md`"},
		{"codex native skill", "See `.codex/skills/codex-agent-pipeline/SKILL.md`.", "`.omp/skills/agent-pipeline/SKILL.md`"},
		{"opencode command root", "See `.opencode/commands/auto.md`.", "`.omp/commands/auto.md`"},
		{"gemini agent root", "See `.gemini/agents/autopus/reviewer.md`.", "`.omp/agents/autopus/reviewer.md`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := content.ReplacePlatformReferences(tt.input, "omp")
			assert.Contains(t, got, tt.want)
			assert.NotContains(t, got, ".claude/")
			assert.Empty(t, ompDoubledRuleNamespaceRe.FindAllString(got, -1),
				"the rule namespace must not double")
		})
	}
}

func TestReplacePlatformReferencesOMP_PreservesRootGlobInventoryOnly(t *testing.T) {
	t.Parallel()

	input := "Exclude `.codex/**`, `.claude/**`, `.gemini/**`, `.opencode/**`; " +
		"load `.codex/skills/autopus/verification.md`, `.claude/commands/auto.md`, " +
		"`.gemini/agents/autopus/reviewer.md`, `.opencode/rules/branding.md`, " +
		"and operational glob `.codex/**/runtime.json`."
	got := content.ReplacePlatformReferences(input, "omp")

	for _, glob := range []string{".codex/**", ".claude/**", ".gemini/**", ".opencode/**"} {
		assert.Equal(t, 1, strings.Count(got, glob), "intentional root inventory glob %s must survive exactly", glob)
	}
	for _, operational := range []string{
		".omp/skills/verification/SKILL.md",
		".omp/commands/auto.md",
		".omp/agents/autopus/reviewer.md",
		".omp/rules/autopus-branding.md",
		".omp/**/runtime.json",
	} {
		assert.Contains(t, got, operational)
	}
	assert.NotContains(t, got, ".codex/skills/")
	assert.NotContains(t, got, ".codex/**/runtime.json")
	assert.NotContains(t, got, ".claude/commands/")
	assert.NotContains(t, got, ".gemini/agents/")
	assert.NotContains(t, got, ".opencode/rules/")
}

// TestReplacePlatformReferencesOMP_S12_SkillRefFinalForm pins the REQ-015
// stage-2 rewrite that stage-1 prefix mapping alone cannot produce.
func TestReplacePlatformReferencesOMP_S12_SkillRefFinalForm(t *testing.T) {
	t.Parallel()

	got := content.ReplacePlatformReferences("Reference: `.claude/skills/autopus/ax-annotation.md`", "omp")

	assert.Contains(t, got, "`.omp/skills/ax-annotation/SKILL.md`")
	assert.Empty(t, ompFlatSkillPathRe.FindAllString(got, -1),
		"the stage-1-only form .agents/skills/<name>.md must not survive")
}

// TestNormalizeAgentReferencesOMP_S12_BrandingRule pins REQ-015 for the branding
// reference, which fell back to the repository-internal source path while omp
// had no brandingRule entry.
func TestNormalizeAgentReferencesOMP_S12_BrandingRule(t *testing.T) {
	t.Parallel()

	input := "- **브랜딩**: `content/rules/branding.md` 준수"
	got := content.NormalizeAgentReferences(input, "omp")

	assert.Contains(t, got, "`.omp/rules/autopus-branding.md`")
	assert.NotContains(t, got, "`content/rules/branding.md`",
		"omp must not fall back to the repository-internal source path")
}

// TestReplacePlatformReferencesOMP_S12_ToolNames pins TodoWrite -> todo for omp.
func TestReplacePlatformReferencesOMP_S12_ToolNames(t *testing.T) {
	t.Parallel()

	got := content.ReplacePlatformReferences("Track progress with the TodoWrite tool.", "omp")

	assert.Contains(t, got, "Track progress with the todo operation tool.")
	assert.NotContains(t, got, "TodoWrite")
	assert.NotContains(t, got, "todowrite", "todowrite is the opencode name, not the omp name")
}

// TestReplacePlatformReferencesOMP_S12_ToolNameIsolation guards the other
// platforms against the omp tool-name change. A codex line that already contains
// the word todo must still lose its TodoWrite reference.
func TestReplacePlatformReferencesOMP_S12_ToolNameIsolation(t *testing.T) {
	t.Parallel()

	input := "Use TodoWrite to track todo items."

	codex := content.ReplacePlatformReferences(input, "codex")
	assert.Equal(t, "// TodoWrite is not available on this platform", codex,
		"codex has no TodoWrite equivalent, so the instruction must still be neutralized")

	opencode := content.ReplacePlatformReferences(input, "opencode")
	assert.Contains(t, opencode, "todowrite")
	assert.NotContains(t, opencode, "TodoWrite")
}

// TestReplacePlatformReferencesOMP_S12_WorkflowToolNames pins the clean
// cutover from legacy coordination names to the native OMP field contracts.
func TestReplacePlatformReferencesOMP_S12_WorkflowToolNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"AskUserQuestion", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
		"TeamCreate", "TeamDelete", "SendMessage", "ToolSearch",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := content.ReplacePlatformReferences("Use the "+name+" tool here.", "omp")
			assert.NotContains(t, got, name)
			assert.NotContains(t, got, "todowrite",
				"todowrite is the opencode name, not the omp name")
		})
	}
}

func TestReplacePlatformReferencesOMP_S12_TeamTodoHubCalls(t *testing.T) {
	t.Parallel()

	input := `TeamCreate(name="delivery")
TaskCreate(subject="Implement the slice")
SendMessage(recipient="executor", content="Apply the review feedback")`
	got := content.ReplacePlatformReferences(input, "omp")

	assert.Contains(t, got, `"i"`)
	assert.Contains(t, got, `"context"`)
	assert.Contains(t, got, `"tasks"`)
	assert.Contains(t, got, `todo with {"i":"Updating parent-owned progress","op":"append"`)
	assert.NotContains(t, got, `"agent":`, "default general workers must omit the optional agent field")
	assert.Contains(t, got, `hub with {"i":"Following up with an existing worker","op":"send"`)
	for _, token := range ompLegacyCoordinationTokens {
		assert.NotContains(t, got, token)
	}
}

// An emitted batch may only name an agent OMP registers. The retired per-role
// names collapse: implementation runs on the bundled default, so it emits no
// agent field at all, while review keeps the bundled reviewer. The declared
// role survives as the item name so the dispatch stays auditable.
func TestReplacePlatformReferencesOMP_S12_MultilineTaskBatch(t *testing.T) {
	t.Parallel()

	input := `Agent(
  subagent_type = "executor",
  prompt = """Implement the assigned slice.""",
  isolation = "worktree"
)
Agent(
  task = "Review the implementation.",
  subagent_type = "reviewer"
)`
	got := content.ReplacePlatformReferences(input, "omp")

	for _, field := range []string{
		`"i"`, `"context"`, `"tasks"`, `"name"`, `"agent"`, `"task"`,
		`"outputSchema"`, `"schemaMode"`,
		`"owned_paths"`, `"changed_files"`, `"verification"`,
		`"blockers"`, `"next_required_step"`,
	} {
		assert.Contains(t, got, field)
	}
	assert.NotContains(t, got, `"isolated": true`)
	assert.NotContains(t, got, `"agent": "executor"`,
		"OMP registers no executor agent; implementation runs on the bundled default")
	assert.Contains(t, got, `"agent": "reviewer"`)
	assert.Contains(t, got, `"name": "executor-1"`)
	assert.Contains(t, got, `"name": "reviewer-2"`)
	for _, token := range ompLegacyCoordinationTokens {
		assert.NotContains(t, got, token)
	}
}
