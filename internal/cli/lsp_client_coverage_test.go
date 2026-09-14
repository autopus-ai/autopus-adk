package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lspProjectWithServer creates a Go project whose detected server ("gopls")
// resolves to a stub executable, and makes it the working directory.
// When installServer is false PATH is emptied so server resolution must fail.
func lspProjectWithServer(t *testing.T, installServer bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub server script requires a POSIX shell")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module stub\n"), 0o644))

	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	if installServer {
		// Stub server only needs to stay alive while holding stdin open.
		script := "#!/bin/sh\ncat > /dev/null\n"
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "gopls"), []byte(script), 0o755))
	}

	t.Setenv("PATH", binDir)
	t.Chdir(dir)
}

// Guards the diagnostic when the project type itself cannot be classified.
func TestCreateLSPClient_UndetectableProjectReportsDetectionFailure(t *testing.T) {
	_, cleanup, err := createLSPClient(t.TempDir())
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "LSP 서버 감지 실패")
}

// Guards the diagnostic when the project type is known but its server is absent.
func TestCreateLSPClient_DetectedServerMissingReportsClientFailure(t *testing.T) {
	lspProjectWithServer(t, false)

	_, cleanup, err := createLSPClient(".")
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "LSP 클라이언트 생성 실패")
	assert.Contains(t, err.Error(), "gopls", "diagnostic must name the server it tried to start")
}

// A resolvable server must produce a usable adapter plus an idempotent cleanup.
func TestCreateLSPClient_StartsDetectedServer(t *testing.T) {
	lspProjectWithServer(t, true)

	client, cleanup, err := createLSPClient(".")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	adapter, ok := client.(*lspClientAdapter)
	require.True(t, ok, "createLSPClient must wrap the raw client in the Commander adapter")
	assert.NotNil(t, adapter.client)
}

// With a live server the command bodies must still surface the unimplemented
// adapter as a per-command diagnostic rather than reporting empty results.
func TestLSPCmds_LiveServerSurfaceQueryFailures(t *testing.T) {
	lspProjectWithServer(t, true)

	for _, tc := range []struct {
		name  string
		cmd   *cobra.Command
		args  []string
		frag  string
		inner string
	}{
		{name: "diagnostics", cmd: newLSPDiagnosticsCmd(), args: []string{"main.go"}, frag: "진단 조회 실패", inner: "main.go"},
		{name: "refs", cmd: newLSPRefsCmd(), args: []string{"Foo"}, frag: "참조 조회 실패", inner: "Foo"},
		{name: "rename", cmd: newLSPRenameCmd(), args: []string{"Old", "New"}, frag: "이름 변경 실패", inner: "Old -> New"},
		{name: "symbols", cmd: newLSPSymbolsCmd(), args: []string{"main.go"}, frag: "심볼 조회 실패", inner: "main.go"},
		{name: "definition", cmd: newLSPDefinitionCmd(), args: []string{"Foo"}, frag: "정의 조회 실패", inner: "Foo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.RunE(tc.cmd, tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.frag)
			assert.Contains(t, err.Error(), tc.inner, "wrapped adapter detail must be preserved")
		})
	}
}

// The diagnostics --format flag must reject nothing silently: its default is text
// and an explicit json value must be accepted by the flag set.
func TestNewLSPDiagnosticsCmd_FormatFlagDefault(t *testing.T) {
	t.Parallel()

	cmd := newLSPDiagnosticsCmd()
	flag := cmd.Flags().Lookup("format")
	require.NotNil(t, flag)
	assert.Equal(t, "text", flag.DefValue)
	require.NoError(t, cmd.Flags().Set("format", "json"))
}

// The lsp parent must expose exactly the five intelligence subcommands.
func TestNewLSPCmd_RegistersSubcommands(t *testing.T) {
	t.Parallel()

	names := map[string]bool{}
	for _, sub := range newLSPCmd().Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"diagnostics", "refs", "rename", "symbols", "definition"} {
		assert.True(t, names[want], "missing lsp subcommand %q", want)
	}
	assert.Len(t, names, 5)
}
