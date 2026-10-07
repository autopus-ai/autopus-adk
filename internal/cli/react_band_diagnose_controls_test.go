package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Provider Read-Only Contract item 3, fail-closed on the projected config:
// only the exact control forms pass.
func TestReactBandDiagnose_ReadOnlyControlsAreExact(t *testing.T) {
	t.Parallel()
	omp := func(mode string, tools ...string) orchestra.ProviderConfig {
		return orchestra.ProviderConfig{Name: "claude", Backend: config.ProviderBackendOMP, SandboxMode: mode, Tools: tools}
	}
	native := func(name string, args ...string) orchestra.ProviderConfig {
		return orchestra.ProviderConfig{Name: name, Args: args}
	}
	cases := []struct {
		name     string
		provider orchestra.ProviderConfig
		want     bool
	}{
		{"claude inline tools last", native("claude", "-p", "--permission-mode", "plan", "--tools=Read,Grep,Glob"), true},
		{"claude inline permission mode", native("claude", "--permission-mode=plan", "--tools=Read,Grep,Glob"), true},
		{"claude separated tools", native("claude", "--permission-mode", "plan", "--tools", "Read,Grep,Glob"), false},
		{"claude second tools item", native("claude", "--permission-mode", "plan", "--tools=Read,Grep,Glob", "--tools=Bash"), false},
		{"claude wider tools", native("claude", "--permission-mode", "plan", "--tools=Read,Grep,Glob,Bash"), false},
		{"claude without plan", native("claude", "--permission-mode", "default", "--tools=Read,Grep,Glob"), false},
		{"claude controls after separator", native("claude", "--", "--permission-mode", "plan", "--tools=Read,Grep,Glob"), false},
		{"codex read-only", native("codex", "exec", "--sandbox", "read-only", "--ephemeral"), true},
		{"codex short alias too", native("codex", "exec", "--sandbox", "read-only", "-s", "workspace-write"), false},
		{"codex two sandboxes", native("codex", "exec", "--sandbox", "read-only", "--sandbox=workspace-write"), false},
		{"codex without sandbox", native("codex", "exec"), false},
		{"gemini plan sandbox", native("gemini", "--print", "", "--mode", "plan", "--sandbox"), true},
		{"gemini without sandbox", native("gemini", "--mode", "plan"), false},
		{"unknown provider", native("opencode", "run"), false},
		{"omp read-only allowlist", omp(orchestra.SandboxModeReadOnly, "read", "glob", "grep", "grep"), true},
		{"omp subset", omp(orchestra.SandboxModeReadOnly, "read", "grep"), false},
		{"omp wider", omp(orchestra.SandboxModeReadOnly, "bash", "glob", "grep", "read"), false},
		{"omp not stamped read-only", omp(orchestra.SandboxModeUnverified, "glob", "grep", "read"), false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, bandReadOnlyControls(tc.provider), tc.name)
	}
}

// REQ-12: every way a run can fail maps to exactly one reason.
func TestReactBandDiagnose_RunFailureReasons(t *testing.T) {
	t.Parallel()
	ok := &orchestra.ProviderResponse{Output: "x"}
	cases := []struct {
		name     string
		response *orchestra.ProviderResponse
		err      error
		want     string
	}{
		{"output", ok, nil, ""},
		{"no route", nil, orchestra.ErrBackendUnavailable, "provider_backend_unavailable"},
		{"not on PATH", nil, fmt.Errorf("claude start: %w", &exec.Error{Name: "claude", Err: exec.ErrNotFound}), "provider_missing"},
		{"other start failure", nil, &os.PathError{Op: "fork/exec", Path: "claude", Err: os.ErrPermission}, "provider_exit_nonzero"},
		{"deadline without response", nil, context.DeadlineExceeded, "provider_timeout"},
		{"timed out response", &orchestra.ProviderResponse{TimedOut: true}, nil, "provider_timeout"},
		{"exit code", &orchestra.ProviderResponse{ExitCode: 3}, nil, "provider_exit_nonzero"},
		{"error with response", ok, errors.New("fast-fail"), "provider_exit_nonzero"},
		{"no response", nil, nil, "provider_exit_nonzero"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, bandRunFailure(tc.response, tc.err), tc.name)
	}
}

