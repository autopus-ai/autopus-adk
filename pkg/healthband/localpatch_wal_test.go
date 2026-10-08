package healthband

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	lpDiagnoseID = "0f0e0d0c0b0a09080706050403020100"
	lpOwner      = "test-host:4242:0123456789abcdef"
	lpBase       = "1111111111111111111111111111111111111111"
)

var lpT0 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// lpDecision and lpClaim are the phase A records of the acceptance fixture.
func lpDecision() LocalPatchRecord {
	return LocalPatchRecord{Kind: LocalPatchKindDecision, Series: lpSeries, EpisodeID: lpEpisode, EvaluationSeq: 42, Decision: LocalPatchDecideClaim}
}

func lpClaim(loc LocalPatchLocation, lease time.Time) LocalPatchRecord {
	paths := loc.Paths(lpFixtureK)
	return LocalPatchRecord{
		Kind: LocalPatchKindClaim, ClaimID: lpClaimID, Owner: lpOwner, LeaseUntil: lease, Series: lpSeries,
		EpisodeID: lpEpisode, DependsOn: lpDiagnoseID, Key: lpFixtureK, WorktreePath: paths.Worktree,
		PatchPath: paths.Patch, Branch: paths.Branch,
	}
}

func lpPrep(code string) LocalPatchRecord {
	due := DueClaim{Claim: Claim{ID: lpDiagnoseID, LeaseUntil: lpT0.Add(990 * time.Second)}, Series: lpSeries, EpisodeID: lpEpisode}
	return NewLocalPatchPrep(due, lpClaimID, lpFixtureK, lpBase, code)
}

var lpTestLocation = LocalPatchLocation{CacheDir: "/cache", RepoHash: "0123456789ab", Path: "/cache/autopus/local-patches/0123456789ab"}

// appendPhaseA appends records under the store lock as phase A does.
func appendPhaseA(t *testing.T, store *Store, records ...LocalPatchRecord) []LocalPatchRecord {
	t.Helper()
	locked, err := store.Lock(context.Background(), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, locked.Unlock()) }()
	appended, err := locked.AppendLocalPatch(records...)
	require.NoError(t, err)
	return appended
}

