package cli

// SPEC-MODELUX-001 REQ-MUX-08 block boundary edges: document markers, a byte
// order mark, retired keys, and empty maps.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestSaveQualityBlock_DocumentEndMarkerBoundsTheBlock(t *testing.T) {
	prefix := "project_name: demo\nmode: full\nplatforms: [claude-code]\n"
	quality := "quality:\n  default: balanced\n  presets:\n    balanced:\n      description: b\n"
	suffix := "...\n# after the document\n"
	for name, content := range map[string]string{"with a quality block": prefix + quality + suffix, "without one": prefix + suffix} {
		t.Run(name, func(t *testing.T) {
			dir := writeBlockFixture(t, content)
			cfg, err := config.LoadPreview(dir)
			require.NoError(t, err)

			require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

			written := readBlockFixture(t, dir)
			assert.True(t, strings.HasPrefix(written, prefix), written)
			assert.True(t, strings.HasSuffix(written, suffix), written)
			loaded, err := config.Load(dir)
			require.NoError(t, err)
			assert.Equal(t, "custom", loaded.Quality.Default)
		})
	}
}

func TestSaveQualityBlock_KeepsTheByteOrderMarkBeforeALeadingQualityBlock(t *testing.T) {
	const bom = "\xef\xbb\xbf"
	quality := "quality:\n  default: balanced\n  presets:\n    balanced:\n      description: b\n"
	suffix := "project_name: demo\nmode: full\nplatforms: [claude-code]\n"
	dir := writeBlockFixture(t, bom+quality+suffix)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

	written := readBlockFixture(t, dir)
	assert.True(t, strings.HasPrefix(written, bom+"quality:\n"), "%q", written)
	assert.True(t, strings.HasSuffix(written, suffix), "%q", written)
}

func TestSaveQualityBlock_WritesOverThePrunedBytesLikeEveryQualityWriter(t *testing.T) {
	// The retired orchestra keys never ride along (SPEC-PANERM-001 REQ-10):
	// the outer bytes are the original's with the retired line cut.
	prefix := "project_name: demo\nmode: full\nplatforms: [claude-code]\n" +
		"orchestra:\n  providers:\n    codex:\n      binary: codex\n"
	retired := "      pane_args: [--yolo]\n"
	quality := "quality:\n  default: balanced\n  presets:\n    balanced:\n      description: b\n"
	dir := writeBlockFixture(t, prefix+retired+quality)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

	written := readBlockFixture(t, dir)
	assert.True(t, strings.HasPrefix(written, prefix+"quality:\n"), "%q", written)
	assert.NotContains(t, written, "pane_args")
}

func TestSaveQualityBlock_EmptyMapsDoNotBlockTheWrite(t *testing.T) {
	// The encoder omits an empty map, so the written block decodes without
	// it; that must not read as a changed value.
	original := "project_name: demo\nmode: full\nplatforms: [claude-code]\n" +
		"quality:\n  default: balanced\n  providers: {}\n  presets:\n    balanced:\n      description: b\n      agents: {}\n"
	dir := writeBlockFixture(t, original)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

	loaded, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "custom", loaded.Quality.Default)
	assert.Equal(t, "b", loaded.Quality.Presets["balanced"].Description)
}
