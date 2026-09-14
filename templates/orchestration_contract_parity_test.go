package templates_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrationSemanticContract_CodexAndClaudeBindCanonicalPolicy(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	canonical := readContractSurface(t, filepath.Join(root, "shared", "orchestration-contract.md.tmpl"))
	codexGo := readContractSurface(t, filepath.Join(root, "codex", "skills", "auto-go.md.tmpl"))
	codexReview := readContractSurface(t, filepath.Join(root, "codex", "skills", "auto-review.md.tmpl"))
	claude := readContractSurface(t, filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"))

	requiredSemanticTokens := []string{
		"orchestration-contract.v1",
		"orchestration_run_receipt.v1",
		"requested_providers",
		"configured_providers",
		"attempted_providers",
		"usable_providers",
		"failed_providers",
		"degraded_reasons",
		"critical_veto",
		"analysis_verdict",
		"gate_status",
	}
	for _, token := range requiredSemanticTokens {
		assert.Contains(t, canonical, token, "canonical contract missing %s", token)
		assert.Contains(t, codexGo+codexReview, token, "Codex surface missing %s", token)
		assert.Contains(t, claude, token, "Claude surface missing %s", token)
	}

	for name, surface := range map[string]string{
		"codex":  codexGo + codexReview,
		"claude": claude,
	} {
		assert.Contains(t, surface, "auto orchestra review", "%s must use the risk-tier review entrypoint", name)
		assert.Contains(t, surface, "--risk-tier", "%s must forward risk tier", name)
		assert.NotContains(t, surface, "auto orchestra run \"{review topic}\"", "%s must not use generic debate for code review", name)
	}
}

func TestOrchestrationSemanticContract_SpecPromotionAndIdeaForwardingParity(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	canonicalContract := readContractSurface(t, filepath.Join(root, "shared", "orchestration-contract.md.tmpl"))
	claude := readContractSurface(t, filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"))
	codexAutoIdea := readContractSurface(t, filepath.Join(root, "codex", "skills", "auto-idea.md.tmpl"))
	codexIdea := readContractSurface(t, filepath.Join(root, "codex", "skills", "idea.md.tmpl"))
	geminiAutoIdea := readContractSurface(t, filepath.Join(root, "gemini", "skills", "auto-idea", "SKILL.md.tmpl"))
	geminiIdea := readContractSurface(t, filepath.Join(root, "gemini", "skills", "idea", "SKILL.md.tmpl"))
	canonicalIdea := readRepoSurface(t, filepath.Join("content", "skills", "idea.md"))
	canonicalReview := readRepoSurface(t, filepath.Join("content", "skills", "spec-review.md"))
	canonicalIdeaContract := contractSection(t, canonicalContract, "## Idea", "\n## Team")
	claudeIdea := contractSection(t, claude, "## idea — Brainstorm and Evaluate Ideas", "\n---\n\n## plan")

	for _, surface := range []string{claude, canonicalReview} {
		for _, token := range []string{"status_changed", "degraded_reasons", "override_applied", "--allow-degraded"} {
			assert.Contains(t, surface, token, "SPEC review promotion contract missing %s", token)
		}
	}
	ideaSurfaces := map[string]string{
		"canonical contract": canonicalIdeaContract,
		"canonical idea":     canonicalIdea,
		"Codex auto idea":    codexAutoIdea,
		"Codex idea":         codexIdea,
		"Gemini auto idea":   geminiAutoIdea,
		"Gemini idea":        geminiIdea,
		"Claude idea":        claudeIdea,
	}
	for name, surface := range ideaSurfaces {
		assert.Contains(t, surface, "orchestration-contract.v1", "idea surface must bind the canonical contract")
		assert.Contains(t, surface, "fresh_judge_session", "%s must require an isolated judge session", name)
		for _, forbidden := range []string{
			"different_model_family",
			"different model family",
			"different model-family",
			"cross-family",
			"다른 모델 계열",
			"같은 model family",
		} {
			assert.NotContains(t, strings.ToLower(surface), forbidden, "%s retains active different-family judge wording", name)
		}
	}
	for name, surface := range map[string]string{
		"canonical idea":   canonicalIdea,
		"Codex auto idea":  codexAutoIdea,
		"Codex idea":       codexIdea,
		"Gemini auto idea": geminiAutoIdea,
		"Gemini idea":      geminiIdea,
		"Claude idea":      claudeIdea,
	} {
		assert.Contains(t, surface, "--strategy", "idea strategy must be forwarded")
		assert.Contains(t, surface, "--providers", "idea providers must be forwarded")
		assert.Contains(t, surface, "orchestra_unavailable", "idea fallback must fail closed without a native worker/judge surface")
		assert.Contains(t, surface, "--judge {invoking_provider}",
			"%s must select the provider that invoked orchestra as the fresh-session judge", name)
		assert.NotContains(t, surface, "--judge {judge}",
			"%s must not use an unspecified judge placeholder", name)
	}
	assert.NotContains(t, geminiAutoIdea, `auto orchestra run "{structured idea}"`,
		"Gemini auto-idea must use the canonical brainstorm entrypoint")
	assert.NotContains(t, geminiAutoIdea, "--rounds standard",
		"Gemini auto-idea must use the numeric two-round debate contract")
	assert.NotContains(t, codexAutoIdea+claudeIdea, "Sequential Thinking으로 fallback")
}

func TestOrchestrationSemanticContract_CodexSourceContainsNoClaudeTeamPrimitives(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	codexTeam := readRepoSurface(t, filepath.Join("pkg", "adapter", "codex", "codex_extended_skill_rewrites_agents.go"))
	codexPipeline := readRepoSurface(t, filepath.Join("pkg", "adapter", "codex", "codex_extended_skill_rewrites.go"))
	surface := codexTeam + codexPipeline

	for _, forbidden := range []string{"TeamCreate(", "TeamDelete(", "SendMessage(", "bypassPermissions", "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"} {
		assert.NotContains(t, surface, forbidden)
	}
	assert.Contains(t, surface, "spawn_agent")
	assert.NotEmpty(t, root)
}

func readContractSurface(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read contract surface %s", path)
	return string(data)
}

func readRepoSurface(t *testing.T, relative string) string {
	t.Helper()
	path := filepath.Join("..", relative)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read repository surface %s", path)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func contractSection(t *testing.T, surface, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(surface, startMarker)
	require.NotEqual(t, -1, start, "contract section missing start marker %q", startMarker)
	section := surface[start:]
	end := strings.Index(section, endMarker)
	require.NotEqual(t, -1, end, "contract section missing end marker %q", endMarker)
	return section[:end]
}
