package cli

// SPEC-MODELUX-001 REQ-MUX-08 and REQ-MUX-12 (acceptance S9, S12, S13): the
// per-tool editor saves through the quality block writer, so it refuses an
// environment value or a YAML reference in the quality block and keeps every
// byte outside it, and it never stores its tiers under a customized built-in
// preset name.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// S13 F1 through the per-tool editor: the alias carries the loader-expanded
// value into balanced.description without a ${ inside the quality block. The
// write is refused whether or not the variable is set, the file stays
// byte-identical, and the value never reaches it.
func TestQualityToolEditor_RefusesTheF001FixtureAndLeaksNothing(t *testing.T) {
	for _, set := range []bool{true, false} {
		t.Run(fmt.Sprintf("PRESET_NOTE set=%t", set), func(t *testing.T) {
			setPresetNote(t, set)
			dir := writeBlockFixture(t, blockLeakAlias)
			want := sha256.Sum256([]byte(blockLeakAlias))

			out, err := runQualityToolWizard(t, dir, "claude-code\nexecutor=fable\n\n\ny\n")

			require.ErrorIs(t, err, errQualityWriteRefused, out)
			assert.Equal(t, qualityWriteRefusalText, err.Error())
			written := readBlockFixture(t, dir)
			assert.Equal(t, want, sha256.Sum256([]byte(written)), "a refused write leaves the file byte-identical")
			assert.NotContains(t, written, presetNoteSecret)
		})
	}
}

// S12's per-tool write on probe A1's hand-written file W: the preset and its
// provider binding land in the quality block, and every byte before and
// after the block stays as the operator wrote it.
func TestQualityToolEditor_WritesThroughTheBlockWriterAndKeepsEveryOuterByte(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	original := blockHandPrefix + blockHandQuality + blockHandSuffix
	dir := writeBlockFixture(t, original)

	out, err := runQualityToolWizard(t, dir, "claude-code\nexecutor=fable\n\n\ny\n")

	require.NoError(t, err, out)
	written := readBlockFixture(t, dir)
	require.True(t, strings.HasPrefix(written, blockHandPrefix), "bytes before the block changed:\n%s", written)
	require.True(t, strings.HasSuffix(written, blockHandSuffix), "bytes after the block changed:\n%s", written)
	before, after := rawQualityOf(t, original), rawQualityOf(t, written)
	assert.Equal(t, map[string]string{config.QualityProviderClaude: "claude-custom"}, after.Providers)
	custom := after.Presets["claude-custom"]
	assert.Equal(t, "Claude Code custom agent tiers", custom.Description)
	assert.Len(t, custom.Agents, len(config.CanonicalAgentNames()))
	assert.Equal(t, "fable", custom.Agents["executor"])
	assert.Equal(t, before.Default, after.Default, "a tool-scoped edit keeps the shared quality mode")
	for name, preset := range before.Presets {
		assert.Equal(t, preset, after.Presets[name], "untouched preset %s keeps its raw value", name)
	}
	assert.Contains(t, out, "quality.presets.claude-custom written; quality.providers.claude = claude-custom")
}

// writeComposeRFixture writes acceptance fixture R (this repository's
// autopus.yaml copy), R2 (R with only Claude Code and Codex), or C (R with
// ultra devops: sonnet) as a temp project's autopus.yaml.
func writeComposeRFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(composeTableTestdata, "R.yaml"))
	require.NoError(t, err)
	content := string(data)
	replace := func(old, replacement string) {
		require.Equal(t, 1, strings.Count(content, old), "fixture R holds %q once", old)
		content = strings.Replace(content, old, replacement, 1)
	}
	switch name {
	case "R":
	case "R2":
		replace("    - antigravity-cli\n    - opencode\n    - omp\n", "")
	case "C":
		replace("                devops: opus\n", "                devops: sonnet\n")
	default:
		t.Fatalf("unknown fixture %q", name)
	}
	return writeBlockFixture(t, content)
}

// S9: the per-tool editor reached from the composition prompt refuses the
// customized ultra of C every time, cancels on the third refusal, and writes
// nothing.
func TestQualityToolEditor_RefusesACustomizedBuiltinPresetName(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	setComposeLanguage(t, "ko")
	dir := writeComposeRFixture(t, "C")
	before := composeSessionHash(t, dir)

	out, err := runQualityToolWizard(t, dir, "claude-code\ndevops=fable\n\nultra\nultra\nultra\n")

	require.NoError(t, err, out)
	refusal := "사용자 수정 프리셋 ultra는 덮어쓰지 않습니다. 다른 이름을 입력하세요.\n"
	assert.Equal(t, 3, strings.Count(out, refusal), out)
	assert.Contains(t, out, "Cancelled; no settings changed.\n")
	assert.NotContains(t, out, "Preview —", "a refused name never reaches the preview")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// A current or historical built-in name stays an explicit target, and an
// English locale reads the refusal in English.
func TestQualityToolEditor_GuardsOnlyCustomizedBuiltins(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	setComposeLanguage(t, "en")
	dir := writeComposeRFixture(t, "C")
	before := composeSessionHash(t, dir)

	out, err := runQualityToolWizard(t, dir, "claude-code\ndevops=fable\n\nultra\nbalanced\nn\n")

	require.NoError(t, err, out)
	assert.Contains(t, out, "Customized preset ultra is not overwritten. Enter another name.\n")
	assert.Contains(t, out, "Preview — Claude Code preset \"balanced\" (no changes yet)",
		"historical balanced is an explicit target")
	assert.Contains(t, out, "Cancelled; no settings changed.\n")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// S9: on C the status line labels ultra customized, and an advanced edit
// equal to the current ultra default binds custom instead, so ultra's agents
// and description keep their values.
func TestQualityCompose_CustomizedBuiltinBindsCustomAndKeepsItsValues(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	dir := writeComposeRFixture(t, "C")
	original := rawQualityOf(t, readBlockFixture(t, dir))
	latest := config.DefaultFullConfig("").Quality.Presets["ultra"].Agents
	var input strings.Builder
	input.WriteString("a\n")
	for _, agent := range config.CanonicalAgentNames() {
		input.WriteString(agent + "=" + latest[agent] + "\n")
	}
	input.WriteString("\ny\n")

	out, err := runQualityCompose(t, dir, strings.NewReader(input.String()))

	require.NoError(t, err, out)
	assert.Contains(t, out, "quality.default = ultra   ultra: 사용자 수정 (12개 다름)   balanced: 이전 버전 기본값 (갱신 가능)\n")
	written := rawQualityOf(t, readBlockFixture(t, dir))
	assert.Equal(t, "custom", written.Default)
	assert.Equal(t, latest, written.Presets["custom"].Agents)
	assert.Equal(t, original.Presets["ultra"], written.Presets["ultra"], "customized ultra keeps its values")
}

// setComposeLanguage pins the locale every composition and editor string
// follows.
func setComposeLanguage(t *testing.T, lang string) {
	t.Helper()
	original := composeLanguage
	t.Cleanup(func() { composeLanguage = original })
	composeLanguage = func() string { return lang }
}
