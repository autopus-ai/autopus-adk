package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultPatternSources_MatchesScannerPatterns pins the accessor to the
// patterns NewSecretScanner compiles, in Scan order, so pkg/secretscan's drift
// test compares its table against what the worker scanner really runs.
func TestDefaultPatternSources_MatchesScannerPatterns(t *testing.T) {
	t.Parallel()
	got := DefaultPatternSources()
	require.Len(t, got, 11)

	scanner := NewSecretScanner()
	require.Len(t, scanner.patterns, len(got))
	for i, re := range scanner.patterns {
		assert.Equal(t, got[i], re.String(), "pattern %d", i)
	}

	got[0] = "mutated"
	assert.NotEqual(t, "mutated", DefaultPatternSources()[0], "each call returns a fresh slice")
}
