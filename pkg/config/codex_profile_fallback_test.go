package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveCodexProfile_AstraFallbackChain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		catalog string
		want    CodexProfile
	}{
		{
			name:    "Sol",
			catalog: `{"models":[{"slug":"gpt-6-sol","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`,
			want:    CodexProfile{Model: CodexSolModel, Effort: CodexEffortMax},
		},
		{
			name:    "previous Sol",
			catalog: `{"models":[{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`,
			want:    CodexProfile{Model: CodexPreviousSolModel, Effort: CodexEffortMax},
		},
		{
			name:    "Terra",
			catalog: `{"models":[{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`,
			want:    CodexProfile{Model: CodexTerraModel, Effort: CodexEffortMax},
		},
		{
			name:    "legacy",
			catalog: `{"models":[{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`,
			want:    CodexProfile{Model: CodexLegacyModel, Effort: CodexEffortXHigh},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveCodexProfile(
				CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax},
				[]byte(tt.catalog),
			)
			assert.Equal(t, CodexResolutionModelUnavailable, got.Reason)
			assert.True(t, got.Fallback)
			assert.Equal(t, tt.want, got.Effective)
		})
	}
}

func TestResolveCodexProfile_SolMissingPrefersTerra(t *testing.T) {
	t.Parallel()
	catalog := []byte(`{"models":[
		{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"xhigh"}]},
		{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
	]}`)

	got := ResolveCodexProfile(CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}, catalog)

	assert.Equal(t, CodexResolutionModelUnavailable, got.Reason)
	assert.Equal(t, CodexProfile{Model: CodexTerraModel, Effort: CodexEffortXHigh}, got.Effective)
}

func TestResolveCodexProfile_TerraMissingSkipsLuna(t *testing.T) {
	t.Parallel()
	catalog := []byte(`{"models":[
		{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"max"}]},
		{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
	]}`)

	got := ResolveCodexProfile(CodexProfile{Model: CodexTerraModel, Effort: CodexEffortMax}, catalog)

	assert.Equal(t, CodexResolutionModelUnavailable, got.Reason)
	assert.Equal(t, CodexProfile{Model: CodexLegacyModel, Effort: CodexEffortXHigh}, got.Effective)
}

func TestResolveCodexProfile_NoKnownSubstituteDefersToRuntime(t *testing.T) {
	t.Parallel()
	catalog := []byte(`{"models":[{"slug":"other-model","supported_reasoning_levels":[{"effort":"medium"}]}]}`)

	got := ResolveCodexProfile(CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}, catalog)

	assert.Equal(t, CodexResolutionRuntimeDefault, got.Reason)
	assert.True(t, got.Fallback)
	assert.Empty(t, got.Effective.Model)
}

// A Codex catalog that predates GPT-6 Sol and Luna keeps each rung on its 5.6
// counterpart instead of dropping straight to Terra or legacy.
func TestResolveCodexProfile_PreGPT6CatalogKeepsRungOn56(t *testing.T) {
	t.Parallel()
	catalog := []byte(`{"models":[
		{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"xhigh"}]},
		{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"xhigh"}]},
		{"slug":"gpt-5.6-luna","supported_reasoning_levels":[{"effort":"max"}]},
		{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
	]}`)

	sol := ResolveCodexProfile(CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}, catalog)
	assert.Equal(t, CodexResolutionModelUnavailable, sol.Reason)
	assert.Equal(t, CodexProfile{Model: CodexPreviousSolModel, Effort: CodexEffortXHigh}, sol.Effective)

	luna := ResolveCodexProfile(CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMax}, catalog)
	assert.Equal(t, CodexResolutionModelUnavailable, luna.Reason)
	assert.Equal(t, CodexProfile{Model: CodexPreviousLunaModel, Effort: CodexEffortMax}, luna.Effective)
}
