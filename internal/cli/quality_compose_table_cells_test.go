package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// S19: a read catalog without GPT-6.1 Sol substitutes every ultra cell, and
// the footer counts the substitutes.
func TestComposeTable_CatalogB_MarksSubstitutes(t *testing.T) {
	table := composeTableFor(t, "R", "CAT-B")
	layout := table.layout("ko", nil)

	assert.Equal(t, "6-sol · xhigh (대체)", composeTableRowTexts(t, layout, "planner")[4])
	assert.Equal(t, []string{"Codex catalog: 읽음 · 대체 16칸", "Codex 기본 세션: 6-astra · ultra"}, layout.footer)
	assert.False(t, table.codexRefused())
}

// S19: an unreadable catalog keeps standard placements as unverified and
// sends everything else to the fallback model.
func TestComposeTable_CatalogC_UnreadableCatalogNotation(t *testing.T) {
	r := composeTableFor(t, "R", "CAT-C").layout("ko", nil)
	assert.Equal(t, "5.6-sol · xhigh (catalog 없음)", composeTableRowTexts(t, r, "planner")[4])
	assert.Equal(t, "Codex catalog: 읽지 못함", r.footer[0])
	assert.Equal(t, "Codex 기본 세션: 5.6-sol · ultra (catalog 없음)", r.footer[1])

	d := composeTableFor(t, "D", "CAT-C").layout("ko", nil)
	assert.Equal(t, "6-astra · max (미확인)", composeTableRowTexts(t, d, "planner")[4])
	assert.Equal(t, "6-luna · max (미확인)", composeTableRowTexts(t, d, "executor")[4])
	assert.Equal(t, []string{"Codex catalog: 읽지 못함"}, d.footer)
}

// S19: a read catalog without GPT-6 Astra refuses the seven top-rung standard
// placements, and the table says the Codex generation fails.
func TestComposeTable_CatalogD_RefusesTopRungPlacements(t *testing.T) {
	table := composeTableFor(t, "D", "CAT-D")
	layout := table.layout("ko", nil)

	var refused []string
	for _, row := range layout.rows {
		if row[4] == "생성 거부" {
			refused = append(refused, row[1])
		}
	}
	assert.ElementsMatch(t, []string{
		"architect", "debugger", "deep-worker", "planner", "reviewer", "security-auditor", "spec-writer",
	}, refused)
	assert.Equal(t, "6-luna · max", composeTableRowTexts(t, layout, "executor")[4])
	assert.True(t, table.codexRefused())
	assert.Equal(t, []string{"Codex catalog: 읽음 · 대체 0칸", "Codex 생성이 실패합니다"}, layout.footer)
}

// S6: the after-table of D's role-group custom marks every Claude cell and
// exactly four Codex cells, comparing raw values of the same snapshot.
func TestComposeTable_ChangedCellsCarryStar(t *testing.T) {
	before := composeTableFor(t, "D", "CAT-A")
	after := composeTableFor(t, "K", "CAT-A")
	layout := after.layout("ko", &before)

	var codexMarked []string
	for _, row := range layout.rows {
		assert.True(t, strings.HasSuffix(row[3], "*"), "Claude cell of %s", row[1])
		if strings.HasSuffix(row[4], "*") {
			codexMarked = append(codexMarked, row[1])
		}
	}
	assert.ElementsMatch(t, []string{"debugger", "deep-worker", "explorer", "annotator"}, codexMarked)
	assert.Equal(t, []string{"설계·판단", "planner", "fable", "fable · max*", "6-astra · max"},
		composeTableRowTexts(t, layout, "planner"))
	assert.Equal(t, []string{"구현", "debugger", "sonnet*", "sonnet · medium*", "6-luna · max*"},
		composeTableRowTexts(t, layout, "debugger"))
	assert.Equal(t, []string{"구현", "executor", "sonnet", "sonnet · medium*", "6-luna · max"},
		composeTableRowTexts(t, layout, "executor"))
	assert.Equal(t, []string{"탐색·보조", "explorer", "haiku*", "haiku · -*", "6-luna · medium*"},
		composeTableRowTexts(t, layout, "explorer"))
	assert.Equal(t, "* = 바뀌는 칸", layout.footer[len(layout.footer)-1])

	unchanged := after.layout("ko", &after)
	for _, row := range unchanged.rows {
		for _, cell := range row {
			assert.False(t, strings.HasSuffix(cell, "*"), "%s: %q", row[1], cell)
		}
	}
}

