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
	checked := 0
	for _, file := range files {
		body := string(file.Content)
		if file.TargetPath == "AGENTS.md" {
			require.Contains(t, body, custom)
			require.Contains(t, body, "OpenCode V2 native contract")
		}
		if file.TargetPath == filepath.Join(".agents", "skills", "auto-go", "SKILL.md") || file.TargetPath == filepath.Join(".agents", "skills", "agent-pipeline", "SKILL.md") {
			checked++
			require.NotContains(t, body, "task(")
			require.NotContains(t, body, "subagent_type")
			require.Contains(t, body, "subagent")
			require.Contains(t, body, "sessionID")
			require.Contains(t, body, "background")
		}
		require.Equal(t, len(body) > 0, true)
	}
	require.Equal(t, 2, checked)
}

func TestV1KeepsDelegationSyntax(t *testing.T) {
	a := NewWithRoot(t.TempDir(), WithCLIVersion("1.18.31"))
	files, err := a.prepareFiles(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)
	for _, file := range files {
		if file.TargetPath == filepath.Join(".agents", "skills", "auto-go", "SKILL.md") {
			require.Contains(t, string(file.Content), "task(")
			require.NotContains(t, string(file.Content), "OpenCode V2 native contract")
			return
		}
	}
	t.Fatal("missing auto-go skill")
}
