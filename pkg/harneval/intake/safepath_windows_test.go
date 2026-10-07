//go:build windows

package intake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests in this file carry PlatformUnsupported in their names: the
// windows-runtime CI job lists them with -list 'PlatformUnsupported' and
// requires at least one to run and pass.

func TestAreaWrites_OnWindows_PlatformUnsupported(t *testing.T) {
	root := t.TempDir()
	a, err := openArea(root)
	require.NoError(t, err)
	defer a.close()

	assert.ErrorIs(t, writeSupported(), errPlatformUnsupported)
	assert.ErrorIs(t, a.ensureDir(IntakeDir), errPlatformUnsupported)
	assert.ErrorIs(t, a.createExclusive(IntakeDir+"/GTC-023e9302ff0b.json", []byte("{}\n")), errPlatformUnsupported)
	assert.ErrorIs(t, a.removeFile(IntakeDir+"/GTC-023e9302ff0b.json"), errPlatformUnsupported)
	assert.ErrorIs(t, a.syncDir("."), errPlatformUnsupported)
	assert.NoDirExists(t, root+`\evals`)
}
