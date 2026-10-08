package brainstorm_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-002 BS Record: the pointer lines and the three local-patch
// sentences of a BS that has a local_patch claim, and the Diagnosis model
// line of every flag-on diagnosis BS.

const (
	lpBSKey     = "ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4"
	lpBSClaimID = "a1b2c3d4e5f60708a1b2c3d4e5f60708"
	lpBSDir     = "/Users/u/Library/Caches/autopus/local-patches/0123456789ab"
)

// lpBSWarning is the reviewer warning line of the BS Record, verbatim.
const lpBSWarning = "Reviewer warning: this local branch holds a patch that an AI model derived from untrusted CI logs and " +
	"that nothing has run. Read the whole patch file before you open the worktree in an IDE, run any command or agent in it, " +
	"or push the branch, because repository hooks and tool configuration files run on checkout, commit, and build, and the " +
	"edit guard does not cover that worktree."

// tier3Request is the S4 diagnosis: tier 3, an ok diagnosis, one log.
func tier3Request() brainstorm.Request {
	req := o2Request()
	three := 3
	req.Evaluation.Tier, req.DiagnosisStatus = &three, "ok"
	req.Diagnosis = sanitized("### Summary\nThe flaky step failed.\n")
	req.Logs = []healthband.RunLog{{RunID: 4242, Attempt: 1, Evidence: sanitized("step 3 failed\n")}}
	return req
}

// lpRequest is tier3Request with the S4 local_patch claim's pointer and the
// diagnosis model of a stream that named the requested model.
func lpRequest() brainstorm.Request {
	req := tier3Request()
	req.LocalPatch = &brainstorm.LocalPatch{Key: lpBSKey, Dir: lpBSDir, ClaimID: lpBSClaimID}
	req.DiagnosisModel = &brainstorm.DiagnosisModel{Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}
	return req
}

func lineWith(t *testing.T, doc, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("no line starts with %q", prefix)
	return ""
}

func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// A request without either field renders SPEC-SIGMABAND-001's BS byte for
// byte: the hashes were taken from the 001 renderer before this SPEC.
func TestRender_LocalPatchFieldsUnset_KeepThe001BSByteForByte(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "accd24545b4a0f85bae0cd7b5e8d2a1ac937f0da78cfa30080da93383854aab5", sha256Hex(render(t, "BS-BAND-001", o2Request())))
	doc := render(t, "BS-BAND-001", tier3Request())
	assert.Equal(t, "d1a88e81037f74e578792940b3ca6b32c5ad70153dff8404f8f6ebf304ec07ad", sha256Hex(doc))
	assert.Equal(t, []string{"Confirm the likely cause against the fenced evidence, then plan the fix with the next step below. " +
		"Tier 3 is diagnosis-only: band opened no branch, commit, or pull request.", ""}, section(t, doc, "추천 방향"))
}

func TestRender_LocalPatchClaim_PointerLinesAndThreeSentences(t *testing.T) {
	t.Parallel()
	doc := render(t, "BS-BAND-001", lpRequest())

	assert.Equal(t, "- When: detected 2026-10-06; tier 3 with health_band.allow_local_patch may leave one local patch "+
		"outside this repository (see 추천 방향); band pushes nothing.", lineWith(t, doc, "- When:"))
	assert.Equal(t, "- Explicit non-goals: changes unrelated to `ci.failure_rate:CI`; band itself pushes nothing and changes "+
		"no GitHub state.", lineWith(t, doc, "- Explicit non-goals:"))
	assert.Equal(t, []string{
		"Confirm the likely cause against the fenced evidence, then plan the fix with the next step below. " +
			"Tier 3 with health_band.allow_local_patch adds at most one local branch, worktree, and patch file and opens no pull request.",
		"",
		"Local patch (3σ, local only, if produced): branch autopus/band/" + lpBSKey + ", worktree " + lpBSDir + "/" + lpBSKey +
			"/worktree/, patch file " + lpBSDir + "/" + lpBSKey + ".patch, outside this repository.",
		"",
		"The outcome, the changed files, and the model that wrote the patch are in the run output and in " +
			".autopus/metrics/localpatch-events.jsonl under claim " + lpBSClaimID + "; nothing was pushed.",
		"",
		lpBSWarning,
		"",
		"Diagnosis model: requested claude-opus-5-5, actual claude-opus-5-5.",
		"",
	}, section(t, doc, "추천 방향"))
	assert.Equal(t, s10Sections, headings(doc))
	assert.Empty(t, brainstorm.Validate([]byte(doc)))
	assert.Equal(t, lpBSWarning, brainstorm.LocalPatchReviewerWarning)
	for _, retired := range []string{"tier 2 and tier 3 are diagnosis-only", "band itself changes no git ref", "is diagnosis-only: band opened"} {
		assert.NotContains(t, doc, retired)
	}
}

