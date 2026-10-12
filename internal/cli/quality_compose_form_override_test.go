package cli

// SPEC-MODELUX-002 T8 (REQ-MUX2-18; acceptance S16): in the form flow the
// provider override question is a huh Confirm whose default is no, asked
// before the preview, and a confirmed removal is written in the same quality
// block edit as the composition.

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// composeFormS16 answers S16 up to the override question over P: 구성 편집,
// 모든 도구 같게, 역할 그룹, 설계·판단 on opus, the other groups kept.
var composeFormS16 = []string{"2\n", "1\n", "\n", "3\n", "\n", "\n", "\n"}

const composeFormOverrideQuestion = "Codex도 새 구성을 따를까요? (quality.providers.codex = codex-custom 제거) [y/N]"

// countQualityRenames counts the atomic replaces of autopus.yaml.
func countQualityRenames(t *testing.T) *int {
	t.Helper()
	original := renameQualityConfig
	t.Cleanup(func() { renameQualityConfig = original })
	renames := new(int)
	renameQualityConfig = func(from, to string) error {
		*renames++
		return original(from, to)
	}
	return renames
}

// Enter keeps the override: the Codex column keeps following codex-custom
// and no providers line is planned.
func TestComposeFormOverride_EnterKeepsTheOverride(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeProjectP(t)
	before := composeSessionHash(t, dir)

	out, err := runQualityForms(t, dir, nil, formInput(composeFormLines(composeFormS16, "\n", "\n", "\n")...))

	require.NoError(t, err, out)
	assert.Contains(t, out, composeFormOverrideQuestion)
	question := strings.Index(out, composeFormOverrideQuestion)
	assert.Less(t, question, strings.Index(out, composeFormPreviewMarker), "asked before the preview")
	assert.Contains(t, composeTableHeader(t, out, composeFormPreviewMarker), "Codex (개별 설정: codex-custom)")
	assert.NotContains(t, out, "quality.providers.codex 제거")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// y removes the override in the preview, and the confirmed write removes it
// with the composition in one atomic replace.
func TestComposeFormOverride_ConfirmedRemovalJoinsTheCompositionWrite(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	renames := countQualityRenames(t)
	dir := writeComposeProjectP(t)

	out, err := runQualityForms(t, dir, nil, formInput(composeFormLines(composeFormS16, "y\n", "2\n", "\n")...))

	require.NoError(t, err, out)
	assert.Contains(t, out, "  quality.providers.codex 제거\n")
	assert.Contains(t, out, "  "+composeToolBlockNotice+"\n")
	preview := composeRenderedTable(t, out, composeFormPreviewMarker)
	assert.True(t, strings.HasSuffix(preview["planner"]["Codex"], "*"), preview["planner"]["Codex"])
	saved := composeBindLoad(t, dir).Quality
	assert.Equal(t, "custom", saved.Default)
	assert.NotContains(t, saved.Providers, "codex")
	assert.Equal(t, 1, *renames)
}

// A refused write leaves both the override and the composition as they
// were, and the table drawn next still follows the override.
func TestComposeFormOverride_FailedWriteKeepsBoth(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeProjectP(t)
	before := composeSessionHash(t, dir)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	out, err := runQualityForms(t, dir, nil, formInput(composeFormLines(composeFormS16, "y\n", "2\n", "\n")...))

	require.NoError(t, err, out)
	assert.Equal(t, 1, strings.Count(out, "  경고: "), out)
	assert.Equal(t, before, composeSessionHash(t, dir))
	redrawn := out[strings.LastIndex(out, "quality.default = "):]
	assert.Contains(t, composeTableHeader(t, redrawn, strings.SplitN(redrawn, "\n", 2)[0]), "Codex (개별 설정: codex-custom)")
}
