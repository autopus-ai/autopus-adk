package learn

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agedStore appends one gate_fail entry per id, each the given number of days
// old, in argument order.
func agedStore(t *testing.T, ages ...any) (*Store, string) {
	t.Helper()
	store, path := newTestStore(t)
	for index := 0; index < len(ages); index += 2 {
		require.NoError(t, store.Append(LearningEntry{
			ID:        ages[index].(string),
			Type:      EntryTypeGateFail,
			Timestamp: time.Now().Add(-time.Duration(ages[index+1].(int)) * 24 * time.Hour),
			Pattern:   "pattern of " + ages[index].(string),
		}))
	}
	return store, path
}

func storeIDs(t *testing.T, store *Store) []string {
	t.Helper()
	entries, err := store.Read()
	require.NoError(t, err)
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func TestPruneExcept_ProtectedOldEntries_KeptAndCounted(t *testing.T) {
	t.Parallel()
	// Given two old and two recent entries, one of each protected, plus a
	// protected id the store does not hold.
	store, _ := agedStore(t, "L-001", 100, "L-002", 100, "L-003", 1, "L-004", 1)
	protect := func() (map[string]bool, error) {
		return map[string]bool{"L-001": true, "L-003": true, "L-404": true}, nil
	}

	// When pruning at 30 days with that protection.
	removed, kept, err := PruneExcept(store, 30, protect)

	// Then only the unprotected old entry goes, and K counts only the old
	// entry that protection kept.
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	assert.Equal(t, 1, kept)
	assert.Equal(t, []string{"L-001", "L-003", "L-004"}, storeIDs(t, store))
}

func TestPruneExcept_SecondRun_KeepsCountAndBytes(t *testing.T) {
	t.Parallel()
	store, path := agedStore(t, "L-001", 100, "L-002", 100, "L-003", 1)
	protect := func() (map[string]bool, error) { return map[string]bool{"L-001": true}, nil }
	_, _, err := PruneExcept(store, 30, protect)
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	removed, kept, err := PruneExcept(store, 30, protect)

	require.NoError(t, err)
	assert.Equal(t, 0, removed)
	assert.Equal(t, 1, kept, "a protected old entry is counted on every run")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestPruneExcept_ProtectFailure_LeavesStoreByteIdentical(t *testing.T) {
	t.Parallel()
	// Given an old entry in a non-canonical encoding and a broken line, both
	// of which any rewrite would change.
	store, path := newTestStore(t)
	raw := `{"id":"L-001","timestamp":"2020-01-01T00:00:00+0000","type":"gate_fail","pattern":"old"}` + "\n{broken\n"
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))
	unreadable := errors.New("eval_links_unreadable")

	// When the protection callback fails.
	removed, kept, err := PruneExcept(store, 30, func() (map[string]bool, error) { return nil, unreadable })

	// Then prune returns its error and writes nothing.
	assert.ErrorIs(t, err, unreadable)
	assert.Zero(t, removed)
	assert.Zero(t, kept)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, raw, string(data))
}

func TestPruneExcept_ProtectRunsInsideTheStoreLock(t *testing.T) {
	t.Parallel()
	store, _ := agedStore(t, "L-001", 100)
	held := false

	_, _, err := PruneExcept(store, 30, func() (map[string]bool, error) {
		if store.mu.TryLock() {
			store.mu.Unlock()
		} else {
			held = true
		}
		return nil, nil
	})

	require.NoError(t, err)
	assert.True(t, held, "protect must run while the store lock is held")
}

func TestPruneExcept_NilProtect_PrunesByAgeAlone(t *testing.T) {
	t.Parallel()
	store, _ := agedStore(t, "L-001", 100, "L-002", 90, "L-003", 1)

	removed, kept, err := PruneExcept(store, 90, nil)

	require.NoError(t, err)
	assert.Equal(t, 2, removed, "an entry exactly at the cutoff is old")
	assert.Equal(t, 0, kept)
	assert.Equal(t, []string{"L-003"}, storeIDs(t, store))
}
