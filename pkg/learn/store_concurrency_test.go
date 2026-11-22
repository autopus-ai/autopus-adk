package learn

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStore_ConcurrentAppendAndUpdateReuseCount is the S2 oracle for REQ-002:
// a concurrent Append load racing with read-modify-rewrite (UpdateReuseCount)
// must not lose appended entries. Run with -race.
func TestStore_ConcurrentAppendAndUpdateReuseCount(t *testing.T) {
	t.Parallel()

	// Given: an empty store seeded with one entry L-001 via AppendAtomic.
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	require.NoError(t, store.AppendAtomic(EntryTypeGateFail, RecordOpts{
		Phase:    "validate",
		SpecID:   "SPEC-SEED",
		Pattern:  "seed entry",
		Severity: SeverityLow,
	}))

	// Sanity: the seed is sequence 1.
	seeded, err := store.Read()
	require.NoError(t, err)
	require.Len(t, seeded, 1)
	seedID := seeded[0].ID
	require.Regexp(t, `^L-001-[0-9a-f]{6}$`, seedID)

	const n = 50

	// When: 50 goroutines append new entries while 50 goroutines bump the
	// reuse count of L-001 concurrently.
	var wg sync.WaitGroup
	appendErrs := make([]error, n)
	updateErrs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			appendErrs[idx] = store.AppendAtomic(EntryTypeGateFail, RecordOpts{
				Phase:    "validate",
				SpecID:   "SPEC-APPEND",
				Pattern:  "concurrent append",
				Severity: SeverityLow,
			})
		}(i)
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			updateErrs[idx] = store.UpdateReuseCount(seedID)
		}(i)
	}
	wg.Wait()

	for _, e := range appendErrs {
		require.NoError(t, e)
	}
	for _, e := range updateErrs {
		require.NoError(t, e)
	}

	// Then: Read() returns exactly 51 entries (1 seed + 50 appends), none lost
	// to a truncate-rewrite race.
	final, err := store.Read()
	require.NoError(t, err)
	assert.Len(t, final, 51, "no appended entry must be lost to a concurrent rewrite")

	// And: L-001's ReuseCount is exactly 50.
	var seed *LearningEntry
	for i := range final {
		if final[i].ID == seedID {
			seed = &final[i]
			break
		}
	}
	require.NotNil(t, seed, "seed entry L-001 must survive the rewrite race")
	assert.Equal(t, 50, seed.ReuseCount, "every UpdateReuseCount increment must be preserved")
}