// S9: refreshing R's historical presets changes Claude 11 cells and Codex 7.
func TestComposeTable_HistoricalRefreshMarksElevenClaudeAndSevenCodex(t *testing.T) {
	before := composeTableFor(t, "R", "CAT-A")
	cfg := composeTableFixture(t, "R")
	defaults := config.DefaultFullConfig("p").Quality.Presets
	cfg.Quality.Presets = map[string]config.QualityPreset{"balanced": defaults["balanced"], "ultra": defaults["ultra"]}
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	after := buildComposeTable(cfg, composeTableSnapshot(t, "CAT-A"), sources)

	claude, codexCells := 0, 0
	for _, row := range after.layout("ko", &before).rows {
		if strings.HasSuffix(row[3], "*") {
			claude++
		}
		if strings.HasSuffix(row[4], "*") {
			codexCells++
		}
	}
	assert.Equal(t, 11, claude)
	assert.Equal(t, 7, codexCells)
}

// REQ-MUX-18: a Claude override names its preset in the Claude header and
// drives the Claude cells; an override naming no preset is not in force.
func TestComposeTable_ClaudeOverrideHeader(t *testing.T) {
	cfg := composeTableFixture(t, "D")
	cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: "ultra"}
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	layout := buildComposeTable(cfg, composeTableSnapshot(t, "CAT-A"), sources).layout("ko", nil)

	assert.Equal(t, "Claude Code (개별 설정: ultra)", layout.header[3])
	assert.Equal(t, "Codex", layout.header[4])
	assert.Equal(t, "fable · max", composeTableRowTexts(t, layout, "planner")[3])
	assert.Equal(t, "6-astra · max", composeTableRowTexts(t, layout, "planner")[4])

	cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: "missing"}
	layout = buildComposeTable(cfg, composeTableSnapshot(t, "CAT-A"), sources).layout("ko", nil)
	assert.Equal(t, "Claude Code", layout.header[3])
}

// Trust Model: text from autopus.yaml is printed with control characters
// escaped, so a preset name cannot drive the terminal.
func TestComposeTable_EscapesUserDerivedText(t *testing.T) {
	cfg := composeTableFixture(t, "D")
	name := "evil\x1b[31m\x07"
	cfg.Quality.Presets[name] = cfg.Quality.Presets["ultra"]
	cfg.Quality.Providers = map[string]string{config.QualityProviderCodex: name}
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, buildComposeTable(cfg, composeTableSnapshot(t, "CAT-A"), sources).render(&out, "ko", nil))

	assert.NotContains(t, out.String(), "\x1b")
	assert.NotContains(t, out.String(), "\x07")
	assert.Contains(t, out.String(), `Codex (개별 설정: evil\x1b[31m\a)`)
}

// REQ-MUX-22: English labels outside the ko locale; raw values are the same.
func TestComposeTable_EnglishLabels(t *testing.T) {
	r := composeTableFor(t, "R", "CAT-A").layout("en", nil)
	assert.Equal(t, []string{"Group", "Agent", "Tier", "Claude Code", "Codex", "OMP", "OpenCode", "Antigravity"}, r.header)
	assert.Equal(t, []string{"Implementation", "deep-worker", "default(opus)", "opus · max", "6.1-sol · xhigh",
		"platform default", "platform default", "platform default"}, composeTableRowTexts(t, r, "deep-worker"))
	assert.Equal(t, []string{"Codex catalog: read · 0 substituted", "Codex default session: 6-astra · ultra"}, r.footer)

	x := composeTableFor(t, "X", "CAT-A").layout("en", nil)
	assert.Equal(t, "invalid→sonnet", composeTableRowTexts(t, x, "tester")[2])
}

