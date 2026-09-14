package content

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
)

// flatSkillRefRe matches the stage-1-only skill path form that points at a file
// omp never emits.
var flatSkillRefRe = regexp.MustCompile(`\.agents/skills/[a-z0-9-]+\.md`)

// doubledRuleNamespaceRe matches the doubled namespace produced when a source
// already carries the autopus namespace and the prefix map prepends the
// `autopus-` filename prefix a second time. omp emits rules at
// `.omp/rules/autopus-<name>.md` only, flat and non-recursive.
var doubledRuleNamespaceRe = regexp.MustCompile(`\.omp/rules/autopus-autopus[-/]`)

var ompForbiddenCoordinationTokens = []string{
	"Agent(", "subagent_type", "prompt =", "prompt=", "task tool", "task(...)", "spawn_agent", "multi_agent",
	"send_input", "wait_agent", "close_agent", "TodoWrite",
	"TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
	"TeamCreate", "TeamDelete", "SendMessage", "ToolSearch",
	"AskUserQuestion", "request_user_input", "auto pipeline worktree",
}

func stripOMPTestInventoryGlobs(body string) string {
	return ompRootGlobInventoryRe.ReplaceAllString(body, "${2}")
}

// TestOMPNormalization_S12_PathPrefixes covers REQ-015 stage-1 prefix mapping.
func TestOMPNormalization_S12_PathPrefixes(t *testing.T) {
	raw, err := fs.ReadFile(contentfs.FS, "rules/doc-storage.md")
	require.NoError(t, err)
	source := string(raw)
	require.Contains(t, source, ".claude/", "fixture precondition: source names .claude/")

	out := ReplacePlatformReferences(source, "omp")

	assert.NotEqual(t, source, out,
		"ReplacePlatformReferences(body, \"omp\") must not be a no-op")
	assert.Contains(t, out, ".omp/",
		"the generic `.claude/` prefix must map to the omp native root")
	assert.NotContains(t, out, ".claude/rules/")
	assert.NotContains(t, out, ".claude/", "no Claude-native path may survive")
}

// TestOMPNormalization_S12_PrefixTable covers each REQ-015 stage-1 mapping.
func TestOMPNormalization_S12_PrefixTable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"skills autopus namespace", "See `.claude/skills/autopus/prd.md`.", "`.omp/skills/prd/SKILL.md`"},
		{"commands", "See `.claude/commands/auto.md`.", "`.omp/commands/auto.md`"},
		{"agents", "See `.claude/agents/autopus/executor.md`.", "`.omp/agents/autopus/executor.md`"},
		{"rules already namespaced", "See `.claude/rules/autopus/branding.md`.", "`.omp/rules/autopus-branding.md`"},
		{"rules bare", "See `.claude/rules/branding.md`.", "`.omp/rules/autopus-branding.md`"},
		{"generic claude dir", "Config lives in `.claude/settings.json`.", "`.omp/settings.json`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReplacePlatformReferences(tt.input, "omp")
			assert.Contains(t, got, tt.want)
			assert.NotContains(t, got, ".claude/")
		})
	}
}

// TestOMPNormalization_S12_SkillRefStageTwo covers the REQ-015 stage-2 rewrite.
func TestOMPNormalization_S12_SkillRefStageTwo(t *testing.T) {
	got := ReplacePlatformReferences("Reference: `.claude/skills/autopus/ax-annotation.md`", "omp")
	assert.Contains(t, got, ".omp/skills/ax-annotation/SKILL.md")
	assert.Empty(t, flatSkillRefRe.FindAllString(got, -1),
		"stage-1-only form .agents/skills/<name>.md must not survive")
}

// TestOMPNormalization_S12_BrandingRuleReference covers REQ-015 branding fallback.
func TestOMPNormalization_S12_BrandingRuleReference(t *testing.T) {
	got := NormalizeAgentReferences("Follow `content/rules/branding.md` for output format.", "omp")
	assert.Contains(t, got, "`.omp/rules/autopus-branding.md`")
	assert.NotContains(t, got, "`content/rules/branding.md`",
		"omp must not fall back to the repository-internal source path")
}

