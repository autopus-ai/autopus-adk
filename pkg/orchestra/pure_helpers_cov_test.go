package orchestra

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExtractListItem_Formats covers the backtick, bold, plain-dash, and
// non-list branches of extractListItem.
func TestExtractListItem_Formats(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"- `cacheManager`", "cacheManager"},
		{"- **AuthService**", "AuthService"},
		{"- plain text only", ""},
		{"not a list line", ""},
		{"- `unterminated", ""},
		{"- **unterminated", ""},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, extractListItem(tc.in), "input=%q", tc.in)
	}
}

// TestLastPromptArgIndex finds the last matching prompt arg and returns -1 when absent.
func TestLastPromptArgIndex(t *testing.T) {
	t.Parallel()
	args := []string{"--flag", "P", "--other", "P"}
	assert.Equal(t, 3, lastPromptArgIndex(args, "P"), "should return last matching index")
	assert.Equal(t, -1, lastPromptArgIndex(args, "missing"))
	assert.Equal(t, -1, lastPromptArgIndex(nil, "x"))
}

// TestInsertArgs inserts values at an index and clamps out-of-range indices.
func TestInsertArgs(t *testing.T) {
	t.Parallel()
	base := []string{"a", "b", "c"}
	assert.Equal(t, []string{"a", "X", "Y", "b", "c"}, insertArgs(base, 1, "X", "Y"))
	// Negative index clamps to append at end.
	assert.Equal(t, []string{"a", "b", "c", "Z"}, insertArgs(base, -5, "Z"))
	// Index beyond length clamps to append at end.
	assert.Equal(t, []string{"a", "b", "c", "W"}, insertArgs(base, 99, "W"))
}

// TestAppendSubprocessDiagnostic covers empty-existing, empty-diagnostic, and
// join branches.
func TestAppendSubprocessDiagnostic(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "first", appendSubprocessDiagnostic("", "  first  "))
	assert.Equal(t, "kept", appendSubprocessDiagnostic("kept", "   "))
	assert.Equal(t, "a\nb", appendSubprocessDiagnostic("a", "b"))
	assert.Equal(t, "  ", appendSubprocessDiagnostic("  ", ""))
}
