package healthband

// MergeObservations appends only candidates whose (series, sample_key) is
// new or whose attempt is higher than every stored and earlier-merged one,
// and returns them, so re-ingesting a payload appends nothing.
func (l *Locked) MergeObservations(name string, candidates []Observation) ([]Observation, error) {
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return nil, err
		}
	}
	existing, _, err := l.store.ReadObservations(name)
	if err != nil {
		return nil, err
	}
	appended := NewerAttempts(existing, candidates)
	if err := l.AppendObservations(name, appended); err != nil {
		return nil, err
	}
	return appended, nil
}

// NewerAttempts returns, with UTC times, the candidates whose (series,
// sample_key) is new or whose attempt is higher than every stored and
// earlier candidate one: the lines MergeObservations appends, which a
// read-only plan (auto react band --dry-run) merges in memory instead.
//
// A series that already holds MaxObservationsPerSeries samples takes no new
// sample older than the oldest one compaction keeps: compaction would drop
// it again, so a fetch window wider than the bound (--limit above 512) would
// otherwise re-add the same runs as late observations on every run. A
// higher attempt of a stored sample is still taken.
func NewerAttempts(stored, candidates []Observation) []Observation {
	best := make(map[seriesSampleKey]int, len(stored))
	for _, observation := range stored {
		key := seriesSampleKey{observation.Series, observation.SampleKey}
		best[key] = max(best[key], observation.Attempt)
	}
	floors := retainedFloors(stored)
	var newer []Observation
	for _, candidate := range candidates {
		key := seriesSampleKey{candidate.Series, candidate.SampleKey}
		attempt, known := best[key]
		if candidate.Attempt <= attempt {
			continue
		}
		if floor, full := floors[candidate.Series]; full && !known && orderKeyLess(candidate, floor) {
			continue
		}
		best[key] = candidate.Attempt
		candidate.ObservedAt = candidate.ObservedAt.UTC()
		newer = append(newer, candidate)
	}
	return newer
}

// retainedFloors returns, for each series holding at least
// MaxObservationsPerSeries collapsed samples, the oldest sample that
// compaction keeps.
func retainedFloors(stored []Observation) map[string]Observation {
	floors := make(map[string]Observation)
	for series, ordered := range OrderedSeries(stored) {
		if len(ordered) >= MaxObservationsPerSeries {
			floors[series] = ordered[len(ordered)-MaxObservationsPerSeries]
		}
	}
	return floors
}
