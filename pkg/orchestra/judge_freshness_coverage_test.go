package orchestra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Judge freshness is fail-closed: evidence may only be marked verified when a
// backend actually proves per-request isolation.

type staleJudgeBackend struct{ name string }

func (b *staleJudgeBackend) Execute(context.Context, ProviderRequest) (*ProviderResponse, error) {
	return nil, nil
}
func (b *staleJudgeBackend) Name() string { return b.name }

func TestVerifyFreshPipelineJudgeSession_RequiresObservedExecution(t *testing.T) {
	t.Parallel()

	declaredFresh := &pipelineJudgeOutcomeBackend{}
	evidence := newFreshSubprocessJudgeSessionEvidence()
	verifyFreshPipelineJudgeSession(evidence, nil, declaredFresh)
	assert.False(t, evidence.Verified, "a declared-fresh backend that returned nothing proves nothing")
	assert.False(t, evidence.Isolated)
	assert.Contains(t, evidence.Reason, "no response")

	evidence = newFreshSubprocessJudgeSessionEvidence()
	verifyFreshPipelineJudgeSession(evidence, &ProviderResponse{Provider: "judge"}, declaredFresh)
	assert.True(t, evidence.Verified)
	assert.True(t, evidence.Isolated)

	evidence = newFreshSubprocessJudgeSessionEvidence()
	verifyFreshPipelineJudgeSession(
		evidence, &ProviderResponse{Provider: "judge", ExecutedBackend: paneBackendName}, &staleJudgeBackend{name: "custom"})
	assert.True(t, evidence.Verified)
	assert.Contains(t, evidence.Reason, "pane backend")

	evidence = newFreshSubprocessJudgeSessionEvidence()
	verifyFreshPipelineJudgeSession(
		evidence, &ProviderResponse{Provider: "judge", ExecutedBackend: "custom"}, &staleJudgeBackend{name: "custom"})
	assert.False(t, evidence.Verified, "an unproven backend must not be treated as fresh")
	assert.Contains(t, evidence.Reason, "not observed")
}

func TestVerifyFreshSubprocessJudgeSession_OnlyTrustsSubprocessExecution(t *testing.T) {
	t.Parallel()

	verifyFreshSubprocessJudgeSession(nil, &ProviderResponse{}) // must not panic

	evidence := newFreshSubprocessJudgeSessionEvidence()
	verifyFreshSubprocessJudgeSession(evidence, nil)
	assert.False(t, evidence.Verified)
	assert.Contains(t, evidence.Reason, "no response")

	evidence = newFreshSubprocessJudgeSessionEvidence()
	verifyFreshSubprocessJudgeSession(evidence, &ProviderResponse{ExecutedBackend: ""})
	assert.False(t, evidence.Verified)
	assert.Contains(t, evidence.Reason, "not observed")

	evidence = newFreshSubprocessJudgeSessionEvidence()
	verifyFreshSubprocessJudgeSession(evidence, &ProviderResponse{ExecutedBackend: "subprocess"})
	assert.True(t, evidence.Verified)
	assert.True(t, evidence.Isolated)
}

func TestFreshJudgeSessionError_ReportsFirstMissingGuarantee(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, freshJudgeSessionError(nil), "missing")
	require.ErrorContains(t, freshJudgeSessionError(&FreshJudgeSessionEvidence{}), "not marked required")
	require.ErrorContains(t, freshJudgeSessionError(&FreshJudgeSessionEvidence{Required: true}), "participant termination")
	require.ErrorContains(t, freshJudgeSessionError(&FreshJudgeSessionEvidence{
		Required: true, ParticipantsTerminated: true}), "isolation")
	require.ErrorContains(t, freshJudgeSessionError(&FreshJudgeSessionEvidence{
		Required: true, ParticipantsTerminated: true, Isolated: true}), "not verified")
	require.NoError(t, freshJudgeSessionError(&FreshJudgeSessionEvidence{
		Required: true, ParticipantsTerminated: true, Isolated: true, Verified: true}))
}

func TestFreshJudgeConfigError_BlocksProviderSpecificResumeTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider ProviderConfig
		blocked  string
	}{
		{name: "claude short continue", provider: ProviderConfig{Name: "claude", Args: []string{"-c"}}, blocked: "-c"},
		{name: "gemini short session", provider: ProviderConfig{Name: "gemini", Args: []string{"-s"}}, blocked: "-s"},
		{name: "long resume flag", provider: ProviderConfig{Name: "codex", PaneArgs: []string{"--resume=abc"}}, blocked: "--resume"},
		{name: "identity from binary", provider: ProviderConfig{Name: "primary", Binary: "/usr/bin/claude", Args: []string{"-r"}}, blocked: "-r"},
		{name: "fork session", provider: ProviderConfig{Name: "codex", Args: []string{"--fork-session"}}, blocked: "--fork-session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := freshJudgeConfigError(tt.provider)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.blocked)
		})
	}

	// -s is a resume token for gemini only; codex must not be blocked by it.
	require.NoError(t, freshJudgeConfigError(ProviderConfig{Name: "codex", Args: []string{"-s"}}))
	require.NoError(t, freshJudgeConfigError(ProviderConfig{Name: "claude", Args: []string{"", "--model", "opus"}}))
}
