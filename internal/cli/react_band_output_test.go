package cli

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

func bandIntPtr(v int) *int           { return &v }
func bandFloatPtr(v float64) *float64 { return &v }
func bandFloatsPtr(v ...float64) []*float64 {
	out := make([]*float64, len(v))
	for i := range v {
		out[i] = bandFloatPtr(v[i])
	}
	return out
}

// bandR2Result is the R2 evaluation of ci.failure_rate:Security Scan.
func bandR2Result() bandSeriesResult {
	values := bandFloatsPtr(0, 0.107143, 0.280306, 0.280306, -0.382235)
	return bandSeriesResult{
		Evaluation: healthband.Evaluation{
			Series: "ci.failure_rate:Security Scan", SampleKey: "88", N: bandIntPtr(21), X: values[0], Mu: values[1],
			SD: values[2], SDEff: values[3], Z: values[4], Tier: bandIntPtr(0), Reasons: []string{"below_baseline"},
		},
		Events: 1, Action: "log",
	}
}

// REQ-20: a row prints n/N_min and the six-decimal values, and every absent
// value as "-"; a dry run names the planned action.
func TestReactBandOutput_RowsPrintValuesOrDashes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "ci.failure_rate:Security Scan n=21/20 x=0.000000 μ=0.107143 sd_eff=0.280306 z=-0.382235 tier=0 action=log episode=-",
		bandR2Result().row())
	noBlock := bandSeriesResult{Evaluation: healthband.Evaluation{Series: "canary.failure_rate:local"}, Events: 1, PlannedAction: "log"}
	assert.Equal(t, "canary.failure_rate:local n=-/20 x=- μ=- sd_eff=- z=- tier=- planned_action=log episode=-", noBlock.row())
}

// Check fields hold each value as the JSON text data.series holds, absent
// values as absent keys, and reason lists comma-joined.
func TestReactBandOutput_FieldsKeepJSONValues(t *testing.T) {
	t.Parallel()
	result := bandR2Result()
	result.Reasons = []string{"below_baseline", "identifier_sanitized"}
	result.ResultPending = true

	fields := result.fields()

	for key, want := range map[string]string{
		"series": "ci.failure_rate:Security Scan", "sample_key": "88", "n": "21", "x": "0", "mu": "0.107143", "z": "-0.382235",
		"tier": "0", "events": "1", "action": "log", "reasons": "below_baseline,identifier_sanitized", "result_pending": "true",
	} {
		assert.Equal(t, want, fields[key], key)
	}
	for _, key := range []string{"planned_action", "episode_id", "bs_id", "claim_status", "constants"} {
		assert.NotContains(t, fields, key)
	}
	result.Z = bandFloatPtr(math.NaN())
	assert.Nil(t, result.fields(), "a value JSON cannot hold yields no fields rather than a wrong one")
}

// The text report states the source, skipped store lines, interrupted
// claims, the run-level reasons the source line does not already name, and
// each claim's result.
func TestReactBandOutput_TextReportsRunState(t *testing.T) {
	t.Parallel()
	claimed := bandR2Result()
	claimed.ClaimKey, claimed.ClaimStatus, claimed.BSID, claimed.DiagnosisStatus, claimed.ResultPending = "88", "done", "BS-BAND-002", "ok", true
	failed := bandSeriesResult{Evaluation: healthband.Evaluation{Series: "ci.failure_rate:Z"}, Events: 2, Action: "log",
		ClaimKey: "7", ClaimStatus: "failed:bs_lock_timeout", DiagnosisStatus: "skipped(no_agent)"}
	report := bandReport{
		CI: bandCIReport{Fetched: true, Reason: "gh_unauthenticated"}, Reasons: []string{"gh_unauthenticated", "series_not_found"},
		NotFound: []string{"nope", "x#0123abcd"}, Read: healthband.ReadCounts{Malformed: 1, UnknownSchema: 2, InvalidValue: 3},
		SkippedEvents: 4, Interrupted: 2, Series: []bandSeriesResult{claimed, failed},
	}
	var out bytes.Buffer

	printBandText(&out, report)

	assert.Equal(t, "ci: skipped gh_unauthenticated\n"+
		"store: skipped malformed=1 unknown_schema=2 invalid_value=3\n"+
		"events: skipped 4\n"+
		"claims: interrupted 2\n"+
		"reason: series_not_found nope x#0123abcd\n"+
		claimed.row()+"\n"+
		"  reasons: below_baseline\n"+
		"  claim: diagnose 88 done bs=BS-BAND-002 diagnosis=ok result=pending\n"+
		failed.row()+"\n"+
		"  claim: diagnose 7 failed:bs_lock_timeout diagnosis=skipped(no_agent)\n", out.String())

	out.Reset()
	printBandText(&out, bandReport{CI: bandCIReport{Fetched: true, Rows: 200, Trusted: 88, Appended: 3}})
	assert.Equal(t, "ci: 200 runs, 88 trusted, 3 new\n", out.String())
}

