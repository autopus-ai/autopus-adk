package cli

import (
	"errors"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeTablePlatforms is the fixed column order of the platform columns.
var composeTablePlatforms = []string{"claude-code", "codex", "omp", "opencode", "antigravity-cli"}

// composeTableTierSource records where a tier cell's value came from, which only
// changes its notation: the raw value is always the effective tier.
type composeTableTierSource int

const (
	composeTableTierWritten composeTableTierSource = iota
	composeTableTierDefault                        // the bound preset does not name the agent
	composeTableTierInvalid                        // the bound preset names a tier outside the vocabulary
)

type composeTableTierCell struct {
	tier   string
	source composeTableTierSource
}

// composeTableClaudeCell is the frontmatter model and effort the Claude generator
// writes; an empty effort means the effort line is dropped.
type composeTableClaudeCell struct {
	model, effort string
}

// composeTableCodexCell is the resolution the Codex agent template renders. A
// refused cell is a resolution error: the template, and with it the whole
// Codex generation, fails.
type composeTableCodexCell struct {
	resolution config.CodexProfileResolution
	refused    bool
}

// composeTableOMPCell is the requested route of the OMP agent serving an agent:
// the first candidate before OMP's own catalog is consulted. The zero value
// means no profile is selected, so OMP keeps its platform default.
type composeTableOMPCell struct {
	native, selector, thinking string
	unresolved                 bool
}

type composeTableRow struct {
	group  int
	agent  string
	tier   composeTableTierCell
	claude composeTableClaudeCell
	codex  composeTableCodexCell
	omp    composeTableOMPCell
}

// composeTable holds the raw values of one composition table: every canonical
// agent's cells and the column and footer facts, computed from one config and
// one Codex catalog snapshot. Raw values, not their notation, decide whether a
// cell changed, and they are what the generators write.
type composeTable struct {
	platforms         []string
	claudeOverride    string
	codexOverride     string
	ompRoutes         bool
	catalogRead       bool
	supervisor        config.CodexProfileResolution
	supervisorManaged bool
	rows              []composeTableRow
	// reaFallbackFilter, when set, keeps the analysis-rung fallback lines the
	// run has not printed yet; a table outside a run prints them all.
	reaFallbackFilter func([]adapter.ReaFallback) []adapter.ReaFallback
}

// buildComposeTable computes the table through the functions the generators
// call with the arguments they pass: the full quality section (each resolver
// applies its own provider view), the embedded agent sources, and the run's
// catalog snapshot. Callers pass the snapshot captureComposeSnapshot froze,
// because the same run's generation reads exactly that snapshot.
func buildComposeTable(
	cfg *config.HarnessConfig,
	snapshot driftGenerationSnapshot,
	sources map[string]composeAgentSource,
) composeTable {
	quality := cfg.Quality
	catalog := snapshot.codexCatalog
	table := composeTable{
		claudeOverride: composeTableProviderOverride(quality, config.QualityProviderClaude),
		codexOverride:  composeTableProviderOverride(quality, config.QualityProviderCodex),
		ompRoutes:      cfg.RoleModelPolicy.Profile != "",
	}
	for _, platform := range composeTablePlatforms {
		if containsPlatform(cfg.Platforms, platform) {
			table.platforms = append(table.platforms, platform)
		}
	}
	if table.shows("codex") {
		_, err := config.ParseCodexModelCatalog(catalog)
		table.catalogRead = err == nil
		table.supervisor, table.supervisorManaged = codex.ResolveSupervisorProfile(quality, catalog)
	}
	ompRoute := composeTableOMPRoutes(cfg)
	// Rows follow the Role Group Contract: group order, then member order.
	for index, group := range composeRoleGroups {
		for _, agent := range group.Agents {
			source := sources[agent]
			model, effort := quality.ClaudeAgentProfile(agent, source.FallbackTier)
			resolution, err := codex.ResolveAgentProfile(
				quality, catalog, agent, source.FallbackTier, source.CodexEffort,
			)
			table.rows = append(table.rows, composeTableRow{
				group:  index,
				agent:  agent,
				tier:   composeTableTier(quality, agent, source.FallbackTier),
				claude: composeTableClaudeCell{model: model, effort: effort},
				codex:  composeTableCodexCell{resolution: resolution, refused: err != nil},
				omp:    ompRoute(agent),
			})
		}
	}
	return table
}

func (t composeTable) shows(platform string) bool {
	return containsPlatform(t.platforms, platform)
}

// codexRefused reports whether the Codex generation of this table would fail
// because the catalog refuses a standard placement.
func (t composeTable) codexRefused() bool {
	if !t.shows("codex") {
		return false
	}
	for _, row := range t.rows {
		if row.codex.refused {
			return true
		}
	}
	return false
}

// codexFallbacks counts the Codex cells a read catalog answered with a
// substitute.
func (t composeTable) codexFallbacks() int {
	count := 0
	for _, row := range t.rows {
		if !row.codex.refused && row.codex.resolution.Fallback &&
			row.codex.resolution.Reason != config.CodexResolutionSessionModel {
			count++
		}
	}
	return count
}

// codexReaFallbackLines is the Form Text line of every Codex cell left on the
// session model by the analysis-rung fallback (SPEC-REA-001), in row order,
// less the ones reaFallbackFilter says the run already printed. The cell
// itself reads as the session model and counts as no substitute.
func (t composeTable) codexReaFallbackLines(lang string) []string {
	var facts []adapter.ReaFallback
	for _, row := range t.rows {
		resolution := row.codex.resolution
		if row.codex.refused || resolution.Reason != config.CodexResolutionSessionModel {
			continue
		}
		facts = append(facts, adapter.ReaFallback{
			Catalog: "Codex", Agent: row.agent,
			Requested: resolution.Requested.Model + "/" + resolution.Requested.Effort,
		})
	}
	if t.reaFallbackFilter != nil {
		facts = t.reaFallbackFilter(facts)
	}
	lines := make([]string, 0, len(facts))
	for _, fact := range facts {
		lines = append(lines, reaFallbackLine(fact, lang))
	}
	return lines
}

// usesAgentModels reports whether any installed platform binds a model per
// agent: Claude Code and Codex always do, OMP only with a selected profile.
func (t composeTable) usesAgentModels() bool {
	return t.shows("claude-code") || t.shows("codex") || t.shows("omp") && t.ompRoutes
}

// composeTableTier resolves the tier column: the effective tier under the global
// view, noting whether the bound preset left the agent out or named an
// invalid tier. The preset lookup is AgentTier's own.
func composeTableTier(quality config.QualityConf, agent, fallbackTier string) composeTableTierCell {
	cell := composeTableTierCell{tier: quality.EffectiveAgentTier(agent, fallbackTier)}
	view := quality.ForProvider("")
	preset, bound := view.Presets[view.Default]
	if !bound {
		preset, bound = view.Presets["balanced"]
	}
	raw, written := preset.Agents[agent]
	switch {
	case !bound || !written:
		cell.source = composeTableTierDefault
	default:
		if _, valid := config.NormalizeQualityTier(raw); !valid {
			cell.source = composeTableTierInvalid
		}
	}
	return cell
}

// composeTableProviderOverride names the preset a provider column follows instead
// of the global binding, or "" when no override is in force. An override that
// names no known preset is ignored by EffectiveMode, so it is not shown.
func composeTableProviderOverride(quality config.QualityConf, provider string) string {
	mode := strings.TrimSpace(quality.Providers[provider])
	if mode == "" || quality.EffectiveMode(provider) != mode {
		return ""
	}
	return mode
}

// composeTableOMPRoutes returns the requested-route lookup the OMP generator
// starts from: no route without a selected profile or pin, and an unresolved
// cell wherever the generator itself would fail before reaching the catalog.
// An agent a pins-only policy does not pin keeps the session model.
func composeTableOMPRoutes(cfg *config.HarnessConfig) func(string) composeTableOMPCell {
	policy := cfg.RoleModelPolicy
	if policy.Profile == "" && len(policy.Agents) == 0 {
		return func(string) composeTableOMPCell { return composeTableOMPCell{} }
	}
	unresolved := func(string) composeTableOMPCell { return composeTableOMPCell{unresolved: true} }
	if policy.Validate() != nil {
		return unresolved
	}
	_, profile, ok := policy.SelectedRoleModelProfileForQuality(cfg.Quality)
	if !ok {
		return unresolved
	}
	// Every agent is routed under its own name.
	return func(agent string) composeTableOMPCell {
		_, route, err := profile.OMPAgentRoute(agent)
		if errors.Is(err, config.ErrOMPAgentUnrouted) {
			return composeTableOMPCell{}
		}
		if err != nil || len(route.Candidates) == 0 {
			return composeTableOMPCell{unresolved: true}
		}
		first := route.Candidates[0]
		return composeTableOMPCell{native: agent, selector: first.Selector, thinking: first.Thinking}
	}
}
