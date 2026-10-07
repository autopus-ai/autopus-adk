package orchestra

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSanitizeScreenOutput verifies screen output sanitization for ANSI codes,
// OSC sequences, status bars, markdown preservation, and blank line collapsing.
func TestSanitizeScreenOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// S4: ANSI color codes
		{"strips ANSI color codes", "\x1b[31mError\x1b[0m", "Error"},
		// S5: OSC sequences
		{"strips OSC window title", "\x1b]0;window title\x07real content", "real content"},
		// S6: status bar lines
		{"strips tmux status bar", "content\n[0] 0:bash* 1:vim\nmore content", "content\nmore content"},
		// S7: preserves markdown quotes
		{"preserves markdown quotes", "> This is a quote\n> Another line", "> This is a quote\n> Another line"},
		// S8: collapses blank lines
		{"collapses consecutive blank lines", "line1\n\n\n\nline2", "line1\n\nline2"},
		// Edge cases
		{"empty input returns empty", "", ""},
		{"plain text unchanged", "hello world", "hello world"},
		// Extended ANSI: CSI cursor movement
		{"strips CSI cursor movement", "\x1b[2J\x1b[H content here", " content here"},
		// DCS sequences
		{"strips DCS sequences", "\x1bPtest\x1b\\content", "content"},
		// Composite: ANSI + OSC + status bar in single input
		{"composite ANSI OSC and status bar", "\x1b[31mheader\x1b[0m\n\x1b]0;title\x07body\n[0] 0:bash* 1:vim\nfooter", "header\nbody\nfooter"},
		// Multi-line status bars
		{"multiple status bar lines", "top\n[0] 0:bash*\nmiddle\n[1] 1:vim*\nbottom", "top\nmiddle\nbottom"},
		// Long input with mixed escapes
		{"long input with trailing whitespace", "line1   \n\x1b[32mline2\x1b[0m  \n\n\n\nline3", "line1\nline2\n\nline3"},
		// Cursor save/restore sequences
		{"strips cursor save restore", "\x1b7saved\x1b8restored", "savedrestored"},
		// OSC terminated by ST
		{"strips OSC with ST terminator", "\x1b]52;c;data\x1b\\visible", "visible"},
		// CLI banner stripping
		{"strips Claude banner", "▐▛███▜▌   Claude Code v2.1.87\ncontent here", "content here"},
		{"strips Gemini banner", "▝▜▄     Antigravity CLI v0.35.3     ▝▜▄\ncontent here", "content here"},
		{"strips multi-line banners", "▐▛███▜▌   Claude Code\n▝▜█████▛▘  Opus 4.6\ncontent", "content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SanitizeScreenOutput(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeScreenOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestInteractive_FilterPromptLines_RemovesProviderPrompts verifies prompt lines are stripped.
func TestInteractive_FilterPromptLines_RemovesProviderPrompts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "claude prompt filtered",
			input:    "some output\n❯\nactual content",
			expected: "some output\nactual content",
		},
		{
			name:     "codex prompt filtered",
			input:    "codex> \nreal output here",
			expected: "real output here",
		},
		{
			name:     "codex v0.135 prompt filtered",
			input:    "result\n› Summarize recent commits\nmore output",
			expected: "result\nmore output",
		},
		{
			name:     "no prompt lines unchanged",
			input:    "just normal output\nsecond line",
			expected: "just normal output\nsecond line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterPromptLines(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestCleanScreenOutput verifies the cleanScreenOutput pipeline that combines
// SanitizeScreenOutput and filterPromptLines.
func TestCleanScreenOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips ANSI and prompts together",
			input:    "\x1b[31mcolored\x1b[0m output\n$ \nreal content",
			expected: "colored output\nreal content",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text with no prompts",
			input:    "just plain output\nsecond line",
			expected: "just plain output\nsecond line",
		},
		{
			name:     "codex prompt after ANSI strip",
			input:    "\x1b[1mresult\x1b[0m\ncodex> \nmore output",
			expected: "result\nmore output",
		},
		{
			name:     "codex v0.135 prompt after ANSI strip",
			input:    "\x1b[1mresult\x1b[0m\n› Find and fix a bug in @filename\nmore output",
			expected: "result\nmore output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cleanScreenOutput(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestIsPromptLine_EdgeCases verifies edge cases in prompt line detection.
func TestIsPromptLine_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"empty string is not prompt", "", false},
		{"whitespace only is not prompt", "   \t  ", false},
		{"dollar prompt", "$ ", true},
		{"hash prompt", "# ", true},
		{"regular text", "hello world", false},
		{"codex prompt", "codex> ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPromptLine(tt.line)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// --- SPEC-ORCH-013 R4: OpenCode Output Refinement ---

// TestIsPromptLine_ShellLoginBanner verifies shell login banner filtering.
// S7: "Last login:" lines must be filtered as noise.
func TestIsPromptLine_ShellLoginBanner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"Last login standard", "Last login: Fri Mar 28 10:00:00 on ttys001", true},
		{"Last login lowercase", "last login: Fri Mar 28 10:00:00 on ttys001", true},
		{"Last login with extra spaces", "  Last login: Fri Mar 28 09:00:00 on ttys002  ", true},
		{"Not a login banner", "Last modified: yesterday", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPromptLine(tt.line)
			assert.Equal(t, tt.expected, got,
				"shell login banner pattern must be recognized as noise")
		})
	}
}

// TestIsPromptLine_UserAtHostPrompt verifies user@host prompt filtering.
// S8: Lines matching "user@hostname $" pattern must be filtered.
func TestIsPromptLine_UserAtHostPrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"user@host dollar", "user@hostname $ ", true},
		{"root@server hash", "root@server.local # ", true},
		{"user@host percent", "dev@macbook % ", true},
		{"user@host no space", "user@host$", true},
		{"not a prompt", "email@example.com is my email", false},
		{"content with at sign", "send to admin@server for help", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPromptLine(tt.line)
			assert.Equal(t, tt.expected, got,
				"user@host prompt pattern must be recognized as noise")
		})
	}
}

// TestIsPromptLine_OpencodeTUIChrome verifies opencode TUI chrome filtering.
// R4: opencode TUI chrome patterns must be filtered as noise.
func TestIsPromptLine_OpencodeTUIChrome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"opencode session header", "Build · gpt-5.4 · OpenAI", true},
		{"opencode build line", "  Build GPT-5.4 OpenAI", true},
		{"opencode escape hint", "⬝⬝⬝ esc to cancel", true},
		{"opencode ctrl hint", "ctrl+c to quit", true},
		{"normal output", "The build process completed successfully", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPromptLine(tt.line)
			assert.Equal(t, tt.expected, got,
				"opencode TUI chrome must be recognized as noise")
		})
	}
}
