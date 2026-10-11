package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// REQ-MUX-20 covers every editing entry point: an interactive update that
// changes the global binding asks about the Codex override before the preview,
// and following it removes only that provider entry.
func TestUpdateCompose_AsksWhetherAnOverrideFollowsBeforeThePreview(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	setComposeLanguage(t, "ko")
	dir := writeComposeProjectP(t)
	interactive := composeTTYProbe
	composeTTYProbe = func() bool { return false }
	out, err := runComposeUpdate(t, strings.NewReader(""), "--dir", dir)
	composeTTYProbe = interactive
	require.NoError(t, err, out)
	require.NoError(t, os.RemoveAll(run.codexLog))

	out, err = runComposeUpdate(t, strings.NewReader("g\n\n3\n\n4\ny\ny\n"), "--dir", dir)

	require.NoError(t, err, out)
	question := "Codex도 새 구성을 따를까요? (quality.providers.codex = codex-custom 제거) [y/N]\n"
	asked := strings.Index(out, question)
	require.GreaterOrEqual(t, asked, 0, out)
	assert.Less(t, asked, strings.Index(out, "미리보기 (아직 바뀌지 않음)\n"), "the question comes before the preview")
	written := rawQualityOf(t, readBlockFixture(t, dir))
	assert.Equal(t, "custom", written.Default)
	assert.Empty(t, written.Providers, "following the composition removes only the override")
	assert.Equal(t, "codex", written.Presets["codex-custom"].Description, "the override's preset keeps its values")
}
