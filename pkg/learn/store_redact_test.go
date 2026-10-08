package learn

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const redactedSecret = "[REDACTED_SECRET]"

func TestStoreWriter_RedactsEveryFreeTextField(t *testing.T) {
	t.Parallel()
	store, path := newTestStore(t)
	token := synthToken()

	require.NoError(t, store.AppendAtomic(EntryTypeReviewIssue, RecordOpts{
		Phase:      "review " + token,
		SpecID:     token,
		Files:      []string{"/Users/alice/pkg/a.go"},
		Packages:   []string{"pkg/content"},
		Pattern:    "p " + token,
		Resolution: "rotated " + token,
		Expected:   "no " + token,
		Actual:     token + " leaked",
		Repro:      "auto init --token " + token,
	}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), token, "no raw token byte reaches the store")

	var got LearningEntry
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got))
	assert.Equal(t, "review "+redactedSecret, got.Phase)
	assert.Equal(t, redactedSecret, got.SpecID)
	assert.Equal(t, "p "+redactedSecret, got.Pattern)
	assert.Equal(t, "rotated "+redactedSecret, got.Resolution)
	assert.Equal(t, "no "+redactedSecret, got.Expected)
	assert.Equal(t, redactedSecret+" leaked", got.Actual)
	assert.Equal(t, "auto init --token "+redactedSecret, got.Repro)
	// Files and packages feed the fingerprint and stay verbatim.
	assert.Equal(t, []string{"/Users/alice/pkg/a.go"}, got.Files)
	assert.Equal(t, []string{"pkg/content"}, got.Packages)
}

func TestStoreAppend_DirectEntry_PassesTheSameBoundary(t *testing.T) {
	t.Parallel()
	store, path := newTestStore(t)
	token := synthToken()

	require.NoError(t, store.Append(LearningEntry{
		ID: "L-001", Timestamp: time.Now(), Type: EntryTypeGateFail,
		Pattern: "p " + token, SpecID: token,
	}))
	err := store.Append(LearningEntry{ID: "L-002", Timestamp: time.Now(), Pattern: "p", Repro: "a\nb"})
	require.Error(t, err, "Append validates evidence like AppendAtomic")

	lines := readLines(t, path)
	require.Len(t, lines, 1)
	assert.NotContains(t, lines[0], token)
	assert.Contains(t, lines[0], `"pattern":"p `+redactedSecret+`"`)
	assert.Contains(t, lines[0], `"spec_id":"`+redactedSecret+`"`)
}

func TestStoreWriter_SamePhraseTwice_SameRedactedBytes(t *testing.T) {
	t.Parallel()
	store, path := newTestStore(t)
	phrase := "deploy failed token=" + strings.Repeat("Q7", 6) + " in ci"

	for range 2 {
		require.NoError(t, RecordFixPattern(store, RecordOpts{Pattern: phrase, Actual: phrase}))
	}

	entries, err := store.Read()
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "deploy failed "+redactedSecret+" in ci", entries[0].Pattern)
	assert.Equal(t, entries[0].Pattern, entries[1].Pattern)
	assert.Equal(t, entries[0].Actual, entries[1].Actual)
	assert.NotContains(t, strings.Join(readLines(t, path), "\n"), "Q7Q7")
}

// TestRewrite_RedactsSkipLinesButNotParsedEntries pins the rewrite contract:
// a line that never parsed never crossed the writer, so the rewrite redacts it;
// a parsed entry is re-encoded as stored, so a legacy raw value and its
// fingerprint input survive prune unchanged.
func TestRewrite_RedactsSkipLinesButNotParsedEntries(t *testing.T) {
	t.Parallel()
	store, path := newTestStore(t)
	token := synthToken()
	recent := time.Now().UTC().Format(time.RFC3339)
	aged := time.Now().Add(-60 * 24 * time.Hour).UTC().Format(time.RFC3339)
	legacy := fmt.Sprintf(`{"id":"L-001","timestamp":%q,"type":"gate_fail","phase":"ci","files":null,"packages":null,"pattern":"deploy failed token=abc123 in ci","resolution":"","severity":"","reuse_count":0}`, recent)
	broken := `{"id":"L-BAD","pattern":"leak ` + token + ` here"`
	old := fmt.Sprintf(`{"id":"L-002","timestamp":%q,"pattern":"old"}`, aged)
	require.NoError(t, os.WriteFile(path, []byte(legacy+"\n"+broken+"\n"+old+"\n"), 0o644))

	removed, err := Prune(store, 30)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	lines := readLines(t, path)
	require.Len(t, lines, 2)
	var kept LearningEntry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &kept))
	assert.Equal(t, "deploy failed token=abc123 in ci", kept.Pattern, "parsed entries are not redacted again")
	assert.Equal(t, `{"id":"L-BAD","pattern":"leak `+redactedSecret+` here"`, lines[1])
	assert.NotContains(t, strings.Join(lines, "\n"), token)

	// UpdateReuseCount shares the rewrite and keeps the same contract.
	require.NoError(t, store.UpdateReuseCount("L-001"))
	lines = readLines(t, path)
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &kept))
	assert.Equal(t, "deploy failed token=abc123 in ci", kept.Pattern)
	assert.Equal(t, 1, kept.ReuseCount)
}
