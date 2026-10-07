package orchestra

import (
	"regexp"
	"strings"
)

var defaultStartupHookProviders = map[string]bool{
	"claude": true,
	"codex":  true,
}

// provider_patterns.go is the single declarative source for provider pattern
// defaults (fast-fail rules, hook-capable providers, prompt patterns). The defaults
// here are exact copies of the previously hardcoded values; when a ProviderConfig
// override is unset the resolved pattern set is identical to the legacy behavior
// (INV-003, backward-compatible).

// FastFailRule maps a substring found in provider output to a fast-fail reason.
// Matching is case-insensitive and order-sensitive: the first matching rule wins.
type FastFailRule struct {
	Substring string // substring to look for (matched case-insensitively)
	Reason    string // fast-fail reason emitted when Substring is present
}

// DefaultFastFailRules returns the built-in provider fast-fail rules in priority
// order. These are the canonical hardcoded defaults preserved for backward
// compatibility; callers must not mutate the returned slice's intent.
func DefaultFastFailRules() []FastFailRule {
	return []FastFailRule{
		{Substring: "model_capacity_exhausted", Reason: "provider capacity exhausted"},
		{Substring: "resource_exhausted", Reason: "provider resource exhausted"},
		{Substring: "no capacity available for model", Reason: "provider model capacity unavailable"},
		{Substring: "ratelimitexceeded", Reason: "provider rate limit exceeded"},
	}
}

// matchFastFailRules returns the reason of the first rule whose substring appears
// (case-insensitively) in output, or "" when no rule matches or rules is empty.
func matchFastFailRules(output string, rules []FastFailRule) string {
	lower := strings.ToLower(output)
	for _, rule := range rules {
		if rule.Substring == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(rule.Substring)) {
			return rule.Reason
		}
	}
	return ""
}

// resolveFastFailRules returns the provider's override rules when set, otherwise
// the built-in defaults. An empty (non-nil) override slice is treated as "no rules".
func resolveFastFailRules(override []FastFailRule) []FastFailRule {
	if override != nil {
		return override
	}
	return DefaultFastFailRules()
}

// DefaultHookProviders returns a fresh copy of the providers that have hooks by
// default (claude, gemini, codex). Returning a copy prevents accidental mutation
// of the shared package-level default.
func DefaultHookProviders() map[string]bool {
	out := make(map[string]bool, len(defaultHookProviders))
	for name, has := range defaultHookProviders {
		out[name] = has
	}
	return out
}

// resolveHookProviders builds the hook-capable provider map from per-provider
// HasHook overrides, starting from DefaultHookProviders(). When no provider sets
// an override the result equals DefaultHookProviders() (INV-003).
func resolveHookProviders(providers []ProviderConfig) map[string]bool {
	out := DefaultHookProviders()
	for _, p := range providers {
		identity := providerArtifactIdentity(p.Name)
		if p.HasHook != nil {
			out[identity] = *p.HasHook
			continue
		}
		if usesAntigravityPromptInteractive(p) {
			out[identity] = false
		}
	}
	return out
}

// usesAntigravityPromptInteractive reports whether p is the gemini provider
// served by the Antigravity CLI (agy).
func usesAntigravityPromptInteractive(p ProviderConfig) bool {
	if p.Binary != "agy" && !strings.HasSuffix(p.Binary, "/agy") {
		return false
	}
	return providerArtifactIdentity(p.Name) == "gemini"
}

// DefaultStartupHookProviders returns providers whose generated wiring emits a
// startup-ready artifact. Completion-only hooks are intentionally excluded.
func DefaultStartupHookProviders() map[string]bool {
	out := make(map[string]bool, len(defaultStartupHookProviders))
	for name, has := range defaultStartupHookProviders {
		out[name] = has
	}
	return out
}

// resolveStartupHookProviders applies startup-specific overrides without
// inferring startup capability from completion hook configuration.
func resolveStartupHookProviders(providers []ProviderConfig) map[string]bool {
	out := DefaultStartupHookProviders()
	for _, provider := range providers {
		if provider.HasStartupHook != nil {
			out[providerArtifactIdentity(provider.Name)] = *provider.HasStartupHook
		}
	}
	return out
}

// DefaultPromptPatterns returns the global default prompt detection patterns used
// as the fallback by isPromptVisible and isPromptLine. It is the single accessor
// for the hardcoded defaultPromptPatterns slice.
func DefaultPromptPatterns() []*regexp.Regexp {
	return defaultPromptPatterns
}