// The pointer names the artifacts where plan task T2 derives them.
func TestRender_LocalPatchClaim_PointerPathsAreTheDerivedPaths(t *testing.T) {
	t.Parallel()
	paths := healthband.LocalPatchLocation{Path: lpBSDir}.Paths(lpBSKey)
	line := lineWith(t, render(t, "BS-BAND-001", lpRequest()), "Local patch (3σ")
	assert.Contains(t, line, "branch "+paths.Branch+", ")
	assert.Contains(t, line, "worktree "+paths.Worktree+"/, ")
	assert.Contains(t, line, "patch file "+paths.Patch+", ")
}

// A flag-on diagnosis without a local_patch claim keeps 001's sentences and
// gets only the Diagnosis model line at the end of 추천 방향.
func TestRender_DiagnosisModelWithoutLocalPatch_Keeps001Sentences(t *testing.T) {
	t.Parallel()
	req := o2Request()
	req.DiagnosisModel = &brainstorm.DiagnosisModel{Actual: "claude-opus-5-5"}
	doc := render(t, "BS-BAND-001", req)

	assert.Equal(t, []string{"Confirm the likely cause against the fenced evidence, then plan the fix with the next step below. " +
		"Tier 2 is diagnosis-only: band opened no branch, commit, or pull request.", "",
		"Diagnosis model: requested none, actual claude-opus-5-5.", ""}, section(t, doc, "추천 방향"))
	assert.Equal(t, "- When: detected 2026-10-06; tier 2 and tier 3 are diagnosis-only, so band changed no git ref, worktree, "+
		"or GitHub state.", lineWith(t, doc, "- When:"))
	assert.NotContains(t, doc, "Local patch (3σ")
	assert.NotContains(t, doc, "Reviewer warning")
	assert.Empty(t, brainstorm.Validate([]byte(doc)))
}

func TestRender_DiagnosisModelSubstituted_NamesTheRefusalCategory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		model brainstorm.DiagnosisModel
		want  string
	}{
		{"refusal fallback", brainstorm.DiagnosisModel{Requested: "claude-opus-5-5", Actual: "claude-opus-4-8", RefusalCategory: "cyber", Substituted: true},
			"Diagnosis model: requested claude-opus-5-5, actual claude-opus-4-8; model_substituted (refusal category cyber)."},
		{"init model differs", brainstorm.DiagnosisModel{Requested: "claude-opus-5-5", Actual: "claude-opus-4-8", Substituted: true},
			"Diagnosis model: requested claude-opus-5-5, actual claude-opus-4-8; model_substituted (refusal category none)."},
		{"no model at all", brainstorm.DiagnosisModel{}, "Diagnosis model: requested none, actual none."},
	}
	for _, tc := range cases {
		req := lpRequest()
		req.DiagnosisModel = &tc.model
		assert.Equal(t, tc.want, lineWith(t, render(t, "BS-BAND-001", req), "Diagnosis model:"), tc.name)
	}
}

