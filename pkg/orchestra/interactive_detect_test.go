package orchestra

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- R10: ANSI escape sequence stripping ---

// TestInteractive_StripANSI_BasicCodes verifies ANSI escape sequences are removed.
func TestInteractive_StripANSI_BasicCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "color codes stripped",
			input:    "\x1b[31mERROR\x1b[0m: something failed",
			expected: "ERROR: something failed",
		},
		{
			name:     "bold and underline stripped",
			input:    "\x1b[1m\x1b[4mTitle\x1b[0m",
			expected: "Title",
		},
		{
			name:     "cursor movement stripped",
			input:    "\x1b[2J\x1b[H$ prompt here",
			expected: "$ prompt here",
		},
		{
			name:     "no escape sequences unchanged",
			input:    "plain text output",
			expected: "plain text output",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "256 color codes stripped",
			input:    "\x1b[38;5;196mred text\x1b[0m",
			expected: "red text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stripANSI(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// --- R7: Completion detection ---

// TestInteractive_PromptPatternDetection_MatchesShellPrompt verifies prompt pattern matching.
func TestInteractive_PromptPatternDetection_MatchesShellPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		screen  string
		matched bool
	}{
		{
			name:    "dollar prompt detected",
			screen:  "output done\n$ ",
			matched: true,
		},
		{
			name:    "hash prompt detected",
			screen:  "# ",
			matched: true,
		},
		{
			name:    "mid-output no prompt",
			screen:  "still running...\nprocessing step 3",
			matched: false,
		},
		{
			name:    "empty screen no match",
			screen:  "",
			matched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPromptVisible(tt.screen, nil)
			assert.Equal(t, tt.matched, got)
		})
	}
}

// TestInteractive_IdleDetection_NoOutputTimeout verifies idle detection logic.
// R7 secondary: pipe-pane output file idle detection (N seconds no output, default 10s).
func TestInteractive_IdleDetection_NoOutputTimeout(t *testing.T) {
	t.Parallel()
	tmpFile := filepath.Join(t.TempDir(), "output.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("data"), 0o644))
	// Set modtime to 15 seconds ago
	past := time.Now().Add(-15 * time.Second)
	require.NoError(t, os.Chtimes(tmpFile, past, past))
	assert.True(t, isOutputIdle(tmpFile, 10*time.Second))
}

// TestInteractive_IdleDetection_ActiveOutput verifies active output is not flagged idle.
func TestInteractive_IdleDetection_ActiveOutput(t *testing.T) {
	t.Parallel()
	tmpFile := filepath.Join(t.TempDir(), "output.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("data"), 0o644))
	// File was just written, modtime is now
	assert.False(t, isOutputIdle(tmpFile, 10*time.Second))
}

// TestInteractive_CompletionDetection_TimeoutFallback verifies timeout is the final fallback.
func TestInteractive_CompletionDetection_TimeoutFallback(t *testing.T) {
	t.Parallel()
	// Timeout is handled by context cancellation in the orchestration layer.
	// Verify that isOutputIdle returns false when file doesn't exist.
	assert.False(t, isOutputIdle("/nonexistent/file.txt", 10*time.Second))
}

// TestIsPromptVisible_WithCustomPatterns verifies provider-specific patterns
// are checked before default fallback patterns.
func TestIsPromptVisible_WithCustomPatterns(t *testing.T) {
	t.Parallel()
	customPatterns := []CompletionPattern{
		{Provider: "custom", Pattern: regexp.MustCompile(`(?m)^READY>`)},
	}
	tests := []struct {
		name     string
		screen   string
		patterns []CompletionPattern
		expected bool
	}{
		{
			name:     "custom pattern matches",
			screen:   "output done\nREADY>",
			patterns: customPatterns,
			expected: true,
		},
		{
			name:     "custom pattern does not match falls through to default",
			screen:   "output done\n$ ",
			patterns: customPatterns,
			expected: true,
		},
		{
			name:     "neither custom nor default matches",
			screen:   "still processing...",
			patterns: customPatterns,
			expected: false,
		},
		{
			name:     "empty patterns nil falls through to default only",
			screen:   "$ ",
			patterns: nil,
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPromptVisible(tt.screen, tt.patterns)
			assert.Equal(t, tt.expected, got)
		})
	}
}
