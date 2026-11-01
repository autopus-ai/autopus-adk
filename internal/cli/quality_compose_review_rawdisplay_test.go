package cli

// SPEC-REVMODEL-001 security round (REQ-RVM-26): every value the review tab
// and its menu summary show comes from autopus.yaml as written. A value the
// raw document does not hold itself — merged, aliased, or behind a
// placeholder — gets a refusal line, never its expanded text. Each test
// plants one sentinel the screen must never show.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reviewSentinel = "rvm-sentinel-7f3a9c"

// An OMP model a merge key brings in has no raw view: the merged provider
// gets the no-view line, not a keep label with the expanded model.
func TestReviewRawDisplay_MergedOMPModel(t *testing.T) {
	t.Setenv("REVIEW_MERGED_MODEL", "google-antigravity/"+reviewSentinel+":high")
	dir := writeReviewR3(t)
	replaceReviewFile(t, dir, "        gemini:\n            backend: omp\n            model: google-antigravity/gemini-3.1-pro:high\n",
		"        gemini: &g\n            backend: omp\n            model: ${REVIEW_MERGED_MODEL}\n")
	replaceReviewFile(t, dir, "    commands:\n", "        zeta:\n            <<: *g\n    commands:\n")

	// participation ×3, zeta 켜기, claude Enter, codex Enter, judge Enter, check Enter, 취소, the menu's default.
	run := runReviewForms(t, dir, nil, reviewLines(reviewOpen,
		[]string{"\n", "\n", "\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "gemini의 model 값이 환경 변수라 이 탭에서 바꾸지 않습니다: ${REVIEW_MERGED_MODEL}\n")
	assert.Contains(t, run.out, "autopus.yaml 원문에서 zeta 항목을 읽지 못해 이 탭에서 model을 바꾸지 않습니다\n")
	assert.NotContains(t, run.out, "zeta model \n")
	assert.NotContains(t, run.out, reviewSentinel)
}

// CLI args a merge key brings in have no raw view either: no keep label
// spells their expanded model.
func TestReviewRawDisplay_MergedCLIArgs(t *testing.T) {
	t.Setenv("REVIEW_MERGED_ARG", reviewSentinel)
	dir := writeReviewRL(t)
	replaceReviewFile(t, dir, "        claude:\n            binary: claude\n"+reviewRLClaudeArgs+"\n",
		"        claude: &c\n            binary: claude\n"+`            args: [--print, --model, "${REVIEW_MERGED_ARG}", --effort, max]`+"\n")
	replaceReviewFile(t, dir, "    commands:\n", "        zeta:\n            <<: *c\n    commands:\n")

	// participation ×3, zeta 켜기, codex Enter, judge Enter, check Enter, 취소, the menu's default.
	run := runReviewCLIForms(t, dir, nil, reviewLines(reviewOpen,
		[]string{"\n", "\n", "\n", "2\n", "\n", "\n", "\n", "\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "claude의 --model 값이 환경 변수라 이 탭에서 바꾸지 않습니다: ${REVIEW_MERGED_ARG}\n")
	assert.Contains(t, run.out, "autopus.yaml 원문에서 zeta 항목을 읽지 못해 이 탭에서 model을 바꾸지 않습니다\n")
	assert.NotContains(t, run.out, reviewSentinel)
}

// A placeholder binary decides no CLI row: the provider gets the value
// refusal line, and the after table spells the binary as written.
func TestReviewRawDisplay_PlaceholderBinary(t *testing.T) {
	t.Setenv("REVIEW_AGY_BIN", reviewSentinel)
	dir := writeReviewRL(t)
	replaceReviewFile(t, dir, "            binary: agy\n", "            binary: ${REVIEW_AGY_BIN}\n")

	// claude Enter, codex 끄기, gemini Enter, claude model Enter, judge Enter, check Enter, 취소, the menu's default.
	run := runReviewCLIForms(t, dir, []string{"omp"}, reviewLines(reviewOpen,
		[]string{"\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "gemini의 binary 값이 환경 변수라 이 탭에서 바꾸지 않습니다: ${REVIEW_AGY_BIN}\n")
	assert.NotContains(t, run.out, "gemini model \n")
	assert.Regexp(t, `(?m)^gemini\s+켜짐\s+\$\{REVIEW_AGY_BIN\}$`, run.out)
	assert.NotContains(t, run.out, reviewSentinel)
}

// A placeholder judge reaches the menu summary as written, and the judge
// select gives way to the value refusal line.
func TestReviewRawDisplay_PlaceholderJudge(t *testing.T) {
	t.Setenv("REVIEW_JUDGE", "codex")
	dir := writeReviewR3(t)
	replaceReviewFile(t, dir, "        judge: claude\n", "        judge: ${REVIEW_JUDGE}\n")

	// participation ×3, claude Enter, codex Enter, gemini Enter, check Enter, the menu's default.
	run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, []string{"\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, strings.TrimSuffix(reviewSummaryR3, "judge claude")+"judge ${REVIEW_JUDGE}\n")
	assert.Contains(t, run.out, "spec.review_gate의 judge 값이 환경 변수라 이 탭에서 바꾸지 않습니다: ${REVIEW_JUDGE}\n")
	assert.Contains(t, run.out, "변경 없음\n")
	assert.NotContains(t, run.out, "judge codex")
	assert.NotContains(t, run.out, "judge \n1.")
}

// A provider name from a placeholder cannot be shown as written: the menu
// prints no summary and the tab changes nothing.
func TestReviewRawDisplay_PlaceholderProviderName(t *testing.T) {
	t.Setenv("REVIEW_EXTRA", reviewSentinel)
	dir := writeReviewR3(t)
	replaceReviewFile(t, dir, "    commands:\n",
		"        ${REVIEW_EXTRA}:\n            backend: omp\n            model: google-antigravity/gemini-3.1-pro:high\n    commands:\n")

	run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, []string{"\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "autopus.yaml의 리뷰 설정을 원문 그대로 읽을 수 없어 이 탭은 아무것도 바꾸지 않습니다\n")
	assert.NotContains(t, run.out, "멀티프로바이더 리뷰: ")
	assert.NotContains(t, run.out, "리뷰 참여")
	assert.NotContains(t, run.out, reviewSentinel)
}

// Names read as written unless a provider key or list item holds a
// placeholder or the review gate borrows its mapping; an aliased list or
// judge reads as its anchor.
func TestReadReviewRaw_NamesAsWritten(t *testing.T) {
	read := func(text string) *reviewRawDocument {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(text), 0o644))
		return readReviewRaw(dir)
	}
	providers := "orchestra:\n    providers:\n        claude:\n            binary: claude\n"
	commands := "    commands:\n        review:\n            providers: &rp [claude, \"${X}\"]\n"

	plain := read("spec:\n    review_gate:\n        providers: [claude]\n        judge: ${J}\n" + providers)
	assert.True(t, plain.Names)
	assert.Equal(t, "${J}", plain.Judge)
	assert.False(t, read(providers+"        ${X}:\n            binary: codex\n").Names, "placeholder key")
	assert.False(t, read("spec:\n    review_gate:\n        providers: [claude, \"${X}\"]\n"+providers).Names, "placeholder item")
	assert.False(t, read(providers+commands+"spec:\n    review_gate:\n        providers: *rp\n").Names, "aliased list naming a placeholder")
	assert.False(t, read(providers+"    commands:\n        review: &g {providers: [claude]}\nspec:\n    review_gate:\n        <<: *g\n").Names, "merged gate")
	aliased := read(providers + "    judge: &j claude\nspec:\n    review_gate:\n        providers: [claude]\n        judge: *j\n")
	assert.True(t, aliased.Names)
	assert.Equal(t, "claude", aliased.Judge)
}

// The live check's missing-binary line names the provider and its binary as
// written, or the provider alone for a placeholder binary; the runtime
// warning, which spells the expanded binary, never runs from the tab. The
// read-only gate admits only the native binary, so the placeholder expands
// to agy here and the line must not name it.
func TestReviewRawDisplay_LiveCheckMissingBinary(t *testing.T) {
	t.Setenv("REVIEW_AGY_BIN", "agy")
	// participation ×3, claude Enter, codex Enter, judge Enter, 확인 호출, the menu's default.
	lines := reviewLines(reviewOpen, []string{"\n", "\n", "\n", "\n", "\n", "\n", "2\n", "\n"})
	for name, tc := range map[string]struct{ binary, want string }{
		"placeholder": {"${REVIEW_AGY_BIN}", "경고: gemini의 바이너리를 찾을 수 없어 확인 호출에서 건너뜁니다\n"},
		"as written":  {"agy", "경고: gemini의 agy 바이너리를 찾을 수 없어 확인 호출에서 건너뜁니다\n"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeReviewRL(t)
			replaceReviewFile(t, dir, "            binary: agy\n", "            binary: "+tc.binary+"\n")

			var run reviewRun
			stderr := captureStderr(t, func() { run = runReviewCLIForms(t, dir, []string{"claude", "codex"}, lines...) })

			require.NoError(t, run.err, run.out)
			assert.Contains(t, run.out, tc.want)
			assert.NotContains(t, stderr, "바이너리를 찾을 수 없습니다")
			if name == "placeholder" {
				assert.NotContains(t, run.out+stderr, "agy 바이너리")
			}
			assert.Len(t, run.smoke.recorded(), 2, "claude and codex still get their request")
		})
	}
}

// A provider key that is not a plain scalar of its own is no name as
// written: an alias key reads as its anchor's name, not the placeholder the
// anchor holds, and an anchored or tagged key is not plain either.
func TestReadReviewRaw_OnlyPlainProviderKeys(t *testing.T) {
	read := func(text string) *reviewRawDocument {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(text), 0o644))
		return readReviewRaw(dir)
	}
	head := "orchestra:\n    commands:\n        review:\n            providers: [&k \"${X}\"]\n    providers:\n        claude:\n            binary: claude\n"

	assert.True(t, read(head).Names, "plain keys")
	assert.True(t, read(head+"        \"codex\":\n            binary: codex\n").Names, "a quoted key is its own text")
	for name, key := range map[string]string{"alias key": "*k ", "anchored key": "&a codex", "tagged key": "!!str codex"} {
		raw := read(head + "        " + key + ":\n            binary: codex\n")
		assert.False(t, raw.Names, name)
		assert.NotContains(t, raw.Providers, "k", name)
	}
}
