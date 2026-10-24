package learn

import (
	"sort"
	"strings"
	"time"
)

const (
	fileMatchWeight    = 3.0
	packageMatchWeight = 2.0
	keywordMatchWeight = 1.0
	recencyBonusDays   = 30
	recencyBonus       = 1.0
	// evidenceBonus ranks a lesson backed by an execution contrast above an
	// equally relevant one that rests on judgment (SPEC-LEARN-003).
	evidenceBonus = 1.0
)

// MatchRelevance scores how relevant an entry is to the given query.
func MatchRelevance(entry LearningEntry, query RelevanceQuery) float64 {
	if len(query.Files) == 0 && len(query.Packages) == 0 && len(query.Keywords) == 0 {
		return 0.0
	}

	var score float64

	for _, qf := range query.Files {
		for _, ef := range entry.Files {
			if qf == ef {
				score += fileMatchWeight
			}
		}
	}

	for _, qp := range query.Packages {
		for _, ep := range entry.Packages {
			if qp == ep {
				score += packageMatchWeight
			}
		}
	}

	combined := strings.ToLower(entry.Pattern + " " + entry.Resolution)
	for _, kw := range query.Keywords {
		if strings.Contains(combined, strings.ToLower(kw)) {
			score += keywordMatchWeight
		}
	}

	if score == 0.0 {
		return 0.0
	}

	if !entry.Timestamp.IsZero() {
		age := time.Since(entry.Timestamp)
		if age < time.Duration(recencyBonusDays)*24*time.Hour {
			score += recencyBonus
		}
	}

	return score + evidenceScore(entry)
}

func evidenceScore(entry LearningEntry) float64 {
	if entry.EvidenceKindOf() == EvidenceDeterministic {
		return evidenceBonus
	}
	return 0
}

// ScoredEntry is one query match with the relevance score that ranked it.
type ScoredEntry struct {
	Entry LearningEntry
	Score float64
}

// QueryRelevant returns entries with relevance score above threshold.
func QueryRelevant(store *Store, query RelevanceQuery, threshold float64) ([]LearningEntry, error) {
	scored, err := QueryScored(store, query, threshold)
	if err != nil {
		return nil, err
	}
	out := make([]LearningEntry, len(scored))
	for i, r := range scored {
		out[i] = r.Entry
	}
	return out, nil
}

// QueryScored is QueryRelevant with each match's score, highest first. A
// spec-only query has no relevance terms, so every entry of that spec scores 1,
// plus the evidence bonus. A scope filter compares the effective scope, so an
// entry without one matches project.
func QueryScored(store *Store, query RelevanceQuery, threshold float64) ([]ScoredEntry, error) {
	entries, err := store.Read()
	if err != nil {
		return nil, err
	}

	var results []ScoredEntry
	isSpecOnly := query.SpecID != "" && len(query.Files) == 0 && len(query.Packages) == 0 && len(query.Keywords) == 0

	for _, e := range entries {
		if query.SpecID != "" && e.SpecID != query.SpecID {
			continue
		}
		if query.Scope != "" && e.EffectiveScope() != query.Scope {
			continue
		}

		if isSpecOnly {
			results = append(results, ScoredEntry{e, 1.0 + evidenceScore(e)})
		} else {
			s := MatchRelevance(e, query)
			if s >= threshold {
				results = append(results, ScoredEntry{e, s})
			}
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results, nil
}
