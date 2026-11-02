package config

import (
	"fmt"
	"math"
)

// ReviewBudgetConf bounds one logical SPEC review session: every reviewer
// call, judge call, JSON repair, lean fallback, revision round, and later
// `auto spec review` invocation of the same SPEC spend from it
// (SPEC-REVIEWPOLICY-001 REQ-RP-BUD-01). A zero or omitted value means no
// limit for that dimension, so an autopus.yaml without the section keeps the
// unlimited behaviour and writes no key. Calls and minutes are hard limits;
// tokens and cost are enforced between calls against the usage providers
// report, because no provider can be stopped mid-call by its token count.
type ReviewBudgetConf struct {
	MaxCalls   int     `yaml:"max_calls,omitempty"`
	MaxMinutes int     `yaml:"max_minutes,omitempty"`
	MaxTokens  int64   `yaml:"max_tokens,omitempty"`
	MaxCostUSD float64 `yaml:"max_cost_usd,omitempty"`
}

// Validate refuses a negative limit and a cost that is not a finite number.
// Every error names the dotted key; an unknown nested key never reaches it,
// because the strict decoder rejects it first.
func (b ReviewBudgetConf) Validate() error {
	const prefix = "spec.review_gate.budget."
	for _, bound := range []struct {
		key   string
		value int64
	}{
		{"max_calls", int64(b.MaxCalls)},
		{"max_minutes", int64(b.MaxMinutes)},
		{"max_tokens", b.MaxTokens},
	} {
		if bound.value < 0 {
			return fmt.Errorf("%s%s must be >= 0, got %d", prefix, bound.key, bound.value)
		}
	}
	if math.IsNaN(b.MaxCostUSD) || math.IsInf(b.MaxCostUSD, 0) {
		return fmt.Errorf("%smax_cost_usd must be finite, got %v", prefix, b.MaxCostUSD)
	}
	if b.MaxCostUSD < 0 {
		return fmt.Errorf("%smax_cost_usd must be >= 0, got %v", prefix, b.MaxCostUSD)
	}
	return nil
}
