package telemetry

import "sort"

// SummarizeTeamUsage aggregates only disjoint self-call receipts. Rollups never
// enter arithmetic, and their presence never proves unobserved parent usage.
func SummarizeTeamUsage(e TeamUsageEvidence) (TeamUsageReport, error) {
	r := TeamUsageReport{Version: 1, TeamRunID: e.TeamRunID, ExpectedAgents: len(e.ExpectedAgents), ObservedAgents: len(e.Observations), MissingAgents: []string{}, ExcludedRollups: []string{}, AttributionConflicts: []string{}, ByAgent: []TeamAgentUsage{}}
	if err := validateTeamUsage(e); err != nil {
		return r, err
	}
	observations := map[string]TeamUsageObservation{}
	owners := map[callIdentity]string{}
	calls := map[callIdentity]UsageEnvelope{}
	conflicts := map[callIdentity]bool{}
	badAgents := map[string]bool{}
	for _, o := range e.Observations {
		observations[o.AgentID] = o
		if o.UsageScope == "inclusive_rollup" {
			r.ExcludedRollups = append(r.ExcludedRollups, o.AgentID)
		}
		if o.UsageScope != "self_calls" {
			continue
		}
		for _, u := range o.Usage {
			key := callIdentity{u.RunID, u.CallID}
			if prior, ok := calls[key]; ok {
				if owners[key] != o.AgentID || AggregateUsage([]UsageEnvelope{prior, u}).PromotionBlocked {
					conflicts[key] = true
					badAgents[owners[key]] = true
					badAgents[o.AgentID] = true
				}
			} else {
				calls[key] = u
				owners[key] = o.AgentID
			}
		}
	}
	for agent := range badAgents {
		r.AttributionConflicts = append(r.AttributionConflicts, agent)
	}
	sort.Strings(r.AttributionConflicts)
	sort.Strings(r.ExcludedRollups)
	agents := append([]TeamAgent(nil), e.ExpectedAgents...)
	sort.Slice(agents, func(i, j int) bool { return agents[i].AgentID < agents[j].AgentID })
	all := []UsageEnvelope{}
	completeCapture := true
	for _, agent := range agents {
		o, ok := observations[agent.AgentID]
		a := TeamAgentUsage{AgentID: agent.AgentID, ParentAgentID: agent.ParentAgentID, Role: agent.Role, Observed: ok, UsageScope: o.UsageScope, CaptureComplete: o.CaptureComplete}
		if !ok {
			r.MissingAgents = append(r.MissingAgents, agent.AgentID)
		}
		capture := ok && o.UsageScope == "self_calls" && o.CaptureComplete && !badAgents[agent.AgentID]
		values := []UsageEnvelope{}
		for key, u := range calls {
			if owners[key] == agent.AgentID && !conflicts[key] {
				values = append(values, u)
			}
		}
		sortTeamCalls(values)
		a.TeamUsageMetrics = teamMetrics(values, capture)
		r.ByAgent = append(r.ByAgent, a)
		all = append(all, values...)
		completeCapture = completeCapture && capture
	}
	sortTeamCalls(all)
	r.TeamUsageMetrics = teamMetrics(all, completeCapture)
	return r, nil
}
func sortTeamCalls(values []UsageEnvelope) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].RunID != values[j].RunID {
			return values[i].RunID < values[j].RunID
		}
		return values[i].CallID < values[j].CallID
	})
}
func teamMetrics(values []UsageEnvelope, capture bool) TeamUsageMetrics {
	aggregate := AggregateUsage(values)
	m := TeamUsageMetrics{UniqueModelCallCount: aggregate.UniqueModelCallCount, EstimatedTokens: aggregate.EstimatedTotalTokens, Complete: capture, CostComplete: capture}
	for _, u := range values {
		if u.EstimatedCostUSD != nil {
			m.KnownEstimatedCostUSD += *u.EstimatedCostUSD
		}
		if u.UsageStatus == UsageStatusActual && u.RawTotalTokens != nil {
			m.KnownActualTokens += *u.RawTotalTokens
		} else {
			m.Complete = false
		}
		if u.ActualCostUSD != nil {
			m.KnownActualCostUSD += *u.ActualCostUSD
		} else {
			m.CostComplete = false
		}
	}
	if m.Complete {
		m.ActualTokens = int64Pointer(m.KnownActualTokens)
	}
	if m.CostComplete {
		m.ActualCostUSD = float64Pointer(m.KnownActualCostUSD)
	}
	return m
}
