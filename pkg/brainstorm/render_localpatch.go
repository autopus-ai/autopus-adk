package brainstorm

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Local patch lines of a band BS (SPEC-SIGMABAND-002 BS Record). With a
// local_patch claim, Render replaces three diagnosis-only sentences of
// SPEC-SIGMABAND-001 and ends the 추천 방향 section with the pointer lines;
// a flag-on diagnosis whose request returned a stream also gets the
// Diagnosis model line there. The BS sections and the structural validator
// stay unchanged, and a request with neither field renders 001's BS byte for
// byte.

// LocalPatch points at the artifacts that a local_patch claim may produce:
// branch autopus/band/<key>, worktree <lp>/<key>/worktree/, and patch file
// <lp>/<key>.patch, outside the repository.
type LocalPatch struct {
	Key     string // <key>, alphabet [a-z0-9-], ending in -<c8> of ClaimID
	Dir     string // absolute <lp>
	ClaimID string // the local_patch claim id, 32 hex
}

// DiagnosisModel is the models[] entry of a flag-on diagnosis request (Local
// Patch Provider Contract item 7); an empty name renders as none.
type DiagnosisModel struct {
	Requested       string // the --model value of the argv
	Actual          string
	RefusalCategory string
	Substituted     bool // model_substituted
}

// LocalPatchReviewerWarning is the reviewer warning line of the BS Record.
const LocalPatchReviewerWarning = "Reviewer warning: this local branch holds a patch that an AI model derived from " +
	"untrusted CI logs and that nothing has run. Read the whole patch file before you open the worktree in an IDE, run any " +
	"command or agent in it, or push the branch, because repository hooks and tool configuration files run on checkout, " +
	"commit, and build, and the edit guard does not cover that worktree."

// The diagnosis-only sentences of SPEC-SIGMABAND-001's templates and the
// forms that a BS of a local_patch claim uses instead.
const (
	whenDiagnosisOnly = "- When: detected {date}; tier 2 and tier 3 are diagnosis-only, so band changed no git ref, " +
		"worktree, or GitHub state.\n"
	whenLocalPatch = "- When: detected {date}; tier 3 with health_band.allow_local_patch may leave one local patch " +
		"outside this repository (see 추천 방향); band pushes nothing.\n"
	nonGoalsDiagnosisOnly = "- Explicit non-goals: changes unrelated to {series_code}; band itself changes no git ref, " +
		"worktree, or GitHub state.\n"
	nonGoalsLocalPatch = "- Explicit non-goals: changes unrelated to {series_code}; band itself pushes nothing and " +
		"changes no GitHub state.\n"
	directionDiagnosisOnly = "Tier {tier} is diagnosis-only: band opened no branch, commit, or pull request."
	directionLocalPatch    = "Tier 3 with health_band.allow_local_patch adds at most one local branch, worktree, and " +
		"patch file and opens no pull request."
	// directionEnd ends the 추천 방향 section of the tail.
	directionEnd = "\n\n## Evolution Ideas\n"
)

var (
	localPatchKeyPattern     = regexp.MustCompile(`^[a-z0-9-]{1,200}$`)
	localPatchClaimIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// localPatchDirBytes bounds <lp> like the store bounds a recorded path.
const localPatchDirBytes = 4096

func (p *LocalPatch) valid() bool {
	return localPatchClaimIDPattern.MatchString(p.ClaimID) && localPatchKeyPattern.MatchString(p.Key) &&
		strings.HasSuffix(p.Key, "-"+p.ClaimID[:8]) && filepath.IsAbs(p.Dir) && filepath.Clean(p.Dir) == p.Dir &&
		len(p.Dir) <= localPatchDirBytes && utf8.ValidString(p.Dir) &&
		!strings.ContainsFunc(p.Dir, func(r rune) bool { return r < 0x20 || r >= 0x7f && r <= 0x9f })
}

func (m *DiagnosisModel) valid() bool {
	return healthband.ValidLocalPatchModel(m.Requested) && healthband.ValidLocalPatchModel(m.Actual) &&
		healthband.ValidLocalPatchModel(m.RefusalCategory)
}

// validateLocalPatch refuses a pointer or a model outside the contract; a
// pointer belongs only to a tier-3 BS, the opening of a local_patch claim.
func (r Request) validateLocalPatch() error {
	switch {
	case r.LocalPatch != nil && (!r.LocalPatch.valid() || *r.Evaluation.Tier != 3):
		return fmt.Errorf("%w: local_patch", ErrInvalidRequest)
	case r.DiagnosisModel != nil && !r.DiagnosisModel.valid():
		return fmt.Errorf("%w: diagnosis_model", ErrInvalidRequest)
	}
	return nil
}

// The templates of a BS that belongs to a local_patch claim.
var (
	headTemplateLocalPatch = strings.NewReplacer(
		whenDiagnosisOnly, whenLocalPatch, nonGoalsDiagnosisOnly, nonGoalsLocalPatch).Replace(headTemplate)
	tailTemplateLocalPatch = strings.Replace(tailTemplate, directionDiagnosisOnly, directionLocalPatch, 1)
)

// headTemplateFor is the head template, with the local-patch sentences when
// the BS belongs to a local_patch claim.
func (r Request) headTemplateFor() string {
	if r.LocalPatch == nil {
		return headTemplate
	}
	return headTemplateLocalPatch
}

// tailTemplateFor is the tail template, with the local-patch sentence when
// the BS belongs to a local_patch claim.
func (r Request) tailTemplateFor() string {
	if r.LocalPatch == nil {
		return tailTemplate
	}
	return tailTemplateLocalPatch
}

// withDirectionLines ends the 추천 방향 section of a filled tail with the
// pointer lines, then the Diagnosis model line, each its own paragraph. They
// are inserted after the fill, so no placeholder inside a path is replaced.
func (r Request) withDirectionLines(tail string) string {
	var lines []string
	if p := r.LocalPatch; p != nil {
		lines = append(lines,
			fmt.Sprintf("Local patch (3σ, local only, if produced): branch autopus/band/%s, worktree %s/, patch file %s, "+
				"outside this repository.", p.Key, filepath.Join(p.Dir, p.Key, "worktree"), filepath.Join(p.Dir, p.Key+".patch")),
			"The outcome, the changed files, and the model that wrote the patch are in the run output and in "+
				".autopus/metrics/localpatch-events.jsonl under claim "+p.ClaimID+"; nothing was pushed.",
			LocalPatchReviewerWarning)
	}
	if m := r.DiagnosisModel; m != nil {
		line := fmt.Sprintf("Diagnosis model: requested %s, actual %s", noneIfEmpty(m.Requested), noneIfEmpty(m.Actual))
		if m.Substituted {
			line += fmt.Sprintf("; model_substituted (refusal category %s)", noneIfEmpty(m.RefusalCategory))
		}
		lines = append(lines, line+".")
	}
	if len(lines) == 0 {
		return tail
	}
	return strings.Replace(tail, directionEnd, "\n\n"+strings.Join(lines, "\n\n")+directionEnd, 1)
}

func noneIfEmpty(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
