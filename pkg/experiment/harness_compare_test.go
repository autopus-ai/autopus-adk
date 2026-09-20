package experiment

import (
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

func harnessFixture() HarnessEvidence {
	id := HarnessIdentity{Provider: "p", Model: "m", ModelVersion: "v", ProviderVersion: "v", TaskHash: "task", OracleHash: "oracle", EnvironmentHash: "env", BudgetHash: "budget", Revision: "rev", Effort: "high", CacheStratum: "cold"}
	e := HarnessEvidence{Version: 1, ExpectedTaskIDs: []string{"t"}}
	for _, arm := range []string{"native", "current", "reduced"} {
		in, out := int64(10), int64(2)
		usage := telemetry.NormalizeUsage(telemetry.UsageInput{RunID: arm, CallID: "c", TaskID: "t", Provider: "p", Model: "m", ProviderVersion: "v", ModelVersion: "v", Effort: "high", CacheStratum: "cold", Source: telemetry.UsageSourceProvider, SourceSchema: "test", InputTokensTotal: &in, OutputTokensTotal: &out})
		elapsed := int64(100)
		e.Observations = append(e.Observations, HarnessObservation{TaskID: "t", Arm: arm, HarnessRevision: "h", HarnessConfigHash: arm, Identity: id, Accepted: boolPtr(true), ElapsedMS: &elapsed, HumanCorrections: intPtr(0), Runs: []telemetry.AgentRun{{TaskID: "t", Status: telemetry.StatusPass, Usage: []telemetry.UsageEnvelope{usage}}}})
	}
	return e
}
func boolPtr(v bool) *bool { return &v }
func TestHarnessIncludesFailedTrialsAndRetries(t *testing.T) {
	e := harnessFixture()
	e.Observations[1].Accepted = boolPtr(false)
	retry := e.Observations[1].Runs[0]
	retry.Usage = append([]telemetry.UsageEnvelope(nil), retry.Usage...)
	retry.Usage[0].CallID = "retry"
	e.Observations[1].Runs = append(e.Observations[1].Runs, retry)
	r, err := CompareHarness(e)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete || r.Arms[1].Accepted != 0 || *r.Arms[1].ActualTokens != 24 || r.Pairs[0].TaskCount != 1 || *r.Pairs[0].TokenDelta != 12 {
		t.Fatalf("unexpected %+v %+v", r.Arms, r.Pairs)
	}
}
func TestHarnessUnknownAndMissing(t *testing.T) {
	e := harnessFixture()
	e.Observations = e.Observations[:2]
	e.Observations[1].Runs = nil
	e.Observations[1].Accepted = nil
	r, err := CompareHarness(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || r.Arms[1].ActualTokens != nil || r.Arms[2].ActualTokens != nil || r.Pairs[0].TokenDelta != nil {
		t.Fatalf("invented measurement: %+v", r)
	}
}
func TestHarnessRejectsDuplicatesAndForeignCalls(t *testing.T) {
	for _, mutate := range []func(*HarnessEvidence){
		func(e *HarnessEvidence) { e.Observations = append(e.Observations, e.Observations[0]) },
		func(e *HarnessEvidence) { e.ExpectedTaskIDs = append(e.ExpectedTaskIDs, "t") },
		func(e *HarnessEvidence) { e.Observations[0].Runs[0].Usage[0].Model = "foreign" },
		func(e *HarnessEvidence) {
			e.Observations[0].Runs[0].Usage = append(e.Observations[0].Runs[0].Usage, e.Observations[0].Runs[0].Usage[0])
		},
	} {
		e := harnessFixture()
		mutate(&e)
		if _, err := CompareHarness(e); err == nil {
			t.Fatal("accepted invalid evidence")
		}
	}
}
func TestHarnessIncompatibleIdentityHasNoDelta(t *testing.T) {
	e := harnessFixture()
	e.Observations[1].Identity.OracleHash = "different"
	r, err := CompareHarness(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Pairs[0].TaskCount != 0 || r.Pairs[0].TokenDelta != nil {
		t.Fatal("paired incompatible task")
	}
}

func TestHarnessMissingAttemptUsagePreventsCompleteSpend(t *testing.T) {
	e := harnessFixture()
	e.Observations[0].Runs = append(e.Observations[0].Runs, telemetry.AgentRun{TaskID: "t", Status: telemetry.StatusFail})
	r, err := CompareHarness(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || r.Arms[0].ActualTokens != nil {
		t.Fatal("missing retry spend reported as complete")
	}
}

func TestHarnessRejectsMismatchedCacheAndUnsafeIdentity(t *testing.T) {
	for _, mutate := range []func(*HarnessEvidence){
		func(e *HarnessEvidence) { e.Observations[0].Runs[0].Usage[0].CacheStratum = "warm" },
		func(e *HarnessEvidence) { e.Observations[0].HarnessRevision = "bad\x1b[31m" },
		func(e *HarnessEvidence) { e.Observations[0].Identity.OracleHash = strings.Repeat("x", 257) },
		func(e *HarnessEvidence) { e.ExpectedTaskIDs[0] = "bad\tidentity" },
	} {
		e := harnessFixture()
		mutate(&e)
		if _, err := CompareHarness(e); err == nil {
			t.Fatal("accepted mismatched or unsafe identity")
		}
	}
}
func TestHarnessKnownSpendSurvivesUnknownTaskAndRetry(t *testing.T) {
	e := harnessFixture()
	e.ExpectedTaskIDs = append(e.ExpectedTaskIDs, "missing")
	e.Observations[0].Runs = append(e.Observations[0].Runs, telemetry.AgentRun{TaskID: "t", Status: telemetry.StatusFail})
	r, err := CompareHarness(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Arms[0].KnownActualTokens != 12 || r.Arms[0].MeasuredTasks != 0 || r.Arms[0].ActualTokens != nil {
		t.Fatalf("lost partial spend %+v", r.Arms[0])
	}
	if r.Arms[1].KnownActualTokens != 12 || r.Arms[1].MeasuredTasks != 1 || r.Arms[1].ActualTokens != nil {
		t.Fatalf("lost complete task spend %+v", r.Arms[1])
	}
}
func TestHarnessInvalidUsageErrorsDoNotEchoInput(t *testing.T) {
	e := harnessFixture()
	e.Observations[0].Runs[0].Usage[0].UsageStatus = "PRIVATE-SENTINEL"
	_, err := CompareHarness(e)
	if err == nil || strings.Contains(err.Error(), "PRIVATE-SENTINEL") {
		t.Fatal("raw input exposed")
	}
}

func TestHarnessOverallCompletenessRequiresComparablePairsAndCorrections(t *testing.T) {
	for _, mutate := range []func(*HarnessEvidence){
		func(e *HarnessEvidence) { e.Observations[1].Identity.OracleHash = "other" },
		func(e *HarnessEvidence) { e.Observations[1].HumanCorrections = nil },
	} {
		e := harnessFixture()
		mutate(&e)
		r, err := CompareHarness(e)
		if err != nil {
			t.Fatal(err)
		}
		if r.Complete {
			t.Fatal("incomplete experiment labeled complete")
		}
	}
	e := harnessFixture()
	e.Observations[1].HumanCorrections = nil
	r, err := CompareHarness(e)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Pairs[0].Complete || r.Pairs[0].TokenDelta == nil {
		t.Fatal("optional corrections must not discard comparable token measurements")
	}
}
