package cli

// `auto quality review` (SPEC-REVMODEL-001 without a terminal): show prints
// what the review tab shows, and on, off, and judge plan with the tab's
// planner, print its change lines, and write with its line splice writer.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// useQualityReviewSeams sets what every `auto quality review` test shares:
// English text, the Codex catalog probe answering CAT-E, and a PATH holding
// only the binaries named, so no real catalog, agy, or omp decides anything.
func useQualityReviewSeams(t *testing.T, binaries ...string) *[]composeProbeCall {
	t.Helper()
	useComposeLanguage(t, "en")
	useReviewPath(t, binaries...)
	return stubComposeCatalogProbe(t, mustReadComposeCatalog(t, "CAT-E"), nil)
}

// runQualityReview runs the real `auto quality review` with args over dir's
// autopus.yaml and its OMP catalog read through runner, nil answering CATG.
// It returns stdout and stderr apart.
func runQualityReview(t *testing.T, dir string, runner *ompCLIFakeRunner, args ...string) (string, string, error) {
	t.Helper()
	if runner == nil {
		runner = &ompCLIFakeRunner{catalog: []byte(reviewCatalogCATG)}
	}
	root := NewRootCmd()
	old, _, err := root.Find([]string{"quality"})
	require.NoError(t, err)
	root.RemoveCommand(old)
	root.AddCommand(newQualityCmdWithOMPDependencies(ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
	}))
	var out, stderr bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "review"}, args...))
	err = root.Execute()
	return out.String(), stderr.String(), err
}

// writeReviewMixed is RL with gemini on the OMP backend and codex taking no
// part: one provider of every kind the show table spells.
func writeReviewMixed(t *testing.T) string {
	t.Helper()
	return writeReviewRL(t, func(cfg *config.HarnessConfig) {
		cfg.Orchestra.Providers["gemini"] = reviewOMPEntry("google-antigravity/gemini-3.1-pro:high")
		cfg.Spec.ReviewGate.Providers = []string{"claude", "gemini"}
	})
}

func TestQualityReview_ShowPrintsWhatTheTabShows(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewMixed(t)
	before := reviewFileSum(t, dir)

	out, _, err := runQualityReview(t, dir, nil)

	require.NoError(t, err, out)
	assert.Equal(t, "spec.review_gate.enabled = true\n"+
		"spec.review_gate.judge = claude\n"+
		"provider  review  backend  model\n"+
		"claude    on      claude   claude-fable-5-1 · max\n"+
		"codex     off     codex    gpt-6-astra · max\n"+
		"gemini    on      omp      google-antigravity/gemini-3.1-pro:high\n"+
		"config = "+filepath.Join(dir, "autopus.yaml")+"\n", out)
	shown, _, err := runQualityReview(t, dir, nil, "show")
	require.NoError(t, err)
	assert.Equal(t, out, shown, "review and review show print the same")
	assert.Equal(t, before, reviewFileSum(t, dir))
}

