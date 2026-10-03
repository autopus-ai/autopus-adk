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
			catalog: `{"models":[{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"max"}]}]}`,
			want:    CodexProfile{Model: CodexSolModel, Effort: CodexEffortMax},
		},
		{
			name:    "GPT-6 Sol before 6.1",
			catalog: `{"models":[{"slug":"gpt-6-sol","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"max"}]}]}`,
			want:    CodexProfile{Model: CodexGPT6SolModel, Effort: CodexEffortMax},
		},
		{
			name:    "previous Sol",
			catalog: `{"models":[{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`,
			want:    CodexProfile{Model: CodexPreviousSolModel, Effort: CodexEffortMax},
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

// Astra never steps down to Luna or to the retiring gpt-5.5: a catalog that
// offers only those leaves the choice to the Codex runtime.
func TestResolveCodexProfile_AstraSkipsLunaAndLegacy(t *testing.T) {
	t.Parallel()
	catalog := []byte(`{"models":[
		{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"max"}]},
		{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
	]}`)

	got := ResolveCodexProfile(CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}, catalog)

	assert.Equal(t, CodexResolutionRuntimeDefault, got.Reason)
	assert.Empty(t, got.Effective.Model)
}

// Luna with neither GPT-6 nor 5.6 Luna climbs to the fallback model rather
// than failing, since running on a stronger model is safe.
func TestResolveCodexProfile_LunaMissingUsesFallbackModel(t *testing.T) {
	t.Parallel()
	catalog := []byte(`{"models":[
		{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"max"}]}
	]}`)

	got := ResolveCodexProfile(CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMax}, catalog)

	assert.Equal(t, CodexResolutionModelUnavailable, got.Reason)
	assert.Equal(t, CodexProfile{Model: CodexFallbackModel, Effort: CodexEffortMax}, got.Effective)
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
// counterpart instead of collapsing onto a single fallback.
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

// A Codex catalog that does not list GPT-6.1 Sol yet keeps the Sol rung on
// GPT-6 Sol before dropping a generation.
func TestResolveCodexProfile_SolFallsBackToGPT6Sol(t *testing.T) {
	t.Parallel()
	catalog := `{"models":[{"slug":"gpt-6-sol","supported_reasoning_levels":[{"effort":"xhigh"}]},{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`
	got := ResolveCodexProfile(CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}, []byte(catalog))
	assert.Equal(t, CodexResolutionModelUnavailable, got.Reason)
	assert.Equal(t, CodexProfile{Model: CodexGPT6SolModel, Effort: CodexEffortXHigh}, got.Effective)
}
