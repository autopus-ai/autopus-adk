package cli

// SPEC-REVMODEL-001 T9, T10 (REQ-RVM-16, 17; acceptance S13): the summary
// line under every menu table while the tab is listed, the English text,
// and locale-independent writes.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// runReviewFormsIn is runReviewForms in lang.
func runReviewFormsIn(t *testing.T, dir, lang string, lines ...string) string {
	t.Helper()
	composeSessionCatalog(t, "CAT-A")
	runner := &ompCLIFakeRunner{catalog: []byte(reviewCatalogCATG)}
	deps := ompPlatformDependencies{newRunner: func() omp.OMPModelCatalogRunner { return runner }}
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	session := newComposeSession(dir, cfg, composeSessionSources(t), lang, composeSessionFile, nil)
	_, out, err := runComposeForms(t, session, composeFormOptions{OMP: &composeOMPTab{deps: deps}}, lines...)
	require.NoError(t, err, out)
	return out
}

func TestReviewTab_S13SummaryAndEnglishText(t *testing.T) {
	s1 := reviewLines(reviewOpen, reviewTurnOffGemini)
	ko, en := writeReviewR3(t), writeReviewR3(t)

	koOut := runReviewFormsIn(t, ko, "ko", s1...)
	enOut := runReviewFormsIn(t, en, "en", s1...)

	assert.Contains(t, koOut, "Codex catalog: 읽음 · 대체 0칸\n"+reviewSummaryR3+"\n구성을 바꿀까요?")
	applied := koOut[strings.Index(koOut, "구성을 적용했습니다.\n"):]
	assert.Contains(t, applied, "\n"+reviewSummaryR3Off+"\n구성을 적용했습니다.\n구성을 바꿀까요?")
	assert.Contains(t, enOut, "Which tool? \n1. same for all tools\n2. Claude Code\n3. Codex\n4. OMP\n5. Multi-provider review\n")
	assert.Contains(t, enOut, "gemini takes part in review \n1. keep — on\n2. turn off\n")
	assert.Contains(t, enOut, "\nMulti-provider review: claude anthropic/claude-fable-5-1:max · codex openai-codex/gpt-6-astra:max · "+
		"gemini google-antigravity/gemini-3.1-pro:high · judge claude\nChange the composition?")
	assert.Contains(t, enOut, "\nMulti-provider review: claude anthropic/claude-fable-5-1:max · codex openai-codex/gpt-6-astra:max · "+
		"gemini (off) google-antigravity/gemini-3.1-pro:high · judge claude\nComposition applied.\n")
	assert.Contains(t, enOut, "Turned-off providers still run in orchestra commands (brainstorm, plan, review, secure) and in auto spec review --multi\n")
	assert.Contains(t, enOut, "provider  review  model")
	assert.NotContains(t, enOut, "리뷰")
	assert.Equal(t, readReviewFile(t, ko), readReviewFile(t, en))
}

// The summary names judge 없음 for an empty judge and stays away when the
// tab is not listed.
func TestReviewTab_SummaryEdges(t *testing.T) {
	dir := writeReviewR3Judge(t, "")
	out := runReviewFormsIn(t, dir, "ko", "\n")
	assert.Contains(t, out, strings.TrimSuffix(reviewSummaryR3, "judge claude")+"judge 없음\n")

	dir = writeReviewR3(t, func(cfg *config.HarnessConfig) { cfg.Spec.ReviewGate.Enabled = false })
	out = runReviewFormsIn(t, dir, "ko", "\n")
	assert.NotContains(t, out, "멀티프로바이더 리뷰")
}
