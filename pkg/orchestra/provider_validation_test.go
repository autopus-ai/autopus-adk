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

func TestValidateOrchestraProviderConfig_AllowsMixedCaseCustomNames(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validateOrchestraProviderConfig(OrchestraConfig{
		Providers:     []ProviderConfig{{Name: "CustomAI"}},
		JudgeProvider: "JudgeAI",
		JudgeConfig:   &ProviderConfig{Name: "JudgeConfigAI"},
	}))
}

func TestRunOrchestra_InvalidProviderFailsBeforeDispatch(t *testing.T) {
	t.Parallel()
	backend := &providerValidationBackend{}

	_, err := RunOrchestra(context.Background(), OrchestraConfig{
		Providers:        []ProviderConfig{{Name: "../claude", Backend: "validation-test"}},
		ProviderBackends: map[string]ExecutionBackend{"validation-test": backend},
		Strategy:         StrategyConsensus,
	})

	require.ErrorContains(t, err, "unsafe")
	assert.Zero(t, backend.calls)
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
