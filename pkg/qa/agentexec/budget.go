package agentexec

import (
	"os"
	"strconv"
	"strings"
)

// DefaultBudgetUSD caps the spend of one headless claude run. One verify
// session in Pilot C (2026-11-07) spent $42 with no cap, and a QA loop may
// call the agent several times, so each call carries its own ceiling.
const DefaultBudgetUSD = "5"

// BudgetEnv overrides DefaultBudgetUSD with a positive dollar amount.
const BudgetEnv = "AUTOPUS_QA_AGENT_BUDGET_USD"

// BudgetUSD returns the per-run spending cap passed to claude as
// --max-budget-usd. Codex, agy, and opencode have no such flag; their runs are
// bounded by the request timeout only.
func BudgetUSD() string {
	raw := strings.TrimSpace(os.Getenv(BudgetEnv))
	if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
		return raw
	}
	return DefaultBudgetUSD
}
