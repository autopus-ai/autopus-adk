package cli

// `auto quality review model`: one provider's model from the list the
// review tab offers it, read from the same source, and nothing the tab would
// not change (SPEC-REVMODEL-001 REQ-RVM-06, 07, 21, 22, 24, 26).

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reviewChangedModelNote = "A changed model applies to auto spec review and to orchestra commands\n"

func TestQualityReviewModel_OMPProviderTakesAnAttestedModel(t *testing.T) {
	probes := useQualityReviewSeams(t)
	dir := writeReviewR3(t)
	b0 := readReviewFile(t, dir)
	runner := &ompCLIFakeRunner{catalog: []byte(reviewCatalogCATG)}

	out, _, err := runQualityReview(t, dir, runner, "model", "codex", "openai-codex/gpt-6-luna")

	require.NoError(t, err, out)
	assert.Equal(t, "Settings that change:\n"+
		"  orchestra.providers.codex.model openai-codex/gpt-6-astra:max → openai-codex/gpt-6-luna:high\n"+
		reviewChangedModelNote+"Composition applied.\n", out, "luna reports no max, so its first level")
	assert.Len(t, reviewFileDiff(t, b0, readReviewFile(t, dir)), 1)
	assert.Equal(t, "openai-codex/gpt-6-luna:high", reviewLoaderValue(t, dir).Orchestra.Providers["codex"].Model)
	assert.Len(t, runner.calls, 2, "one catalog read: version and models")
	assert.Empty(t, *probes, "an OMP provider reads no Codex catalog")

	out, _, err = runQualityReview(t, dir, nil, "model", "claude", "anthropic/claude-opus-5-5", "--effort", "high", "--dry-run")
	require.NoError(t, err, out)
	assert.Contains(t, out, "  orchestra.providers.claude.model anthropic/claude-fable-5-1:max → anthropic/claude-opus-5-5:high\n")
	out, _, err = runQualityReview(t, dir, nil, "model", "claude", "anthropic/claude-opus-5-5", "--dry-run")
	require.NoError(t, err, out)
	assert.Contains(t, out, "→ anthropic/claude-opus-5-5:max\n", "the current level stays when the model reports it")
}

func TestQualityReviewModel_OMPRefusalsListTheAcceptedValues(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewR3(t)
	before := readReviewFile(t, dir)

	_, _, err := runQualityReview(t, dir, nil, "model", "claude", "example/foo-1")
	require.EqualError(t, err, "The installed OMP catalog does not attest claude model example/foo-1 (accepted: "+
		"anthropic/claude-fable-5-1 (high, low, max, medium), anthropic/claude-opus-5-5 (high, max))")
	_, _, err = runQualityReview(t, dir, nil, "model", "gemini", "google-antigravity/gemini-3.1-pro:max")
	require.EqualError(t, err, "google-antigravity/gemini-3.1-pro reports no thinking level max for gemini (accepted: high, low)")
	_, _, err = runQualityReview(t, dir, nil, "model", "claude", "anthropic/claude-opus-5-5:max", "--effort", "high")
	require.EqualError(t, err, "anthropic/claude-opus-5-5:max already names its level; drop --effort high or give the same level")
	_, _, err = runQualityReview(t, dir, &ompCLIFakeRunner{fail: true}, "model", "claude", "anthropic/claude-opus-5-5")
	require.EqualError(t, err, "The OMP catalog is unreadable; models stay unchanged")
	_, _, err = runQualityReview(t, dir, nil, "model", "nope", "x")
	require.EqualError(t, err, "nope is not an orchestra.providers entry (accepted: claude, codex, gemini)")
	assert.Equal(t, before, readReviewFile(t, dir))
}

