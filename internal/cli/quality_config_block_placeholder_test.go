package cli

// SPEC-MODELUX-001 REQ-MUX-08, security review F1: the raw decode resolves
// YAML escapes and tags, so a quality value spelled "\x24{NAME}" decodes to
// the text ${NAME}. Re-encoding that value spells the placeholder out, and
// config.Load expands it on the next read. The block writer must never turn
// an inactive placeholder into one the loader expands, whether or not the
// variable is set while it writes.

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// blockPlaceholderFixture is a quality block whose balanced description is
// spelled as description, with comment as an extra line inside the block.
func blockPlaceholderFixture(comment, description string) string {
	return "project_name: demo\nmode: full\nplatforms: [claude-code]\nquality:\n" + comment +
		"  default: balanced\n  presets:\n    balanced:\n      description: " + description +
		"\n      agents:\n        executor: sonnet\n    ultra:\n      description: u\n"
}

// blockInactivePlaceholders spells ${PRESET_NOTE} in the four forms the
// loader does not expand but the raw decode resolves to that text.
func blockInactivePlaceholders() map[string]string {
	return map[string]string{
		"hex escape":                      `"\x24{PRESET_NOTE}"`,
		"unicode escape":                  "\"\\u0024{PRESET_NOTE}\"",
		"binary tag":                      "!!binary " + base64.StdEncoding.EncodeToString([]byte("${PRESET_NOTE}")),
		"double-quoted line continuation": "\"$\\\n        {PRESET_NOTE}\"",
	}
}

func TestSaveQualityBlock_RefusesToActivateAnInactivePlaceholder(t *testing.T) {
	cases := map[string]string{}
	for form, description := range blockInactivePlaceholders() {
		cases[form] = blockPlaceholderFixture("", description)
	}
	// A placeholder in a comment of the block is never expanded into data,
	// so it cannot vouch for the one the rewrite would spell out.
	cases["hex escape beside a commented placeholder"] = blockPlaceholderFixture(
		"  # ${PRESET_NOTE}\n", `"\x24{PRESET_NOTE}"`)
	for name, content := range cases {
		for _, set := range []bool{true, false} {
			t.Run(name+map[bool]string{true: "/variable set", false: "/variable unset"}[set], func(t *testing.T) {
				setPresetNote(t, set)
				dir := writeBlockFixture(t, content)
				want := sha256.Sum256([]byte(content))
				cfg, err := config.LoadPreview(dir)
				require.NoError(t, err)
				require.Equal(t, "${PRESET_NOTE}", cfg.Quality.Presets["balanced"].Description,
					"the loader reads the placeholder as text")

				err = saveQualityBlock(dir, cfg, compositionCustomEdit)

				require.ErrorIs(t, err, errQualityWriteRefused)
				assert.Equal(t, qualityWriteRefusalText, err.Error())
				assert.Equal(t, want, sha256.Sum256([]byte(readBlockFixture(t, dir))), "the file stays byte-identical")
				assert.Equal(t, "balanced", cfg.Quality.Default, "a refused write leaves the caller's config alone")
				setPresetNote(t, true)
				reloaded, err := config.LoadPreview(dir)
				require.NoError(t, err)
				assert.NotContains(t, reloaded.Quality.Presets["balanced"].Description, presetNoteSecret)
			})
		}
	}
}

// Placeholders are compared where they land, not only by name: the active
// ${PRESET_NOTE} agent key of the custom preset the edit overwrites cannot
// vouch for the escaped description the rewrite would activate (F1-b).
func TestSaveQualityBlock_RefusesToMoveAnActivePlaceholder(t *testing.T) {
	setPresetNote(t, false)
	content := blockPlaceholderFixture("", `"\x24{PRESET_NOTE}"`) +
		"    custom:\n      agents:\n        ${PRESET_NOTE}: opus\n"
	dir := writeBlockFixture(t, content)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	err = saveQualityBlock(dir, cfg, compositionCustomEdit)

	require.ErrorIs(t, err, errQualityWriteRefused)
	assert.Equal(t, content, readBlockFixture(t, dir))
}

// A placeholder the block already spells out keeps its text and its count,
// and one only in a comment of the block goes with the comment: neither is a
// placeholder the rewrite activates.
func TestSaveQualityBlock_KeepsPlaceholdersItDoesNotActivate(t *testing.T) {
	setPresetNote(t, false)
	for name, tc := range map[string]struct {
		content, description string
	}{
		"literal placeholder": {content: blockPlaceholderFixture("", "'${PRESET_NOTE}'"), description: "${PRESET_NOTE}"},
		"commented placeholder": {
			content: blockPlaceholderFixture("  # ${PRESET_NOTE}\n", "b"), description: "b",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeBlockFixture(t, tc.content)
			cfg, err := config.LoadPreview(dir)
			require.NoError(t, err)

			require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

			written := rawQualityOf(t, readBlockFixture(t, dir))
			assert.Equal(t, tc.description, written.Presets["balanced"].Description)
			assert.Equal(t, "custom", written.Default)
		})
	}
}

// qualityPlaceholderPattern must match exactly the placeholders the loader
// expands, or the writer would count a different set than the loader reads.
func TestQualityPlaceholderPattern_MatchesTheLoader(t *testing.T) {
	t.Setenv("ZQ_PIN", "expanded")
	t.Setenv("_ZQ9", "expanded")
	for _, spelled := range []string{
		"${ZQ_PIN}", "${_ZQ9}", "$${ZQ_PIN}", "a${ZQ_PIN}b",
		"$ZQ_PIN", "${ZQ_PIN", "${ ZQ_PIN}", "${ZQ-PIN}", "${}", "$(ZQ_PIN)",
	} {
		t.Run(spelled, func(t *testing.T) {
			data := "project_name: demo\nmode: full\nplatforms: [claude-code]\nquality:\n  default: balanced\n" +
				"  presets:\n    balanced:\n      description: '" + spelled + "'\n    ultra:\n      description: u\n"
			dir := writeBlockFixture(t, data)
			loaded, err := config.LoadPreview(dir)
			require.NoError(t, err)

			expanded := loaded.Quality.Presets["balanced"].Description != spelled

			assert.Equal(t, qualityPlaceholderPattern.MatchString(spelled), expanded)
		})
	}
}
