package agentprobe

import (
	"fmt"
	"math"
	"sort"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// OpenCodeTeamUsage preserves observed per-message spend without claiming that
// native message enumeration captured every provider call or aborted request.
func OpenCodeTeamUsage(e Evidence, transport OpenCodeTransport) (telemetry.TeamUsageEvidence, error) {
	result := telemetry.TeamUsageEvidence{Version: 1, TeamRunID: e.RunID}
	if _, err := Evaluate(e); err != nil {
		return result, fmt.Errorf("invalid lifecycle evidence")
	}
	if e.Platform != "opencode" || transport.Protocol != "opencode-http" || transport.SupervisorID != e.SupervisorID || transport.ObservedRuntimeVersion != e.RuntimeVersion || len(transport.Messages) > MaxEvents {
		return result, fmt.Errorf("native usage identity mismatch")
	}
	children := map[string]bool{}
	for _, event := range e.Events {
		if event.Kind == "spawn" {
			children[event.ChildID] = true
		}
	}
	seen := map[string]bool{}
	for _, id := range transport.ChildIDs {
		if !children[id] || seen[id] {
			return result, fmt.Errorf("native child binding mismatch")
		}
		seen[id] = true
	}
	if len(seen) != len(children) {
		return result, fmt.Errorf("native child roster incomplete")
	}
	ids := []string{e.SupervisorID}
	for id := range children {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	observations := map[string]*telemetry.TeamUsageObservation{}
	for _, id := range ids {
		agent := telemetry.TeamAgent{AgentID: id, Role: "worker", ParentAgentID: e.SupervisorID}
		o := &telemetry.TeamUsageObservation{AgentID: id, UsageScope: "self_calls", Usage: []telemetry.UsageEnvelope{}}
		if id == e.SupervisorID {
			agent.Role = "supervisor"
			agent.ParentAgentID = ""
			o.CaptureComplete = transport.SupervisorEmptyObserved
		}
		result.ExpectedAgents = append(result.ExpectedAgents, agent)
		observations[id] = o
	}
	for _, message := range transport.Messages {
		o, ok := observations[message.SessionID]
		if !ok || !token(message.MessageID) {
			return result, fmt.Errorf("native message ownership unverified")
		}
		if message.SessionID == e.SupervisorID && transport.SupervisorEmptyObserved {
			return result, fmt.Errorf("native supervisor empty observation conflicts with message")
		}
		usage, err := normalizeOpenCodeMessage(e.RuntimeVersion, message)
		if err != nil {
			return result, err
		}
		o.Usage = append(o.Usage, usage)
	}
	for _, id := range ids {
		result.Observations = append(result.Observations, *observations[id])
	}
	if _, err := telemetry.SummarizeTeamUsage(result); err != nil {
		return result, fmt.Errorf("invalid normalized native usage")
	}
	return result, nil
}
func normalizeOpenCodeMessage(version string, m OpenCodeMessageUsage) (telemetry.UsageEnvelope, error) {
	input := telemetry.UsageInput{RunID: m.SessionID, CallID: m.MessageID, Provider: m.ProviderID, Model: m.ModelID, Source: telemetry.UsageSourceProvider, SourceSchema: "opencode/" + version + "/assistant-message"}
	unknown := func() telemetry.UsageEnvelope { return telemetry.NormalizeUsage(input) }
	if m.Cost != nil && (math.IsNaN(*m.Cost) || math.IsInf(*m.Cost, 0) || *m.Cost < 0 || *m.Cost > 1e9) {
		return telemetry.UsageEnvelope{}, fmt.Errorf("invalid native estimated cost")
	}
	if version != "1.18.7" || !m.Completed || !token(m.ProviderID) || !token(m.ModelID) {
		return unknown(), nil
	}
	t := m.Tokens
	if t == nil {
		return unknown(), nil
	}
	components := []*int64{t.Input, t.Output, t.Reasoning, t.Cache.Read, t.Cache.Write}
	for _, v := range append(append([]*int64{}, components...), t.Total) {
		if v != nil && (*v < 0 || *v > 1e12) {
			return telemetry.UsageEnvelope{}, fmt.Errorf("invalid native token component")
		}
	}
	for _, v := range components {
		if v == nil {
			return unknown(), nil
		}
	}
	in := *t.Input + *t.Cache.Read + *t.Cache.Write
	out := *t.Output + *t.Reasoning
	total := in + out
	if t.Total != nil && *t.Total != total {
		return telemetry.UsageEnvelope{}, fmt.Errorf("inconsistent native token total")
	}
	// Failed/aborted messages count only explicit, reconciled positive usage;
	// native zero-initialized accounting is not proof of a free aborted call.
	if total == 0 || (m.ErrorName != "" && t.Total == nil) {
		return unknown(), nil
	}
	input.InputTokensTotal = &in
	input.UncachedInputTokens = t.Input
	input.CacheReadInputTokens = t.Cache.Read
	input.CacheCreationInputTokens = t.Cache.Write
	input.OutputTokensTotal = &out
	input.ReasoningTokens = t.Reasoning
	input.ReasoningRelation = telemetry.ComponentSubsetOfOutput
	input.EstimatedCostUSD = m.Cost
	usage := telemetry.NormalizeUsage(input)
	if err := telemetry.ValidateUsageEnvelope(usage); err != nil {
		return telemetry.UsageEnvelope{}, fmt.Errorf("native usage normalization failed")
	}
	return usage, nil
}
