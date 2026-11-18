package learn

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordEscape(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	assert.ErrorIs(t, RecordEscape(store, RecordOpts{Pattern: "p"}), ErrEscapeIncomplete)
	assert.ErrorIs(t, RecordEscape(store, RecordOpts{Pattern: "p", Escape: &Escape{MissedBy: "lint"}}), ErrEscapeIncomplete)

	before, after := 1, 0
	require.NoError(t, RecordEscape(store, RecordOpts{
		Pattern: "publish follows a symlink", EvidenceCommand: "./repro.sh", EvidenceBeforeExit: &before, EvidenceAfterExit: &after,
		Escape: &Escape{MissedBy: "unit tests", WhenPaths: []string{"src/**"}, Commit: "abc", Provider: "claude"},
	}))
	entries, err := store.Read()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, EntryTypeEscape, e.Type)
	assert.Equal(t, EvidenceDeterministic, e.EvidenceKindOf())
	assert.Equal(t, []string{"src/**"}, e.Escape.WhenPaths)
	assert.Equal(t, "abc", e.Escape.Commit)

	long := make([]byte, escapeWhenCap+1)
	for i := range long {
		long[i] = 'a'
	}
	path := store.path
	sum, _ := os.ReadFile(path)
	err = RecordEscape(store, RecordOpts{Pattern: "p", Escape: &Escape{MissedBy: "x", WhenPaths: []string{string(long)}}})
	var fe *FieldError
	require.ErrorAs(t, err, &fe)
	assert.Equal(t, FieldWhenPath, fe.Field)
	now, _ := os.ReadFile(path)
	assert.Equal(t, sum, now, "a refused record writes nothing")
}
