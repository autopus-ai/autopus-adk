package orchestra

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProviderConfigs_RejectsUnsafeAndDuplicateNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		providers []ProviderConfig
		want      string
	}{
		{name: "empty", providers: []ProviderConfig{{Name: ""}}, want: "empty"},
		{name: "unsafe", providers: []ProviderConfig{{Name: "../claude"}}, want: "unsafe"},
		{name: "raw duplicate", providers: []ProviderConfig{{Name: "claude"}, {Name: "claude"}}, want: "duplicate raw"},
		{name: "canonical duplicate", providers: []ProviderConfig{{Name: "claude"}, {Name: "Claude"}}, want: "duplicate canonical"},
		{name: "claude artifact alias", providers: []ProviderConfig{{Name: "claude"}, {Name: "claude-code"}}, want: "duplicate canonical"},
		{name: "gemini artifact alias", providers: []ProviderConfig{{Name: "gemini"}, {Name: "agy"}}, want: "duplicate canonical"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateProviderConfigs(tt.providers)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateProviderConfigs_SingleUppercaseCustomNameIsAllowed(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validateProviderConfigs([]ProviderConfig{{Name: "CustomAI"}}))
	assert.NoError(t, validateProviderConfigs([]ProviderConfig{{Name: "CustomAI"}, {Name: "OtherAI"}}))
}

func TestValidateHookSessionID_RequiresCanonicalLowercase(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validateHookSessionID("orch-abc_123"))
	assert.ErrorContains(t, validateHookSessionID("Orch-ABC_123"), "lowercase")
	assert.NoError(t, validateOrchestraProviderConfig(OrchestraConfig{
		Providers:     []ProviderConfig{{Name: "CustomAI"}},
		JudgeProvider: "JudgeAI",
		JudgeConfig:   &ProviderConfig{Name: "JudgeConfigAI"},
	}))
}

func TestOrchestraEntries_InvalidProviderFailsBeforePaneCreation(t *testing.T) {
	entries := []struct {
		name string
		run  func(context.Context, OrchestraConfig) error
	}{
		{name: "run", run: func(ctx context.Context, cfg OrchestraConfig) error {
			_, err := RunOrchestra(ctx, cfg)
			return err
		}},
		{name: "pane", run: func(ctx context.Context, cfg OrchestraConfig) error {
			_, err := RunPaneOrchestra(ctx, cfg)
			return err
		}},
		{name: "interactive", run: func(ctx context.Context, cfg OrchestraConfig) error {
			_, err := RunInteractivePaneOrchestra(ctx, cfg)
			return err
		}},
		{name: "detached", run: func(ctx context.Context, cfg OrchestraConfig) error {
			_, err := RunPaneOrchestraDetached(ctx, cfg)
			return err
		}},
	}
	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			term := newCmuxMock()
			cfg := OrchestraConfig{
				Providers: []ProviderConfig{{Name: "../claude", Binary: "echo"}},
				Strategy:  StrategyConsensus,
				Terminal:  term,
			}

			err := entry.run(context.Background(), cfg)

			require.ErrorContains(t, err, "unsafe")
			assert.Empty(t, term.splitPaneCalls)
		})
	}
}

func TestInteractivePaneBackend_InvalidProviderFailsBeforePaneCreation(t *testing.T) {
	t.Parallel()
	term := newCmuxMock()
	backend := NewInteractivePaneBackend(OrchestraConfig{Terminal: term})

	response, err := backend.Execute(context.Background(), ProviderRequest{
		Provider: "../claude",
		Config:   ProviderConfig{Name: "../claude", Binary: "claude"},
	})

	require.ErrorContains(t, err, "unsafe")
	assert.Nil(t, response)
	assert.Empty(t, term.splitPaneCalls)
}

func TestInteractivePaneBackend_InvalidBoundConfigFailsBeforePaneCreation(t *testing.T) {
	t.Parallel()
	term := newCmuxMock()
	backend := NewInteractivePaneBackend(OrchestraConfig{
		Terminal: term, Providers: []ProviderConfig{{Name: "unsafe/provider"}},
	})

	response, err := backend.Execute(context.Background(), ProviderRequest{
		Provider: "claude",
		Config:   ProviderConfig{Name: "claude", Binary: "claude"},
	})

	require.ErrorContains(t, err, "unsafe")
	assert.Nil(t, response)
	assert.Empty(t, term.splitPaneCalls)
}

