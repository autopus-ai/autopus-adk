package healthband

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"sort"
	"time"
)

// localpatch-state.json is the checkpoint of this SPEC's log: the local
// patch claims per series and the episodes with a bs_not_tier3 decision,
// replayed like SPEC-SIGMABAND-001's checkpoint. It is a cache of the
// folded log, so an unparsable or invalid state is rebuilt from the log.

// Retention bounds of the log and the state; tests lower them.
var (
	maxLocalPatchRecords  = 2048
	maxLocalPatchEpisodes = 64
)

// LocalPatchState is localpatch-state.json.
type LocalPatchState struct {
	Schema  string                         `json:"schema"`
	LastSeq int64                          `json:"last_seq"`
	Series  map[string][]LocalPatchEpisode `json:"series,omitempty"`
}

// LocalPatchEpisode is one episode with a local patch claim or decision.
type LocalPatchEpisode struct {
	ID         string                 `json:"id"`
	BSNotTier3 bool                   `json:"bs_not_tier3,omitempty"`
	Claims     []LocalPatchStateClaim `json:"claims,omitempty"`
}

// LocalPatchStateClaim is one local_patch claim with its status: claimed,
// done, or failed:<code>.
type LocalPatchStateClaim struct {
	ID         string    `json:"id"`
	Key        string    `json:"key"`
	DependsOn  string    `json:"depends_on"`
	LeaseUntil time.Time `json:"lease_until"`
	Status     string    `json:"status"`
}

// EpisodePatched reports whether the episode holds a local_patch claim
// (Decision Table row 3).
func (l *LocalPatchLog) EpisodePatched(series, episodeID string) bool {
	episode := l.State.episode(series, episodeID)
	return episode != nil && len(episode.Claims) > 0
}

// EpisodeBSNotTier3 reports whether this SPEC's records hold a bs_not_tier3
// decision for the episode (Opening tier rule).
func (l *LocalPatchLog) EpisodeBSNotTier3(series, episodeID string) bool {
	episode := l.State.episode(series, episodeID)
	return episode != nil && episode.BSNotTier3
}

func (s *LocalPatchState) episode(series, episodeID string) *LocalPatchEpisode {
	episodes := s.Series[series]
	for i := range episodes {
		if episodes[i].ID == episodeID {
			return &episodes[i]
		}
	}
	return nil
}

func (s *LocalPatchState) claim(claimID string) (LocalPatchStateClaim, bool) {
	for _, episodes := range s.Series {
		for _, episode := range episodes {
			for _, claim := range episode.Claims {
				if claim.ID == claimID {
					return claim, true
				}
			}
		}
	}
	return LocalPatchStateClaim{}, false
}

// apply folds one record; replay and the live append share it.
func (s *LocalPatchState) apply(record LocalPatchRecord) {
	s.LastSeq = max(s.LastSeq, record.Seq)
	if s.Series == nil {
		s.Series = make(map[string][]LocalPatchEpisode)
	}
	switch record.Kind {
	case LocalPatchKindDecision:
		if record.Reason == LocalPatchSkippedBSNotTier3 {
			s.ensure(record.Series, record.EpisodeID).BSNotTier3 = true
		}
	case LocalPatchKindClaim:
		episode := s.ensure(record.Series, record.EpisodeID)
		if !slices.ContainsFunc(episode.Claims, func(c LocalPatchStateClaim) bool { return c.ID == record.ClaimID }) {
			episode.Claims = append(episode.Claims, LocalPatchStateClaim{
				ID: record.ClaimID, Key: record.Key, DependsOn: record.DependsOn, LeaseUntil: record.LeaseUntil, Status: ClaimClaimed,
			})
		}
		s.retain(record.Series)
	case LocalPatchKindResult:
		for _, episodes := range s.Series {
			for i := range episodes {
				for j := range episodes[i].Claims {
					if claim := &episodes[i].Claims[j]; claim.ID == record.ClaimID && claim.Status == ClaimClaimed {
						claim.Status = record.Status
					}
				}
			}
		}
	}
}

// ensure returns the episode entry, appending it when the series has none.
func (s *LocalPatchState) ensure(series, episodeID string) *LocalPatchEpisode {
	if episode := s.episode(series, episodeID); episode != nil {
		return episode
	}
	s.Series[series] = append(s.Series[series], LocalPatchEpisode{ID: episodeID})
	return &s.Series[series][len(s.Series[series])-1]
}

