package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeTableFor builds the table one fixture shows with one catalog fixture.
func composeTableFor(t *testing.T, fixture, catalog string) composeTable {
	t.Helper()
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	return buildComposeTable(composeTableFixture(t, fixture), composeTableSnapshot(t, catalog), sources)
}

// composeTableRowTexts returns the rendered cells of one agent's row.
func composeTableRowTexts(t *testing.T, layout composeTableLayout, agent string) []string {
	t.Helper()
	for _, row := range layout.rows {
		if len(row) > 1 && row[1] == agent {
			return row
		}
	}
	t.Fatalf("no table row for agent %q", agent)
	return nil
}

func composeTableAgentRow(t *testing.T, table composeTable, agent string) composeTableRow {
	t.Helper()
	for _, row := range table.rows {
		if row.agent == agent {
			return row
		}
	}
	t.Fatalf("no table row for agent %q", agent)
	return composeTableRow{}
}

// S4: D shows only the Claude Code and Codex columns, and its cells are the
// standard balanced placement the generators write: exact Claude ids and the
// native Codex rung.
func TestComposeTable_FixtureD_ShowsStandardPlacementCells(t *testing.T) {
	table := composeTableFor(t, "D", "CAT-A")
	layout := table.layout("ko", nil)

	assert.Equal(t, []string{"그룹", "에이전트", "등급", "Claude Code", "Codex"}, layout.header)
	assert.Equal(t, []string{"검증·리뷰", "reviewer", "opus", "opus-5-5 · max", "6-astra · max"},
		composeTableRowTexts(t, layout, "reviewer"))
	assert.Equal(t, []string{"구현", "frontend-specialist", "sonnet", "sonnet-5-5 · max", "6-luna · max"},
		composeTableRowTexts(t, layout, "frontend-specialist"))
	assert.Equal(t, []string{"검증·리뷰", "ux-validator", "sonnet", "sonnet-5-5 · high", "6-luna · max"},
		composeTableRowTexts(t, layout, "ux-validator"))
	// D leaves the root Codex model to the user, so no supervisor line.
	assert.Equal(t, []string{"Codex catalog: 읽음 · 대체 0칸"}, layout.footer)

	reviewer := composeTableAgentRow(t, table, "reviewer")
	assert.Equal(t, composeTableClaudeCell{model: "claude-opus-5-5", effort: "max"}, reviewer.claude)
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "max"}, reviewer.codex.resolution.Effective)
}

// S4: rows follow the Role Group Contract order, one per canonical agent.
func TestComposeTable_RowsFollowRoleGroupOrder(t *testing.T) {
	layout := composeTableFor(t, "D", "CAT-A").layout("ko", nil)

	var agents, groups []string
	for _, row := range layout.rows {
		groups = append(groups, row[0])
		agents = append(agents, row[1])
	}
	assert.Equal(t, []string{"frontend-specialist", "reviewer", "qa-explorer", "ux-validator"}, agents)
	assert.ElementsMatch(t, config.CanonicalAgentNames(), agents)
	assert.Equal(t, []string{"구현", "검증·리뷰", "검증·리뷰", "검증·리뷰"}, groups)
}

// S4: R runs ultra with HU1 written, five platforms, and a quality-managed
// root session. Absent agents show the source fallback tier they resolve to.
func TestComposeTable_FixtureR_ShowsUltraCellsAndFooter(t *testing.T) {
	layout := composeTableFor(t, "R", "CAT-A").layout("ko", nil)

	assert.Equal(t, []string{"그룹", "에이전트", "등급", "Claude Code", "Codex", "OMP", "OpenCode", "Antigravity"},
		layout.header)
	assert.Equal(t, []string{"검증·리뷰", "reviewer", "opus", "opus · max", "6.1-sol · xhigh",
		"플랫폼 기본값", "플랫폼 기본값", "플랫폼 기본값"}, composeTableRowTexts(t, layout, "reviewer"))
	assert.Equal(t, []string{"검증·리뷰", "qa-explorer", "기본값(opus)", "opus · max", "6.1-sol · xhigh",
		"플랫폼 기본값", "플랫폼 기본값", "플랫폼 기본값"}, composeTableRowTexts(t, layout, "qa-explorer"))
	assert.Equal(t, []string{"구현", "frontend-specialist", "기본값(sonnet)", "sonnet · high", "6.1-sol · xhigh",
		"플랫폼 기본값", "플랫폼 기본값", "플랫폼 기본값"}, composeTableRowTexts(t, layout, "frontend-specialist"))
	assert.Equal(t, []string{"Codex catalog: 읽음 · 대체 0칸", "Codex 기본 세션: 6-astra · ultra"}, layout.footer)
}