// Claims map to their series in order: a claim that never ran (the run
// ended first) stays claimed, a pending result is marked, and a phase C
// reason joins the row reasons.
func TestReactBandOutput_AddClaimsMapsOutcomesToSeries(t *testing.T) {
	t.Parallel()
	report := bandReport{Series: []bandSeriesResult{
		{Evaluation: healthband.Evaluation{Series: "ci.failure_rate:A"}},
		{Evaluation: healthband.Evaluation{Series: "ci.failure_rate:B"}},
	}}
	claims := []healthband.DueClaim{
		{Claim: healthband.Claim{ID: "a1"}, Series: "ci.failure_rate:A", SampleKey: "5"},
		{Claim: healthband.Claim{ID: "b1"}, Series: "ci.failure_rate:B", SampleKey: "9"},
		{Claim: healthband.Claim{ID: "c1"}, Series: "ci.failure_rate:C", SampleKey: "1"},
	}
	outcomes := map[string]healthband.ClaimOutcome{"a1": {DiagnosisStatus: "ok", BSID: "BS-BAND-004", BSStatus: "written"}}
	recorded := []healthband.Recorded{{ClaimID: "a1", Pending: "pending/a1.json", Reason: "late_result"}}

	report.addClaims(claims, outcomes, recorded)

	a, b := report.Series[0], report.Series[1]
	assert.Equal(t, []string{"5", "done", "ok", "BS-BAND-004"}, []string{a.ClaimKey, a.ClaimStatus, a.DiagnosisStatus, a.BSID})
	assert.True(t, a.ResultPending)
	assert.Equal(t, []string{"late_result"}, a.Reasons)
	assert.Equal(t, []string{"9", "claimed", ""}, []string{b.ClaimKey, b.ClaimStatus, b.BSID})
	assert.False(t, b.ResultPending)

	dry := bandReport{DryRun: true, Series: []bandSeriesResult{{Evaluation: healthband.Evaluation{Series: "ci.failure_rate:A"}}}}
	dry.addClaims(claims[:1], nil, nil)
	assert.Equal(t, "planned", dry.Series[0].ClaimStatus)
}

// Check and envelope statuses: an evaluated series inside an open episode
// warns, an idle series is skipped, and any run-level reason warns.
func TestReactBandOutput_Statuses(t *testing.T) {
	t.Parallel()
	open := bandSeriesResult{Events: 1, open: true}
	idle := bandSeriesResult{open: true}
	quiet := bandSeriesResult{Events: 3}
	assert.Equal(t, []string{"warn", "skip", "pass"}, []string{open.status(), idle.status(), quiet.status()})

	assert.Equal(t, jsonStatusOK, bandReport{Series: []bandSeriesResult{idle, quiet}}.status())
	assert.Equal(t, jsonStatusWarn, bandReport{Series: []bandSeriesResult{quiet, open}}.status())
	assert.Equal(t, jsonStatusWarn, bandReport{Reasons: []string{"store_locked"}}.status())
}
