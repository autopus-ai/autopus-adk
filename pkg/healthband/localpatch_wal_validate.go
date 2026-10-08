package healthband

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Record validation. A repository can commit a tampered metrics file, so a
// record reaches the log, a read, or the output only when every identifier,
// code, and hash holds a contract value. Keys and paths only need to be
// printable: they are compared with their derived values (Derived paths)
// and never used, so a tampered one ends its claim failed:record_invalid.

// ErrInvalidLocalPatchRecord refuses a record outside the contract before
// anything is written.
var ErrInvalidLocalPatchRecord = errors.New("healthband: local patch record outside the contract")

var (
	localPatchKeyPattern   = regexp.MustCompile(`^[a-z0-9-]{1,200}$`)
	localPatchCodePattern  = regexp.MustCompile(`^[A-Za-z0-9_:().-]{1,200}$`)
	localPatchModelPattern = regexp.MustCompile(`^[A-Za-z0-9._:/@-]{0,128}$`)
)

// localPatchTextBytes bounds a stored key, path, or branch.
const localPatchTextBytes = 4096

// stageField returns the phase's required field check; an unknown phase has
// none.
var stageField = map[string]func(LocalPatchRecord) bool{
	StageWorktreeIntent: func(r LocalPatchRecord) bool { return printable(r.Path, true) },
	StageWorktreeDone:   func(r LocalPatchRecord) bool { return validSHA256(r.StatusSHA256) },
	StageWorktreeFailed: func(r LocalPatchRecord) bool { return localPatchCodePattern.MatchString(r.Code) },
	StageMessage:        func(r LocalPatchRecord) bool { return validSHA256(r.MessageSHA256) },
	StageApplyIntent:    func(r LocalPatchRecord) bool { return validSHA256(r.DiffSHA256) && validGitOID(r.Tree) },
	StageApplyDone:      func(r LocalPatchRecord) bool { return validGitOID(r.Tree) },
	StageCommitDone:     func(r LocalPatchRecord) bool { return validGitOID(r.CommitOID) },
	StageBranchIntent:   func(r LocalPatchRecord) bool { return validGitOID(r.CommitOID) },
	StageBranchDone:     func(r LocalPatchRecord) bool { return r.CommitOID == "" || validGitOID(r.CommitOID) },
	StagePatchIntent:    func(r LocalPatchRecord) bool { return printable(r.Path, true) && validSHA256(r.PatchSHA256) },
	StagePatchDone:      func(r LocalPatchRecord) bool { return validSHA256(r.PatchSHA256) },
}

// valid reports whether a record holds only contract values for its kind.
func (r LocalPatchRecord) valid() bool {
	if r.Schema != SchemaLocalPatch || r.Seq < 1 {
		return false
	}
	switch r.Kind {
	case LocalPatchKindDecision:
		return ValidSeriesID(r.Series) && episodeIDPattern.MatchString(r.EpisodeID) && r.EvaluationSeq >= 1 &&
			(r.Decision == LocalPatchDecideClaim && r.Reason == "" ||
				r.Decision == LocalPatchDecideSkip && strings.HasPrefix(r.Reason, "local_patch_skipped:") && localPatchCodePattern.MatchString(r.Reason))
	case LocalPatchKindClaim:
		return claimIDPattern.MatchString(r.ClaimID) && ownerPattern.MatchString(r.Owner) && !r.LeaseUntil.IsZero() &&
			ValidSeriesID(r.Series) && episodeIDPattern.MatchString(r.EpisodeID) && claimIDPattern.MatchString(r.DependsOn) &&
			printable(r.Key, true) && printable(r.WorktreePath, true) && printable(r.PatchPath, true) && printable(r.Branch, true)
	case LocalPatchKindPrep:
		return ValidSeriesID(r.Series) && episodeIDPattern.MatchString(r.EpisodeID) && claimIDPattern.MatchString(r.DiagnoseClaimID) &&
			!r.LeaseUntil.IsZero() && optional(claimIDPattern, r.ClaimID) && printable(r.Key, true) &&
			(r.BaseSHA == "" || validGitOID(r.BaseSHA)) && localPatchCodePattern.MatchString(r.Code)
	case LocalPatchKindStage:
		check, known := stageField[r.Phase]
		return known && claimIDPattern.MatchString(r.ClaimID) && check(r)
	case LocalPatchKindResult:
		return r.validResult()
	}
	return false
}

func (r LocalPatchRecord) validResult() bool {
	code, failed := strings.CutPrefix(r.Status, ClaimFailedPrefix)
	if !claimIDPattern.MatchString(r.ClaimID) || r.Status != ClaimDone && !(failed && localPatchCodePattern.MatchString(code)) ||
		!optional(bsIDPattern, r.BSID) || r.BaseSHA != "" && !validGitOID(r.BaseSHA) || r.CommitSHA != "" && !validGitOID(r.CommitSHA) ||
		!printable(r.Branch, false) || !printable(r.WorktreePath, false) || !printable(r.PatchPath, false) || !validManifest(r.PromptManifest) {
		return false
	}
	for _, kept := range r.Kept {
		if !localPatchCodePattern.MatchString(kept.Artifact) || !localPatchCodePattern.MatchString(kept.Reason) {
			return false
		}
	}
	for _, model := range r.Models {
		if model.Request != ModelRequestDiagnosis && model.Request != ModelRequestPatch || !localPatchModelPattern.MatchString(model.Requested) ||
			!localPatchModelPattern.MatchString(model.Actual) || !localPatchModelPattern.MatchString(model.RefusalCategory) {
			return false
		}
	}
	for _, file := range r.Files {
		if !printable(file.Path, true) || file.Added < 0 || file.Removed < 0 {
			return false
		}
	}
	return true
}

func validSHA256(s string) bool { return manifestHashPattern.MatchString(s) }

// printable accepts valid UTF-8 of at most localPatchTextBytes with no C0,
// DEL, or C1 control, so no stored text can steer a terminal; required
// refuses the empty string.
func printable(s string, required bool) bool {
	if s == "" {
		return !required
	}
	if len(s) > localPatchTextBytes || !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r >= 0x7f && r <= 0x9f })
}

// ValidLocalPatchModel reports a model name that a models[] entry and a BS
// model line may hold; a provider stream's model outside it is treated as
// absent, so no record or BS line is refused for it.
func ValidLocalPatchModel(model string) bool { return localPatchModelPattern.MatchString(model) }
