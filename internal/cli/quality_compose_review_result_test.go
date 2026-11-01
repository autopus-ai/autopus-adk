package cli

// The review summary names each provider's model, the line that ends the
// review tab stays visible under the redrawn table, and a run without the
// tab names where review models change.

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	reviewSummaryR3 = "멀티프로바이더 리뷰: claude anthropic/claude-fable-5-1:max · codex openai-codex/gpt-6-astra:max · " +
		"gemini google-antigravity/gemini-3.1-pro:high · judge claude"
	reviewSummaryR3Off = "멀티프로바이더 리뷰: claude anthropic/claude-fable-5-1:max · codex openai-codex/gpt-6-astra:max · " +
		"gemini(꺼짐) google-antigravity/gemini-3.1-pro:high · judge claude"
)

// A CLI provider shows its binary only when it is not the provider's own
// name, and its model and effort as written; an OMP one its selector.
func TestReviewSummary_NamesEachProvidersModel(t *testing.T) {
	dir := writeReviewRL(t, func(cfg *config.HarnessConfig) { cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"} })
	replaceReviewFile(t, dir, `            args: [--print, ""]`, `            args: [--print, "", --model, gemini-3.8-flash-high]`)

	ko := runReviewFormsIn(t, dir, "ko", "\n")
	en := runReviewFormsIn(t, dir, "en", "\n")

	assert.Contains(t, ko, "\n멀티프로바이더 리뷰: claude claude-fable-5-1 · max · codex gpt-6-astra · max · "+
		"gemini(꺼짐) agy gemini-3.8-flash-high · judge claude\n구성을 바꿀까요?")
	assert.Contains(t, en, "\nMulti-provider review: claude claude-fable-5-1 · max · codex gpt-6-astra · max · "+
		"gemini (off) agy gemini-3.8-flash-high · judge claude\nChange the composition?")
	assert.Contains(t, runReviewFormsIn(t, writeReviewR3(t), "ko", "\n"), "\n"+reviewSummaryR3+"\n구성을 바꿀까요?")
}

// The line that ends the tab follows the summary of the redrawn table once:
// applied, cancelled, or no change.
func TestReviewTab_ResultLineStaysUnderTheRedrawnTable(t *testing.T) {
	cases := []struct {
		name, summary, result string
		answers               []string
	}{
		{"applied", reviewSummaryR3Off, "구성을 적용했습니다.", reviewTurnOffGemini},
		{"cancelled", reviewSummaryR3, "취소했습니다. 설정을 바꾸지 않았습니다.", []string{"\n", "\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n"}},
		{"no change", reviewSummaryR3, "변경 없음", reviewKeepAll},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := runReviewForms(t, writeReviewR3(t), nil, reviewLines(reviewOpen, tc.answers)...)

			require.NoError(t, run.err, run.out)
			assert.Contains(t, run.out, "\n"+tc.summary+"\n"+tc.result+"\n구성을 바꿀까요?")
			assert.Equal(t, 2, strings.Count(run.out, tc.result+"\n"), "printed by the tab, then once under the table")
		})
	}
}

// Entering any tab drops the kept line: a review pass left with Esc after
// its no-change line, then another tab, brings no review line back.
func TestReviewTab_EnteringAnotherTabDropsTheResultLine(t *testing.T) {
	head := reviewLines(reviewOpen, []string{"\n", "\n", "\n", "\n", "\n", "\n", "\n", "b\n"})
	run := func(t *testing.T, lines ...string) string {
		composeSessionCatalog(t, "CAT-A")
		runner := &ompCLIFakeRunner{catalog: []byte(reviewCatalogCATG)}
		useReviewSmokeBackend(t, "omp")
		session := newComposeFormSession(t, writeReviewR3(t))
		useNavComposeForms(t)
		cmd, out := newComposeTestCommand()
		cmd.SetIn(formInput(lines...))
		_, err := session.runForms(cmd, composeFormOptions{OMP: &composeOMPTab{deps: ompPlatformDependencies{
			newRunner: func() omp.OMPModelCatalogRunner { return runner },
		}}})
		require.NoError(t, err, out.String())
		return out.String()
	}

	back := run(t, reviewLines(head, []string{"b\n", "\n"})...)
	other := run(t, reviewLines(head, []string{"1\n", "b\n", "b\n", "\n"})...)

	assert.Equal(t, 2, strings.Count(back, "변경 없음\n"), "back at the menu, the pass's line is still its last word")
	assert.Equal(t, 1, strings.Count(other, "변경 없음\n"))
}

// Non-interactive update and init, and auto quality without a terminal,
// name where review models change while the config lists the review
// settings; an interactive run and a project without them print nothing.
func TestReviewHint_RunsWithoutTheTab(t *testing.T) {
	hint := composeReviewTextKO.Hint + "\n"
	t.Run("update --yes", func(t *testing.T) {
		newComposeUpdateRun(t, true, "")
		dir, _ := composeSessionProject(t, "D")
		out, err := runComposeUpdate(t, strings.NewReader(""), "--dir", dir, "--yes")
		require.NoError(t, err, out)
		assert.Equal(t, 1, strings.Count(out, hint))

		cfg, err := config.Load(dir)
		require.NoError(t, err)
		cfg.Spec.ReviewGate.Enabled = false
		require.NoError(t, config.Save(dir, cfg))
		out, err = runComposeUpdate(t, strings.NewReader(""), "--dir", dir, "--yes")
		require.NoError(t, err, out)
		assert.NotContains(t, out, hint)
	})
	t.Run("auto quality without a terminal", func(t *testing.T) {
		newComposeUpdateRun(t, false, "")
		dir, _ := composeSessionProject(t, "D")
		root := NewRootCmd()
		var out, stderr bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&stderr)
		root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality"})

		require.EqualError(t, root.Execute(), qualityComposeTTYError)
		assert.Empty(t, out.String())
		assert.Equal(t, hint, stderr.String())
	})
	t.Run("init --yes", func(t *testing.T) {
		newReviewInitRun(t, true)
		_, out, err := runComposeInit(t, strings.NewReader(""), "--yes")
		require.NoError(t, err, out)
		assert.Contains(t, out, "Review Gate     true (3 providers)")
		assert.Contains(t, out, "  ℹ "+hint)
	})
	t.Run("interactive init", func(t *testing.T) {
		newReviewInitRun(t, true)
		_, out, err := runComposeInit(t, formInput("\n"))
		require.NoError(t, err, out)
		assert.NotContains(t, out, hint)
	})
}
