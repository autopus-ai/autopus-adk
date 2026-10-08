package healthband

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lpWriteLines(t *testing.T, store *Store, lines ...string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(store.Dir(), 0o700))
	require.NoError(t, os.WriteFile(store.Path(LocalPatchEventsFile), []byte(strings.Join(lines, "\n")+"\n"), 0o600))
}

func lpJSON(t *testing.T, record LocalPatchRecord) string {
	t.Helper()
	record.Schema = SchemaLocalPatch
	data, err := json.Marshal(record)
	require.NoError(t, err)
	return string(data)
}

func TestReadLocalPatchLog_TamperedLines_AreSkippedAndCounted(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	good := lpClaim(lpTestLocation, lpT0)
	good.Seq = 3
	escape := good
	escape.Seq, escape.WorktreePath = 4, "/cache/\x1b[31mred"
	badStatus := NewLocalPatchResult(lpClaimID, "")
	badStatus.Seq, badStatus.Status = 5, "failed:has space"
	badKind := lpDecision()
	badKind.Seq, badKind.Kind = 6, "checkpoint"
	badReason := lpDecision()
	badReason.Seq, badReason.Decision, badReason.Reason = 7, LocalPatchDecideSkip, "cap_reached"
	lpWriteLines(t, store, `{"schema":"other","seq":1,"kind":"decision"}`, "not json", lpJSON(t, good), lpJSON(t, escape),
		lpJSON(t, badStatus), lpJSON(t, badKind), lpJSON(t, badReason))
	log, err := store.ReadLocalPatchLog()
	require.NoError(t, err)
	assert.Equal(t, 6, log.Skipped)
	require.Len(t, log.Records, 1)
	assert.Equal(t, int64(3), log.Records[0].Seq)
	assert.Equal(t, int64(4), log.nextSeq)
}

func TestReadLocalPatchLog_RecordsNewerThanTheState_AreFolded(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	appendPhaseA(t, store, lpDecision())
	claim := lpClaim(lpTestLocation, lpT0)
	claim.Seq = 2
	data, err := os.ReadFile(store.Path(LocalPatchEventsFile))
	require.NoError(t, err)
	lpWriteLines(t, store, strings.TrimSuffix(string(data), "\n"), lpJSON(t, claim))
	log, err := store.ReadLocalPatchLog()
	require.NoError(t, err)
	assert.Equal(t, int64(2), log.State.LastSeq)
	assert.True(t, log.EpisodePatched(lpSeries, lpEpisode))
}

func TestReadLocalPatchLog_InvalidState_IsRebuiltFromTheLog(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	appendPhaseA(t, store, lpDecision(), lpClaim(lpTestLocation, lpT0))
	for _, state := range []string{`{"schema":"other"}`, `{"schema":"` + SchemaLocalPatchState + `","last_seq":0,"series":{"bad id":[]}}`, "{"} {
		require.NoError(t, os.WriteFile(store.Path(LocalPatchStateFile), []byte(state), 0o600))
		log, err := store.ReadLocalPatchLog()
		require.NoError(t, err)
		assert.True(t, log.EpisodePatched(lpSeries, lpEpisode), state)
	}
}

func TestReadLocalPatchLog_BSNotTier3Decision_IsKeptInTheState(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	skip := lpDecision()
	skip.Decision, skip.Reason = LocalPatchDecideSkip, LocalPatchSkippedBSNotTier3
	appendPhaseA(t, store, skip)
	log, err := store.ReadLocalPatchLog()
	require.NoError(t, err)
	assert.True(t, log.EpisodeBSNotTier3(lpSeries, lpEpisode))
	assert.False(t, log.EpisodePatched(lpSeries, lpEpisode))
	assert.False(t, log.EpisodeBSNotTier3(lpSeries, "e9"))
}

// lpNthClaim is the claim record of the i-th episode of the series.
func lpNthClaim(i int, lease time.Time) LocalPatchRecord {
	claim := lpClaim(lpTestLocation, lease)
	claim.ClaimID = fmt.Sprintf("%032x", i+1)
	claim.EpisodeID = fmt.Sprintf("e%d", 1000+i)
	claim.Key = LocalPatchKey(lpSeries, claim.EpisodeID, claim.ClaimID)
	return claim
}

func TestCompactLocalPatch_PastTheBound_KeepsNonTerminalClaimsAndTheirState(t *testing.T) {
	maxRecords, maxEpisodes := maxLocalPatchRecords, maxLocalPatchEpisodes
	maxLocalPatchRecords, maxLocalPatchEpisodes = 4, 3
	t.Cleanup(func() { maxLocalPatchRecords, maxLocalPatchEpisodes = maxRecords, maxEpisodes })
	store := NewStore(t.TempDir())
	ctx := context.Background()
	appendPhaseA(t, store, lpNthClaim(0, lpT0), lpNthClaim(1, lpT0)) // claim 1 stays non-terminal
	_, err := store.AppendLocalPatchResult(ctx, NewLocalPatchResult(fmt.Sprintf("%032x", 1), ""))
	require.NoError(t, err)
	for i := 2; i < 6; i++ {
		appendPhaseA(t, store, lpNthClaim(i, lpT0))
		_, err := store.AppendLocalPatchResult(ctx, NewLocalPatchResult(fmt.Sprintf("%032x", i+1), ""))
		require.NoError(t, err)
	}
	appendPhaseA(t, store, lpDecision())
	log, err := store.ReadLocalPatchLog()
	require.NoError(t, err)
	assert.Equal(t, 0, log.Skipped)
	var ids []string
	for _, record := range log.Records {
		ids = append(ids, record.RecordClaimID())
	}
	assert.Contains(t, ids, fmt.Sprintf("%032x", 2), "the non-terminal claim keeps its record")
	assert.NotContains(t, ids, fmt.Sprintf("%032x", 1), "an ended claim outside the window is dropped")
	assert.LessOrEqual(t, len(log.Records), 6)
	assert.True(t, log.Ended(fmt.Sprintf("%032x", 4)), "the state keeps the end of a compacted claim")
	assert.True(t, log.EpisodePatched(lpSeries, "e1001"), "an episode with a claimed claim stays in the state")
	assert.Equal(t, log.Records[len(log.Records)-1].Seq+1, log.nextSeq)
}
