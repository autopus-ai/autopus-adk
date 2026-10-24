package learn

import (
	"crypto/sha256"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intp(v int) *int { return &v }

// The kind is derived from the exit codes: only a failing-then-passing command
// is deterministic. Any other evidence input is llm_only, and no input leaves
// the entry unspecified with no evidence or scope key on disk.
func TestRecord_DerivesEvidenceKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts RecordOpts
		want EvidenceKind
	}{
		{"fail then pass", RecordOpts{EvidenceCommand: "go test ./x", EvidenceBeforeExit: intp(1), EvidenceAfterExit: intp(0)}, EvidenceDeterministic},
		{"pass then pass", RecordOpts{EvidenceCommand: "go test ./x", EvidenceBeforeExit: intp(0), EvidenceAfterExit: intp(0)}, EvidenceLLMOnly},
		{"command only", RecordOpts{EvidenceCommand: "go test ./x"}, EvidenceLLMOnly},
		{"exits without command", RecordOpts{EvidenceBeforeExit: intp(1), EvidenceAfterExit: intp(0)}, EvidenceLLMOnly},
		{"still failing", RecordOpts{EvidenceCommand: "go test ./x", EvidenceBeforeExit: intp(1), EvidenceAfterExit: intp(1)}, EvidenceLLMOnly},
		{"no evidence", RecordOpts{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, path := newTestStore(t)
			tc.opts.Pattern = "p"
			require.NoError(t, RecordFixPattern(store, tc.opts))
			entries, err := store.Read()
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, tc.want, entries[0].EvidenceKindOf())
			line := readLines(t, path)[0]
			assert.Equal(t, tc.want != "", strings.Contains(line, `"evidence"`), line)
			assert.NotContains(t, line, `"scope"`)
			assert.Equal(t, ScopeProject, entries[0].EffectiveScope())
		})
	}
}

// A bad scope or an evidence command that fails the evidence checks is
// refused before anything is written.
func TestRecord_RefusesBadScopeAndCommand(t *testing.T) {
	t.Parallel()
	store, path := newTestStore(t)
	require.NoError(t, RecordFixPattern(store, RecordOpts{Pattern: "seed", Scope: ScopeProcess}))
	before := sha256File(t, path)

	for _, opts := range []RecordOpts{
		{Pattern: "p", Scope: "global"},
		{Pattern: "p", EvidenceCommand: "go test\x1b[2J"},
		{Pattern: "p", EvidenceCommand: strings.Repeat("a", 513)},
	} {
		err := RecordFixPattern(store, opts)
		var fieldErr *FieldError
		require.True(t, errors.As(err, &fieldErr), "%v", err)
		require.NoError(t, CheckRecordOpts(RecordOpts{Pattern: "ok"}))
		assert.Error(t, CheckRecordOpts(opts))
	}
	assert.Equal(t, before, sha256File(t, path))
}

// A deterministic lesson outranks an equally relevant one by the evidence
// bonus, and the scope filter treats a missing scope as project.
func TestQuery_EvidenceBonusAndScope(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	old := time.Now().Add(-60 * 24 * time.Hour)
	require.NoError(t, store.Append(LearningEntry{ID: "L-001", Timestamp: old, Pattern: "nil map", SpecID: "S"}))
	require.NoError(t, store.Append(LearningEntry{ID: "L-002", Timestamp: old, Pattern: "nil map", SpecID: "S", Scope: ScopeProcess,
		Evidence: &Evidence{Kind: EvidenceDeterministic, Command: "go test", BeforeExit: intp(1), AfterExit: intp(0)}}))
	require.NoError(t, store.Append(LearningEntry{ID: "L-003", Timestamp: old, Pattern: "nil map", SpecID: "S",
		Evidence: &Evidence{Kind: EvidenceLLMOnly}}))

	got, err := QueryScored(store, RelevanceQuery{Keywords: []string{"nil map"}}, 1.0)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "L-002", got[0].Entry.ID)
	assert.Equal(t, 2.0, got[0].Score)
	assert.Equal(t, 1.0, got[1].Score)
	assert.Equal(t, 1.0, got[2].Score)

	got, err = QueryScored(store, RelevanceQuery{SpecID: "S"}, 1.0)
	require.NoError(t, err)
	assert.Equal(t, []float64{2, 1, 1}, scoresOf(got))

	got, err = QueryScored(store, RelevanceQuery{SpecID: "S", Scope: ScopeProject}, 1.0)
	require.NoError(t, err)
	assert.Equal(t, []string{"L-001", "L-003"}, idsOf(got))
	got, err = QueryScored(store, RelevanceQuery{SpecID: "S", Scope: ScopeProcess}, 1.0)
	require.NoError(t, err)
	assert.Equal(t, []string{"L-002"}, idsOf(got))
}

