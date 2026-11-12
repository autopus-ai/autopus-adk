package config

import (
	"maps"
	"slices"
	"sort"
)

// retiredOrchestraCommandStrategies names the orchestra commands releases
// before the verification harness shipped as defaults, with the strategy each
// default carried. The harness no longer runs them; `auto orchestra plan` and
// `auto orchestra brainstorm` still work without a config entry.
var retiredOrchestraCommandStrategies = map[string]string{
	"plan":       "consensus",
	"brainstorm": "debate",
}

// shippedOrchestraCommandProviders is the provider list every shipped command
// default carried.
var shippedOrchestraCommandProviders = []string{"claude", "codex", "gemini"}

// PruneRetiredOrchestraCommands removes the retired plan and brainstorm
// command entries that are still exactly what ADK wrote: the shipped
// strategy, no judge or threshold, and a provider list that is either the
// shipped one or the configured providers (MigrateOrchestraConfig keeps every
// command's list in step with them). Any other value is the operator's and
// stays. It returns the removed key paths in sorted order.
func PruneRetiredOrchestraCommands(cfg *HarnessConfig) []string {
	if cfg == nil || cfg.Orchestra.Commands == nil {
		return nil
	}
	var removed []string
	for name, strategy := range retiredOrchestraCommandStrategies {
		entry, ok := cfg.Orchestra.Commands[name]
		if !ok || !isShippedOrchestraCommand(entry, strategy, cfg.Orchestra.Providers) {
			continue
		}
		delete(cfg.Orchestra.Commands, name)
		removed = append(removed, "orchestra.commands."+name)
	}
	sort.Strings(removed)
	return removed
}

func isShippedOrchestraCommand(entry CommandEntry, strategy string, providers map[string]ProviderEntry) bool {
	if entry.Strategy != strategy || entry.Judge != "" || entry.ConsensusThreshold != 0 {
		return false
	}
	return sameProviderSet(entry.Providers, shippedOrchestraCommandProviders) ||
		sameProviderSet(entry.Providers, slices.Collect(maps.Keys(providers)))
}

// sameProviderSet compares two non-empty provider lists as sets of names
// without duplicates.
func sameProviderSet(left, right []string) bool {
	a, b := slices.Sorted(slices.Values(left)), slices.Sorted(slices.Values(right))
	return len(a) > 0 && slices.Equal(a, b) && len(slices.Compact(slices.Clone(a))) == len(a)
}
