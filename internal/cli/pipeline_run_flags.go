package cli

import (
	"fmt"
	"os/exec"
)

// strategyValue is a pflag.Value implementation that validates strategy on Set.
type strategyValue struct {
	val *string
}

// newStrategyValue creates a strategyValue with the given default.
func newStrategyValue(defaultVal string, p *string) *strategyValue {
	*p = defaultVal
	return &strategyValue{val: p}
}

// String returns the current value.
func (s *strategyValue) String() string { return *s.val }

// Type returns the flag type name.
func (s *strategyValue) Type() string { return "strategy" }

// Set validates and stores the strategy value.
func (s *strategyValue) Set(v string) error {
	if v != "sequential" {
		return fmt.Errorf("invalid strategy %q: must be sequential", v)
	}
	*s.val = v
	return nil
}

// @AX:NOTE [AUTO]: magic constants — platform probe order ["claude", "codex", "agy"] and fallback "claude" are implicit policy
// resolvePlatform returns the platform to use: the value as-is when non-empty,
// or the first AI binary found in PATH (claude, codex, agy).
func resolvePlatform(platform string) string {
	if platform != "" {
		return platform
	}
	for _, candidate := range []struct {
		binary   string
		provider string
	}{
		{binary: "claude", provider: "claude"},
		{binary: "codex", provider: "codex"},
		{binary: "agy", provider: "gemini"},
	} {
		if _, err := exec.LookPath(candidate.binary); err == nil {
			return candidate.provider
		}
	}
	// Fall back to "claude" as the default when nothing is found in PATH.
	return "claude"
}
