package healthband

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLocalPatchRecordValid_ResultFields_HoldOnlyContractValues(t *testing.T) {
	t.Parallel()
	good := NewLocalPatchResult(lpClaimID, "")
	good.Schema, good.Seq = SchemaLocalPatch, 1
	good.BSID, good.BaseSHA, good.CommitSHA = "BS-BAND-001", lpBase, lpBase
	good.Kept = []LocalPatchKept{{ArtifactWorktree, KeptWorktreeModified}}
	good.Models = []LocalPatchModel{{Request: ModelRequestPatch, Requested: "claude-opus-5-5", Actual: "claude-opus-4-8", RefusalCategory: "cyber"}}
	good.Files = []PatchFile{{Path: "pkg/foo/été.go", Added: 2, Removed: 0}}
	assert.True(t, good.valid())
	cases := map[string]func(*LocalPatchRecord){
		"status without failed prefix":  func(r *LocalPatchRecord) { r.Status = "broken" },
		"failed code with a space":      func(r *LocalPatchRecord) { r.Status = "failed:a b" },
		"bs id outside BS-BAND":         func(r *LocalPatchRecord) { r.BSID = "BS-001" },
		"short base sha":                func(r *LocalPatchRecord) { r.BaseSHA = "abc" },
		"commit sha with uppercase":     func(r *LocalPatchRecord) { r.CommitSHA = strings.ToUpper(lpBase[:39]) + "A" },
		"branch with an escape":         func(r *LocalPatchRecord) { r.Branch = "autopus/band/\x1b[2J" },
		"kept reason with a space":      func(r *LocalPatchRecord) { r.Kept[0].Reason = "worktree modified" },
		"model request of another kind": func(r *LocalPatchRecord) { r.Models[0].Request = "review" },
		"model id with a space":         func(r *LocalPatchRecord) { r.Models[0].Actual = "claude opus" },
		"file path with a C1 control":   func(r *LocalPatchRecord) { r.Files[0].Path = "pkg/\u009bx.go" },
		"negative added count":          func(r *LocalPatchRecord) { r.Files[0].Added = -1 },
		"empty file path":               func(r *LocalPatchRecord) { r.Files[0].Path = "" },
		"invalid UTF-8 worktree path":   func(r *LocalPatchRecord) { r.WorktreePath = "/cache/\xff" },
		"worktree path past the bound":  func(r *LocalPatchRecord) { r.WorktreePath = "/" + strings.Repeat("a", localPatchTextBytes) },
	}
	for name, edit := range cases {
		record := good
		record.Kept, record.Models, record.Files = append([]LocalPatchKept(nil), good.Kept...),
			append([]LocalPatchModel(nil), good.Models...), append([]PatchFile(nil), good.Files...)
		edit(&record)
		assert.False(t, record.valid(), name)
	}
}

func TestLocalPatchRecordValid_PrepAndClaim_RequireTheirFields(t *testing.T) {
	t.Parallel()
	prep := lpPrep(LocalPatchCodeOK)
	prep.Schema, prep.Seq = SchemaLocalPatch, 1
	assert.True(t, prep.valid())
	claim := lpClaim(lpTestLocation, lpT0)
	claim.Schema, claim.Seq = SchemaLocalPatch, 2
	assert.True(t, claim.valid())
	for name, record := range map[string]LocalPatchRecord{
		"prep without lease":       func() LocalPatchRecord { r := prep; r.LeaseUntil = time.Time{}; return r }(),
		"prep with a bad base":     func() LocalPatchRecord { r := prep; r.BaseSHA = "main"; return r }(),
		"prep without code":        func() LocalPatchRecord { r := prep; r.Code = ""; return r }(),
		"prep without key":         func() LocalPatchRecord { r := prep; r.Key = ""; return r }(),
		"claim with a bad owner":   func() LocalPatchRecord { r := claim; r.Owner = "host"; return r }(),
		"claim without depends_on": func() LocalPatchRecord { r := claim; r.DependsOn = ""; return r }(),
		"claim without branch":     func() LocalPatchRecord { r := claim; r.Branch = ""; return r }(),
		"seq zero":                 func() LocalPatchRecord { r := claim; r.Seq = 0; return r }(),
		"decision claim reason":    {Schema: SchemaLocalPatch, Seq: 3, Kind: LocalPatchKindDecision, Series: lpSeries, EpisodeID: lpEpisode, EvaluationSeq: 1, Decision: LocalPatchDecideClaim, Reason: LocalPatchSkippedNoAgent},
	} {
		assert.False(t, record.valid(), name)
	}
	assert.Empty(t, LocalPatchRecord{Kind: LocalPatchKindDecision}.RecordClaimID())
	assert.Equal(t, lpClaimID, prep.RecordClaimID())
	prep.ClaimID = ""
	assert.Equal(t, lpDiagnoseID, prep.RecordClaimID())
}
