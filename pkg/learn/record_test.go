package learn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordGateFail_CreatesEntry(t *testing.T) {
	t.Parallel()

	// Given: a store
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	// When: recording a gate failure
	err = RecordGateFail(store, RecordOpts{
		Phase:      "build",
		Files:      []string{"pkg/learn/store.go"},
		Packages:   []string{"learn"},
		Pattern:    "lint check failed",
		Resolution: "fixed lint errors",
		Severity:   SeverityHigh,
	})

	// Then: entry created with correct type
	require.NoError(t, err)
	entries, err := store.Read()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, EntryTypeGateFail, entries[0].Type)
	assert.Equal(t, "build", entries[0].Phase)
	assert.Equal(t, SeverityHigh, entries[0].Severity)
	assert.NotEmpty(t, entries[0].ID)
	assert.False(t, entries[0].Timestamp.IsZero())
}

func TestRecordCoverageGap_CreatesEntryWithDelta(t *testing.T) {
	t.Parallel()

	// Given: a store
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	// When: recording a coverage gap
	err = RecordCoverageGap(store, RecordOpts{
		Phase:      "test",
		Files:      []string{"pkg/learn/query.go"},
		Packages:   []string{"learn"},
		Pattern:    "coverage 72% < 85% threshold",
		Resolution: "added edge case tests",
		Severity:   SeverityMedium,
	})

	// Then: entry has coverage_gap type
	require.NoError(t, err)
	entries, err := store.Read()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, EntryTypeCoverageGap, entries[0].Type)
}

func TestRecordReviewIssue_CreatesEntryWithPattern(t *testing.T) {
	t.Parallel()

	// Given: a store
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	// When: recording a review issue
	err = RecordReviewIssue(store, RecordOpts{
		Phase:      "review",
		Files:      []string{"pkg/learn/prune.go"},
		Packages:   []string{"learn"},
		Pattern:    "missing error check on file close",
		Resolution: "added defer close with error check",
		Severity:   SeverityMedium,
	})

	// Then: entry has review_issue type
	require.NoError(t, err)
	entries, err := store.Read()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, EntryTypeReviewIssue, entries[0].Type)
	assert.Equal(t, "missing error check on file close", entries[0].Pattern)
}

func TestRecordExecutorError_CreatesEntry(t *testing.T) {
	t.Parallel()

	// Given: a store
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	// When: recording an executor error
	err = RecordExecutorError(store, RecordOpts{
		Phase:      "implement",
		Files:      []string{"pkg/learn/summary.go"},
		Packages:   []string{"learn"},
		Pattern:    "nil pointer dereference in summary",
		Resolution: "added nil guard",
		Severity:   SeverityCritical,
	})

	// Then: entry has executor_error type
	require.NoError(t, err)
	entries, err := store.Read()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, EntryTypeExecutorError, entries[0].Type)
	assert.Equal(t, SeverityCritical, entries[0].Severity)
}

func TestRecordFixPattern_CreatesEntry(t *testing.T) {
	t.Parallel()

	// Given: a store
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	// When: recording a fix pattern
	err = RecordFixPattern(store, RecordOpts{
		Phase:      "fix",
		Files:      []string{"pkg/learn/store.go"},
		Packages:   []string{"learn"},
		Pattern:    "file not closed after write",
		Resolution: "use defer f.Close()",
		Severity:   SeverityMedium,
	})

	// Then: entry has fix_pattern type
	require.NoError(t, err)
	entries, err := store.Read()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, EntryTypeFixPattern, entries[0].Type)
}

func TestRecordGateFail_AutoGeneratesID(t *testing.T) {
	t.Parallel()

	// Given: a store with existing entries
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	// When: recording two entries
	require.NoError(t, RecordGateFail(store, RecordOpts{
		Phase: "build", Pattern: "first", Severity: SeverityLow,
	}))
	require.NoError(t, RecordGateFail(store, RecordOpts{
		Phase: "build", Pattern: "second", Severity: SeverityLow,
	}))

	// Then: IDs are auto-incremented
	entries, err := store.Read()
	require.NoError(t, err)
	assert.Regexp(t, `^L-001-[0-9a-f]{6}$`, entries[0].ID)
	assert.Regexp(t, `^L-002-[0-9a-f]{6}$`, entries[1].ID)
}

func TestRecordGateFail_NilStore(t *testing.T) {
	t.Parallel()

	// When: recording with nil store
	err := RecordGateFail(nil, RecordOpts{
		Phase: "build", Pattern: "test", Severity: SeverityLow,
	})

	// Then: error returned
	assert.Error(t, err)
}

func TestRecordGateFail_EmptyPattern(t *testing.T) {
	t.Parallel()

	// Given: a store
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	// When: recording with empty pattern
	err = RecordGateFail(store, RecordOpts{
		Phase:    "build",
		Pattern:  "",
		Severity: SeverityLow,
	})

	// Then: error for missing required field
	assert.Error(t, err)
}
