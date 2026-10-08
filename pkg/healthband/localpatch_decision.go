package healthband

import "slices"

// Local Patch Decision Table of SPEC-SIGMABAND-002 (REQ-02). SPEC-SIGMABAND-
// 001's Plan calls it through the PlanOptions.LocalPatch hook (plan task T8)
// once per evaluated tier-3 event, in plan order, while the flag is true.
// Rows are checked top down and the first match wins:
//
//	1 the run has --no-agent                        skipped no_agent
//	2 the event carries superseded_in_batch         skipped superseded_in_batch
//	3 the episode holds a local_patch claim          skipped episode_already_patched
//	4 action diagnose and the retention count is 5   skipped cap_reached
//	5 action diagnose: the tier-3 opening            claim, depends_on its diagnose claim
//	6 the episode's opening tier is 2                skipped bs_not_tier3
//	7 any other tier-3 position                      skipped no_opening_claim

// LocalPatchDecider applies the table to the events of one phase A plan.
// Its fields are read under the store lock that phase A holds.
type LocalPatchDecider struct {
	NoAgent bool
	// Checkpoint is SPEC-SIGMABAND-001's checkpoint before this plan's
	// events, which keeps max_tier but not the opening tier.
	Checkpoint Checkpoint
	// Log is this SPEC's records; nil holds none.
	Log *LocalPatchLog
	// Location is <lp>, which the claim record's derived paths name.
	Location LocalPatchLocation
	// Kept is the retention count of <lp> at phase A (CountKeptKeys).
	Kept int
	// Owner is the run's owner token; NewClaimID draws a claim id (nil is
	// NewClaimID).
	Owner      string
	NewClaimID func() string

	claimed  int             // row-5 claims decided in this plan
	patched  map[string]bool // episodes that got a claim in this plan
	notTier3 map[string]bool // episodes that got bs_not_tier3 in this plan
}

// LocalPatchDecision is the table's outcome for one tier-3 event: the
// decision record and, for row 5, the claim record, whose LeaseUntil the
// caller sets from its lease chain (LocalPatchClaimBudget after the diagnose
// claim). Both are appended with AppendLocalPatch after wal.Commit.
type LocalPatchDecision struct {
	Record LocalPatchRecord
	Claim  *LocalPatchRecord
}

// Decide applies the table to series[i], an event of one series plan after
// SPEC-SIGMABAND-001's batch rule, with every earlier event of that plan
// before it. ok is false for an event that is not an evaluated tier-3
// position. A diagnose event must carry its diagnose claim (Plan attaches it
// before the hook runs); without one, row 5 cannot apply.
func (d *LocalPatchDecider) Decide(series []Event, i int) (decision LocalPatchDecision, ok bool) {
	event := series[i]
	if event.Tier == nil || *event.Tier != maxTier || event.EpisodeID == "" {
		return LocalPatchDecision{}, false
	}
	decision.Record = LocalPatchRecord{
		Kind: LocalPatchKindDecision, Series: event.Series, EpisodeID: event.EpisodeID, EvaluationSeq: event.Seq,
		Decision: LocalPatchDecideSkip,
	}
	episode := event.Series + "\x00" + event.EpisodeID
	diagnose := diagnoseClaimOf(event)
	switch {
	case d.NoAgent:
		decision.Record.Reason = LocalPatchSkippedNoAgent
	case slices.Contains(event.Reasons, ReasonSupersededInBatch):
		decision.Record.Reason = LocalPatchSkippedSuperseded
	case d.patched[episode] || d.Log != nil && d.Log.EpisodePatched(event.Series, event.EpisodeID):
		decision.Record.Reason = LocalPatchSkippedAlreadyPatched
	case event.Action == ActionDiagnose && d.Kept+d.claimed >= LocalPatchRetentionCap:
		decision.Record.Reason = LocalPatchSkippedCapReached
	case event.Action == ActionDiagnose && diagnose != "":
		decision.Record.Decision = LocalPatchDecideClaim
		decision.Claim = d.claim(event, diagnose)
		d.claimed++
		d.mark(&d.patched, episode)
	case d.openingTier(series, i) == 2:
		decision.Record.Reason = LocalPatchSkippedBSNotTier3
		d.mark(&d.notTier3, episode)
	default:
		decision.Record.Reason = LocalPatchSkippedNoOpeningClaim
	}
	return decision, true
}

// claim builds the row-5 claim record with the key and its derived paths.
func (d *LocalPatchDecider) claim(event Event, diagnose string) *LocalPatchRecord {
	newID := d.NewClaimID
	if newID == nil {
		newID = NewClaimID
	}
	id := newID()
	key := LocalPatchKey(event.Series, event.EpisodeID, id)
	paths := d.Location.Paths(key)
	return &LocalPatchRecord{
		Kind: LocalPatchKindClaim, ClaimID: id, Owner: d.Owner, Series: event.Series, EpisodeID: event.EpisodeID,
		DependsOn: diagnose, Key: key, WorktreePath: paths.Worktree, PatchPath: paths.Patch, Branch: paths.Branch,
	}
}

func (d *LocalPatchDecider) mark(set *map[string]bool, episode string) {
	if *set == nil {
		*set = make(map[string]bool)
	}
	(*set)[episode] = true
}

// openingTier is the Opening tier rule: the tier of the opening event when
// this plan holds it (its episode ID is "e" plus its sample key), else 2 when
// 001's checkpoint gives the episode max_tier 2, else 2 when this SPEC's
// records hold a bs_not_tier3 decision for it, else 3.
func (d *LocalPatchDecider) openingTier(series []Event, i int) int {
	event := series[i]
	for _, earlier := range series[:i+1] {
		if earlier.EpisodeID == event.EpisodeID && "e"+earlier.SampleKey == earlier.EpisodeID && earlier.Tier != nil {
			return *earlier.Tier
		}
	}
	for _, episode := range d.Checkpoint.Series[event.Series].Episodes {
		if episode.ID == event.EpisodeID && episode.MaxTier == 2 {
			return 2
		}
	}
	if d.notTier3[event.Series+"\x00"+event.EpisodeID] || d.Log != nil && d.Log.EpisodeBSNotTier3(event.Series, event.EpisodeID) {
		return 2
	}
	return maxTier
}

// diagnoseClaimOf returns the id of the event's diagnose claim.
func diagnoseClaimOf(event Event) string {
	for _, claim := range event.Claims {
		if claim.Kind == ClaimKindDiagnose && claimIDPattern.MatchString(claim.ID) {
			return claim.ID
		}
	}
	return ""
}
