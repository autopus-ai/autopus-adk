package telemetry

import (
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

func teamID(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return false
		}
	}
	return true
}
func validateTeamUsage(e TeamUsageEvidence) error {
	if e.Version != 1 || !teamID(e.TeamRunID) || len(e.ExpectedAgents) == 0 || len(e.ExpectedAgents) > 1000 || len(e.Observations) > 1000 {
		return fmt.Errorf("invalid team evidence schema or size")
	}
	roster := map[string]TeamAgent{}
	for _, a := range e.ExpectedAgents {
		if !teamID(a.AgentID) || (a.ParentAgentID != "" && !teamID(a.ParentAgentID)) || (a.Role != "supervisor" && a.Role != "worker") {
			return fmt.Errorf("invalid agent identity")
		}
		if _, ok := roster[a.AgentID]; ok {
			return fmt.Errorf("duplicate expected agent")
		}
		roster[a.AgentID] = a
	}
	for _, a := range e.ExpectedAgents {
		seen := map[string]bool{}
		current := a.AgentID
		for current != "" {
			if seen[current] {
				return fmt.Errorf("agent ancestry cycle")
			}
			seen[current] = true
			ancestor, ok := roster[current]
			if !ok {
				return fmt.Errorf("unknown parent agent")
			}
			current = ancestor.ParentAgentID
		}
	}
	observed := map[string]bool{}
	count := 0
	for _, o := range e.Observations {
		if _, ok := roster[o.AgentID]; !ok {
			return fmt.Errorf("unexpected observed agent")
		}
		if observed[o.AgentID] {
			return fmt.Errorf("duplicate observed agent")
		}
		observed[o.AgentID] = true
		if o.UsageScope != "self_calls" && o.UsageScope != "inclusive_rollup" && o.UsageScope != "unknown" {
			return fmt.Errorf("invalid usage scope")
		}
		count += len(o.Usage)
		if count > 10000 {
			return fmt.Errorf("too many usage receipts")
		}
		for _, u := range o.Usage {
			if !teamID(u.RunID) || !teamID(u.CallID) {
				return fmt.Errorf("invalid call identity")
			}
			if err := ValidateUsageEnvelope(u); err != nil {
				return fmt.Errorf("invalid usage envelope")
			}
			if (u.UsageStatus == UsageStatusActual || u.ActualCostUSD != nil) && u.UsageSource != UsageSourceProvider {
				return fmt.Errorf("actual usage requires provider source")
			}
			for _, n := range []*int64{u.RawTotalTokens, u.InputTokensTotal, u.OutputTokensTotal, u.EstimatedTotalTokens} {
				if n != nil && (*n < 0 || *n > 1e12) {
					return fmt.Errorf("usage exceeds numeric bounds")
				}
			}
			for _, n := range []*float64{u.ActualCostUSD, u.EstimatedCostUSD} {
				if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0) || *n < 0 || *n > 1e9) {
					return fmt.Errorf("cost exceeds numeric bounds")
				}
			}
		}
	}
	return nil
}
