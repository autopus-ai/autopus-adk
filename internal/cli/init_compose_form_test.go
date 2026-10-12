package cli

// SPEC-MODELUX-002 T7 (REQ-MUX2-14; acceptance S12): Ctrl+C in init's
// composition forms cancels init before it writes any file.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

func TestInitComposeForms_InterruptAtTheToolTabCancelsInit(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	stubInitWizard(t, tui.QualityRoleGroups)
	abortComposeFormAt(t, 2)

	dir, out, err := runComposeInit(t, formInput("2\n"))

	require.NoError(t, err, out)
	assert.Contains(t, out, "중단했습니다. 구성을 더 바꾸지 않습니다.\n")
	assert.Contains(t, out, "init cancelled.\n")
	for _, path := range []string{"autopus.yaml", "DESIGN.md", ".claude", ".codex"} {
		assert.NoFileExists(t, filepath.Join(dir, path))
		assert.NoDirExists(t, filepath.Join(dir, path))
	}
}
