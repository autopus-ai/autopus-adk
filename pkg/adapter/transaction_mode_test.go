package adapter

// A transaction rewrite keeps the mode of the file it replaces, so a private
// file (an opencode.json or settings file holding credentials at 0600) is
// never widened to the generated default; a new file takes the requested mode.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyTransaction_RewriteKeepsTheExistingFileMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		existing  os.FileMode // 0 means the file does not exist yet
		requested os.FileMode
		want      os.FileMode
	}{
		{name: "private file is not widened", existing: 0o600, requested: 0o644, want: 0o600},
		{name: "unset perm keeps the existing mode", existing: 0o640, requested: 0, want: 0o640},
		{name: "script gains requested exec bits where readable", existing: 0o644, requested: 0o755, want: 0o755},
		{name: "private script gains only the owner exec bit", existing: 0o600, requested: 0o755, want: 0o700},
		{name: "new file takes the requested mode", requested: 0o755, want: 0o755},
		{name: "new file defaults to 0644", requested: 0, want: 0o644},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "opencode.json")
			if tc.existing != 0 {
				require.NoError(t, os.WriteFile(path, []byte(`{"apiKey":"sk-x"}`), tc.existing))
				require.NoError(t, os.Chmod(path, tc.existing))
			}

			_, err := ApplyTransaction(root, "opencode", TransactionPlan{Writes: []TransactionWrite{
				{Path: "opencode.json", Content: []byte(`{"apiKey":"sk-x","plugin":[]}`), Perm: tc.requested},
			}})
			require.NoError(t, err)

			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.Mode().Perm())
		})
	}
}