// The pointer and the model line sit in the tail, which the size cut never
// touches: a diagnosis past the body budget is cut instead.
func TestRender_LocalPatchClaim_LongDiagnosisIsCutBeforeThePointer(t *testing.T) {
	t.Parallel()
	req := lpRequest()
	req.Diagnosis = sanitized(strings.Repeat("a diagnosis line that repeats\n", 2000))
	doc := render(t, "BS-BAND-001", req)

	assert.LessOrEqual(t, len(doc), healthband.MaxBSBodyBytes)
	assert.Contains(t, doc, "\nevidence_status: size_cap\n")
	assert.Contains(t, doc, "\n\n"+lpBSWarning+"\n\nDiagnosis model: requested claude-opus-5-5, actual claude-opus-5-5.\n\n## Evolution Ideas\n")
	assert.Empty(t, brainstorm.Validate([]byte(doc)))
}

func TestRender_RejectsLocalPatchFieldsOutsideTheContract(t *testing.T) {
	t.Parallel()
	two := 2
	for name, edit := range map[string]func(*brainstorm.Request){
		"tier 2 with a pointer":      func(r *brainstorm.Request) { r.Evaluation.Tier = &two },
		"key outside the alphabet":   func(r *brainstorm.Request) { r.LocalPatch.Key = "CI-failure-a1b2c3d4" },
		"key of another claim":       func(r *brainstorm.Request) { r.LocalPatch.Key = "ci-failure-rate-ci-c6d37d0a-e1042-0f1e2d3c" },
		"key with a newline":         func(r *brainstorm.Request) { r.LocalPatch.Key = "x\n## Outcome Lock\n-a1b2c3d4" },
		"empty key":                  func(r *brainstorm.Request) { r.LocalPatch.Key = "" },
		"short claim id":             func(r *brainstorm.Request) { r.LocalPatch.ClaimID = "a1b2c3d4" },
		"relative dir":               func(r *brainstorm.Request) { r.LocalPatch.Dir = "cache/autopus/local-patches/0123456789ab" },
		"unclean dir":                func(r *brainstorm.Request) { r.LocalPatch.Dir = lpBSDir + "/../x" },
		"dir with a newline":         func(r *brainstorm.Request) { r.LocalPatch.Dir = lpBSDir + "\n## Outcome Lock" },
		"dir with an escape":         func(r *brainstorm.Request) { r.LocalPatch.Dir = lpBSDir + "\x1b[2J" },
		"dir with a C1 control":      func(r *brainstorm.Request) { r.LocalPatch.Dir = lpBSDir + "\u009b" },
		"dir not UTF-8":              func(r *brainstorm.Request) { r.LocalPatch.Dir = lpBSDir + "\xff" },
		"dir past 4096 bytes":        func(r *brainstorm.Request) { r.LocalPatch.Dir = "/" + strings.Repeat("d", 4096) },
		"model with a space":         func(r *brainstorm.Request) { r.DiagnosisModel.Actual = "claude opus" },
		"model with a newline":       func(r *brainstorm.Request) { r.DiagnosisModel.Requested = "claude\n## Outcome Lock" },
		"category with a backtick":   func(r *brainstorm.Request) { r.DiagnosisModel.RefusalCategory = "cy`ber" },
		"model past the name bounds": func(r *brainstorm.Request) { r.DiagnosisModel.Actual = strings.Repeat("m", 129) },
	} {
		req := lpRequest()
		edit(&req)
		_, err := brainstorm.Render("BS-BAND-001", req)
		assert.ErrorIs(t, err, brainstorm.ErrInvalidRequest, name)
	}
	ok := lpRequest()
	ok.LocalPatch.Dir = "/Users/정/Library/Caches/autopus/local-patches/0123456789ab"
	_, err := brainstorm.Render("BS-BAND-001", ok)
	require.NoError(t, err, "a user cache directory with non-ASCII letters is a valid pointer")
}