// claude keeps its effort unless the model takes none; haiku drops the
// effort pair, and the next model gets max back (REQ-RVM-21).
func TestQualityReviewModel_ClaudeCLIRow(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewRL(t)
	b0 := readReviewFile(t, dir)

	out, _, err := runQualityReview(t, dir, nil, "model", "claude", "claude-opus-5-5")
	require.NoError(t, err, out)
	assert.Equal(t, "Settings that change:\n  orchestra.providers.claude.args --model claude-fable-5-1 → claude-opus-5-5\n"+
		reviewChangedModelNote+"Composition applied.\n", out)
	assert.Equal(t, [][2]string{{reviewRLClaudeArgs, "            args: [--print, --model, claude-opus-5-5, --effort, max]"}},
		reviewFileDiff(t, b0, readReviewFile(t, dir)))

	out, _, err = runQualityReview(t, dir, nil, "model", "claude", "claude-haiku-4-5")
	require.NoError(t, err, out)
	assert.Contains(t, out, "  orchestra.providers.claude.args --effort max → none\n")
	assert.Equal(t, "args: [--print, --model, claude-haiku-4-5]", reviewArgsLine(t, readReviewFile(t, dir), "claude"))
	_, _, err = runQualityReview(t, dir, nil, "model", "claude", "claude-haiku-4-5", "--effort", "high")
	require.EqualError(t, err, "claude-haiku-4-5 does not take effort high for claude (accepted: none)")

	out, _, err = runQualityReview(t, dir, nil, "model", "claude", "claude-sonnet-5-5")
	require.NoError(t, err, out)
	assert.Equal(t, "args: [--print, --model, claude-sonnet-5-5, --effort, max]", reviewArgsLine(t, readReviewFile(t, dir), "claude"))
	_, _, err = runQualityReview(t, dir, nil, "model", "claude", "gpt-6-astra")
	require.ErrorContains(t, err, "gpt-6-astra is not a choice for claude (accepted: claude-fable-5-1 (low, medium, high, xhigh, max), ")
}

// codex reads the run's Codex catalog once, keeps or lowers the effort to
// the model's highest level, and pins (REQ-RVM-22).
func TestQualityReviewModel_CodexCLIRow(t *testing.T) {
	probes := useQualityReviewSeams(t, "codex")
	dir := writeReviewRL(t)

	out, _, err := runQualityReview(t, dir, nil, "model", "codex", "gpt-6.1-sol")

	require.NoError(t, err, out)
	assert.Equal(t, "Settings that change:\n"+
		"  orchestra.providers.codex.model_policy quality → pinned\n"+
		"  orchestra.providers.codex.args -m gpt-6-astra → gpt-6.1-sol\n"+
		"  orchestra.providers.codex.args model_reasoning_effort max → xhigh\n"+
		reviewChangedModelNote+"Composition applied.\n", out)
	assert.Len(t, *probes, 1)
	pinned := reviewRuntimeArgs(t, dir)
	assert.True(t, reviewArgsHasRun(pinned, "-m", "gpt-6.1-sol"), "%v", pinned)

	_, _, err = runQualityReview(t, dir, nil, "model", "codex", "gpt-9")
	require.ErrorContains(t, err, "gpt-9 is not a choice for codex (accepted: gpt-6-astra (")
	stubComposeCatalogProbe(t, "", errors.New("codex: not signed in"))
	_, _, err = runQualityReview(t, dir, nil, "model", "codex", "gpt-6-astra")
	require.EqualError(t, err, "The Codex catalog is unreadable; the codex model stays")
}

