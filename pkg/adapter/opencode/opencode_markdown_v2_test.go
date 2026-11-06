package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestV2GeneratedNativeDelegation(t *testing.T) {
	dir := t.TempDir()
	custom := "User notes: task(subagent_type = custom) and --variant remain literal. TeamCreate and TeamDelete are user notes.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(custom), 0600))
	a := NewWithRoot(dir, WithCLIVersion("opencode v2.0.10"))
	files, err := a.prepareFiles(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)
	for _, file := range files {
		body := string(file.Content)
		if file.TargetPath == "AGENTS.md" {
			require.Contains(t, body, custom)
			require.Contains(t, body, "OpenCode V2 native contract")
		}
		// No generated surface keeps the V1 delegation spelling under V2; the
		// user's own AGENTS.md notes above the marker stay literal.
		if file.TargetPath != "AGENTS.md" {
			require.NotContains(t, body, "subagent_type = ", file.TargetPath)
		}
		require.Equal(t, len(body) > 0, true)
	}
}
