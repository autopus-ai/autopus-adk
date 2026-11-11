package diskspace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimAutoReclaim_RateLimit(t *testing.T) {
	t.Parallel()
	state := testState(t)
	start := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	interval := 30 * time.Minute
	steps := []struct {
		at   time.Duration
		want bool
	}{
		{0, true},                 // first claim
		{time.Minute, false},      // inside the interval
		{29 * time.Minute, false}, // still inside
		{30 * time.Minute, true},  // interval elapsed, restamped
		{45 * time.Minute, false}, // measured from the new stamp
		{-time.Hour, true},        // clock moved back: due, not blocked forever
	}
	for _, step := range steps {
		got, err := ClaimAutoReclaim(state, start.Add(step.at), interval)
		require.NoError(t, err)
		assert.Equal(t, step.want, got, "claim at %s", step.at)
	}
}

func TestClaimAutoReclaim_CorruptStampIsDue(t *testing.T) {
	t.Parallel()
	state := testState(t)
	require.NoError(t, state.WriteFileAtomic(autoReclaimStampFile, []byte("not a time\n")))
	got, err := ClaimAutoReclaim(state, time.Now(), time.Hour)
	require.NoError(t, err)
	assert.True(t, got)
}
