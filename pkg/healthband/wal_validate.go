package healthband

import (
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Identifier patterns of claims, episodes, and results. Events and state
// hold only these, numbers, filtered series IDs, and reason codes
// (Untrusted Input item 9), so a tampered store line is skipped on read.
var (
	claimIDPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	episodeIDPattern = regexp.MustCompile(`^e[A-Za-z0-9._-]{1,64}$`)
	ownerPattern     = regexp.MustCompile(`^[A-Za-z0-9.-]{1,64}:[0-9]{1,10}:[0-9a-f]{16}$`)
	codePattern      = regexp.MustCompile(`^[a-z0-9_:()]{1,96}$`)
	bsIDPattern      = regexp.MustCompile(`^BS-BAND-[0-9]{3,9}$`)
)

// manifest fields hold layer IDs, fixed source references, and codes only.
var (
	manifestTextPattern = regexp.MustCompile(`^[A-Za-z0-9._:/#,=-]{0,128}$`)
	manifestHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func optional(pattern *regexp.Regexp, value string) bool {
	return value == "" || pattern.MatchString(value)
}

// validResultStatus accepts done and failed:<reason>.
func validResultStatus(status string) bool {
	reason, failed := strings.CutPrefix(status, ClaimFailedPrefix)
	return status == ClaimDone || failed && codePattern.MatchString(reason) && !strings.Contains(reason, ":")
}

// validClaim checks a claim as events and the checkpoint carry it; only an
// interrupted claim has an interruption time.
func validClaim(claim Claim) bool {
	status := claim.Status
	return claimIDPattern.MatchString(claim.ID) && codePattern.MatchString(claim.Kind) && ownerPattern.MatchString(claim.Owner) &&
		!claim.LeaseUntil.IsZero() && (status == "" || status == ClaimClaimed || status == ClaimInterrupted || validResultStatus(status)) &&
		(status == ClaimInterrupted) == (claim.InterruptedAt != nil)
}

// validEvent accepts what band writes: an evaluation event with an action of
// the three-value enum, or an action_result event for exactly one claim.
func validEvent(event Event) bool {
	if event.Schema != SchemaBandEvaluation || event.Seq < 1 || !ValidSeriesID(event.Series) ||
		!sampleKeyPattern.MatchString(event.SampleKey) || !optional(episodeIDPattern, event.EpisodeID) ||
		!optional(bsIDPattern, event.BSID) || event.MaxTier != nil && (*event.MaxTier < 0 || *event.MaxTier > maxTier) {
		return false
	}
	for _, claim := range event.Claims {
		if !validClaim(claim) {
			return false
		}
	}
	switch event.Kind {
	case EventKindEvaluation:
		return event.Action == ActionLog || event.Action == ActionDiagnose || event.Action == ActionSuppressed
	case EventKindActionResult:
		return event.EpisodeID != "" && len(event.Claims) == 1 && event.Claims[0].ID == event.ClaimID &&
			validResultStatus(event.Claims[0].Status)
	}
	return false
}

// validCheckpoint accepts what band writes; anything else is rebuilt from
// the log.
func validCheckpoint(state Checkpoint) bool {
	if state.Schema != SchemaBandState || state.LastSeq < 0 {
		return false
	}
	for id, series := range state.Series {
		if !ValidSeriesID(id) || !sampleKeyPattern.MatchString(series.LastKey) {
			return false
		}
		for _, episode := range series.Episodes {
			if !episodeIDPattern.MatchString(episode.ID) || episode.MaxTier < 0 || episode.MaxTier > maxTier || !optional(bsIDPattern, episode.BSID) {
				return false
			}
			for _, claim := range episode.Claims {
				if claim.Status == "" || !validClaim(claim) {
					return false
				}
			}
		}
	}
	return true
}

// valid reports whether a normalized result holds only contract values, so
// it may reach an event, the checkpoint, or a pending file name.
func (r Result) valid() bool {
	return validClaim(r.Claim) && ValidSeriesID(r.Series) && sampleKeyPattern.MatchString(r.SampleKey) &&
		episodeIDPattern.MatchString(r.EpisodeID) && validResultStatus(r.Status) && optional(codePattern, r.DiagnosisStatus) &&
		optional(codePattern, r.BSStatus) && optional(bsIDPattern, r.BSID) && validManifest(r.PromptManifest)
}

// validManifest accepts prompt manifest entries without content: layer IDs,
// fixed source references, codes, and SHA-256 hashes.
func validManifest(entries []promptlayer.ManifestEntry) bool {
	for _, entry := range entries {
		switch entry.Kind {
		case promptlayer.KindStable, promptlayer.KindSnapshot, promptlayer.KindEphemeral:
		default:
			return false
		}
		for _, text := range []string{entry.ID, string(entry.Group), entry.SourceRef, entry.RedactionStatus, entry.InvalidationReason} {
			if !manifestTextPattern.MatchString(text) {
				return false
			}
		}
		if entry.ID == "" || !manifestHashPattern.MatchString(entry.Hash) || entry.TokenEstimate < 0 {
			return false
		}
	}
	return true
}
