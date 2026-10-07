package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func sampleResolvedTimeout() ResolvedOrchestraTimeout {
	return ResolvedOrchestraTimeout{
		Seconds: 540,
		Source:  "config",
		Providers: []ResolvedProviderTimeout{
			{Provider: "claude", Duration: 9 * time.Minute, Source: "default"},
		},
	}
}

func sampleFailedResult() *orchestra.OrchestraResult {
	return &orchestra.OrchestraResult{
		RunID:    "run-123",
		Duration: 1500 * time.Millisecond,
		Summary:  "all failed",
		FailedProviders: []orchestra.FailedProvider{
			{
				Name:            "claude",
				FailureClass:    "timeout",
				Error:           "deadline exceeded",
				NextRemediation: "increase timeout",
				StderrPreview:   "panic: boom",
			},
			{
				Name:            "codex",
				FailureClass:    "binary_or_transport",
				Error:           "not found",
				NextRemediation: "increase timeout", // duplicate hint, must dedup
			},
		},
	}
}

// TestRenderOrchestraFailureSummary_FullDetail asserts each rendered line.
func TestRenderOrchestraFailureSummary_FullDetail(t *testing.T) {
	t.Parallel()

	out := renderOrchestraFailureSummary(sampleResolvedTimeout(), sampleFailedResult(), "/tmp/report.json")
	assert.Contains(t, out, "effective timeout: 540s (config)")
	assert.Contains(t, out, "provider timeout claude: 9m0s (default)")
	assert.Contains(t, out, "failure claude [timeout]: deadline exceeded")
	assert.Contains(t, out, "next: increase timeout")
	assert.Contains(t, out, "stderr: panic: boom")
	assert.Contains(t, out, "failure codex [binary_or_transport]: not found")
	// The duplicate remediation appears once as a hint.
	assert.Equal(t, 1, countSubstr(out, "- hint: increase timeout"))
	assert.Contains(t, out, "diagnostics report: /tmp/report.json")
}

// TestRenderOrchestraFailureSummary_NilResultNoPath omits failure/hint/report lines.
func TestRenderOrchestraFailureSummary_NilResultNoPath(t *testing.T) {
	t.Parallel()

	out := renderOrchestraFailureSummary(sampleResolvedTimeout(), nil, "")
	assert.Contains(t, out, "effective timeout: 540s")
	assert.NotContains(t, out, "failure")
	assert.NotContains(t, out, "diagnostics report")
}

func TestRenderOrchestraFailureSummary_BlockedYield_ExposesCleanupHandle(t *testing.T) {
	t.Parallel()
	result := sampleFailedResult()
	result.TerminalState = orchestra.TerminalBlocked
	result.Yield = &orchestra.YieldOutput{SessionID: "orch-recover-123"}

	out := renderOrchestraFailureSummary(sampleResolvedTimeout(), result, "/tmp/report.json")

	assert.Contains(t, out, "session: orch-recover-123")
	assert.Contains(t, out, "cleanup: auto orchestra cleanup --session-id orch-recover-123")
}

// TestSynthesizeOrchestraFailureError_NilAndPopulated covers both branches.
func TestSynthesizeOrchestraFailureError_NilAndPopulated(t *testing.T) {
	t.Parallel()

	err := synthesizeOrchestraFailureError(nil)
	require.Error(t, err)
	assert.Equal(t, "모든 프로바이더가 실패했습니다", err.Error())

	err = synthesizeOrchestraFailureError(sampleFailedResult())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude(timeout)")
	assert.Contains(t, err.Error(), "codex(binary_or_transport)")
}

// TestSummarizeFailedProviders_ClassFallsBackToError uses Error when class is empty.
func TestSummarizeFailedProviders_ClassFallsBackToError(t *testing.T) {
	t.Parallel()

	got := summarizeFailedProviders([]orchestra.FailedProvider{
		{Name: "a", FailureClass: "timeout"},
		{Name: "b", Error: "raw error"},
	})
	assert.Equal(t, "a(timeout), b(raw error)", got)
}

// TestCollectRetryHints_DedupAndSkipEmpty filters blanks and duplicates.
func TestCollectRetryHints_DedupAndSkipEmpty(t *testing.T) {
	t.Parallel()

	hints := collectRetryHints([]orchestra.FailedProvider{
		{NextRemediation: "  retry  "},
		{NextRemediation: ""},
		{NextRemediation: "retry"},
		{NextRemediation: "rebuild"},
	})
	assert.Equal(t, []string{"retry", "rebuild"}, hints)
}