// S4 and REQ-MUX-18: a provider override computes that column from the
// override preset and names it in the header; Claude keeps D's cells.
func TestComposeTable_FixtureP_CodexOverrideColumn(t *testing.T) {
	layout := composeTableFor(t, "P", "CAT-A").layout("ko", nil)

	assert.Equal(t, []string{"그룹", "에이전트", "등급", "Claude Code", "Codex (개별 설정: codex-custom)"}, layout.header)
	assert.Equal(t, "6-luna · high", composeTableRowTexts(t, layout, "qa-explorer")[4])
	assert.Equal(t, "6-luna · medium", composeTableRowTexts(t, layout, "frontend-specialist")[4])
	assert.Equal(t, "6-luna · high", composeTableRowTexts(t, layout, "reviewer")[4])

	d := composeTableFor(t, "D", "CAT-A").layout("ko", nil)
	for _, agent := range config.CanonicalAgentNames() {
		assert.Equal(t, composeTableRowTexts(t, d, agent)[:4], composeTableRowTexts(t, layout, agent)[:4], agent)
	}
}

// S4: the OMP column shows the requested route of the selected profile, with
// the header saying the catalog has not been consulted.
func TestComposeTable_OMPColumnShowsRequestedRoute(t *testing.T) {
	o1 := composeTableFor(t, "O1", "CAT-A").layout("ko", nil)
	assert.Equal(t, "OMP (요청 route · catalog 확인 전)", o1.header[5])
	assert.Equal(t, "frontend-specialist · sonnet-5-5:max", composeTableRowTexts(t, o1, "frontend-specialist")[5])
	assert.Equal(t, "ux-validator · sonnet-5-5:high", composeTableRowTexts(t, o1, "ux-validator")[5])

	o2 := composeTableFor(t, "O2", "CAT-A").layout("ko", nil)
	assert.Equal(t, "OMP (요청 route · catalog 확인 전)", o2.header[5])
	assert.Equal(t, "frontend-specialist · opus-5-5:xhigh", composeTableRowTexts(t, o2, "frontend-specialist")[5])

	r := composeTableFor(t, "R", "CAT-A").layout("ko", nil)
	assert.Equal(t, "OMP", r.header[5])
}

// S5: OMP's oracle is the requested route itself, the first candidate of
// OMPAgentRoute for the agent, which OMP serves under its own name.
func TestComposeTable_OMPCellsEqualTheAgentRoute(t *testing.T) {
	for _, fixture := range []string{"O1", "O2"} {
		t.Run(fixture, func(t *testing.T) {
			cfg := composeTableFixture(t, fixture)
			sources, err := loadComposeAgentSources()
			require.NoError(t, err)
			table := buildComposeTable(cfg, composeTableSnapshot(t, "CAT-A"), sources)
			_, profile, ok := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
			require.True(t, ok)
			for _, agent := range config.CanonicalAgentNames() {
				_, route, err := profile.OMPAgentRoute(agent)
				require.NoError(t, err)
				require.NotEmpty(t, route.Candidates)
				want := composeTableOMPCell{
					native:   agent,
					selector: route.Candidates[0].Selector,
					thinking: route.Candidates[0].Thinking,
				}
				assert.Equal(t, want, composeTableAgentRow(t, table, agent).omp, agent)
			}
		})
	}
}

// S5: an invalid written tier is shown as such, and R's qa-explorer shows the
// tier generation projects rather than the per-tool table's empty fallback.
func TestComposeTable_InvalidTierAndSourceFallback(t *testing.T) {
	x := composeTableFor(t, "X", "CAT-A").layout("ko", nil)
	assert.Equal(t, []string{"잘못된 값→sonnet", "sonnet · high", "6.1-sol · xhigh"},
		composeTableRowTexts(t, x, "ux-validator")[2:5])

	r := composeTableFor(t, "R", "CAT-A").layout("ko", nil)
	assert.Equal(t, "opus · max", composeTableRowTexts(t, r, "qa-explorer")[3])
}

// S4: rendering is deterministic and carries no terminal escape sequences.
func TestComposeTable_RenderIsDeterministicAndPlain(t *testing.T) {
	for _, fixture := range []string{"D", "R", "P", "O1", "O2"} {
		t.Run(fixture, func(t *testing.T) {
			table := composeTableFor(t, fixture, "CAT-A")
			var first, second bytes.Buffer
			require.NoError(t, table.render(&first, "ko", nil))
			require.NoError(t, composeTableFor(t, fixture, "CAT-A").render(&second, "ko", nil))

			assert.Equal(t, first.Bytes(), second.Bytes())
			assert.NotContains(t, first.String(), "\x1b")
			for _, line := range strings.Split(strings.TrimSuffix(first.String(), "\n"), "\n") {
				assert.Equal(t, strings.TrimRight(line, " "), line, "no trailing spaces")
			}
		})
	}
}