// TestOMPNormalization_S12_ToolNames covers REQ-015 tool-name rewriting.
func TestOMPNormalization_S12_ToolNames(t *testing.T) {
	got := ReplacePlatformReferences("Track progress with the TodoWrite tool.", "omp")
	assert.Contains(t, got, "todo")
	assert.NotContains(t, got, "TodoWrite",
		"TodoWrite must be rewritten to the omp canonical tool name")
}

// TestOMPNormalization_S12_WorkflowToolNames covers REQ-015 for the Claude-only
// workflow tool surface that omp does not expose.
func TestOMPNormalization_S12_WorkflowToolNames(t *testing.T) {
	claudeOnly := []string{"AskUserQuestion", "TaskCreate", "TaskUpdate", "TeamCreate", "SendMessage"}
	for _, name := range claudeOnly {
		t.Run(name, func(t *testing.T) {
			got := ReplacePlatformReferences("Use the "+name+" tool here.", "omp")
			assert.NotContains(t, got, name,
				"%s has no omp equivalent and must be rewritten, got %q", name, got)
		})
	}
}

// TestOMPNormalization_S12_AgentInvocationSyntax covers the native batch
// contract and rejects the legacy single-agent pseudo-call fields. `reviewer`
// keeps an explicit agent field; the default agent is selected by omitting it,
// which pkg/content's collapse test covers per role.
func TestOMPNormalization_S12_AgentInvocationSyntax(t *testing.T) {
	got := ReplacePlatformReferences(`Agent(subagent_type="reviewer", task="build")`, "omp")
	for _, field := range []string{
		`"i"`, `"context"`, `"tasks"`, `"agent"`, `"task"`, `"outputSchema"`,
		`"schemaMode"`, `"owned_paths"`, `"changed_files"`,
		`"verification"`, `"blockers"`, `"next_required_step"`,
	} {
		assert.Contains(t, got, field)
	}
	for _, token := range ompForbiddenCoordinationTokens {
		assert.NotContains(t, got, token)
	}
}

// TestOMPNormalization_S12_EmittedSkillBodies covers REQ-015 across the catalog.
func TestOMPNormalization_S12_EmittedSkillBodies(t *testing.T) {
	transformer, err := NewSkillTransformerFromFS(contentfs.FS, "skills")
	require.NoError(t, err)
	skills, _, err := transformer.TransformForPlatform("omp")
	require.NoError(t, err)
	require.NotEmpty(t, skills, "omp must compile at least one catalog skill")

	for _, skill := range skills {
		for _, token := range ompForbiddenCoordinationTokens {
			assert.NotContains(t, skill.Content, token,
				"skill %s retained legacy coordination token %q", skill.Name, token)
		}
		sweepContent := stripOMPTestInventoryGlobs(skill.Content)
		for _, token := range []string{".claude/", ".codex/", ".opencode/", ".gemini/"} {
			assert.NotContains(t, sweepContent, token,
				"skill %s retained operational foreign platform path %q", skill.Name, token)
		}
		assert.Empty(t, flatSkillRefRe.FindAllString(skill.Content, -1),
			"skill %s must not reference .agents/skills/<name>.md", skill.Name)
		assert.Empty(t, doubledRuleNamespaceRe.FindAllString(skill.Content, -1),
			"skill %s must reference .omp/rules/autopus-<name>.md, not a doubled namespace", skill.Name)
	}
}

// TestOMPNormalization_S12_EmittedRuleBodies covers REQ-015 across the rule set.
func TestOMPNormalization_S12_EmittedRuleBodies(t *testing.T) {
	entries, err := fs.ReadDir(contentfs.FS, "rules")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			raw, readErr := fs.ReadFile(contentfs.FS, "rules/"+entry.Name())
			require.NoError(t, readErr)
			out := ReplacePlatformReferences(string(raw), "omp")

			assert.NotContains(t, out, ".claude/",
				"rule %s must not reference a Claude-native path", entry.Name())
			assert.Empty(t, flatSkillRefRe.FindAllString(out, -1),
				"rule %s must not reference .agents/skills/<name>.md", entry.Name())
			assert.Empty(t, doubledRuleNamespaceRe.FindAllString(out, -1),
				"rule %s must not produce a doubled autopus namespace", entry.Name())
		})
	}
}
