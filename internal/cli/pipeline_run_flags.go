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
// pipelinePlatformOrder is the order `auto pipeline run` probes PATH in when
// --platform is omitted; agy runs the gemini provider.
var pipelinePlatformOrder = []struct {
	binary   string
	provider string
}{
	{binary: "claude", provider: "claude"},
	{binary: "codex", provider: "codex"},
	{binary: "agy", provider: "gemini"},
}

// Why `auto pipeline run` chose its platform (SPEC-STATUSEVIDENCE-001
// REQ-SEV-RUN-02); `auto status` reports the same reasons.
const (
	pipelinePlatformReasonFlag     = "--platform"
	pipelinePlatformReasonDetected = "first installed of claude → codex → agy"
	pipelinePlatformReasonFallback = "no AI CLI installed; falls back to claude"
)

// resolvePlatform returns the platform to use: the value as-is when non-empty,
// or the first AI binary found in PATH (claude, codex, agy).
func resolvePlatform(platform string) string {
	selected, _ := resolvePlatformWith(platform, exec.LookPath)
	return selected
}

// resolvePlatformWith resolves the platform with the given PATH lookup and
// says why it was chosen.
func resolvePlatformWith(platform string, lookPath func(string) (string, error)) (string, string) {
	if platform != "" {
		return platform, pipelinePlatformReasonFlag
	}
	for _, candidate := range pipelinePlatformOrder {
		if _, err := lookPath(candidate.binary); err == nil {
			return candidate.provider, pipelinePlatformReasonDetected
		}
	}
	// Fall back to "claude" as the default when nothing is found in PATH.
	return "claude", pipelinePlatformReasonFallback
}

// pipelinePlatformLabel names a platform with the binary it runs when the two
// differ.
func pipelinePlatformLabel(platform string) string {
	if platform == "gemini" {
		return "gemini (agy)"
	}
	return platform
}
