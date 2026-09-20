package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/insajin/autopus-adk/pkg/agentprobe"
	"github.com/stretchr/testify/require"
)

func TestDoctorAgentsLiveFailurePreservesSafeProviderReason(t *testing.T) {
	original := doctorOpenCodeLifecycle
	t.Cleanup(func() { doctorOpenCodeLifecycle = original })
	t.Setenv("AUTOPUS_TEST_PROBE_PASSWORD", "private-probe-credential")
	doctorOpenCodeLifecycle = func(_ context.Context, options agentprobe.OpenCodeOptions) (agentprobe.Evidence, agentprobe.OpenCodeTransport, error) {
		require.Equal(t, "private-probe-credential", options.Password)
		status := 403
		return agentprobe.Evidence{Version: 1, Platform: "opencode", RuntimeVersion: "1.18.7", RunID: "run", SupervisorID: "unobserved", Challenge: "nonce", Events: []agentprobe.Event{}},
			agentprobe.OpenCodeTransport{CleanupConfirmed: true, Messages: []agentprobe.OpenCodeMessageUsage{{ErrorStatusCode: &status}}}, errors.New("provider unavailable")
	}
	cmd := newDoctorAgentsCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--live", "--platform", "opencode", "--endpoint", "http://127.0.0.1:4321", "--password-env", "AUTOPUS_TEST_PROBE_PASSWORD", "--provider", "fixture", "--model", "fixture", "--format", "json"})
	require.Error(t, cmd.Execute())
	var report doctorAgentsReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	require.Equal(t, "live_endpoint_capture", report.Mode)
	require.Equal(t, "blocked", report.Observations[0].Status)
	require.Equal(t, "provider_http_403", report.Observations[0].FailureReason)
	require.False(t, report.Observations[0].RuntimeVerified)
	require.NotContains(t, output.String(), "private-probe-credential")
}

func TestDoctorAgentsUnsupportedLiveDoesNotClaimCapture(t *testing.T) {
	cmd := newDoctorAgentsCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--live", "--platform", "gemini-cli", "--format", "json"})
	require.Error(t, cmd.Execute())
	var report doctorAgentsReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	require.Equal(t, "live_requested", report.Mode)
	require.Equal(t, "probe_not_implemented", report.Observations[0].Status)
	require.Nil(t, report.Observations[0].Evidence)
}
