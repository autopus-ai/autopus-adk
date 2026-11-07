package cli

// SPEC-MODELUX-002 T3 (REQ-MUX2-05, REQ-MUX2-06; acceptance S4, S5, S6, S14):
// every form option names the cells its candidate binding resolves with the
// run's snapshot, through the functions the table and the generators call.

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// newComposeLabelRun is a real session over fixture whose table was last
// drawn with catalog, the state every label of a form run reads.
func newComposeLabelRun(t *testing.T, fixture, catalog string) *composeSessionPlanner {
	t.Helper()
	dir, cfg := composeSessionProject(t, fixture)
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)
	session.shared.snapshot = composeTableSnapshot(t, catalog)
	return session.Planner.(*composeSessionPlanner)
}

func composeOptionLabels[T comparable](options []composeOption[T]) []string {
	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.label)
	}
	return labels
}

func composeToolNamed(t *testing.T, platform string) qualityModelTool {
	t.Helper()
	for _, tool := range qualityModelTools {
		if tool.platform == platform {
			return tool
		}
	}
	t.Fatalf("no tool %q", platform)
	return qualityModelTool{}
}

// S4: the 모든 도구 같게 tier options of 구현 in D, CAT-A. sonnet binds
// balanced again; fable, opus, and haiku bind custom, which gives up the
// standard placement.
func TestComposeLabels_AllToolsGroupOptionsNameEachToolsCells(t *testing.T) {
	planner := newComposeLabelRun(t, "D", "CAT-A")
	labeler := newComposeAllToolsLabeler(planner)

	options := labeler.groupOptions(planner.Start(), composeRoleGroups[0], "그대로")

	assert.Equal(t, []string{
		"그대로 — sonnet",
		"fable — Claude Code: fable-5-1 · max | Codex: 6-astra · max",
		"opus — Claude Code: opus-5-5 · high | Codex: 6.1-sol · xhigh",
		"sonnet — Claude Code: sonnet-5-5 · max | Codex: 6-luna · max",
		"haiku — Claude Code: haiku-4-5 · - | Codex: 6-luna · medium",
	}, composeOptionLabels(options))
	assert.Equal(t, []string{"", "fable", "opus", "sonnet", "haiku"}, composeOptionValues(options))
}

// S5: the Codex tab's options of 구현 and the haiku option of 검증·리뷰,
// whose members declare two efforts.
func TestComposeLabels_CodexToolGroupOptions(t *testing.T) {
	planner := newComposeLabelRun(t, "D", "CAT-A")
	labeler := newComposeToolLabeler(planner.run, composeToolNamed(t, "codex"))
	start := planner.run.toolStart(composeToolNamed(t, "codex"))

	implementation := labeler.groupOptions(start, composeRoleGroups[0], "그대로")
	verification := labeler.groupOptions(start, composeRoleGroups[1], "그대로")

	assert.Equal(t, []string{
		"그대로 — 6-luna · max",
		"6-astra · max  [fable]",
		"6.1-sol · xhigh  [opus]",
		"6-luna · max  [sonnet]",
		"6-luna · medium  [haiku]",
	}, composeOptionLabels(implementation))
	assert.Equal(t, "6-luna · high / 6-luna · medium  [haiku]", verification[4].label)
}

// S6: the Claude tab's agent list and frontend-specialist values, before and
// after frontend-specialist moves to opus, which binds claude-custom and so
// drops the standard placement every other agent had.
func TestComposeLabels_ClaudeToolAgentListAndValues(t *testing.T) {
	planner := newComposeLabelRun(t, "D", "CAT-A")
	tool := composeToolNamed(t, "claude-code")
	labeler := newComposeToolLabeler(planner.run, tool)
	working := planner.run.toolStart(tool)

	assert.Equal(t, "reviewer — opus-5-5 · max", labeler.agentEntry(working, "reviewer"))
	assert.Equal(t, "frontend-specialist — sonnet-5-5 · max", labeler.agentEntry(working, "frontend-specialist"))
	assert.Equal(t, "ux-validator — sonnet-5-5 · high", labeler.agentEntry(working, "ux-validator"))
	assert.Equal(t, []string{
		"그대로 — sonnet-5-5 · max",
		"fable-5-1 · max  [fable]",
		"opus-5-5 · high  [opus]",
		"sonnet-5-5 · max  [sonnet]",
		"haiku-4-5 · -  [haiku]",
	}, composeOptionLabels(labeler.agentOptions(working, "frontend-specialist", "그대로")))

	working["frontend-specialist"] = "opus"
	assert.Equal(t, "frontend-specialist — opus-5-5 · high", labeler.agentEntry(working, "frontend-specialist"))
	assert.Equal(t, "ux-validator — sonnet-5-5 · medium", labeler.agentEntry(working, "ux-validator"))
	// A top-rung agent on opus keeps the top-rung cell under the custom binding.
	assert.Equal(t, "reviewer — opus-5-5 · max", labeler.agentEntry(working, "reviewer"))
}