// The llm_only rule removes only llm_only entries past its age; unspecified
// and deterministic entries of the same age follow Days, and protection wins.
func TestPrunePolicy_LLMOnlyDays(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	forty := time.Now().Add(-40 * 24 * time.Hour)
	require.NoError(t, store.Append(LearningEntry{ID: "L-001", Timestamp: forty, Pattern: "unspecified"}))
	require.NoError(t, store.Append(LearningEntry{ID: "L-002", Timestamp: forty, Pattern: "llm", Evidence: &Evidence{Kind: EvidenceLLMOnly}}))
	require.NoError(t, store.Append(LearningEntry{ID: "L-003", Timestamp: forty, Pattern: "det", Evidence: &Evidence{Kind: EvidenceDeterministic}}))
	require.NoError(t, store.Append(LearningEntry{ID: "L-004", Timestamp: forty, Pattern: "llm linked", Evidence: &Evidence{Kind: EvidenceLLMOnly}}))
	require.NoError(t, store.Append(LearningEntry{ID: "L-005", Timestamp: time.Now(), Pattern: "llm fresh", Evidence: &Evidence{Kind: EvidenceLLMOnly}}))

	result, err := PrunePolicyExcept(store, PrunePolicy{Days: 90, LLMOnlyDays: 30}, func() (map[string]bool, error) {
		return map[string]bool{"L-004": true}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, PruneResult{Removed: 1, KeptProtected: 1, RemovedLLMOnly: 1}, result)
	entries, err := store.Read()
	require.NoError(t, err)
	assert.Equal(t, []string{"L-001", "L-003", "L-004", "L-005"}, entryIDs(entries))
}

// Summary counts scope and evidence and lists only process lessons with
// deterministic evidence as template candidates.
func TestSummary_EvidenceAndCandidates(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	det := &Evidence{Kind: EvidenceDeterministic}
	require.NoError(t, store.Append(LearningEntry{ID: "L-001", Timestamp: time.Now(), Pattern: "reopen outputs", Scope: ScopeProcess, Evidence: det, ReuseCount: 2}))
	require.NoError(t, store.Append(LearningEntry{ID: "L-002", Timestamp: time.Now(), Pattern: "process opinion", Scope: ScopeProcess, Evidence: &Evidence{Kind: EvidenceLLMOnly}}))
	require.NoError(t, store.Append(LearningEntry{ID: "L-003", Timestamp: time.Now(), Pattern: "project fact", Evidence: det, Expected: "e", Actual: "a"}))

	s, err := GenerateSummary(store, 5)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{ScopeProcess: 2, ScopeProject: 1}, s.ScopeCounts)
	assert.Equal(t, map[EvidenceKind]int{EvidenceDeterministic: 2, EvidenceLLMOnly: 1}, s.EvidenceCounts)
	assert.Equal(t, []PatternStat{{Pattern: "reopen outputs", ReuseCount: 2}}, s.TemplateCandidates)
	assert.Equal(t, 1, s.IntakeEligible)
}

func sha256File(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return sha256.Sum256(data)
}

func scoresOf(in []ScoredEntry) []float64 {
	out := make([]float64, len(in))
	for i, e := range in {
		out[i] = e.Score
	}
	return out
}

func idsOf(in []ScoredEntry) []string {
	out := make([]string, len(in))
	for i, e := range in {
		out[i] = e.Entry.ID
	}
	return out
}

func entryIDs(in []LearningEntry) []string {
	out := make([]string, len(in))
	for i, e := range in {
		out[i] = e.ID
	}
	return out
}
