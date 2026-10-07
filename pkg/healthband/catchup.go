package healthband

import (
	"errors"
	"slices"
	"sort"
	"time"
)

var errInvalidOwner = errors.New("healthband: owner token must be <hostname>:<pid>:<16 hex>")

// PlanOptions configures phase A planning.
type PlanOptions struct {
	Owner string        // owner token of this run (NewOwner)
	Only  []string      // --series filter; empty plans every stored series
	Fresh []Observation // observations this run appended, for late detection
	// NewClaimID returns a claim id; nil uses NewClaimID.
	NewClaimID func() string
	// Evaluate evaluates one position of an ordered series; nil uses the
	// detector (EvaluateAt).
	Evaluate func(ordered []Observation, position int) Evaluation
}

// RunRef is one failed CI run attempt whose log is diagnosis evidence.
type RunRef struct {
	RunID   int64
	Attempt int
}

// DueClaim is one claim this run owns, in phase B execution order.
type DueClaim struct {
	Claim
	Series    string
	SampleKey string
	EpisodeID string
	Tier      int
	// Event is the evaluation event that holds the claim; its frozen record
	// is the prompt snapshot, never re-rendered for a later position.
	Event Event
	// FailedRuns are the failed CI runs of the current block, at most K.
	FailedRuns []RunRef
}

// SeriesPlan is the planned outcome of one series.
type SeriesPlan struct {
	Series string
	Events []Event // evaluation events, oldest position first
	Late   int     // late observations: stored, no past position re-evaluated
}

// Plan is phase A's decision for every pending position; Commit writes it.
type Plan struct {
	Series   []SeriesPlan // sorted by series ID
	Claims   []DueClaim   // phase B order: series sorted, one claim at a time
	NotFound []string     // Only entries that name no stored series
	base     int64        // first seq of the plan
	complete bool         // every stored series was planned, so compaction may run
}

// Events returns every planned event in append order.
func (p Plan) Events() []Event {
	var events []Event
	for _, series := range p.Series {
		events = append(events, series.Events...)
	}
	return events
}

// Plan evaluates every pending position of every selected series oldest
// first (REQ-09) and applies the Decision Table, the batch rule, and chained
// leases (REQ-24): a claim's lease is the phase A time plus the budgets of
// every earlier claim of this run and its own. Plan writes nothing.
func (w *WAL) Plan(series map[string][]Observation, opts PlanOptions) (Plan, error) {
	if !ownerPattern.MatchString(opts.Owner) {
		return Plan{}, errInvalidOwner
	}
	newID, evaluate := opts.NewClaimID, opts.Evaluate
	if newID == nil {
		newID = NewClaimID
	}
	if evaluate == nil {
		evaluate = EvaluateAt
	}
	plan := Plan{base: w.nextSeq}
	var ids []string
	ids, plan.NotFound, plan.complete = selectSeries(series, opts.Only)
	seq, chained := w.nextSeq, time.Duration(0)
	for _, id := range ids {
		ordered := series[id]
		state, checkpointed := w.state.Series[id]
		at := lastKeyIndex(ordered, state, checkpointed)
		positions := pendingPositions(ordered, at)
		seriesPlan := SeriesPlan{Series: id, Late: lateCount(ordered, at, opts.Fresh), Events: decideSeries(ordered, positions, state, evaluate)}
		for i := range seriesPlan.Events {
			event := &seriesPlan.Events[i]
			event.Seq, seq = seq, seq+1
			if event.Action != ActionDiagnose {
				continue
			}
			chained += ClaimBudget(ClaimKindDiagnose)
			claim := Claim{ID: newID(), Kind: ClaimKindDiagnose, Owner: opts.Owner, LeaseUntil: w.now.Add(chained)}
			event.Claims = []Claim{claim}
			plan.Claims = append(plan.Claims, DueClaim{
				Claim: claim, Series: id, SampleKey: event.SampleKey, EpisodeID: event.EpisodeID, Tier: *event.Tier,
				Event: *event, FailedRuns: failedRuns(ordered, positions[i]),
			})
		}
		plan.Series = append(plan.Series, seriesPlan)
	}
	return plan, nil
}

