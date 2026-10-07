package healthband

import (
	"maps"
	"slices"
	"time"
)

// Step timeouts of one diagnose claim and its margin (Durability Protocol
// step table). The gh calls of the network step run before phase A and
// belong to no claim.
const (
	FailedLogTimeout = 60 * time.Second  // gh run view --log-failed, per failed run of the current block
	ProviderTimeout  = 600 * time.Second // the provider call
	BSWriteTimeout   = 30 * time.Second  // BS lock wait and write
	ClaimMargin      = 60 * time.Second  // covers the per-claim result write (phase C)
	// DiagnoseBudget is 4×60 + 600 + 30 + 60 = 930 s.
	DiagnoseBudget = BlockSize*FailedLogTimeout + ProviderTimeout + BSWriteTimeout + ClaimMargin
)

// claimBudgets is the worst-case budget per claim kind. A kind added by a
// sibling SPEC registers its budget here, so the lease chain counts it.
var claimBudgets = map[string]time.Duration{ClaimKindDiagnose: DiagnoseBudget}

// ClaimBudget returns the step budget of a claim kind; an unknown kind has
// none.
func ClaimBudget(kind string) time.Duration { return claimBudgets[kind] }

// Decision is the Decision Table outcome of one evaluated position.
type Decision struct {
	Action    string // log, diagnose, or suppressed
	Reason    string // decision reason; empty where the table names none
	EpisodeID string // episode the position belongs to afterwards; empty for none
	MaxTier   int    // that episode's highest tier afterwards
}

// Decide applies the Episode and Action Decision Table (REQ-07, REQ-08). The
// action depends on the evaluated tier and the series' newest episode alone:
// no tier and tier 1 log and keep the episode, tier 0 logs and closes an open
// episode, and tier 2 or 3 opens an episode with diagnose or, inside an open
// one, is suppressed while raising its max_tier.
func Decide(evaluation Evaluation, state SeriesState) Decision {
	decision := Decision{Action: ActionLog}
	open := newestOpen(state)
	if open != nil {
		decision.EpisodeID, decision.MaxTier = open.ID, open.MaxTier
	}
	tier := -1
	if evaluation.Tier != nil {
		tier = *evaluation.Tier
	}
	switch {
	case tier == 0 && open != nil:
		decision.Reason = ReasonEpisodeClosed
	case tier >= 2 && open != nil:
		decision.Action, decision.Reason = ActionSuppressed, ReasonEpisodeAlreadyDiagnosed
		decision.MaxTier = max(decision.MaxTier, tier)
	case tier >= 2:
		decision.Action, decision.EpisodeID, decision.MaxTier = ActionDiagnose, "e"+evaluation.SampleKey, tier
	}
	return decision
}

// newestOpen returns the newest episode when it is still open.
func newestOpen(state SeriesState) *Episode {
	if n := len(state.Episodes); n > 0 && state.Episodes[n-1].Open {
		return &state.Episodes[n-1]
	}
	return nil
}

// evaluationEvent records one evaluated position and its decision. Its
// reasons are the detector reasons followed by the decision reason, and an
// event inside an episode carries that episode's max_tier.
func evaluationEvent(evaluation Evaluation, decision Decision) Event {
	event := Event{
		Schema: SchemaBandEvaluation, Kind: EventKindEvaluation, Evaluation: evaluation,
		Action: decision.Action, EpisodeID: decision.EpisodeID,
	}
	event.Reasons = slices.Clone(evaluation.Reasons)
	if decision.Reason != "" {
		event.Reasons = append(event.Reasons, decision.Reason)
	}
	if decision.EpisodeID != "" {
		maxTier := decision.MaxTier
		event.MaxTier = &maxTier
	}
	return event
}

// applyEvent folds one event into the checkpoint. Replay and the live path
// share it, so a replayed log rebuilds exactly the state a live run wrote.
func applyEvent(state *Checkpoint, event Event) {
	state.LastSeq = max(state.LastSeq, event.Seq)
	if state.Series == nil {
		state.Series = make(map[string]SeriesState)
	}
	series, known := state.Series[event.Series]
	switch event.Kind {
	case EventKindEvaluation:
		series.LastKey = event.SampleKey
		if event.EpisodeID != "" {
			series.Episodes = applyEpisode(series.Episodes, event)
		}
	case EventKindActionResult:
		if !known || slices.Contains(event.Reasons, ReasonClaimUnknown) || !applyClaimResult(series.Episodes, event) {
			return
		}
	default:
		return
	}
	state.Series[event.Series] = series
}

