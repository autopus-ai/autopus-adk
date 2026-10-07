package healthband_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// plannedClaim runs phase A on the O2 fixture and returns its one claim.
func plannedClaim(t *testing.T, dir string) healthband.DueClaim {
	t.Helper()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	run := phaseA(t, dir, bandT0)
	require.Len(t, run.plan.Claims, 1)
	return run.plan.Claims[0]
}

func resultOf(claim healthband.DueClaim, outcome healthband.ClaimOutcome) healthband.Result {
	return healthband.Result{Claim: claim.Claim, Series: claim.Series, SampleKey: claim.SampleKey, EpisodeID: claim.EpisodeID, ClaimOutcome: outcome}
}

// S7: a phase C that cannot take the lock writes pending/<claim-id>.json;
// the next run appends exactly 1 action_result event for it and deletes the
// file, and a file left by a crash after that append is never appended twice.
func TestRecordResult_S7PendingResultIsAppendedExactlyOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	holder, err := filelock.Acquire(context.Background(), metricsPath(dir, healthband.LockFile), time.Second)
	require.NoError(t, err)

	recorded, err := healthband.NewStore(dir).ExecuteClaims(context.Background(), []healthband.DueClaim{claim}, (&fakeProvider{}).run,
		healthband.ExecuteOptions{Clock: func() time.Time { return bandT0.Add(20 * time.Second) }, ResultWait: 50 * time.Millisecond})

	require.NoError(t, err)
	require.NoError(t, holder.Unlock())
	pending := metricsPath(dir, filepath.Join(healthband.PendingDir, claim.ID+".json"))
	require.Len(t, recorded, 1)
	assert.Equal(t, pending, recorded[0].Pending)
	assert.Zero(t, recorded[0].Seq)
	assert.Empty(t, eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult))
	saved, err := os.ReadFile(pending)
	require.NoError(t, err)

	phaseA(t, dir, bandT0.Add(time.Minute))

	results := eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult)
	require.Len(t, results, 1)
	assert.Equal(t, claim.ID, results[0].ClaimID)
	assert.Empty(t, results[0].Reasons, "the pending result reached a claimed claim before its lease expired")
	assert.NoFileExists(t, pending)
	state := storedCheckpoint(t, dir)
	assert.Equal(t, "done", claimStatus(state, seriesCI, claim.ID))
	assert.Equal(t, "BS-BAND-001", state.Series[seriesCI].Episodes[0].BSID)

	require.NoError(t, os.WriteFile(pending, saved, 0o600))
	phaseA(t, dir, bandT0.Add(2*time.Minute))

	assert.Len(t, eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult), 1)
	assert.NoFileExists(t, pending)
}

// writePendingFor holds the store lock so phase C must persist the result.
func writePendingFor(t *testing.T, dir string, result healthband.Result, at time.Time) string {
	t.Helper()
	holder, err := filelock.Acquire(context.Background(), metricsPath(dir, healthband.LockFile), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, holder.Unlock()) }()
	recorded, err := healthband.NewStore(dir).RecordResult(context.Background(), result, at, 20*time.Millisecond)
	require.NoError(t, err)
	require.NotEmpty(t, recorded.Pending)
	return recorded.Pending
}

// Pending results are appended before leases expire (Durability item 2):
// a finished claim whose next run comes after its lease is done, never
// interrupted first.
func TestOpenWAL_AppendsPendingResultsBeforeLeasesExpire(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	writePendingFor(t, dir, resultOf(claim, healthband.ClaimOutcome{BSID: "BS-BAND-001"}), bandT0.Add(900*time.Second))

	run := phaseA(t, dir, bandT0.Add(2000*time.Second))

	assert.Empty(t, run.interrupted)
	results := eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Reasons)
	assert.Equal(t, "done", claimStatus(storedCheckpoint(t, dir), seriesCI, claim.ID))
}

// A pending result whose claim is unknown is appended once as
// claim_unknown; the same file left by a crash is never appended again,
// although its claim is still unknown.
func TestOpenWAL_PendingResultForAnUnknownClaimIsAppendedOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stranger := plannedClaim(t, dir)
	stranger.ID = "0123456789abcdef0123456789abcdef"
	pending := writePendingFor(t, dir, resultOf(stranger, healthband.ClaimOutcome{}), bandT0.Add(time.Minute))
	saved, err := os.ReadFile(pending)
	require.NoError(t, err)

	phaseA(t, dir, bandT0.Add(2*time.Minute))
	require.NoError(t, os.WriteFile(pending, saved, 0o600))
	phaseA(t, dir, bandT0.Add(3*time.Minute))

	results := eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult)
	require.Len(t, results, 1)
	assert.Equal(t, []string{"claim_unknown"}, results[0].Reasons)
	assert.NoFileExists(t, pending)
}

