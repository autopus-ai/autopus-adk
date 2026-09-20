package telemetry

import "testing"

func teamFixture() TeamUsageEvidence {
	in, out := int64(10), int64(2)
	u := NormalizeUsage(UsageInput{RunID: "worker-run", CallID: "call", Source: UsageSourceProvider, InputTokensTotal: &in, OutputTokensTotal: &out})
	return TeamUsageEvidence{Version: 1, TeamRunID: "team", ExpectedAgents: []TeamAgent{{AgentID: "lead", Role: "supervisor"}, {AgentID: "worker", ParentAgentID: "lead", Role: "worker"}}, Observations: []TeamUsageObservation{{AgentID: "lead", UsageScope: "self_calls", CaptureComplete: true}, {AgentID: "worker", UsageScope: "self_calls", CaptureComplete: true, Usage: []UsageEnvelope{u}}}}
}
func TestTeamUsageExplicitZeroAndDedup(t *testing.T) {
	e := teamFixture()
	e.Observations[1].Usage = append(e.Observations[1].Usage, e.Observations[1].Usage[0])
	r, err := SummarizeTeamUsage(e)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete || r.ActualTokens == nil || *r.ActualTokens != 12 || r.UniqueModelCallCount != 1 || r.ActualCostUSD != nil {
		t.Fatalf("%+v", r)
	}
}
func TestTeamUsageRollupAndMissingRemainUnknown(t *testing.T) {
	for _, scope := range []string{"inclusive_rollup", "unknown", "missing"} {
		e := teamFixture()
		e.Observations[0].Usage = append([]UsageEnvelope(nil), e.Observations[1].Usage...)
		e.Observations[0].UsageScope = scope
		if scope == "missing" {
			e.Observations = e.Observations[1:]
		}
		r, err := SummarizeTeamUsage(e)
		if err != nil {
			t.Fatal(err)
		}
		if r.Complete || r.ActualTokens != nil || r.KnownActualTokens != 12 {
			t.Fatalf("%s: %+v", scope, r)
		}
	}
}
func TestTeamUsageConflictFailsClosed(t *testing.T) {
	e := teamFixture()
	e.Observations[0].Usage = e.Observations[1].Usage
	r, err := SummarizeTeamUsage(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || len(r.AttributionConflicts) == 0 || r.ActualTokens != nil || r.KnownActualTokens != 0 {
		t.Fatalf("%+v", r)
	}
}
func TestTeamUsagePartialCostAndRetry(t *testing.T) {
	e := teamFixture()
	a := e.Observations[1].Usage[0]
	c := 0.25
	a.ActualCostUSD = &c
	b := a
	b.CallID = "retry"
	b.ActualCostUSD = nil
	e.Observations[1].Usage = []UsageEnvelope{a, b}
	r, err := SummarizeTeamUsage(e)
	if err != nil {
		t.Fatal(err)
	}
	if *r.ActualTokens != 24 || r.ActualCostUSD != nil || r.KnownActualCostUSD != 0.25 || r.CostComplete {
		t.Fatalf("%+v", r)
	}
}
func TestTeamUsageInvalidRoster(t *testing.T) {
	for _, mutate := range []func(*TeamUsageEvidence){func(e *TeamUsageEvidence) { e.ExpectedAgents[0].ParentAgentID = "worker" }, func(e *TeamUsageEvidence) { e.ExpectedAgents[1].ParentAgentID = "missing" }, func(e *TeamUsageEvidence) { e.Observations = append(e.Observations, e.Observations[0]) }} {
		e := teamFixture()
		mutate(&e)
		if _, err := SummarizeTeamUsage(e); err == nil {
			t.Fatal("accepted invalid roster")
		}
	}
}

func TestTeamUsageEstimatedAndIncompleteCapture(t *testing.T) {
	e := teamFixture()
	n := int64(30)
	e.Observations[1].Usage = []UsageEnvelope{NormalizeUsage(UsageInput{RunID: "r", CallID: "c", Source: UsageSourceEstimate, EstimatedTotalTokens: &n})}
	r, err := SummarizeTeamUsage(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || r.ActualTokens != nil || r.EstimatedTokens == nil || *r.EstimatedTokens != 30 || r.ActualCostUSD != nil {
		t.Fatalf("estimate promoted to actual: %+v", r)
	}
	e = teamFixture()
	e.Observations[1].CaptureComplete = false
	r, err = SummarizeTeamUsage(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || r.ActualTokens != nil || r.KnownActualTokens != 12 {
		t.Fatal("incomplete capture reported as complete")
	}
}
func TestTeamUsageConflictingDuplicateExcluded(t *testing.T) {
	e := teamFixture()
	u := e.Observations[1].Usage[0]
	in, out := int64(11), int64(2)
	e.Observations[1].Usage = append(e.Observations[1].Usage, NormalizeUsage(UsageInput{RunID: u.RunID, CallID: u.CallID, Source: UsageSourceProvider, InputTokensTotal: &in, OutputTokensTotal: &out}))
	r, err := SummarizeTeamUsage(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || len(r.AttributionConflicts) != 1 || r.ActualTokens != nil || r.KnownActualTokens != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestTeamUsageRejectsNonProviderActualCost(t *testing.T) {
	e := teamFixture()
	cost := 0.1
	e.Observations[1].Usage = []UsageEnvelope{{Version: 1, RunID: "r", CallID: "c", UsageStatus: UsageStatusCostOnly, UsageSource: UsageSourceEstimate, ActualCostUSD: &cost}}
	if _, err := SummarizeTeamUsage(e); err == nil {
		t.Fatal("estimate source accepted as actual cost")
	}
}

func TestTeamUsagePreservesKnownEstimatedCostWithoutPromotingBilling(t *testing.T) {
	e := teamFixture()
	cost := 0.125
	e.Observations[1].Usage[0].EstimatedCostUSD = &cost
	e.Observations[1].Usage = append(e.Observations[1].Usage, e.Observations[1].Usage[0])
	e.Observations[1].CaptureComplete = false
	r, err := SummarizeTeamUsage(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.KnownEstimatedCostUSD != 0.125 || r.ActualCostUSD != nil || r.KnownActualCostUSD != 0 || r.CostComplete || r.Complete {
		t.Fatalf("%+v", r)
	}
}
