package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/agentprobe"
	"github.com/stretchr/testify/require"
)

func TestDoctorAgentsTraceIsNotRuntimeProof(t *testing.T) {
	e := agentprobe.Evidence{Version: 1, Platform: "codex", RuntimeVersion: "test", RunID: "run", SupervisorID: "root", Challenge: "nonce", Events: []agentprobe.Event{}}
	data, err := json.Marshal(e)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "trace.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	cmd := newDoctorAgentsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--trace-json", path, "--format", "json"})
	require.NoError(t, cmd.Execute())
	var report doctorAgentsReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	require.Equal(t, "supplied_trace", report.Mode)
	require.Equal(t, "unknown", report.Observations[0].Lifecycle.Overall)
	require.False(t, report.Observations[0].RuntimeVerified)
	require.Nil(t, report.Observations[0].Installed, "imported trace does not establish local installation")
}

func TestDoctorAgentsRejectsContradictoryOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--live", "--trace-json", "unused"},
		{"--platform", "invented"},
		{"--live", "--platform", "all"},
		{"--live", "--platform", "opencode"},
		{"--format", "xml"},
		{"--timeout", "0s"},
	} {
		cmd := newDoctorAgentsCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		require.Error(t, cmd.Execute(), "%v", args)
	}
}

func TestDoctorAgentsTraceRejectsNonRegularInput(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{dir, filepath.Join(dir, "missing")} {
		cmd := newDoctorAgentsCmd()
		cmd.SetArgs([]string{"--trace-json", path})
		require.Error(t, cmd.Execute())
	}
}
