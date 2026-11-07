package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

func TestReaFallbackLine_FormTextPerCatalogAndLanguage(t *testing.T) {
	t.Parallel()
	codex := adapter.ReaFallback{Catalog: "Codex", Agent: "reverse-engineer", Requested: "gpt-6.1-sol/xhigh"}
	omp := adapter.ReaFallback{Catalog: "OMP", Agent: "reverse-engineer", Requested: "openai-codex/gpt-6.1-sol"}
	assert.Equal(t, "reverse-engineer: gpt-6.1-sol/xhigh not in Codex catalog → session model", reaFallbackLine(codex, "en"))
	assert.Equal(t, "reverse-engineer: gpt-6.1-sol/xhigh이 Codex catalog에 없어 세션 모델을 씁니다", reaFallbackLine(codex, "ko"))
	assert.Equal(t, "reverse-engineer: openai-codex/gpt-6.1-sol not in catalog → session model", reaFallbackLine(omp, "en"))
	assert.Equal(t, "reverse-engineer: openai-codex/gpt-6.1-sol이 OMP catalog에 없어 세션 모델을 씁니다", reaFallbackLine(omp, "ko"))

	var out bytes.Buffer
	notice := &reaFallbackNotice{out: &out, lang: "en", seen: map[adapter.ReaFallback]bool{}}
	notice.report(codex)
	notice.report(codex)
	notice.report(omp)
	assert.Equal(t, "  "+reaFallbackLine(codex, "en")+"\n  "+reaFallbackLine(omp, "en")+"\n", out.String(),
		"a fact a preview and the apply both report prints once")
}

// Scope expansion of SPEC-REA-001 Gate 1 through a real update: a Codex
// catalog without gpt-6.1-sol (the fake codex answers CAT-B) generates every
// file, leaves qa-explorer on the session model, and prints one line in
// the run's language.
func TestReaFallbackNotice_CodexUpdatePrintsOneLine(t *testing.T) {
	for lang, line := range map[string]string{
		"ko": "  qa-explorer: gpt-6.1-sol/xhigh이 Codex catalog에 없어 세션 모델을 씁니다\n",
		"en": "  qa-explorer: gpt-6.1-sol/xhigh not in Codex catalog → session model\n",
	} {
		t.Run(lang, func(t *testing.T) {
			newComposeUpdateRun(t, false, "")
			useComposeLanguage(t, lang)
			dir := writeComposeFixture(t, "claude-code", "codex")

			out, err := runComposeUpdate(t, strings.NewReader(""), "--dir", dir, "--yes")

			require.NoError(t, err, out)
			assert.Equal(t, 1, strings.Count(out, line), out)
			assert.NotContains(t, out, "Codex 카탈로그가 관리형 native balanced 프로필을 지원하지 않음")
			agent, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", "qa-explorer.toml"))
			require.NoError(t, err)
			assert.NotContains(t, string(agent), "\nmodel = ")
			assert.NotContains(t, string(agent), "model_reasoning_effort")
			reviewer, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", "reviewer.toml"))
			require.NoError(t, err)
			assert.Contains(t, string(reviewer), `model = "gpt-6-astra"`)
		})
	}
}

// The Form Text contract is one line per run: an interactive update whose
// compose step shows the fallback in its table and then generates prints the
// table's line only, never a second one from the generation.
func TestReaFallbackNotice_InteractiveComposeAndGeneratePrintsOnce(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	composeSessionCatalog(t, "CAT-B")
	dir := writeComposeFixture(t, "claude-code", "codex")

	out, err := runComposeUpdate(t, strings.NewReader("\n"), "--dir", dir)

	require.NoError(t, err, out)
	line := "qa-explorer: gpt-6.1-sol/xhigh이 Codex catalog에 없어 세션 모델을 씁니다"
	assert.Equal(t, 1, strings.Count(out, line), out)
	assert.Contains(t, out, "\n"+line+"\n", "the table footer carries the line")
	agent, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", "qa-explorer.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(agent), "\nmodel = ", "the generation took the same fallback")
}