func TestQualityReview_ShowJSON(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewMixed(t)

	out, _, err := runQualityReview(t, dir, nil, "show", "--json")

	require.NoError(t, err, out)
	var envelope struct {
		Command string                `json:"command"`
		Status  string                `json:"status"`
		Data    qualityReviewSettings `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope), out)
	assert.Equal(t, "auto quality review show", envelope.Command)
	assert.Equal(t, "ok", envelope.Status)
	assert.Equal(t, qualityReviewSettings{
		Config: filepath.Join(dir, "autopus.yaml"), ReviewGateEnabled: true,
		ReviewProviders: []string{"claude", "gemini"}, Judge: "claude",
		Providers: []qualityReviewProvider{
			{Name: "claude", TakesPart: true, Backend: "cli", Binary: "claude", Model: "claude-fable-5-1", Effort: "max"},
			{Name: "codex", Backend: "cli", Binary: "codex", Model: "gpt-6-astra", Effort: "max"},
			{Name: "gemini", TakesPart: true, Backend: "omp", Model: "google-antigravity/gemini-3.1-pro", Effort: "high"},
		},
	}, envelope.Data)
}

// off and on keep the tab's participation rule, its count check, and its
// judge check; a --dry-run and an edit back to the file write nothing.
func TestQualityReview_OnOffFollowTheTabRules(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewR3(t)
	b0 := readReviewFile(t, dir)
	offGemini := "Settings that change:\n  spec.review_gate.providers [claude, codex, gemini] → [claude, codex]\n" +
		"Turned-off providers still run in orchestra commands (brainstorm, plan, review, secure) and in auto spec review --multi\n"

	out, _, err := runQualityReview(t, dir, nil, "off", "gemini", "--dry-run")
	require.NoError(t, err, out)
	assert.Equal(t, offGemini+"--dry-run: nothing written.\n", out)
	assert.Equal(t, b0, readReviewFile(t, dir))

	out, _, err = runQualityReview(t, dir, nil, "off", "gemini")
	require.NoError(t, err, out)
	assert.Equal(t, offGemini+"Composition applied.\n", out)
	assert.Len(t, reviewFileDiff(t, b0, readReviewFile(t, dir)), 1)
	loaded := reviewLoaderValue(t, dir)
	assert.Equal(t, []string{"claude", "codex"}, loaded.Spec.ReviewGate.Providers)
	assert.Equal(t, []string{"claude", "codex", "gemini"}, loaded.Orchestra.Commands["review"].Providers)
	b1 := readReviewFile(t, dir)

	_, _, err = runQualityReview(t, dir, nil, "off", "codex")
	require.EqualError(t, err, "The review needs at least two providers (now 1)")
	_, _, err = runQualityReview(t, dir, nil, "off", "nope")
	require.EqualError(t, err, "nope is not an orchestra.providers entry (accepted: claude, codex, gemini)")
	assert.Equal(t, b1, readReviewFile(t, dir))

	out, _, err = runQualityReview(t, dir, nil, "on", "gemini")
	require.NoError(t, err, out)
	assert.Equal(t, b0, readReviewFile(t, dir), "turned on again, the list reads as before")
	out, _, err = runQualityReview(t, dir, nil, "on", "gemini")
	require.NoError(t, err)
	assert.Equal(t, "No changes; nothing written.\n", out)
	assert.Equal(t, b0, readReviewFile(t, dir))

	_, _, err = runQualityReview(t, dir, nil, "off", "claude")
	require.EqualError(t, err, "judge claude does not take part in the review and cannot be chosen; "+
		"choose another judge first with auto quality review judge <provider>")
	assert.Equal(t, b0, readReviewFile(t, dir))
}

// Several names turn on together, so a review below two providers reaches
// two in one edit.
func TestQualityReview_OnTakesSeveralProviders(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewR3(t, func(cfg *config.HarnessConfig) { cfg.Spec.ReviewGate.Providers = nil })

	_, _, err := runQualityReview(t, dir, nil, "on", "claude")
	require.EqualError(t, err, "The review needs at least two providers (now 1)")

	out, _, err := runQualityReview(t, dir, nil, "on", "codex", "claude")
	require.NoError(t, err, out)
	assert.Contains(t, out, "  spec.review_gate.providers [] → [claude, codex]\n")
	assert.Equal(t, []string{"claude", "codex"}, reviewLoaderValue(t, dir).Spec.ReviewGate.Providers)
}

func TestQualityReview_JudgeTakesAParticipantOrNone(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewR3(t, func(cfg *config.HarnessConfig) { cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"} })
	b0 := readReviewFile(t, dir)

	_, _, err := runQualityReview(t, dir, nil, "judge", "gemini")
	require.EqualError(t, err, "judge gemini does not take part in the review and cannot be chosen (accepted: claude, codex, none)")
	assert.Equal(t, b0, readReviewFile(t, dir))

	out, _, err := runQualityReview(t, dir, nil, "judge", "none")
	require.NoError(t, err, out)
	assert.Equal(t, "Settings that change:\n  spec.review_gate.judge claude → no judge\nComposition applied.\n", out)
	assert.Equal(t, [][2]string{{"        judge: claude", `        judge: ""`}}, reviewFileDiff(t, b0, readReviewFile(t, dir)))
	assert.Empty(t, reviewLoaderValue(t, dir).Spec.ReviewGate.Judge)

	out, _, err = runQualityReview(t, dir, nil, "judge", "codex")
	require.NoError(t, err, out)
	assert.Contains(t, out, "  spec.review_gate.judge no judge → codex\n")
	assert.Equal(t, "codex", reviewLoaderValue(t, dir).Spec.ReviewGate.Judge)
}

// A judge written as a placeholder stays, shown as written; names the file
// does not hold as written refuse show and every edit; --apply is refused.
func TestQualityReview_RefusesWhatTheTabRefuses(t *testing.T) {
	useQualityReviewSeams(t)
	t.Setenv("REVIEW_JUDGE", "codex")
	dir := writeReviewR3Judge(t, "        judge: ${REVIEW_JUDGE}\n")
	before := readReviewFile(t, dir)

	out, _, err := runQualityReview(t, dir, nil)
	require.NoError(t, err)
	assert.Contains(t, out, "spec.review_gate.judge = ${REVIEW_JUDGE}\n")
	_, _, err = runQualityReview(t, dir, nil, "judge", "claude")
	require.EqualError(t, err, "judge of spec.review_gate comes from an environment variable; this tab leaves it: ${REVIEW_JUDGE}")
	_, _, err = runQualityReview(t, dir, nil, "off", "gemini", "--apply")
	require.EqualError(t, err, composeReviewTextEN.ApplyRefused)
	assert.Equal(t, before, readReviewFile(t, dir))

	t.Setenv("REVIEW_EXTRA", reviewSentinel)
	names := writeReviewR3(t)
	replaceReviewFile(t, names, "    commands:\n",
		"        ${REVIEW_EXTRA}:\n            backend: omp\n            model: google-antigravity/gemini-3.1-pro:high\n    commands:\n")
	for _, args := range [][]string{nil, {"off", "gemini"}, {"model", "claude", "anthropic/claude-opus-5-5"}} {
		_, _, err = runQualityReview(t, names, nil, args...)
		require.EqualError(t, err, composeReviewTextEN.NamesRefused, "%v", args)
	}
}
