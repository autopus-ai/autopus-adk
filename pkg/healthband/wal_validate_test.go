package healthband_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

func marshalLine(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

// A repository can commit a tampered log; every line outside what band
// writes is skipped, so no foreign string or tier reaches the checkpoint.
func TestOpenWAL_SkipsTamperedEventLines(t *testing.T) {
	t.Parallel()
	seven, interrupted := 7, bandT0
	for name, tamper := range map[string]func(*healthband.Event){
		"schema":            func(e *healthband.Event) { e.Schema = "autopus.band_evaluation.v2" },
		"seq":               func(e *healthband.Event) { e.Seq = 0 },
		"kind":              func(e *healthband.Event) { e.Kind = "patch" },
		"action":            func(e *healthband.Event) { e.Action = "draft_pr" },
		"episode id":        func(e *healthband.Event) { e.EpisodeID = "e\x1b[2J" },
		"max tier":          func(e *healthband.Event) { e.MaxTier = &seven },
		"BS id":             func(e *healthband.Event) { e.BSID = "BS-1" },
		"claim id":          func(e *healthband.Event) { e.Claims[0].ID = "zz" },
		"claim owner":       func(e *healthband.Event) { e.Claims[0].Owner = "root" },
		"claim interrupted": func(e *healthband.Event) { e.Claims[0].InterruptedAt = &interrupted },
		"result claims":     func(e *healthband.Event) { e.Kind, e.ClaimID = healthband.EventKindActionResult, e.Claims[0].ID },
		"result status": func(e *healthband.Event) {
			e.Kind, e.ClaimID, e.Claims[0].Status = healthband.EventKindActionResult, e.Claims[0].ID, "claimed"
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			claim := plannedClaim(t, dir)
			event := claim.Event
			event.Seq, event.Claims = 2, []healthband.Claim{claim.Claim}
			tamper(&event)
			writeMetricsFile(t, dir, healthband.EventsFile, marshalLine(t, claim.Event)+"\n"+marshalLine(t, event)+"\n")
			require.NoError(t, os.Remove(metricsPath(dir, healthband.StateFile)))

			wal, err := lockStore(t, dir).OpenWAL(bandT0.Add(time.Minute))

			require.NoError(t, err)
			assert.Equal(t, 1, wal.Skipped)
			state := wal.Checkpoint()
			assert.Equal(t, int64(1), state.LastSeq)
			assert.Equal(t, 2, state.Series[seriesCI].Episodes[0].MaxTier)
		})
	}
}

// Result precedence on replay: the first result of a claim stands, so a
// duplicated result line can never turn done into failed (Durability 6).
func TestOpenWAL_ReplayKeepsTheFirstResultOfAClaim(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	done := healthband.Claim{ID: claim.ID, Kind: claim.Kind, Status: "done", Owner: claim.Owner, LeaseUntil: claim.LeaseUntil}
	failed := done
	failed.Status = "failed:bs_lock_timeout"
	result := func(seq int64, c healthband.Claim, bsID string) string {
		return marshalLine(t, healthband.Event{Schema: healthband.SchemaBandEvaluation, Seq: seq, Kind: healthband.EventKindActionResult,
			Evaluation: healthband.Evaluation{Series: seriesCI, SampleKey: "1042"}, Action: "diagnose", EpisodeID: "e1042",
			Claims: []healthband.Claim{c}, ClaimID: c.ID, BSID: bsID})
	}
	writeMetricsFile(t, dir, healthband.EventsFile, marshalLine(t, claim.Event)+"\n"+result(2, done, "BS-BAND-001")+"\n"+result(3, failed, "BS-BAND-002")+"\n")
	require.NoError(t, os.Remove(metricsPath(dir, healthband.StateFile)))

	wal, err := lockStore(t, dir).OpenWAL(bandT0.Add(time.Minute))

	require.NoError(t, err)
	state := wal.Checkpoint()
	assert.Equal(t, "done", claimStatus(state, seriesCI, claim.ID))
	assert.Equal(t, "BS-BAND-001", state.Series[seriesCI].Episodes[0].BSID)
	assert.Equal(t, int64(3), state.LastSeq)
}

