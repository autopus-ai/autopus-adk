package cli

import (
	"maps"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeLabeler builds the Label Contract labels of one tab of a form run
// (SPEC-MODELUX-002 REQ-MUX2-05, REQ-MUX2-06). Every label is a table cell:
// the option's composition goes into a candidate config, whose table is
// built with the run's snapshot through the resolvers the generators call,
// so a label, the preview, and the same run's generation cannot disagree.
// An option that changes nothing reads the unchanged candidate.
type composeLabeler struct {
	run *composeSessionRun
	// platform is the tool tab's platform, or "" for 모든 도구 같게.
	platform string
	// candidate returns the config a composition leaves once it is written.
	candidate func(composition map[string]string) *config.HarnessConfig
}

// newComposeAllToolsLabeler labels the 모든 도구 같게 tab: a composition's
// candidate is the SPEC-MODELUX-001 binding the planner plans for it, on top
// of a pending `u` refresh when there is one.
func newComposeAllToolsLabeler(planner *composeSessionPlanner) composeLabeler {
	return composeLabeler{run: planner.run, candidate: func(composition map[string]string) *config.HarnessConfig {
		binding, _ := planner.bind(composition)
		return binding.Candidate
	}}
}

// newComposeToolLabeler labels tool's tab by the Per-Tool Binding Contract.
func newComposeToolLabeler(run *composeSessionRun, tool qualityModelTool) composeLabeler {
	return composeLabeler{run: run, platform: tool.platform, candidate: run.toolCandidate(tool)}
}

// groupOptions are the options of one role group's select: keep, whose value
// is "", then one option per tier in config.QualityTiers() order. keep is
// the locale's keep word.
func (l composeLabeler) groupOptions(working map[string]string, group composeRoleGroup, keep string) []composeOption[string] {
	display := qualityComposition{tiers: working}.GroupDisplay(group, l.run.lang)
	return l.options(working, group.Agents, keep, display)
}

// agentOptions are the options of one agent's value select.
func (l composeLabeler) agentOptions(working map[string]string, agent, keep string) []composeOption[string] {
	return l.options(working, []string{agent}, keep, working[agent])
}

// options labels keep with display on 모든 도구 같게 and with the working
// candidate's cells on a tool tab, then each tier with the cells of the
// candidate that sets agents to it.
func (l composeLabeler) options(working map[string]string, agents []string, keep, display string) []composeOption[string] {
	current := display
	if l.platform != "" {
		current = l.cells(l.candidate(working), agents)
	}
	options := []composeOption[string]{{label: keep + " — " + current}}
	for _, tier := range config.QualityTiers() {
		next := maps.Clone(working)
		for _, agent := range agents {
			next[agent] = tier
		}
		cells := l.cells(l.candidate(next), agents)
		label := cells + "  [" + tier + "]"
		if l.platform == "" {
			label = tier
			if cells != "" {
				label += " — " + cells
			}
		}
		options = append(options, composeOption[string]{label: label, value: tier})
	}
	return options
}

// agentEntry is agent's line in the per-agent list: its tier with both
// tools' cells for 모든 도구 같게, the tool's cells on a tool tab.
func (l composeLabeler) agentEntry(working map[string]string, agent string) string {
	cells := l.cells(l.candidate(working), []string{agent})
	switch {
	case l.platform != "":
		return agent + " — " + cells
	case cells == "":
		return agent + " — " + working[agent]
	}
	return agent + " — " + working[agent] + " (" + cells + ")"
}

// cells is the label text of agents under candidate: the tool's cells on a
// tool tab; for 모든 도구 같게 `Claude Code: <C> | Codex: <X>` over the
// installed columns, and "" when neither is installed.
func (l composeLabeler) cells(candidate *config.HarnessConfig, agents []string) string {
	view := l.run.viewOf(candidate)
	table := buildComposeTable(view, l.run.snapshot, l.run.sources)
	if l.platform != "" {
		return l.join(view, table, l.platform, agents)
	}
	var parts []string
	for _, column := range []struct{ platform, label string }{
		{"claude-code", "Claude Code"}, {"codex", "Codex"},
	} {
		if table.shows(column.platform) {
			parts = append(parts, column.label+": "+l.join(view, table, column.platform, agents))
		}
	}
	return strings.Join(parts, " | ")
}

// join writes one tool's cell of each agent in member order, each distinct
// text once, joined by " / ". A Claude cell names the model slug
// ClaudeAgentModel resolves, not the frontmatter alias the table shows, with
// the effort ClaudeAgentProfile writes; a Codex cell is the table's.
func (l composeLabeler) join(view *config.HarnessConfig, table composeTable, platform string, agents []string) string {
	labels := composeTableLabelsFor(l.run.lang)
	rows := make(map[string]composeTableRow, len(table.rows))
	for _, row := range table.rows {
		rows[row.agent] = row
	}
	var texts []string
	for _, agent := range agents {
		row := rows[agent]
		text := composeTableCodexText(row.codex, labels)
		if platform == "claude-code" {
			model := view.Quality.ClaudeAgentModel(agent, l.run.sources[agent].FallbackTier)
			text = strings.TrimPrefix(model, "claude-") + " · " + composeTableEffortText(row.claude.effort)
		}
		if !slices.Contains(texts, text) {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, " / ")
}
