package healthband_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

func ciObservation(runID int64, observedAt time.Time, value float64, attempt int) healthband.Observation {
	return healthband.Observation{
		Schema: healthband.SchemaObservation, Series: "ci.failure_rate:CI",
		SampleKey: strconv.FormatInt(runID, 10), ObservedAt: observedAt,
		Tiebreak: runID, Value: value, Attempt: attempt, Source: "gh",
	}
}

func sampleKeys(observations []healthband.Observation) []string {
	keys := make([]string, len(observations))
	for i, observation := range observations {
		keys[i] = observation.SampleKey
	}
	return keys
}

// S2 / G1: blocks are anchored at the newest observation and ties on
// createdAt break on the numeric run_id. A lexicographic tie-break would give
// x = 1.00 and a run_id-only order x = 0.50.
func TestOrderedSeries_GroupingG1NumericTieBreak(t *testing.T) {
	t.Parallel()
	at := func(minute int) time.Time { return time.Date(2026, 9, 14, 9, minute, 0, 0, time.UTC) }
	appended := []healthband.Observation{
		ciObservation(105, at(6), 1, 1), ciObservation(2000, at(0), 0, 1), ciObservation(999, at(4), 1, 1),
		ciObservation(101, at(1), 1, 1), ciObservation(106, at(7), 1, 1), ciObservation(1000, at(4), 0, 1),
		ciObservation(103, at(3), 0, 1), ciObservation(104, at(5), 1, 1), ciObservation(102, at(2), 0, 1),
	}

	ordered := healthband.OrderedSeries(appended)["ci.failure_rate:CI"]
	require.Len(t, ordered, 9)
	assert.Equal(t, []string{"2000", "101", "102", "103", "999", "1000", "104", "105", "106"}, sampleKeys(ordered))

	got := healthband.EvaluateAt(ordered, len(ordered)-1)
	assert.Equal(t, "ci.failure_rate:CI", got.Series)
	assert.Equal(t, "106", got.SampleKey)
	if assert.NotNil(t, got.N) && assert.NotNil(t, got.X) {
		assert.Equal(t, 1, *got.N)
		assert.InDelta(t, 0.75, *got.X, oracleTolerance)
	}
	assert.Equal(t, []string{"insufficient_samples"}, got.Reasons)
	assert.Nil(t, got.Z)
	assert.Nil(t, got.Tier)
}

// S2: canary observations with an equal observed_at order by sequence.
func TestOrderedSeries_EqualObservedAtOrdersBySequence(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 6, 1, 2, 3, 123456789, time.UTC)
	canary := func(sequence int64) healthband.Observation {
		return healthband.Observation{
			Schema: healthband.SchemaObservation, Series: "canary.failure_rate:local",
			SampleKey: "c" + strconv.FormatInt(sequence, 10), ObservedAt: at,
			Tiebreak: sequence, Value: 0, Attempt: 1, Source: "canary",
		}
	}

	ordered := healthband.OrderedSeries([]healthband.Observation{canary(8), canary(7)})

	assert.Equal(t, []string{"c7", "c8"}, sampleKeys(ordered["canary.failure_rate:local"]))
}

// Detector Contract item 2: lines with the same (series, sample_key) collapse
// to the highest attempt, and equal attempts keep the first written line.
func TestCollapse_HighestAttemptWinsAndEqualAttemptsKeepFirstLine(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	security := ciObservation(500, at, 1, 1)
	security.Series = "ci.failure_rate:Security Scan"
	lines := []healthband.Observation{
		ciObservation(500, at, 0, 2), ciObservation(500, at, 1, 3), ciObservation(500, at, 1, 1),
		ciObservation(7, at, 0, 1), ciObservation(7, at, 1, 1), security,
	}

	collapsed := healthband.Collapse(lines)

	require.Len(t, collapsed, 3)
	assert.Equal(t, ciObservation(500, at, 1, 3), collapsed[0])
	assert.Equal(t, ciObservation(7, at, 0, 1), collapsed[1])
	assert.Equal(t, security, collapsed[2])
}

// Detector Contract item 8: evaluating position p uses only observations at
// or before p, so the position before the current block of the O2 fixture is
// still one block short of N_min.
func TestEvaluateAt_UsesOnlyObservationsAtOrBeforePosition(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	values := valuesFromBlocks(repeat(0, 20), 0.5)
	ordered := make([]healthband.Observation, len(values))
	for i, value := range values {
		ordered[i] = ciObservation(int64(1000+i), start.Add(time.Duration(i)*time.Hour), value, 1)
	}

	newest := healthband.EvaluateAt(ordered, len(ordered)-1)
	earlier := healthband.EvaluateAt(ordered, len(ordered)-5)

	assert.Equal(t, "1083", newest.SampleKey)
	if assert.NotNil(t, newest.Tier) && assert.NotNil(t, newest.Z) {
		assert.Equal(t, 2, *newest.Tier)
		assert.InDelta(t, 2.0, *newest.Z, oracleTolerance)
	}
	assert.Equal(t, "1079", earlier.SampleKey)
	if assert.NotNil(t, earlier.N) {
		assert.Equal(t, 19, *earlier.N)
	}
	assert.Equal(t, []string{"insufficient_samples"}, earlier.Reasons)
}

// Detector Contract item 4: fewer than K observations give no current block,
// so neither n nor x is reported; an out-of-range position is the same.
func TestEvaluateAt_FewerThanKObservationsHaveNoCurrentBlock(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	ordered := []healthband.Observation{
		ciObservation(1, at, 1, 1), ciObservation(2, at.Add(time.Minute), 1, 1), ciObservation(3, at.Add(2*time.Minute), 1, 1),
	}

	for _, position := range []int{2, -1, 3} {
		got := healthband.EvaluateAt(ordered, position)
		assert.Equal(t, []string{"no_current_block"}, got.Reasons, "position %d", position)
		assert.Nil(t, got.N, "position %d", position)
		assert.Nil(t, got.X, "position %d", position)
		assert.NotNil(t, got.Constants, "position %d", position)
	}
	assert.Equal(t, "3", healthband.EvaluateAt(ordered, 2).SampleKey)
}