// retain keeps per series the newest maxLocalPatchEpisodes episodes and
// every earlier one that still holds a claimed claim.
func (s *LocalPatchState) retain(series string) {
	episodes := s.Series[series]
	if len(episodes) <= maxLocalPatchEpisodes {
		return
	}
	cut := len(episodes) - maxLocalPatchEpisodes
	kept := make([]LocalPatchEpisode, 0, maxLocalPatchEpisodes+1)
	for i, episode := range episodes {
		live := slices.ContainsFunc(episode.Claims, func(c LocalPatchStateClaim) bool { return c.Status == ClaimClaimed })
		if i >= cut || live {
			kept = append(kept, episode)
		}
	}
	s.Series[series] = kept
}

// readLocalPatchState returns the state and its bytes; a missing file is an
// empty state and an invalid one is rebuilt from the log.
func readLocalPatchState(path string) (LocalPatchState, []byte, error) {
	empty := LocalPatchState{Schema: SchemaLocalPatchState, Series: make(map[string][]LocalPatchEpisode)}
	file, err := openStoreFile(path, os.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil, nil
	}
	if err != nil {
		return LocalPatchState{}, nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCheckpointBytes))
	if err != nil {
		return LocalPatchState{}, nil, err
	}
	var state LocalPatchState
	if json.Unmarshal(data, &state) != nil || !state.valid() {
		return empty, data, nil
	}
	if state.Series == nil {
		state.Series = make(map[string][]LocalPatchEpisode)
	}
	return state, data, nil
}

func (s LocalPatchState) valid() bool {
	if s.Schema != SchemaLocalPatchState || s.LastSeq < 0 {
		return false
	}
	for series, episodes := range s.Series {
		if !ValidSeriesID(series) {
			return false
		}
		for _, episode := range episodes {
			if !episodeIDPattern.MatchString(episode.ID) {
				return false
			}
			for _, claim := range episode.Claims {
				if !claimIDPattern.MatchString(claim.ID) || !claimIDPattern.MatchString(claim.DependsOn) || !printable(claim.Key, true) ||
					claim.Status != ClaimClaimed && !(LocalPatchRecord{Kind: LocalPatchKindResult, ClaimID: claim.ID, Status: claim.Status}).validResult() {
					return false
				}
			}
		}
	}
	return true
}

// writeState renames the folded state into place unless the bytes on disk
// already hold it.
func (l *LocalPatchLog) writeState(store *Store) error {
	data, err := json.Marshal(l.State)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if bytes.Equal(data, l.written) {
		return nil
	}
	if err := writeStoreFileAtomic(store.Path(LocalPatchStateFile), data); err != nil {
		return err
	}
	l.written = data
	return nil
}

// compactLocalPatch rewrites the log to the newest maxLocalPatchRecords
// records plus every record of a claim that has not ended or that has a
// record inside that window, so no record of a non-terminal claim is ever
// dropped. Kept lines stay byte-identical and in file order. It runs after
// writeState, so every dropped record is already folded into the state.
func (l *Locked) compactLocalPatch(log *LocalPatchLog) error {
	path := l.store.Path(LocalPatchEventsFile)
	lines, err := readStoreLines(path)
	if err != nil || len(lines) <= maxLocalPatchRecords {
		return err
	}
	type line struct {
		raw    []byte
		record LocalPatchRecord
	}
	var valid []line
	newest := make(map[string]int64)
	for _, raw := range lines {
		if record, ok := parseLocalPatchLine(raw); ok {
			valid = append(valid, line{raw, record})
			if id := record.RecordClaimID(); id != "" {
				newest[id] = max(newest[id], record.Seq)
			}
		}
	}
	var cutoff int64 // seq of the newest record outside the window
	if len(valid) > maxLocalPatchRecords {
		seqs := make([]int64, len(valid))
		for i, entry := range valid {
			seqs[i] = entry.record.Seq
		}
		sort.Slice(seqs, func(a, b int) bool { return seqs[a] > seqs[b] })
		cutoff = seqs[maxLocalPatchRecords]
	}
	var buf bytes.Buffer
	kept := 0
	for _, entry := range valid {
		id := entry.record.RecordClaimID()
		if entry.record.Seq > cutoff || id != "" && (!log.Ended(id) || newest[id] > cutoff) {
			buf.Write(entry.raw)
			buf.WriteByte('\n')
			kept++
		}
	}
	if kept == len(lines) {
		return nil
	}
	return writeStoreFileAtomic(path, buf.Bytes())
}
