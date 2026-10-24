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
	result, err := PrunePolicyExcept(store, PrunePolicy{Days: days}, protect)
	return result.Removed, result.KeptProtected, err
}

// PrunePolicy is the retention rule prune applies. LLMOnlyDays, when
// positive, also removes entries whose evidence kind is llm_only once they
// are older than that many days (SPEC-LEARN-003); unspecified and
// deterministic entries follow Days alone.
type PrunePolicy struct {
	Days        int
	LLMOnlyDays int
}

// PruneResult counts what one prune did. RemovedLLMOnly is the part of
// Removed that only the llm_only rule removed.
type PruneResult struct {
	Removed        int
	KeptProtected  int
	RemovedLLMOnly int
}

// PrunePolicyExcept is PruneExcept under a full PrunePolicy. Protection wins
// over both age rules.
func PrunePolicyExcept(store *Store, policy PrunePolicy, protect func() (map[string]bool, error)) (result PruneResult, err error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	var protected map[string]bool
	if protect != nil {
		if protected, err = protect(); err != nil {
			return PruneResult{}, err
		}
	}
	entries, skips, err := store.ReadTolerant()
	if err != nil {
		return PruneResult{}, err
	}

	now := time.Now()
	cutoff := now.Add(-time.Duration(policy.Days) * 24 * time.Hour)
	llmCutoff := now.Add(-time.Duration(policy.LLMOnlyDays) * 24 * time.Hour)
	var kept []LearningEntry
	for _, e := range entries {
		fresh := e.Timestamp.After(cutoff)
		llmOnlyExpired := fresh && policy.LLMOnlyDays > 0 &&
			e.EvidenceKindOf() == EvidenceLLMOnly && !e.Timestamp.After(llmCutoff)
		switch {
		case fresh && !llmOnlyExpired:
			kept = append(kept, e)
		case protected[e.ID]:
			kept = append(kept, e)
			result.KeptProtected++
		default:
			result.Removed++
			if llmOnlyExpired {
				result.RemovedLLMOnly++
			}
		}
	}

	if err := rewriteStore(store, kept, skips); err != nil {
		return PruneResult{}, err
	}
	return result, nil
}
