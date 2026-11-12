package cli

import (
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// initQualityPresetNames are the presets the wizard's quality step offers,
// with the name each line starts with.
var initQualityPresetNames = []struct{ preset, name string }{
	{"ultra", "Ultra"},
	{"balanced", "Balanced"},
}

// initQualityLabels builds the quality step's line for each preset it offers:
// the preset name and the tier choosing it gives every generated agent, as
// the generators resolve it, grouped strongest tier first. That is all a
// preset sets, so the line claims nothing else.
func initQualityLabels(quality config.QualityConf) map[string]string {
	labels := make(map[string]string, len(initQualityPresetNames))
	for _, entry := range initQualityPresetNames {
		labels[entry.preset] = entry.name + " — " + qualityTierSummary(quality.WithGlobalOverride(entry.preset))
	}
	return labels
}

// qualityTierSummary lists the generated agents by the tier view gives them,
// such as "reviewer, qa-explorer: opus · frontend-specialist, ux-validator: sonnet".
func qualityTierSummary(view config.QualityConf) string {
	byTier := make(map[string][]string)
	for _, agent := range config.CanonicalAgentNames() {
		tier := view.EffectiveAgentTier(agent, "sonnet")
		byTier[tier] = append(byTier[tier], agent)
	}
	var groups []string
	for _, tier := range config.QualityTiers() {
		if agents := byTier[tier]; len(agents) > 0 {
			groups = append(groups, strings.Join(orderReviewFirst(agents), ", ")+": "+tier)
		}
	}
	return strings.Join(groups, " · ")
}

// orderReviewFirst puts the review roles ahead of the UX roles inside one
// tier group, the order the harness runs them in.
func orderReviewFirst(agents []string) []string {
	rank := map[string]int{"reviewer": 0, "qa-explorer": 1, "frontend-specialist": 2, "ux-validator": 3}
	ordered := slices.Clone(agents)
	slices.SortStableFunc(ordered, func(a, b string) int { return rank[a] - rank[b] })
	return ordered
}
