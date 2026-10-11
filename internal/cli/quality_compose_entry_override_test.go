package cli

// SPEC-MODELUX-001 T13 (REQ-MUX-20, REQ-MUX-21, REQ-MUX-22; acceptance S15,
// S16, S17) through the real `auto quality` command.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// writeComposeProjectP writes acceptance fixture P: D whose Codex follows
// quality.providers.codex = codex-custom, every agent haiku.
func writeComposeProjectP(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("p")
	cfg.Platforms = []string{"claude-code", "codex"}
	cfg.Quality.SupervisorModelPolicy = config.SupervisorModelPolicyInherit
	haiku := make(map[string]string)
	for _, agent := range config.CanonicalAgentNames() {
		haiku[agent] = "haiku"
	}
	cfg.Quality.Presets["codex-custom"] = config.QualityPreset{Description: "codex", Agents: haiku}
	cfg.Quality.Providers = map[string]string{config.QualityProviderCodex: "codex-custom"}
	require.NoError(t, config.Save(dir, cfg))
	return dir
}

// plainComposeHeaders keys every row by platform name alone, without the
// override label a column header may carry.
func plainComposeHeaders(table map[string]map[string]string) map[string]map[string]string {
	plain := make(map[string]map[string]string, len(table))
	for agent, row := range table {
		plain[agent] = make(map[string]string, len(row))
		for header, cell := range row {
			name, _, _ := strings.Cut(header, " (")
			plain[agent][name] = cell
		}
	}
	return plain
}

// S15: after the S6 edit and before its preview, the operator decides whether
// Codex follows the new composition. The answer reaches the after-table and
// the change lines, `y` removes only quality.providers.codex, and the same
// run's generation equals the approved preview either way.
func TestQualityCompose_AsksWhetherAnOverrideFollowsBeforeThePreview(t *testing.T) {
	cases := []struct {
		name, lang, answer, question, preview, removal string
		follows                                        bool
	}{
		{
			name: "n keeps the override", lang: "ko", answer: "n",
			question: "Codex도 새 구성을 따를까요? (quality.providers.codex = codex-custom 제거) [y/N]\n",
			preview:  "미리보기 (아직 바뀌지 않음)", removal: "  quality.providers.codex 제거\n",
		},
		{
			name: "y follows the composition", lang: "ko", answer: "y", follows: true,
			question: "Codex도 새 구성을 따를까요? (quality.providers.codex = codex-custom 제거) [y/N]\n",
			preview:  "미리보기 (아직 바뀌지 않음)", removal: "  quality.providers.codex 제거\n",
		},
		{
			name: "English", lang: "en", answer: "yes", follows: true,
			question: "Should Codex follow the new composition too? (removes quality.providers.codex = codex-custom) [y/N]\n",
			preview:  "Preview (nothing changed yet)", removal: "  quality.providers.codex removed\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := newComposeUpdateRun(t, true, "")
			setComposeLanguage(t, tc.lang)
			dir := writeComposeProjectP(t)

			out, err := runQualityCompose(t, dir, strings.NewReader("g\n\n3\n\n4\n"+tc.answer+"\ny\n"), "--apply")

			require.NoError(t, err, out)
			asked := strings.Index(out, tc.question)
			require.GreaterOrEqual(t, asked, 0, out)
			assert.Equal(t, 1, strings.Count(out, tc.question))
			assert.Less(t, asked, strings.Index(out, tc.preview+"\n"), "the question comes before the preview")
			table := composeRenderedTable(t, out, tc.preview)
			written := rawQualityOf(t, readBlockFixture(t, dir))
			assert.Equal(t, "custom", written.Default)
			if tc.follows {
				assert.Equal(t, "6-luna · max*", table["executor"]["Codex"])
				assert.Contains(t, out, tc.removal)
				assert.Empty(t, written.Providers)
			} else {
				assert.Equal(t, "6-luna · medium", table["executor"]["Codex (개별 설정: codex-custom)"])
				assert.NotContains(t, out, tc.removal)
				assert.Equal(t, map[string]string{config.QualityProviderCodex: "codex-custom"}, written.Providers)
			}
			kept := written.Presets["codex-custom"]
			assert.Equal(t, "codex", kept.Description, "the override's preset keeps its values")
			assert.Len(t, kept.Agents, len(config.CanonicalAgentNames()))
			for agent, tier := range kept.Agents {
				assert.Equal(t, "haiku", tier, agent)
			}
			assertGeneratedEqualsTable(t, dir, plainComposeHeaders(table))
			assert.Len(t, *run.probes, 1)
			assert.False(t, run.adapterProbed(t), "generation must read the run's snapshot")
		})
	}
}

// The change lines resolve the Codex default session without a removed
// override: a Codex pinned to ultra keeps its session profile while it keeps
// the override, and moves to the new binding's profile once it follows.
func TestQualityCompose_OverrideAnswerReachesTheDefaultSessionLine(t *testing.T) {
	const moved = "  Codex 기본 세션: 6-astra · ultra → 6-astra · xhigh\n"
	for _, answer := range []string{"n", "y"} {
		t.Run(answer, func(t *testing.T) {
			newComposeUpdateRun(t, true, "")
			dir := t.TempDir()
			cfg := config.DefaultFullConfig("p")
			cfg.Platforms = []string{"claude-code", "codex"}
			cfg.Quality.SupervisorModelPolicy = config.SupervisorModelPolicyQuality
			cfg.Quality.Providers = map[string]string{config.QualityProviderCodex: "ultra"}
			require.NoError(t, config.Save(dir, cfg))
			before := composeSessionHash(t, dir)

			out, err := runQualityCompose(t, dir, strings.NewReader("a\nplanner=sonnet\n\n"+answer+"\nn\n"))

			require.NoError(t, err, out)
			assert.Contains(t, out, "Codex도 새 구성을 따를까요? (quality.providers.codex = ultra 제거) [y/N]\n")
			if answer == "y" {
				assert.Contains(t, out, moved)
			} else {
				assert.NotContains(t, out, "  Codex 기본 세션: 6-astra · ultra →", "the kept override keeps the session")
			}
			assert.Contains(t, out, "취소했습니다. 설정을 바꾸지 않았습니다.\n")
			assert.Equal(t, before, composeSessionHash(t, dir))
		})
	}
}

// No question without an edit that writes: an unchanged group pass ends with
// 변경 없음 and leaves the override alone.
func TestQualityCompose_AsksNothingWhenTheEditWritesNothing(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	dir := writeComposeProjectP(t)
	before := composeSessionHash(t, dir)

	out, err := runQualityCompose(t, dir, strings.NewReader("g\n\n\n\n\n"))

	require.NoError(t, err, out)
	assert.Contains(t, out, "변경 없음\n")
	assert.NotContains(t, out, "Codex도 새 구성을 따를까요?")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// S16: `auto quality show --agents` keeps every existing show line first and
// then prints the R table, on a stdin it never reads, without writing.
func TestQualityShowAgents_PrintsTheTableAfterTheShowLinesReadOnly(t *testing.T) {
	run := newComposeUpdateRun(t, false, "")
	dir := writeComposeRFixture(t, "R")
	before := composeSessionHash(t, dir)

	plain, err := runQualityCompose(t, dir, driverFailingReader{t}, "show")
	require.NoError(t, err, plain)
	out, err := runQualityCompose(t, dir, driverFailingReader{t}, "show", "--agents")
	require.NoError(t, err, out)

	assert.Equal(t, plain+composeSessionGolden(t, "table-R-CAT-A.golden"), out)
	assert.Len(t, *run.probes, 1)
	assert.Equal(t, before, composeSessionHash(t, dir))
}