// Table Cell Contract footer: a project whose installed platforms bind no
// per-agent model says so instead of showing a column of defaults only.
func TestComposeTable_NoPerAgentPlatformFooter(t *testing.T) {
	cfg := composeTableFixture(t, "D")
	cfg.Platforms = []string{"opencode", "antigravity-cli", "omp"}
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	layout := buildComposeTable(cfg, composeTableSnapshot(t, "CAT-A"), sources).layout("ko", nil)

	assert.Equal(t, []string{"그룹", "에이전트", "등급", "OMP", "OpenCode", "Antigravity"}, layout.header)
	assert.Equal(t, []string{"설치된 플랫폼은 에이전트별 모델을 쓰지 않습니다"}, layout.footer)
}

// Columns line up in terminal cells even though Korean runes are two cells
// wide: every row places its second column at the same display offset.
func TestComposeTable_ColumnsAlignInTerminalCells(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, composeTableFor(t, "R", "CAT-A").render(&out, "ko", nil))
	lines := strings.Split(out.String(), "\n")

	offsets := map[int]bool{}
	for _, line := range lines[:17] {
		prefix, _, found := strings.Cut(line, "  ")
		require.True(t, found, line)
		rest := strings.TrimLeft(line[len(prefix):], " ")
		offsets[composeTableTextWidth(line[:len(line)-len(rest)])] = true
	}
	assert.Len(t, offsets, 1, "agent column starts at one display offset")
	assert.Equal(t, 4, composeTableTextWidth("구현"))
	assert.Equal(t, 3, composeTableTextWidth("a·b"))
}

// Table Cell Contract: a runtime_default resolution reads `Codex 기본값` when
// no model is left and `<model> · -` when only the effort is, and both cells
// equal what the real Codex generation writes from the same snapshot.
func TestComposeTable_RuntimeDefaultNotationMatchesGeneration(t *testing.T) {
	cases := []struct {
		name, catalog, planner, supervisor string
	}{
		{
			name:       "no model",
			catalog:    `{"models":[{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"max"}]}]}`,
			planner:    "Codex 기본값",
			supervisor: "Codex 기본 세션: Codex 기본값",
		},
		{
			name: "no effort",
			catalog: `{"models":[{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"ultra"}]},` +
				`{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"ultra"}]}]}`,
			planner:    "6.1-sol · -",
			supervisor: "Codex 기본 세션: 6-astra · ultra",
		},
	}
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := composeTableFixture(t, "R2")
			snapshot := driftGenerationSnapshot{codexCatalog: []byte(tc.catalog), codexCatalogFixed: true}
			table := buildComposeTable(cfg, snapshot, sources)
			layout := table.layout("ko", nil)

			assert.Equal(t, tc.planner, composeTableRowTexts(t, layout, "planner")[4])
			assert.Equal(t, []string{"Codex catalog: 읽음 · 대체 16칸", tc.supervisor}, layout.footer)
			dir, codexErr := generateComposeTableOracle(t, cfg, snapshot)
			require.NoError(t, codexErr)
			for _, row := range table.rows {
				assert.Equal(t, readComposeCodexProfile(t, dir, row.agent), row.codex.resolution.Effective, row.agent)
			}
		})
	}
}

// An OMP profile the generator cannot resolve shows `route 오류` in every OMP
// cell instead of a route the generation would never request.
func TestComposeTable_UnresolvableOMPProfile(t *testing.T) {
	cfg := composeTableFixture(t, "O1")
	cfg.RoleModelPolicy.Profile = "missing"
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	layout := buildComposeTable(cfg, composeTableSnapshot(t, "CAT-A"), sources).layout("ko", nil)

	assert.Equal(t, "OMP (요청 route · catalog 확인 전)", layout.header[5])
	for _, row := range layout.rows {
		assert.Equal(t, "route 오류", row[5], row[1])
	}
}