// Untrusted Input Contract items 1, 3, 4, 5, and 8 on provider output: only
// the first 1 MiB is kept, redacted whole before the 32 KiB cut, and the BS
// body stays within 32 KiB.
func TestReactBandDiagnose_ProviderOutputIsBoundedAndRedacted(t *testing.T) {
	t.Parallel()
	fixture := newBandDiagnoseFixture(t)
	d := fixture.diagnoser(bandHarness("claude", "", map[string]config.ProviderEntry{"claude": config.DefaultClaudeProviderEntry()}), nil)
	huge := "### Summary\nread " + fixture.projectDir + "/.env with ghp_" + strings.Repeat("D", 36) + "\n" +
		strings.Repeat("filler line of the diagnosis\n", (2<<20)/29)
	d.run = func(context.Context, orchestra.OrchestraConfig, orchestra.ProviderConfig, string) (*orchestra.ProviderResponse, error) {
		return &orchestra.ProviderResponse{Output: huge}, nil
	}

	outcome := d.Run(context.Background(), fixture.claim)

	require.Equal(t, "ok", outcome.DiagnosisStatus)
	bs := fixture.bs(t, outcome.BSID)
	assert.LessOrEqual(t, len(bs), healthband.MaxBSBodyBytes)
	assert.Contains(t, bs, "### Summary\nread <project>/.env with [REDACTED_SECRET]\n")
	assert.NotContains(t, bs, "ghp_")
	assert.NotContains(t, bs, fixture.projectDir)
}

// Phase B and C with the real store: the diagnoser is the claim runner, its
// outcome passes the result validator, and the action_result event records
// codes and the manifest, never log or provider text (REQ-22, S19).
func TestReactBandDiagnose_ClaimRunnerRecordsTheActionResult(t *testing.T) {
	t.Parallel()
	fixture := newBandDiagnoseFixture(t)
	d := fixture.diagnoser(bandHarness("claude", "", map[string]config.ProviderEntry{"claude": config.DefaultClaudeProviderEntry()}),
		bandS19Evidence(fixture.projectDir))
	d.run = func(context.Context, orchestra.OrchestraConfig, orchestra.ProviderConfig, string) (*orchestra.ProviderResponse, error) {
		return &orchestra.ProviderResponse{Output: "### Summary\nThe flaky step failed.\n"}, nil
	}
	var runner healthband.ClaimRunner = d.Run

	store := healthband.NewStore(fixture.projectDir)
	recorded, err := store.ExecuteClaims(context.Background(), []healthband.DueClaim{fixture.claim},
		runner, healthband.ExecuteOptions{Clock: func() time.Time { return bandDiagnoseT0.Add(time.Minute) }})

	require.NoError(t, err)
	require.Len(t, recorded, 1)
	require.NotZero(t, recorded[0].Seq)
	data, err := os.ReadFile(store.Path(healthband.EventsFile))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	last := lines[len(lines)-1]
	for _, forbidden := range []string{"ghp_", "step 3 failed", "step 9 failed", "flaky step", "Untrusted evidence"} {
		assert.NotContains(t, last, forbidden)
	}
	var event healthband.Event
	require.NoError(t, json.Unmarshal([]byte(last), &event))
	assert.Equal(t, healthband.EventKindActionResult, event.Kind)
	assert.Equal(t, "ok", event.DiagnosisStatus)
	assert.Equal(t, "BS-BAND-001", event.BSID)
	assert.Equal(t, "written", event.BSStatus)
	assert.Len(t, event.PromptManifest, 4)
}
