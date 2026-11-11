package diskspace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordConsumerSample_RingBuffer(t *testing.T) {
	t.Parallel()
	state := testState(t)
	start := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	for i := 0; i < consumerSamplesMax+20; i++ {
		_, err := RecordConsumerSample(state, ConsumerSample{Time: start.Add(time.Duration(i) * consumerSampleInterval),
			Bytes: map[string]int64{"colima": int64(i)}})
		require.NoError(t, err)
	}
	samples := LoadConsumerSamples(state)
	require.Len(t, samples, consumerSamplesMax, "the ledger keeps a bounded number of samples")
	assert.Equal(t, int64(20), samples[0].Bytes["colima"], "the oldest samples are dropped first")
	assert.Equal(t, int64(consumerSamplesMax+19), samples[len(samples)-1].Bytes["colima"])
}

func TestRecordConsumerSample_WithinIntervalMerges(t *testing.T) {
	t.Parallel()
	state := testState(t)
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	_, err := RecordConsumerSample(state, ConsumerSample{Time: now, Bytes: map[string]int64{"colima": 1}})
	require.NoError(t, err)
	samples, err := RecordConsumerSample(state, ConsumerSample{Time: now.Add(time.Minute),
		Bytes: map[string]int64{"colima": 99, "worktrees": 5}})
	require.NoError(t, err)
	require.Len(t, samples, 1, "a sample inside the interval is not appended")
	assert.Equal(t, map[string]int64{"colima": 1, "worktrees": 5}, samples[0].Bytes,
		"it only fills in the consumers the newest sample lacks")
	assert.False(t, ConsumerSampleDue(samples, now.Add(4*time.Minute)))
	assert.True(t, ConsumerSampleDue(samples, now.Add(5*time.Minute)))
	assert.True(t, ConsumerSampleDue(nil, now))
}

func TestComputeGrowth_Ranking(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	at := func(ago time.Duration, bytes map[string]int64) ConsumerSample {
		return ConsumerSample{Time: now.Add(-ago), Bytes: bytes}
	}
	samples := []ConsumerSample{
		at(3*time.Hour, map[string]int64{"colima": 0, "worktrees": 50 * GiB, "lima": 1 * GiB}),
		at(52*time.Minute, map[string]int64{"colima": 2 * GiB, "worktrees": 40 * GiB}),
		at(10*time.Minute, map[string]int64{"colima": 30 * GiB, "worktrees": 41 * GiB}),
	}
	current := map[string]int64{"colima": 40 * GiB, "worktrees": 42 * GiB, "lima": 1 * GiB, "new": 7 * GiB}

	growth := ComputeGrowth(samples, current, now)
	require.Len(t, growth, 3, "a consumer with no earlier sample is left out")
	assert.Equal(t, "colima", growth[0].Name)
	assert.Equal(t, 38*GiB, growth[0].DeltaBytes, "the delta runs from the oldest sample of the last hour")
	assert.Equal(t, now.Add(-52*time.Minute), growth[0].Since)
	assert.Equal(t, 10*GiB, growth[0].LastDeltaBytes, "and since the last sample")
	assert.Equal(t, "worktrees", growth[1].Name)
	assert.Equal(t, 2*GiB, growth[1].DeltaBytes)
	assert.Equal(t, "lima", growth[2].Name)
	assert.Equal(t, int64(0), growth[2].DeltaBytes)
	assert.Equal(t, now.Add(-3*time.Hour), growth[2].Since, "with no sample in the hour, the newest older one is the base")

	top, ok := TopGrower(growth, GiB)
	require.True(t, ok)
	assert.Equal(t, "colima +38.0 GiB in 52m", GrowthPhrase(top, now))
	_, ok = TopGrower(growth, 100*GiB)
	assert.False(t, ok)
	_, ok = TopGrower(nil, 0)
	assert.False(t, ok)

	ledger := append(samples, ConsumerSample{Time: now, Bytes: current})
	assert.Equal(t, growth, LedgerGrowth(ledger), "the ledger form reads the newest sample as current")
	assert.Nil(t, LedgerGrowth(ledger[:1]))
}