// A checkpoint outside the contract is not trusted: the state is rebuilt
// by replaying the log, and the next commit rewrites the file.
func TestOpenWAL_RebuildsTamperedCheckpoints(t *testing.T) {
	t.Parallel()
	for name, tamper := range map[string]func(string) string{
		"schema":            func(s string) string { return strings.Replace(s, "band_state.v1", "band_state.v2", 1) },
		"last seq":          func(s string) string { return strings.Replace(s, `"last_seq":1`, `"last_seq":-1`, 1) },
		"last key":          func(s string) string { return strings.Replace(s, `"last_key":"1042"`, `"last_key":"10 42"`, 1) },
		"episode id":        func(s string) string { return strings.Replace(s, `"id":"e1042"`, `"id":"x1042"`, 1) },
		"max tier":          func(s string) string { return strings.Replace(s, `"max_tier":2`, `"max_tier":9`, 1) },
		"BS id":             func(s string) string { return strings.Replace(s, `"open":true`, `"open":true,"bs_id":"BS-1"`, 1) },
		"claim status":      func(s string) string { return strings.Replace(s, `"status":"claimed"`, `"status":"running"`, 1) },
		"claim no status":   func(s string) string { return strings.Replace(s, `"status":"claimed",`, ``, 1) },
		"interrupted no at": func(s string) string { return strings.Replace(s, `"status":"claimed"`, `"status":"interrupted"`, 1) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			plannedClaim(t, dir)
			original, err := os.ReadFile(metricsPath(dir, healthband.StateFile))
			require.NoError(t, err)
			tampered := tamper(string(original))
			require.NotEqual(t, string(original), tampered, "the tamper must apply")
			writeMetricsFile(t, dir, healthband.StateFile, tampered)
			locked := lockStore(t, dir)
			wal, err := locked.OpenWAL(bandT0.Add(time.Minute))
			require.NoError(t, err)
			series, _, err := locked.Store().ReadSeries()
			require.NoError(t, err)
			plan, err := wal.Plan(series, healthband.PlanOptions{Owner: testOwner})
			require.NoError(t, err)

			require.NoError(t, wal.Commit(plan))

			assert.Empty(t, plan.Events(), "the rebuilt state already holds the newest position")
			rewritten, err := os.ReadFile(metricsPath(dir, healthband.StateFile))
			require.NoError(t, err)
			assert.Equal(t, string(original), string(rewritten))
		})
	}
}

// Manifest entries reach the event log only as ids, fixed source
// references, codes, and SHA-256 hashes.
func TestRecordResult_RefusesManifestEntriesOutsideTheContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	good := promptlayer.ManifestEntry{ID: "band.instructions.v1", Kind: promptlayer.KindStable, Group: promptlayer.GroupIdentityRules,
		SourceRef: "healthband/prompt.go#band.instructions.v1", Hash: strings.Repeat("a", 64), RedactionStatus: "passed", InvalidationReason: "none"}
	for name, tamper := range map[string]func(*promptlayer.ManifestEntry){
		"kind":       func(e *promptlayer.ManifestEntry) { e.Kind = "volatile" },
		"empty id":   func(e *promptlayer.ManifestEntry) { e.ID = "" },
		"id escape":  func(e *promptlayer.ManifestEntry) { e.ID = "band.\x1b[2J" },
		"source ref": func(e *promptlayer.ManifestEntry) { e.SourceRef = "step 3 failed: using ghp_x" },
		"hash":       func(e *promptlayer.ManifestEntry) { e.Hash = "abc" },
		"tokens":     func(e *promptlayer.ManifestEntry) { e.TokenEstimate = -1 },
	} {
		entry := good
		tamper(&entry)
		outcome := healthband.ClaimOutcome{PromptManifest: []promptlayer.ManifestEntry{good, entry}}
		_, err := healthband.NewStore(dir).RecordResult(context.Background(), resultOf(claim, outcome), bandT0.Add(time.Minute), time.Second)
		assert.ErrorIs(t, err, healthband.ErrInvalidResult, name)
	}
	outcome := healthband.ClaimOutcome{PromptManifest: []promptlayer.ManifestEntry{good}}
	_, err := healthband.NewStore(dir).RecordResult(context.Background(), resultOf(claim, outcome), bandT0.Add(time.Minute), time.Second)
	assert.NoError(t, err)
}

// A pending directory that is a symlink is never written through or read.
func TestPending_RefusesASymlinkedPendingDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, metricsPath(dir, healthband.PendingDir)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	holder, err := filelock.Acquire(context.Background(), metricsPath(dir, healthband.LockFile), time.Second)
	require.NoError(t, err)

	_, recordErr := healthband.NewStore(dir).RecordResult(context.Background(), resultOf(claim, healthband.ClaimOutcome{}), bandT0, 20*time.Millisecond)
	require.NoError(t, holder.Unlock())
	_, openErr := runPhaseA(dir, bandT0.Add(time.Minute))

	assert.Error(t, recordErr)
	assert.Error(t, openErr)
	entries, err := os.ReadDir(elsewhere)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.NoFileExists(t, filepath.Join(elsewhere, claim.ID+".json"))
}
