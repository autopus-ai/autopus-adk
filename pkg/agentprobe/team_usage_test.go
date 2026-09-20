package agentprobe

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

func nativeUsageFixture(t *testing.T) (Evidence, OpenCodeTransport) {
	t.Helper()
	e := fixture()
	e.Platform = "opencode"
	e.RuntimeVersion = "1.18.7"
	var tokens OpenCodeTokens
	if err := json.Unmarshal([]byte(`{"input":10,"output":5,"reasoning":2,"cache":{"read":3,"write":4},"total":24}`), &tokens); err != nil {
		t.Fatal(err)
	}
	cost := 0.125
	tr := OpenCodeTransport{Protocol: "opencode-http", Provenance: "live_endpoint_capture", ObservedRuntimeVersion: e.RuntimeVersion, SupervisorID: e.SupervisorID, SupervisorEmptyObserved: true, ChildIDs: []string{"one", "two"}, Messages: []OpenCodeMessageUsage{{SessionID: "one", MessageID: "message", ProviderID: "p", ModelID: "m", Completed: true, Tokens: &tokens, Cost: &cost}}}
	return e, tr
}
func TestOpenCodeTeamUsageInclusiveArithmeticAndPartialCapture(t *testing.T) {
	e, tr := nativeUsageFixture(t)
	got, err := OpenCodeTeamUsage(e, tr)
	if err != nil {
		t.Fatal(err)
	}
	report, err := telemetry.SummarizeTeamUsage(got)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.ActualTokens != nil || report.KnownActualTokens != 24 || report.ActualCostUSD != nil {
		t.Fatalf("%+v", report)
	}
	for _, o := range got.Observations {
		if o.AgentID == "one" {
			u := o.Usage[0]
			if *u.InputTokensTotal != 17 || *u.OutputTokensTotal != 7 || u.ActualCostUSD != nil || u.EstimatedCostUSD == nil || *u.EstimatedCostUSD != 0.125 {
				t.Fatalf("%+v", u)
			}
		}
	}
}
func TestOpenCodeTeamUsageUnknownsStayUnknown(t *testing.T) {
	for _, mode := range []string{"version", "absent", "zero"} {
		e, tr := nativeUsageFixture(t)
		switch mode {
		case "aborted":
			tr.Messages[0].ErrorName = "MessageAbortedError"
		case "version":
			e.RuntimeVersion = "future"
			tr.ObservedRuntimeVersion = "future"
		case "absent":
			tr.Messages[0].Tokens = &OpenCodeTokens{}
		case "zero":
			var zero OpenCodeTokens
			_ = json.Unmarshal([]byte(`{"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0},"total":0}`), &zero)
			tr.Messages[0].Tokens = &zero
		}
		got, err := OpenCodeTeamUsage(e, tr)
		if err != nil {
			t.Fatal(err)
		}
		r, err := telemetry.SummarizeTeamUsage(got)
		if err != nil {
			t.Fatal(err)
		}
		if r.KnownActualTokens != 0 || r.ActualTokens != nil {
			t.Fatalf("%s %+v", mode, r)
		}
	}
}
func TestOpenCodeTeamUsageRejectsForeignAndInconsistent(t *testing.T) {
	for _, mode := range []string{"foreign", "total", "negative", "root_conflict"} {
		e, tr := nativeUsageFixture(t)
		switch mode {
		case "foreign":
			tr.Messages[0].SessionID = "foreign"
		case "total":
			n := int64(25)
			tr.Messages[0].Tokens.Total = &n
		case "negative":
			n := int64(-1)
			tr.Messages[0].Tokens.Total = &n
		case "root_conflict":
			tr.Messages[0].SessionID = e.SupervisorID
		}
		if _, err := OpenCodeTeamUsage(e, tr); err == nil {
			t.Fatalf("accepted %s", mode)
		}
	}
}

func TestOpenCodeTeamUsagePreservesPositiveAbortedSpend(t *testing.T) {
	e, tr := nativeUsageFixture(t)
	tr.Messages[0].ErrorName = "MessageAbortedError"
	got, err := OpenCodeTeamUsage(e, tr)
	if err != nil {
		t.Fatal(err)
	}
	r, err := telemetry.SummarizeTeamUsage(got)
	if err != nil {
		t.Fatal(err)
	}
	if r.KnownActualTokens != 24 || r.Complete {
		t.Fatalf("%+v", r)
	}
}

func TestOpenCodeTeamUsageMissingFieldNeverZeroFilled(t *testing.T) {
	e, tr := nativeUsageFixture(t)
	tr.Messages[0].Tokens.Cache.Write = nil
	got, err := OpenCodeTeamUsage(e, tr)
	if err != nil {
		t.Fatal(err)
	}
	r, err := telemetry.SummarizeTeamUsage(got)
	if err != nil {
		t.Fatal(err)
	}
	if r.KnownActualTokens != 0 {
		t.Fatal("missing cache write zero-filled")
	}
}
func TestOpenCodeTeamUsageRejectsOverflowAndNonfiniteCost(t *testing.T) {
	for _, mode := range []string{"overflow", "negative", "nan"} {
		e, tr := nativeUsageFixture(t)
		switch mode {
		case "overflow":
			n := int64(math.MaxInt64)
			tr.Messages[0].Tokens.Input = &n
		case "negative":
			n := int64(-1)
			tr.Messages[0].Tokens.Reasoning = &n
		case "nan":
			n := math.NaN()
			tr.Messages[0].Cost = &n
		}
		if _, err := OpenCodeTeamUsage(e, tr); err == nil {
			t.Fatalf("accepted %s", mode)
		}
	}
}