// TestShouldTreatOrchestraResultAsFailure_Cases covers the decision matrix.
func TestShouldTreatOrchestraResultAsFailure_Cases(t *testing.T) {
	t.Parallel()

	assert.False(t, shouldTreatOrchestraResultAsFailure(nil))
	assert.False(t, shouldTreatOrchestraResultAsFailure(&orchestra.OrchestraResult{}))
	assert.True(t, shouldTreatOrchestraResultAsFailure(&orchestra.OrchestraResult{
		GateStatus: "blocked", Responses: []orchestra.ProviderResponse{{Provider: "healthy", Output: "ok"}},
	}))
	assert.Contains(t, synthesizeOrchestraFailureError(&orchestra.OrchestraResult{
		GateStatus: "blocked", ConfiguredProviders: []string{"a", "b", "c"},
		UsableProviders: []string{"a"}, QuorumRequired: 2, DegradedReasons: []string{"provider_quorum"},
	}).Error(), "quorum 미충족")

	// Failed providers but no responses → failure.
	assert.True(t, shouldTreatOrchestraResultAsFailure(&orchestra.OrchestraResult{
		FailedProviders: []orchestra.FailedProvider{{Name: "a"}},
	}))

	// A healthy non-failed response means not a total failure.
	assert.False(t, shouldTreatOrchestraResultAsFailure(&orchestra.OrchestraResult{
		FailedProviders: []orchestra.FailedProvider{{Name: "a"}},
		Responses: []orchestra.ProviderResponse{
			{Provider: "b", ExitCode: 0},
		},
	}))

	// All responses are either failed or unhealthy → failure.
	assert.True(t, shouldTreatOrchestraResultAsFailure(&orchestra.OrchestraResult{
		FailedProviders: []orchestra.FailedProvider{{Name: "a"}},
		Responses: []orchestra.ProviderResponse{
			{Provider: "a", ExitCode: 0},
			{Provider: "c", ExitCode: 1},
		},
	}))
}

// TestSaveOrchestraDiagnosticsReport_WritesJSON persists a parseable report.
func TestSaveOrchestraDiagnosticsReport_WritesJSON(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	path, err := saveOrchestraFailureReport(
		"go", "consensus", []string{"claude", "codex"},
		sampleResolvedTimeout(), sampleFailedResult(), assertErr("boom"),
	)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var report orchestraFailureReport
	require.NoError(t, json.Unmarshal(raw, &report))
	assert.Equal(t, "go", report.Command)
	assert.Equal(t, "consensus", report.Strategy)
	assert.Equal(t, "run-123", report.RunID)
	assert.Equal(t, "boom", report.Error)
	require.Len(t, report.FailedProviders, 2)
	assert.Equal(t, "claude", report.FailedProviders[0].Name)
	assert.Equal(t, []string{"increase timeout"}, report.RetryHints)
}

func TestSaveOrchestraFailureReport_BlockedYield_PersistsCleanupHandle(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	result := sampleFailedResult()
	result.TerminalState = orchestra.TerminalBlocked
	result.Yield = &orchestra.YieldOutput{SessionID: "orch-report-recover"}

	path, err := saveOrchestraFailureReport(
		"brainstorm", "debate", []string{"claude", "codex"},
		sampleResolvedTimeout(), result, assertErr("quorum blocked"),
	)
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var report orchestraFailureReport
	require.NoError(t, json.Unmarshal(raw, &report))
	assert.Equal(t, "orch-report-recover", report.SessionID)
	assert.Equal(t,
		"auto orchestra cleanup --session-id orch-report-recover",
		report.CleanupCommand,
	)
}

func TestRunOrchestraCommand_BlockedYield_WritesRecoveryHandleToStderr(t *testing.T) {
	t.Chdir(t.TempDir())
	originalRun := runOrchestraExecute
	t.Cleanup(func() { runOrchestraExecute = originalRun })
	runOrchestraExecute = func(context.Context, orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{
			TerminalState:       orchestra.TerminalBlocked,
			GateStatus:          "blocked",
			DegradedReasons:     []string{"provider_quorum"},
			ConfiguredProviders: []string{"claude", "gemini"},
			QuorumRequired:      2,
			Yield:               &orchestra.YieldOutput{SessionID: "orch-command-recover"},
			FailedProviders: []orchestra.FailedProvider{
				{Name: "claude", FailureClass: "timeout", Error: "deadline exceeded"},
				{Name: "gemini", FailureClass: "timeout", Error: "deadline exceeded"},
			},
		}, nil
	}
	var runErr error
	stderr := captureSpecReviewStderr(t, func() {
		runErr = runOrchestraCommand(
			context.Background(), "brainstorm", "consensus", []string{"claude", "gemini"},
			30, "", "topic", 0, 0, OrchestraFlags{},
		)
	})

	require.Error(t, runErr)
	assert.Contains(t, stderr, "session: orch-command-recover")
	assert.Contains(t, stderr, "cleanup: auto orchestra cleanup --session-id orch-command-recover")
}

func countSubstr(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}

type stubErr string

func (e stubErr) Error() string { return string(e) }

func assertErr(msg string) error { return stubErr(msg) }
