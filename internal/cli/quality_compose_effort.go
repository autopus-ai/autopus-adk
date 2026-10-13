package cli

import (
	"maps"
	"slices"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeEffortTab is the effort half of one Claude Code or Codex tab's
// working composition (SPEC-MODELUX-003 REQ-MUX3-03, REQ-MUX3-04): the
// explicit efforts the tool's preset stored when the tab opened, and those
// the operator's answers leave. The tab's planner and labeler share one, so
// a later group's labels, the preview, and the write read the same efforts.
// Every effort it holds is a config.QualityEfforts level.
type composeEffortTab struct {
	run        *composeSessionRun
	tool       qualityModelTool
	startTiers map[string]string
	start      map[string]string
	working    map[string]string
	// unreadNoted reports that the unreadable-catalog notice was printed.
	unreadNoted bool
}

// newComposeToolTab wires tool's tab: a planner and a labeler over one
// effort state that starts from the efforts the tool's preset stores.
func newComposeToolTab(run *composeSessionRun, tool qualityModelTool) (composeToolPlanner, composeLabeler) {
	start := qualityToolEfforts(run.cfg.Quality, tool)
	tab := &composeEffortTab{
		run: run, tool: tool, startTiers: run.toolStart(tool), start: start, working: maps.Clone(start),
	}
	labeler := composeLabeler{run: run, platform: tool.platform, effort: tab,
		candidate: func(tiers map[string]string) *config.HarnessConfig { return tab.candidate(tiers, tab.working) }}
	return composeToolPlanner{run: run, tool: tool, effort: tab}, labeler
}

// candidate is the config tiers and efforts leave once written: the loaded
// config when both equal the tab's start, otherwise the tool bound to
// <provider>-custom holding them (Per-Tool Binding Contract).
func (e *composeEffortTab) candidate(tiers, efforts map[string]string) *config.HarnessConfig {
	if maps.Equal(tiers, e.startTiers) && maps.Equal(efforts, e.start) {
		return e.run.view()
	}
	return e.custom(tiers, efforts)
}

// custom binds the tool to <provider>-custom whatever the start was: the
// binding any stored effort lands the tab in.
func (e *composeEffortTab) custom(tiers, efforts map[string]string) *config.HarnessConfig {
	return withQualityToolPresetEfforts(e.run.view(), e.tool, qualityToolPresetName(e.tool), tiers, efforts)
}

// affected lists the members an effort select covers: every member on
// Codex, every non-haiku member on Claude Code, which writes no effort for
// haiku (Effort Option Contract).
func (e *composeEffortTab) affected(tiers map[string]string, members []string) []string {
	if e.tool.provider != config.QualityProviderClaude {
		return slices.Clone(members)
	}
	return slices.DeleteFunc(slices.Clone(members), func(agent string) bool { return tiers[agent] == "haiku" })
}

// levels are the Effort Option Contract levels for agents: Claude's five,
// or for Codex those every agent's effective model reports in the run's
// snapshot. readable is false when that snapshot is unreadable, whose
// notice the form prints instead of a select.
func (e *composeEffortTab) levels(tiers map[string]string, agents []string) (levels []string, readable bool) {
	levels = config.QualityEfforts()
	if e.tool.provider == config.QualityProviderClaude {
		return levels, true
	}
	catalog, err := config.ParseCodexModelCatalog(e.run.snapshot.codexCatalog)
	if err != nil {
		return nil, false
	}
	cfg := e.candidate(tiers, e.working)
	for _, agent := range agents {
		resolution, err := e.codexResolution(cfg, agent)
		model := resolution.Effective.Model
		if err != nil || model == "" {
			return nil, true
		}
		levels = slices.DeleteFunc(levels, func(level string) bool { return !catalog.Supports(model, level) })
	}
	return levels, true
}

// option returns the efforts the working composition holds once level is
// chosen for agents, and whether level is their automatic effort. An agent
// whose automatic effort is level stores nothing (REQ-MUX3-03), judged in
// the binding the choice lands in: nothing when every agent shows level
// without its stored effort; otherwise the agents whose effort under
// <provider>-custom differs; and every agent when storing those alone
// would land back on the start binding, which shows something else.
func (e *composeEffortTab) option(tiers map[string]string, agents []string, level string) (map[string]string, bool) {
	bare := maps.Clone(e.working)
	for _, agent := range agents {
		delete(bare, agent)
	}
	if e.shows(e.candidate(tiers, bare), agents, level) {
		return bare, true
	}
	custom := e.custom(tiers, bare)
	stored := composeEffortsWith(bare, slices.DeleteFunc(slices.Clone(agents), func(agent string) bool {
		return e.effort(custom, agent) == level
	}), level)
	if e.shows(e.candidate(tiers, stored), agents, level) {
		return stored, false
	}
	return composeEffortsWith(bare, agents, level), false
}

// shows reports whether every agent's effort cell under cfg is level.
func (e *composeEffortTab) shows(cfg *config.HarnessConfig, agents []string, level string) bool {
	for _, agent := range agents {
		if e.effort(cfg, agent) != level {
			return false
		}
	}
	return true
}

// effort is agent's effort cell under cfg through the resolvers the table
// and the generators call: the Claude frontmatter effort, or the Codex
// effort the run's snapshot resolves.
func (e *composeEffortTab) effort(cfg *config.HarnessConfig, agent string) string {
	if e.tool.provider == config.QualityProviderClaude {
		_, effort := cfg.Quality.ClaudeAgentProfile(agent, e.run.sources[agent].FallbackTier)
		return effort
	}
	resolution, err := e.codexResolution(cfg, agent)
	if err != nil {
		return ""
	}
	return resolution.Effective.Effort
}

func (e *composeEffortTab) codexResolution(cfg *config.HarnessConfig, agent string) (config.CodexProfileResolution, error) {
	source := e.run.sources[agent]
	return codex.ResolveAgentProfile(cfg.Quality, e.run.snapshot.codexCatalog, agent, source.FallbackTier, source.CodexEffort)
}

// composeEffortsWith returns a copy of base with every agent at level.
func composeEffortsWith(base map[string]string, agents []string, level string) map[string]string {
	efforts := make(map[string]string, len(base)+len(agents))
	maps.Copy(efforts, base)
	for _, agent := range agents {
		efforts[agent] = level
	}
	return efforts
}