// decideSeries evaluates positions oldest first against a scratch copy of
// the series state, then applies the batch rule: only the newest episode
// opened in this run keeps its diagnose; every earlier opening is recorded
// as suppressed with reason superseded_in_batch.
func decideSeries(ordered []Observation, positions []int, state SeriesState, evaluate func([]Observation, int) Evaluation) []Event {
	if len(positions) == 0 {
		return nil
	}
	id := ordered[0].Series
	scratch := Checkpoint{Series: map[string]SeriesState{id: cloneSeries(state)}}
	events := make([]Event, 0, len(positions))
	opening := -1
	for _, position := range positions {
		evaluation := evaluate(ordered, position)
		evaluation.Series, evaluation.SampleKey = id, ordered[position].SampleKey
		if evaluation.Constants == nil {
			constants := DefaultConstants()
			evaluation.Constants = &constants
		}
		event := evaluationEvent(evaluation, Decide(evaluation, scratch.Series[id]))
		applyEvent(&scratch, event)
		if event.Action == ActionDiagnose {
			if opening >= 0 {
				events[opening].Action = ActionSuppressed
				events[opening].Reasons = append(events[opening].Reasons, ReasonSupersededInBatch)
			}
			opening = len(events)
		}
		events = append(events, event)
	}
	return events
}

// lastKeyIndex returns the position of the checkpoint key in an ordered
// series, or -1 without a checkpoint or when the series no longer holds it.
func lastKeyIndex(ordered []Observation, state SeriesState, checkpointed bool) int {
	if !checkpointed {
		return -1
	}
	for i := len(ordered) - 1; i >= 0; i-- {
		if ordered[i].SampleKey == state.LastKey {
			return i
		}
	}
	return -1
}

// pendingPositions returns every position after the checkpoint key, oldest
// first, or only the newest position without a usable checkpoint key, so a
// first run or a rewritten store never floods the log with past positions.
func pendingPositions(ordered []Observation, at int) []int {
	if len(ordered) == 0 {
		return nil
	}
	if at < 0 {
		return []int{len(ordered) - 1}
	}
	var positions []int
	for p := at + 1; p < len(ordered); p++ {
		positions = append(positions, p)
	}
	return positions
}

// lateCount counts this series' fresh observations whose order key is not
// newer than the checkpoint key: they are stored and change only later
// evaluations (late_observation).
func lateCount(ordered []Observation, at int, fresh []Observation) int {
	if at < 0 {
		return 0
	}
	late := 0
	for _, observation := range fresh {
		if observation.Series == ordered[at].Series && !orderKeyLess(ordered[at], observation) {
			late++
		}
	}
	return late
}

// failedRuns returns the failed CI runs of the current block at position.
func failedRuns(ordered []Observation, position int) []RunRef {
	var runs []RunRef
	for _, observation := range ordered[max(position-BlockSize+1, 0) : position+1] {
		if observation.Source == SourceGH && observation.Value == 1 {
			runs = append(runs, RunRef{RunID: observation.Tiebreak, Attempt: observation.Attempt})
		}
	}
	return runs
}

// selectSeries returns the sorted series to plan, the filter entries that
// name no stored series, and whether every stored series is planned.
func selectSeries(series map[string][]Observation, only []string) ([]string, []string, bool) {
	var ids, notFound []string
	if len(only) == 0 {
		for id := range series {
			ids = append(ids, id)
		}
	}
	for _, id := range only {
		if _, ok := series[id]; !ok {
			notFound = append(notFound, id)
		} else if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, notFound, len(ids) == len(series)
}

// ReadSeries reads both observation files and returns every series ordered
// oldest first, with the skipped-line counts of both files.
func (s *Store) ReadSeries() (map[string][]Observation, ReadCounts, error) {
	var all []Observation
	var counts ReadCounts
	for _, name := range []string{CIRunsFile, CanaryRunsFile} {
		observations, fileCounts, err := s.ReadObservations(name)
		if err != nil {
			return nil, ReadCounts{}, err
		}
		all = append(all, observations...)
		counts.Malformed += fileCounts.Malformed
		counts.UnknownSchema += fileCounts.UnknownSchema
		counts.InvalidValue += fileCounts.InvalidValue
	}
	return OrderedSeries(all), counts, nil
}
