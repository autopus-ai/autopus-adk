package orchestra

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderArtifactIdentity_CanonicalizesKnownAliasesOnly(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"claude":            "claude",
		"claude-code":       "claude",
		"antigravity":       "gemini",
		"antigravity-cli":   "gemini",
		"gemini-cli":        "gemini",
		"agy":               "gemini",
		"CustomProvider_01": "CustomProvider_01",
	}
	for provider, want := range tests {
		provider, want := provider, want
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, providerArtifactIdentity(provider))
		})
	}
}