// applyEpisode opens, raises, or closes the event's episode and attaches the
// claims the event carries as claimed.
func applyEpisode(episodes []Episode, event Event) []Episode {
	maxTier := 0
	if event.MaxTier != nil {
		maxTier = *event.MaxTier
	}
	closed := slices.Contains(event.Reasons, ReasonEpisodeClosed)
	for i := range episodes {
		if episodes[i].ID == event.EpisodeID {
			episodes[i].MaxTier = max(episodes[i].MaxTier, maxTier)
			episodes[i].Open = episodes[i].Open && !closed
			episodes[i].Claims = attachClaims(episodes[i].Claims, event.Claims)
			return episodes
		}
	}
	return append(episodes, Episode{ID: event.EpisodeID, Open: !closed, MaxTier: maxTier, Claims: attachClaims(nil, event.Claims)})
}

// attachClaims adds the claims whose id is new, each with status claimed.
func attachClaims(claims, added []Claim) []Claim {
	for _, claim := range added {
		if !slices.ContainsFunc(claims, func(held Claim) bool { return held.ID == claim.ID }) {
			claim.Status, claim.InterruptedAt = ClaimClaimed, nil
			claims = append(claims, claim)
		}
	}
	return claims
}

// applyClaimResult gives the event's claim its result status and, for done,
// sets the episode's BS ID. A claim that already ended keeps its result.
func applyClaimResult(episodes []Episode, event Event) bool {
	if len(event.Claims) != 1 {
		return false
	}
	for i := range episodes {
		for j := range episodes[i].Claims {
			claim := &episodes[i].Claims[j]
			if claim.ID != event.ClaimID {
				continue
			}
			if claim.Status != ClaimClaimed && claim.Status != ClaimInterrupted {
				return false
			}
			claim.Status, claim.InterruptedAt = event.Claims[0].Status, nil
			if claim.Status == ClaimDone && event.BSID != "" {
				episodes[i].BSID = event.BSID
			}
			return true
		}
	}
	return false
}

// LateResultWindow both accepts a late result and bounds how long an earlier
// episode with an interrupted claim stays in the checkpoint (Durability
// items 6 and 7).
const LateResultWindow = 24 * time.Hour

// expireLeases marks every claimed claim whose lease has ended interrupted
// and returns their ids, series sorted. Only claimed claims expire, so
// interrupted never replaces done or failed (Durability item 6).
func expireLeases(state *Checkpoint, now time.Time) []string {
	var interrupted []string
	for _, id := range slices.Sorted(maps.Keys(state.Series)) {
		episodes := state.Series[id].Episodes
		for e := range episodes {
			for c := range episodes[e].Claims {
				claim := &episodes[e].Claims[c]
				if claim.Status == ClaimClaimed && now.After(claim.LeaseUntil) {
					at := now.UTC()
					claim.Status, claim.InterruptedAt = ClaimInterrupted, &at
					interrupted = append(interrupted, claim.ID)
				}
			}
		}
	}
	return interrupted
}

// resultReason decides how a result applies (Durability items 4, 6, 7): no
// reason for a claimed claim, late_result for an interrupted claim younger
// than LateResultWindow, and claim_unknown for a claim no kept episode holds
// or whose window has passed. duplicate reports a claim that already ended.
func resultReason(state Checkpoint, result Result, now time.Time) (reason string, duplicate bool) {
	for _, episode := range state.Series[result.Series].Episodes {
		for _, claim := range episode.Claims {
			switch {
			case claim.ID != result.Claim.ID:
				continue
			case claim.Status == ClaimClaimed:
				return "", false
			case claim.Status != ClaimInterrupted:
				return "", true
			case youngerThanWindow(claim, now):
				return ReasonLateResult, false
			}
			return ReasonClaimUnknown, false
		}
	}
	return ReasonClaimUnknown, false
}

// retain keeps per series the newest episode and every earlier episode that
// still holds a claimed claim or an interrupted claim younger than
// LateResultWindow (Durability item 7).
func retain(state *Checkpoint, now time.Time) {
	for id, series := range state.Series {
		n := len(series.Episodes)
		if n < 2 {
			continue
		}
		kept := make([]Episode, 0, n)
		for i, episode := range series.Episodes {
			if i == n-1 || holdsLiveClaim(episode, now) {
				kept = append(kept, episode)
			}
		}
		if len(kept) != n {
			series.Episodes = kept
			state.Series[id] = series
		}
	}
}

// holdsLiveClaim reports a claimed claim or an interrupted claim younger
// than LateResultWindow.
func holdsLiveClaim(episode Episode, now time.Time) bool {
	return slices.ContainsFunc(episode.Claims, func(claim Claim) bool {
		return claim.Status == ClaimClaimed || claim.Status == ClaimInterrupted && youngerThanWindow(claim, now)
	})
}

func youngerThanWindow(claim Claim, now time.Time) bool {
	return claim.InterruptedAt != nil && now.Sub(*claim.InterruptedAt) < LateResultWindow
}

// cloneSeries deep-copies a series state, so planning never aliases it.
func cloneSeries(state SeriesState) SeriesState {
	state.Episodes = slices.Clone(state.Episodes)
	for i := range state.Episodes {
		state.Episodes[i].Claims = slices.Clone(state.Episodes[i].Claims)
	}
	return state
}