func TestHookModeInvalidSessionIDFailsBeforePaneCreation(t *testing.T) {
	entries := []struct {
		name string
		run  func(context.Context, OrchestraConfig) error
	}{
		{name: "pane", run: func(ctx context.Context, cfg OrchestraConfig) error {
			_, err := RunPaneOrchestra(ctx, cfg)
			return err
		}},
		{name: "interactive", run: func(ctx context.Context, cfg OrchestraConfig) error {
			_, err := RunInteractivePaneOrchestra(ctx, cfg)
			return err
		}},
	}
	for _, sessionID := range []string{"", "../unsafe-session", "Orch-ABC"} {
		for _, entry := range entries {
			t.Run(entry.name+"/"+sessionID, func(t *testing.T) {
				term := newCmuxMock()
				err := entry.run(context.Background(), OrchestraConfig{
					Providers: []ProviderConfig{{Name: "claude", Binary: "claude"}},
					Strategy:  StrategyConsensus,
					Terminal:  term,
					HookMode:  true,
					SessionID: sessionID,
				})

				require.Error(t, err)
				assert.Empty(t, term.splitPaneCalls)
			})
		}
	}
}

func TestInteractivePaneBackend_InvalidHookSessionFailsBeforePaneCreation(t *testing.T) {
	t.Parallel()
	term := newCmuxMock()
	backend := NewInteractivePaneBackend(OrchestraConfig{
		Terminal: term, HookMode: true, SessionID: "../unsafe-session",
	})

	response, err := backend.Execute(context.Background(), ProviderRequest{
		Provider: "claude",
		Config:   ProviderConfig{Name: "claude", Binary: "claude"},
	})

	require.ErrorContains(t, err, "unsafe")
	assert.Nil(t, response)
	assert.Empty(t, term.splitPaneCalls)
}

func TestWaitAndCollectHookResults_InvalidIdentityFailsBeforeSessionCreation(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	tests := []struct {
		name      string
		providers []ProviderConfig
		sessionID string
		want      string
	}{
		{name: "unsafe provider", providers: []ProviderConfig{{Name: "../claude"}}, sessionID: "safe-session", want: "unsafe"},
		{name: "raw duplicate", providers: []ProviderConfig{{Name: "claude"}, {Name: "claude"}}, sessionID: "safe-session", want: "duplicate raw"},
		{name: "canonical duplicate", providers: []ProviderConfig{{Name: "claude"}, {Name: "Claude"}}, sessionID: "safe-session", want: "duplicate canonical"},
		{name: "unsafe session", providers: []ProviderConfig{{Name: "claude"}}, sessionID: "../session", want: "unsafe"},
		{name: "noncanonical session", providers: []ProviderConfig{{Name: "claude"}}, sessionID: "Session-ABC", want: "lowercase"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responses, err := WaitAndCollectHookResults(OrchestraConfig{
				Providers: tt.providers,
			}, tt.sessionID)

			require.ErrorContains(t, err, tt.want)
			assert.Nil(t, responses)
		})
	}
}

type providerValidationBackend struct {
	mu    sync.Mutex
	calls int
}

func (b *providerValidationBackend) Name() string { return "validation-test" }

func (b *providerValidationBackend) Execute(_ context.Context, req ProviderRequest) (*ProviderResponse, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	return &ProviderResponse{Provider: req.Provider, Output: `{}`}, nil
}

func TestRunSubprocessPipeline_InvalidProviderSetFailsBeforeDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		providers []ProviderConfig
		judge     ProviderConfig
		want      string
	}{
		{name: "unsafe participant", providers: []ProviderConfig{{Name: "../claude"}}, judge: ProviderConfig{Name: "judge"}, want: "unsafe"},
		{name: "canonical participant duplicate", providers: []ProviderConfig{{Name: "claude"}, {Name: "Claude"}}, judge: ProviderConfig{Name: "judge"}, want: "duplicate canonical"},
		{name: "unsafe judge", providers: []ProviderConfig{{Name: "claude"}}, judge: ProviderConfig{Name: "../judge"}, want: "unsafe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			backend := &providerValidationBackend{}
			_, err := RunSubprocessPipeline(context.Background(), SubprocessPipelineConfig{
				Backend: backend, Providers: tt.providers, Judge: tt.judge,
			})

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.want)
			assert.Zero(t, backend.calls)
		})
	}
}

// TestSanitizeProviderName verifies path traversal prevention.
func TestSanitizeProviderName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"clean name", "claude", "claude"},
		{"with hyphens", "my-provider", "my-provider"},
		{"path traversal", "../../../etc/passwd", "etcpasswd"},
		{"slashes", "foo/bar", "foobar"},
		{"empty after sanitize", "///", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, sanitizeProviderName(tt.input))
		})
	}
}
