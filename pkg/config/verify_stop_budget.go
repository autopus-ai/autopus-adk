package config

import (
	"fmt"
	"time"
)

// The verification Stop hook's budget: the default, and the range
// hooks.verify_stop_budget accepts. Below the minimum a Go package's test
// binary rarely even builds; above the maximum a single stop holds the
// session for longer than a person waits for a turn to end.
const (
	DefaultVerifyStopBudget = 8 * time.Minute
	MinVerifyStopBudget     = time.Minute
	MaxVerifyStopBudget     = 30 * time.Minute
	// VerifyStopHookSlack is what the hook's timeout adds to the budget: the
	// secret and version checks before the run, killing a step's process
	// group, and writing the verdict. Without it Claude Code kills the hook
	// at the budget's end and no verdict is returned at all.
	VerifyStopHookSlack = 90 * time.Second
	// DefaultVerifyStopHookTimeout is the hook timeout generation has always
	// written for the default budget.
	DefaultVerifyStopHookTimeout = 600 * time.Second
)

// VerifyStopBudgetDuration returns hooks.verify_stop_budget, or
// DefaultVerifyStopBudget when unset. A value that does not parse or falls
// outside MinVerifyStopBudget..MaxVerifyStopBudget is an error.
func (h HooksConf) VerifyStopBudgetDuration() (time.Duration, error) {
	if h.VerifyStopBudget == "" {
		return DefaultVerifyStopBudget, nil
	}
	d, err := time.ParseDuration(h.VerifyStopBudget)
	if err != nil {
		return 0, fmt.Errorf("hooks.verify_stop_budget %q is not a duration such as \"15m\": %w", h.VerifyStopBudget, err)
	}
	if d < MinVerifyStopBudget || d > MaxVerifyStopBudget {
		return 0, fmt.Errorf("hooks.verify_stop_budget %s must be between %s and %s", d, MinVerifyStopBudget, MaxVerifyStopBudget)
	}
	return d, nil
}

// VerifyStopHookTimeout is the Stop hook timeout for a budget: the budget
// plus VerifyStopHookSlack, never below DefaultVerifyStopHookTimeout.
func VerifyStopHookTimeout(budget time.Duration) time.Duration {
	return max(budget+VerifyStopHookSlack, DefaultVerifyStopHookTimeout)
}
