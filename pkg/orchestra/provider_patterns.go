package orchestra

import "strings"

// provider_patterns.go is the single declarative source for provider pattern
// defaults (fast-fail rules). The defaults here are exact copies of the
// previously hardcoded values; when a ProviderConfig override is unset the
// resolved pattern set is identical to the legacy behavior (INV-003,
// backward-compatible).

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
