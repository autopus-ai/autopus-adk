package cli

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeRoleGroup is one UI-only role group of the composition step
// (SPEC-MODELUX-001 REQ-MUX-01, Role Group Contract). A group is a selection
// shortcut: choosing its tier sets every member agent's tier, and nothing
// about the group itself is ever written to autopus.yaml.
type composeRoleGroup struct {
	ko, en string
	Agents []string
}

// composeRoleGroups lists the groups in contract order, members in display
// order. Together they cover config.CanonicalAgentNames() exactly once; the
// coverage test fails as soon as the canonical role set changes.
var composeRoleGroups = []composeRoleGroup{
	{ko: "설계·판단", en: "Design·Judgment", Agents: []string{
		"planner", "architect", "spec-writer", "security-auditor",
	}},
	{ko: "구현", en: "Implementation", Agents: []string{
		"executor", "debugger", "deep-worker", "devops", "frontend-specialist", "perf-engineer",
	}},
	{ko: "검증·리뷰", en: "Verification·Review", Agents: []string{
		"reviewer", "validator", "tester", "ux-validator",
	}},
	{ko: "탐색·보조", en: "Exploration·Support", Agents: []string{
		"explorer", "annotator",
	}},
}

// Label returns the group name for lang as tui.DetectSystemLang reports it:
// Korean for "ko", English otherwise.
func (g composeRoleGroup) Label(lang string) string {
	if lang == "ko" {
		return g.ko
	}
	return g.en
}

// qualityComposition is the per-agent tier state of one composition session,
// keyed by canonical agent name. It only ever holds names from the tier
// vocabulary, so a preset written from it cannot carry a value the loader
// expanded from the environment.
type qualityComposition struct {
	tiers map[string]string
}

// newQualityComposition starts a session from the tiers the native generators
// project today (REQ-MUX-02): EffectiveAgentTier under the global view, with
// the fallback tier each agent's embedded source declares. A historical
// balanced layout therefore starts on its standard placement rather than on
// the tiers its YAML spells out, and a provider override never moves the
// global start.
func newQualityComposition(quality config.QualityConf, sources map[string]composeAgentSource) qualityComposition {
	agents := config.CanonicalAgentNames()
	tiers := make(map[string]string, len(agents))
	for _, agent := range agents {
		tiers[agent] = quality.EffectiveAgentTier(agent, sources[agent].FallbackTier)
	}
	return qualityComposition{tiers: tiers}
}

// Agents returns a detached agent → tier map of every canonical agent.
func (c qualityComposition) Agents() map[string]string {
	return maps.Clone(c.tiers)
}

// Equal reports whether both compositions assign every agent the same tier.
func (c qualityComposition) Equal(other qualityComposition) bool {
	return maps.Equal(c.tiers, other.tiers)
}

// SetGroupTier sets every member of group to tier (REQ-MUX-03). An unknown
// tier is refused and changes nothing.
func (c *qualityComposition) SetGroupTier(group composeRoleGroup, tier string) error {
	canonical, ok := config.NormalizeQualityTier(tier)
	if !ok {
		return fmt.Errorf("unknown tier %q (choose one of: %s)",
			strings.TrimSpace(tier), strings.Join(config.QualityTiers(), ", "))
	}
	for _, agent := range group.Agents {
		c.tiers[agent] = canonical
	}
	return nil
}

// GroupDisplay renders the group's tier (REQ-MUX-03): the tier name when every
// member holds it, otherwise `혼합 (fable 2 · sonnet 4)` (en: `mixed (...)`)
// with counts in config.QualityTiers() order and zero counts left out. It is
// computed from the current state, so it is fresh after every edit.
func (c qualityComposition) GroupDisplay(group composeRoleGroup, lang string) string {
	counts := make(map[string]int, len(group.Agents))
	for _, agent := range group.Agents {
		counts[c.tiers[agent]]++
	}
	if len(counts) == 1 {
		return c.tiers[group.Agents[0]]
	}
	parts := make([]string, 0, len(counts))
	for _, tier := range config.QualityTiers() {
		if n := counts[tier]; n > 0 {
			parts = append(parts, tier+" "+strconv.Itoa(n))
		}
	}
	mixed := "mixed"
	if lang == "ko" {
		mixed = "혼합"
	}
	return mixed + " (" + strings.Join(parts, " · ") + ")"
}