// agy takes a listed id, or its group name with the suffix as --effort, and
// runs `agy models` only when agy is on PATH by name (REQ-RVM-24).
func TestQualityReviewModel_AgyCLIRow(t *testing.T) {
	useQualityReviewSeams(t, "agy")
	runs := useAgyModels(t, reviewAgySample+"gemini-3.8-flash-low\tGemini 3.8 Flash (Low)\n", nil)
	dir := writeReviewRL(t)
	before := readReviewFile(t, dir)

	out, _, err := runQualityReview(t, dir, nil, "model", "gemini", "gemini-3.8-flash", "--effort", "low", "--dry-run")
	require.NoError(t, err, out)
	assert.Equal(t, "Settings that change:\n  orchestra.providers.gemini.args --model none → gemini-3.8-flash-low\n"+
		reviewChangedModelNote+"--dry-run: nothing written.\n", out)
	assert.Equal(t, 1, *runs)
	assert.Equal(t, before, readReviewFile(t, dir))

	out, _, err = runQualityReview(t, dir, nil, "model", "gemini", "gemini-3.8-flash")
	require.NoError(t, err, out)
	assert.Equal(t, `args: [--print, "", --model, gemini-3.8-flash-high]`, reviewArgsLine(t, readReviewFile(t, dir), "gemini"))
	out, _, err = runQualityReview(t, dir, nil, "model", "gemini", "gemini-3.8-flash")
	require.NoError(t, err)
	assert.Equal(t, "No changes; nothing written.\n", out, "a group keeps the current id")

	_, _, err = runQualityReview(t, dir, nil, "model", "gemini", "gemini-3.8-flash-high", "--effort", "low")
	require.EqualError(t, err, "gemini-3.8-flash-high already names its level; drop --effort low or give the same level")
	_, _, err = runQualityReview(t, dir, nil, "model", "gemini", "gemini-4-argon", "--effort", "high")
	require.EqualError(t, err, "gemini-4-argon does not take effort high for gemini (accepted: medium)")
	_, _, err = runQualityReview(t, dir, nil, "model", "gemini", "claude-opus-5-5-low")
	require.EqualError(t, err, "claude-opus-5-5-low is not a choice for gemini "+
		"(accepted: gemini-3.8-flash-high, gemini-3.8-flash-low, gemini-4-argon-medium)")

	useReviewPath(t)
	_, _, err = runQualityReview(t, dir, nil, "model", "gemini", "gemini-3.8-flash")
	require.EqualError(t, err, "gemini (agy) has no model list Autopus can attest, so its model stays; "+
		"install OMP to switch it to the OMP backend")
}

// An id listed without a suffix is both a model and the name of its group.
func TestQualityReviewModel_AgyIDThatNamesAGroup(t *testing.T) {
	useQualityReviewSeams(t, "agy")
	useAgyModels(t, "gemini-3.1-pro\tGemini 3.1 Pro\ngemini-3.1-pro-high\tGemini 3.1 Pro (High)\n", nil)
	dir := writeReviewRL(t)

	out, _, err := runQualityReview(t, dir, nil, "model", "gemini", "gemini-3.1-pro", "--dry-run")
	require.NoError(t, err, out)
	assert.Contains(t, out, "--model none → gemini-3.1-pro\n")
	out, _, err = runQualityReview(t, dir, nil, "model", "gemini", "gemini-3.1-pro", "--effort", "high", "--dry-run")
	require.NoError(t, err, out)
	assert.Contains(t, out, "--model none → gemini-3.1-pro-high\n")
	_, _, err = runQualityReview(t, dir, nil, "model", "gemini", "gemini-3.1-pro", "--effort", "low")
	require.EqualError(t, err, "gemini-3.1-pro does not take effort low for gemini (accepted: high)")
}

// A target value written as a placeholder keeps the provider as it is, and
// the expanded value never reaches the error (REQ-RVM-26).
func TestQualityReviewModel_PlaceholderValueStays(t *testing.T) {
	useQualityReviewSeams(t)
	t.Setenv("REVIEW_CLAUDE_MODEL", "claude-sonnet-5-5")
	dir := writeReviewRL(t)
	replaceReviewFile(t, dir, reviewRLClaudeArgs, `            args: [--print, --model, "${REVIEW_CLAUDE_MODEL}", --effort, max]`)
	before := readReviewFile(t, dir)

	_, _, err := runQualityReview(t, dir, nil, "model", "claude", "claude-opus-5-5")

	require.EqualError(t, err, "--model of claude comes from an environment variable; this tab leaves it: ${REVIEW_CLAUDE_MODEL}")
	assert.Equal(t, before, readReviewFile(t, dir))
}