// The 모든 도구 같게 agent list names the tier with both tools' cells.
func TestComposeLabels_AllToolsAgentEntry(t *testing.T) {
	planner := newComposeLabelRun(t, "D", "CAT-A")
	labeler := newComposeAllToolsLabeler(planner)

	assert.Equal(t, "reviewer — opus (Claude Code: opus-5-5 · max | Codex: 6-astra · max)",
		labeler.agentEntry(planner.Start(), "reviewer"))
}

// S14, first half: a substitute stays visible in the label.
func TestComposeLabels_CodexSubstituteIsNamed(t *testing.T) {
	planner := newComposeLabelRun(t, "D", "CAT-B")
	tool := composeToolNamed(t, "codex")
	labeler := newComposeToolLabeler(planner.run, tool)

	// The 검증·리뷰 group's opus moves qa-explorer and ux-validator to the
	// Sol rung, which CAT-B substitutes; the top-rung reviewer keeps Astra.
	options := labeler.groupOptions(planner.run.toolStart(tool), composeRoleGroups[1], "그대로")

	assert.Equal(t, "6-astra · max / 6-sol · xhigh (대체)  [opus]", options[2].label)
}

// S14, oracle: for D and R, both tool tabs, and every group and tier, each
// label is what the real adapters generate from that option's candidate
// with the same snapshot; each candidate generates only its tab's tool.
// Claude labels name the slug, so an alias in the frontmatter reads through
// ClaudeModelForTier.
func TestComposeLabels_OracleLabelsEqualRealGeneration(t *testing.T) {
	for _, fixture := range []string{"D", "R"} {
		planner := newComposeLabelRun(t, fixture, "CAT-A")
		snapshot := planner.run.snapshot
		for _, platform := range []string{"claude-code", "codex"} {
			tool := composeToolNamed(t, platform)
			labeler := newComposeToolLabeler(planner.run, tool)
			start := planner.run.toolStart(tool)
			for _, group := range composeRoleGroups {
				options := labeler.groupOptions(start, group, "그대로")
				for _, option := range options[1:] {
					next := maps.Clone(start)
					for _, agent := range group.Agents {
						next[agent] = option.value
					}
					dir := t.TempDir()
					_, err := driftAdapterFor(platform, dir, snapshot).Generate(context.Background(), planner.run.toolCandidate(tool)(next))
					require.NoError(t, err, "%s %s %s", fixture, platform, option.label)
					var cells []string
					for _, agent := range group.Agents {
						cell := composeGeneratedLabelCell(t, dir, platform, agent)
						if !containsOMPString(cells, cell) {
							cells = append(cells, cell)
						}
					}
					assert.Equal(t, strings.Join(cells, " / ")+"  ["+option.value+"]", option.label,
						"%s %s %s", fixture, platform, group.en)
				}
			}
		}
	}
}

// composeGeneratedLabelCell reads one agent's generated model and effort in
// the label notation.
func composeGeneratedLabelCell(t *testing.T, dir, platform, agent string) string {
	t.Helper()
	effort := func(value string) string {
		if value == "" {
			return "-"
		}
		return value
	}
	if platform == "codex" {
		profile := readComposeCodexProfile(t, dir, agent)
		return strings.TrimPrefix(profile.Model, "gpt-") + " · " + effort(profile.Effort)
	}
	claude := readComposeTableClaudeProfile(t, dir, agent)
	model := claude.model
	if _, alias := config.NormalizeQualityTier(model); alias {
		model = config.ClaudeModelForTier(model)
	}
	return strings.TrimPrefix(model, "claude-") + " · " + effort(claude.effort)
}

func composeOptionValues[T comparable](options []composeOption[T]) []T {
	values := make([]T, 0, len(options))
	for _, option := range options {
		values = append(values, option.value)
	}
	return values
}