func lpLines(t *testing.T, store *Store) []string {
	t.Helper()
	data, err := os.ReadFile(store.Path(LocalPatchEventsFile))
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestAppendLocalPatch_PhaseARecords_AssignSeqAndFoldIntoTheState(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	lease := time.Date(2026, 10, 6, 21, 30, 0, 0, time.FixedZone("KST", 9*3600))
	appended := appendPhaseA(t, store, lpDecision(), lpClaim(lpTestLocation, lease))
	require.Len(t, appended, 2)
	assert.Equal(t, []int64{1, 2}, []int64{appended[0].Seq, appended[1].Seq})
	log, err := store.ReadLocalPatchLog()
	require.NoError(t, err)
	require.Len(t, log.Records, 2)
	assert.Equal(t, SchemaLocalPatch, log.Records[1].Schema)
	assert.Equal(t, lease.UTC(), log.Records[1].LeaseUntil)
	assert.Equal(t, time.UTC, log.Records[1].LeaseUntil.Location())
	assert.True(t, log.EpisodePatched(lpSeries, lpEpisode))
	assert.False(t, log.Ended(lpClaimID))
	state, _, err := readLocalPatchState(store.Path(LocalPatchStateFile))
	require.NoError(t, err)
	assert.Equal(t, int64(2), state.LastSeq)
	assert.Equal(t, []LocalPatchStateClaim{{ID: lpClaimID, Key: lpFixtureK, DependsOn: lpDiagnoseID, LeaseUntil: lease.UTC(), Status: ClaimClaimed}},
		state.Series[lpSeries][0].Claims)
	assert.Contains(t, lpLines(t, store)[0], `"kind":"decision"`)
}

func TestAppendLocalPatch_OtherKindOrInvalidRecord_WritesNothing(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	locked, err := store.Lock(context.Background(), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, locked.Unlock()) }()
	bad := lpClaim(lpTestLocation, lpT0)
	bad.ClaimID = "A1B2"
	for _, records := range [][]LocalPatchRecord{
		{lpDecision(), NewLocalPatchResult(lpClaimID, "")},
		{lpDecision(), bad},
		{lpPrep(LocalPatchCodeOK)},
	} {
		_, err := locked.AppendLocalPatch(records...)
		assert.ErrorIs(t, err, ErrInvalidLocalPatchRecord)
	}
	_, err = os.Stat(store.Path(LocalPatchEventsFile))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestAppendLocalPatchPrep_ResultFoundByTheReRead_AppendsNothing(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	ctx := context.Background()
	appendPhaseA(t, store, lpDecision(), lpClaim(lpTestLocation, lpT0.Add(1800*time.Second)))
	ended, err := store.AppendLocalPatchResult(ctx, NewLocalPatchResult(lpClaimID, LocalPatchCodeInterrupted))
	require.NoError(t, err)
	require.True(t, ended)
	appended, err := store.AppendLocalPatchPrep(ctx, lpPrep(LocalPatchCodeOK))
	require.NoError(t, err)
	assert.False(t, appended)
	assert.Len(t, lpLines(t, store), 3)
}

func TestAppendLocalPatchPrep_NoResult_AppendsOnePrep(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	appended, err := store.AppendLocalPatchPrep(context.Background(), lpPrep(GitBaseUnavailable))
	require.NoError(t, err)
	assert.True(t, appended)
	log, err := store.ReadLocalPatchLog()
	require.NoError(t, err)
	require.Len(t, log.ClaimRecords(lpClaimID), 1)
	prep := log.ClaimRecords(lpClaimID)[0]
	assert.Equal(t, LocalPatchKindPrep, prep.Kind)
	assert.Equal(t, lpDiagnoseID, prep.DiagnoseClaimID)
	assert.Equal(t, lpT0.Add(990*time.Second), prep.LeaseUntil)
	assert.Equal(t, GitBaseUnavailable, prep.Code)
}

func TestAppendLocalPatchResult_SecondEnd_IsDropped(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	ctx := context.Background()
	appendPhaseA(t, store, lpDecision(), lpClaim(lpTestLocation, lpT0))
	first, err := store.AppendLocalPatchResult(ctx, NewLocalPatchResult(lpClaimID, ""))
	require.NoError(t, err)
	second, err := store.AppendLocalPatchResult(ctx, NewLocalPatchResult(lpClaimID, LocalPatchCodeInterrupted))
	require.NoError(t, err)
	assert.True(t, first)
	assert.False(t, second)
	log, err := store.ReadLocalPatchLog()
	require.NoError(t, err)
	result, found := log.Result(lpClaimID)
	require.True(t, found)
	assert.Equal(t, ClaimDone, result.Status)
	assert.True(t, log.Ended(lpClaimID))
	assert.Equal(t, ClaimDone, log.State.Series[lpSeries][0].Claims[0].Status)
}

func TestAppendLocalPatchStage_StoreLockBusy_ReportsErrStoreLocked(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	locked, err := store.Lock(context.Background(), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, locked.Unlock()) }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stage := NewLocalPatchStage(lpClaimID, StageWorktreeIntent)
	stage.Path = lpTestLocation.Paths(lpFixtureK).Worktree
	err = store.AppendLocalPatchStage(ctx, stage)
	assert.Error(t, err)
	_, statErr := os.Stat(store.Path(LocalPatchEventsFile))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestAppendLocalPatchStage_PhaseFieldMissing_IsRefused(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	ctx := context.Background()
	for _, phase := range []string{StageWorktreeIntent, StageWorktreeDone, StageWorktreeFailed, StageMessage, StageApplyIntent,
		StageApplyDone, StageCommitDone, StageBranchIntent, StagePatchIntent, StagePatchDone, "checkout"} {
		assert.ErrorIs(t, store.AppendLocalPatchStage(ctx, NewLocalPatchStage(lpClaimID, phase)), ErrInvalidLocalPatchRecord, phase)
	}
	branchDone := NewLocalPatchStage(lpClaimID, StageBranchDone)
	require.NoError(t, store.AppendLocalPatchStage(ctx, branchDone))
	assert.ErrorIs(t, store.AppendLocalPatchStage(ctx, lpPrep(LocalPatchCodeOK)), ErrInvalidLocalPatchRecord)
	assert.ErrorIs(t, store.AppendLocalPatchStage(ctx, NewLocalPatchResult(lpClaimID, "")), ErrInvalidLocalPatchRecord)
}
