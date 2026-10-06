package cli

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Value strings of acceptance S3 (R1, R2), oldest first.
const (
	s3CIValues   = "00101000000000000010000000000000000000001000000111010000000000000010000011100000"
	s3ScanValues = "0000000000000000000000000000000000000000000000000111111111000000000000000000000000000000"
)

// s3Payload rebuilds the probe A2 event mix of 200 runs, newest first like
// gh: CI push 83 (3 cancelled), Security Scan push 83 and schedule 5, 6 pull
// requests on a branch named main, 10 Release tag pushes, and 13
// workflow_dispatch runs. Every excluded row is a failure or worse, so any
// leak through the trust filter changes a value string or adds a series.
func s3Payload() (rows []map[string]any, schedules []string) {
	at := time.Date(2026, 9, 3, 2, 8, 5, 0, time.UTC)
	id := int64(33706449796)
	add := func(workflow, event, branch, conclusion string) {
		rows = append(rows, ghRunAt(id, workflow, event, branch, "completed", conclusion, 1, at))
		id += 1013
		at = at.Add(17 * time.Minute)
	}
	conclusion := func(value byte) string {
		if value == '1' {
			return "failure"
		}
		return "success"
	}
	for i := 0; i < len(s3ScanValues); i++ {
		if i < len(s3CIValues) {
			add("CI", "push", "main", conclusion(s3CIValues[i]))
		}
		event := "push"
		if i%18 == 5 {
			event = "schedule"
			schedules = append(schedules, fmt.Sprint(id))
		}
		add("Security Scan", event, "main", conclusion(s3ScanValues[i]))
		switch {
		case i < 3:
			add("CI", "push", "main", "cancelled")
			add("CI", "pull_request", "main", "failure")
			add("Security Scan", "pull_request", "main", "action_required")
		case i < 13:
			add("Release", "push", fmt.Sprintf("v0.50.%d", 111+i), "failure")
		case i < 21:
			add("Upgrade Canary", "workflow_dispatch", "main", "failure")
		case i < 24:
			add("Recover Homebrew Formula Bridge", "workflow_dispatch", "main", "failure")
		case i < 26:
			add("Receive signed ADK channel", "workflow_dispatch", "main", "failure")
		}
	}
	slices.Reverse(rows)
	return rows, schedules
}

func seriesValues(observations []healthband.Observation) ([]float64, string) {
	values := make([]float64, len(observations))
	var text strings.Builder
	for i, observation := range observations {
		values[i] = observation.Value
		fmt.Fprintf(&text, "%d", int(observation.Value))
	}
	return values, text.String()
}

func requireNear(t *testing.T, want float64, got *float64, name string) {
	t.Helper()
	require.NotNil(t, got, name)
	assert.LessOrEqual(t, math.Abs(want-*got), 5e-7, "%s = %v, want %v", name, *got, want)
}

// S3 with the trusted-event filter: the probe mix ingests into exactly two
// series whose value strings and detector results equal R1 and R2, and the
// 2 workflow_dispatch runs of Receive signed ADK channel create no series.
func TestReactBandIngest_S3_TrustedEventFilterOnTheProbeMix(t *testing.T) {
	t.Parallel()
	rows, schedules := s3Payload()
	require.Len(t, rows, 200)
	require.Len(t, schedules, 5)
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", ghPayload(t, rows...))

	fetch, appended := ingestBandCI(t, testBandClient(runner), t.TempDir(), bandDefaultLimit)
	require.Empty(t, fetch.Reason)
	assert.Equal(t, 168, appended)
	assert.Equal(t, 200, fetch.Rows)
	assert.Equal(t, 32, fetch.Excluded)
	assert.Empty(t, fetch.SeriesReasons)

	series := healthband.OrderedSeries(fetch.Observations)
	assert.ElementsMatch(t, []string{"ci.failure_rate:CI", "ci.failure_rate:Security Scan"}, slices.Collect(maps.Keys(series)))
	ciValues, ciText := seriesValues(series["ci.failure_rate:CI"])
	assert.Equal(t, s3CIValues, ciText)
	scanValues, scanText := seriesValues(series["ci.failure_rate:Security Scan"])
	assert.Equal(t, s3ScanValues, scanText)
	var scanKeys []string
	for _, observation := range series["ci.failure_rate:Security Scan"] {
		scanKeys = append(scanKeys, observation.SampleKey)
	}
	assert.Subset(t, scanKeys, schedules, "schedule runs on the default branch are trusted")

	r1 := healthband.EvaluateValues(ciValues)
	require.NotNil(t, r1.N)
	assert.Equal(t, 19, *r1.N)
	assert.Equal(t, []string{healthband.ReasonInsufficientSamples}, r1.Reasons)
	r2 := healthband.EvaluateValues(scanValues)
	require.NotNil(t, r2.N)
	assert.Equal(t, 21, *r2.N)
	requireNear(t, 0, r2.X, "x")
	requireNear(t, 0.107143, r2.Mu, "mu")
	requireNear(t, 0.280306, r2.SD, "sd")
	requireNear(t, 0.280306, r2.SDEff, "sd_eff")
	requireNear(t, -0.382235, r2.Z, "z")
	require.NotNil(t, r2.Tier)
	assert.Equal(t, 0, *r2.Tier)
	assert.Equal(t, []string{healthband.ReasonBelowBaseline}, r2.Reasons)
}
