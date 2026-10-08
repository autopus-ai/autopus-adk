package learn

import "time"

// Prune removes entries older than the given number of days.
// Returns the number of entries removed.
func Prune(store *Store, days int) (int, error) {
	removed, _, err := PruneExcept(store, days, nil)
	return removed, err
}

// PruneExcept removes entries older than the given number of days unless
// protect names their id. protect runs inside the store lock, so the
// protected set is computed against the entries this call rewrites; when it
// fails, its error is returned and the store is left byte-identical. A nil
// protect protects nothing. keptProtected counts the entries at or before the
// cutoff that stayed only because protect named them.
func PruneExcept(store *Store, days int, protect func() (map[string]bool, error)) (removed, keptProtected int, err error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	var protected map[string]bool
	if protect != nil {
		if protected, err = protect(); err != nil {
			return 0, 0, err
		}
	}
	entries, skips, err := store.ReadTolerant()
	if err != nil {
		return 0, 0, err
	}

	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var kept []LearningEntry
	for _, e := range entries {
		switch {
		case e.Timestamp.After(cutoff):
			kept = append(kept, e)
		case protected[e.ID]:
			kept = append(kept, e)
			keptProtected++
		default:
			removed++
		}
	}

	if err := rewriteStore(store, kept, skips); err != nil {
		return 0, 0, err
	}
	return removed, keptProtected, nil
}