// A pending file that does not parse, names another claim, or sits beside a
// stray name is skipped and left alone; nothing is appended for it.
func TestOpenWAL_SkipsPendingFilesOutsideTheContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	other := "ffffffffffffffffffffffffffffffff"
	writeMetricsFile(t, dir, filepath.Join(healthband.PendingDir, claim.ID+".json"), `{"claim":`)
	writeMetricsFile(t, dir, filepath.Join(healthband.PendingDir, other+".json"),
		`{"claim":{"id":"`+claim.ID+`","kind":"diagnose","owner":"`+testOwner+`","lease_until":"2026-10-06T12:15:30Z"},`+
			`"series":"ci.failure_rate:CI","sample_key":"1042","episode_id":"e1042","status":"done"}`)
	writeMetricsFile(t, dir, filepath.Join(healthband.PendingDir, "notes.txt"), "x")

	locked := lockStore(t, dir)
	wal, err := locked.OpenWAL(bandT0.Add(time.Minute))
	require.NoError(t, err)

	assert.Equal(t, 2, wal.Skipped)
	assert.Empty(t, eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult))
	assert.FileExists(t, metricsPath(dir, filepath.Join(healthband.PendingDir, other+".json")))
}

// A second result for a claim that already ended appends nothing.
func TestRecordResult_SecondResultForAnEndedClaimIsADuplicate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	store := healthband.NewStore(dir)
	failed := resultOf(claim, healthband.ClaimOutcome{Status: "failed:bs_lock_timeout", DiagnosisStatus: "ok", BSID: "BS-BAND-004"})

	first, err := store.RecordResult(context.Background(), failed, bandT0.Add(time.Minute), time.Second)
	require.NoError(t, err)
	second, err := store.RecordResult(context.Background(), resultOf(claim, healthband.ClaimOutcome{BSID: "BS-BAND-001"}), bandT0.Add(2*time.Minute), time.Second)
	require.NoError(t, err)

	assert.False(t, first.Duplicate)
	assert.True(t, second.Duplicate)
	assert.Len(t, eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult), 1)
	state := storedCheckpoint(t, dir)
	assert.Equal(t, "failed:bs_lock_timeout", claimStatus(state, seriesCI, claim.ID))
	assert.Empty(t, state.Series[seriesCI].Episodes[0].BSID, "a failed claim leaves the episode without a BS")
}

// A result for a claim nobody planned is appended as claim_unknown and
// changes no state.
func TestRecordResult_UnknownClaimIsAppendedWithoutChangingState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	before := storedCheckpoint(t, dir)
	stranger := claim
	stranger.ID = "0123456789abcdef0123456789abcdef"

	recorded, err := healthband.NewStore(dir).RecordResult(context.Background(), resultOf(stranger, healthband.ClaimOutcome{}), bandT0.Add(time.Minute), time.Second)

	require.NoError(t, err)
	assert.Equal(t, "claim_unknown", recorded.Reason)
	assert.Equal(t, before.Series, storedCheckpoint(t, dir).Series)
}

// The newest episode is always kept, but its interrupted claim still takes
// a late result only within 24 h of the interruption.
func TestRecordResult_NewestEpisodeResultAfterTheWindowIsClaimUnknown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	interruptedAt := bandT0.Add(931 * time.Second)
	require.Equal(t, []string{claim.ID}, phaseA(t, dir, interruptedAt).interrupted)

	recorded, err := healthband.NewStore(dir).RecordResult(context.Background(), resultOf(claim, healthband.ClaimOutcome{}), interruptedAt.Add(24*time.Hour), time.Second)

	require.NoError(t, err)
	assert.Equal(t, "claim_unknown", recorded.Reason)
	assert.Equal(t, "interrupted", claimStatus(storedCheckpoint(t, dir), seriesCI, claim.ID))
}

// Results outside the contract are refused before anything is written, so
// no untrusted string reaches an event, the checkpoint, or a pending file.
func TestRecordResult_RejectsResultsOutsideTheContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	badID := claim
	badID.ID = "../../escape"
	for name, result := range map[string]healthband.Result{
		"claim id":         resultOf(badID, healthband.ClaimOutcome{}),
		"status":           resultOf(claim, healthband.ClaimOutcome{Status: "failed:Bad Reason"}),
		"BS id":            resultOf(claim, healthband.ClaimOutcome{BSID: "BS-042"}),
		"diagnosis status": resultOf(claim, healthband.ClaimOutcome{DiagnosisStatus: "ok\x1b[2J"}),
	} {
		_, err := healthband.NewStore(dir).RecordResult(context.Background(), result, bandT0.Add(time.Minute), time.Second)
		assert.Error(t, err, name)
	}
	assert.Empty(t, eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult))
	assert.NoDirExists(t, metricsPath(dir, healthband.PendingDir))
}

// A cancelled run starts no further claim.
func TestExecuteClaims_StopsAtACancelledContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &fakeProvider{}

	recorded, err := healthband.NewStore(dir).ExecuteClaims(ctx, []healthband.DueClaim{claim}, provider.run, healthband.ExecuteOptions{})

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, recorded)
	assert.Zero(t, provider.callCount())
}

// Owner tokens are <hostname>:<pid>:<16 hex> and claim ids 32 hex digits,
// both different on every call.
func TestNewOwnerAndNewClaimID_FollowTheTokenFormat(t *testing.T) {
	t.Parallel()
	owner := healthband.NewOwner()
	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9.-]{1,64}:`+strconv.Itoa(os.Getpid())+`:[0-9a-f]{16}$`), owner)
	assert.NotEqual(t, owner, healthband.NewOwner())
	id := healthband.NewClaimID()
	assert.Regexp(t, `^[0-9a-f]{32}$`, id)
	assert.NotEqual(t, id, healthband.NewClaimID())
}
