package orchestra

import (
	"regexp"
	"strings"
)

// output_sanitizer.go cleans provider output before it is merged or shown:
// escape sequences, status bars, CLI banners, inline noise, and prompt or
// TUI chrome lines (R3, R10).

// Compiled regex patterns for ANSI escape sequence stripping.
// @AX:NOTE: [AUTO] magic constants — compiled regexes encode terminal escape grammar; update when adding new escape types
var (
	// csiRe matches CSI sequences: \x1b[ followed by params and a final letter.
	csiRe = regexp.MustCompile(`\x1b\[\??[0-9;]*[a-zA-Z]`)

	// oscBelRe matches OSC sequences terminated by BEL (\x07).
	oscBelRe = regexp.MustCompile(`\x1b\][^\x07]*\x07`)

	// oscStRe matches OSC sequences terminated by ST (\x1b\\).
	oscStRe = regexp.MustCompile(`\x1b\].*?\x1b\\`)

	// dcsRe matches DCS sequences: \x1bP ... \x1b\\.
	dcsRe = regexp.MustCompile(`\x1bP[^\x1b]*\x1b\\`)

	// cursorSaveRestoreRe matches cursor save/restore escapes.
	cursorSaveRestoreRe = regexp.MustCompile(`\x1b[78]`)

	// statusBarRe matches tmux-style status bar lines.
	statusBarRe = regexp.MustCompile(`(?m)^\[\d+\]\s+\d+:.*$`)

	// cliBannerRe matches CLI banner lines containing Unicode block characters
	// (e.g., ▐▛███▜▌ Claude Code, ▝▜▄ Antigravity CLI, etc.).
	cliBannerRe = regexp.MustCompile(`(?m)^[^\n]*[▀▁▂▃▄▅▆▇█▉▊▋▌▍▎▏▐░▒▓▔▕▖▗▘▙▚▛▜▝▞▟]+[^\n]*$`)

	// multiBlankRe matches 3+ consecutive newlines.
	multiBlankRe = regexp.MustCompile(`\n{3,}`)
)

// SanitizeScreenOutput applies all output sanitization steps:
// 1. Strip extended ANSI escape sequences (CSI, OSC, DCS)
// 2. Strip terminal status bar lines
// 3. Trim trailing whitespace per line
// 4. Collapse consecutive blank lines to at most one
func SanitizeScreenOutput(raw string) string {
	if raw == "" {
		return ""
	}
	s := stripANSIExtended(raw)
	s = stripStatusBar(s)
	s = stripCLIBanners(s)
	s = trimTrailingWhitespace(s)
	s = collapseBlankLines(s)
	return s
}

// stripANSIExtended removes all ANSI escape sequences including CSI, OSC, DCS,
// and cursor save/restore escapes.
func stripANSIExtended(s string) string {
	s = csiRe.ReplaceAllString(s, "")
	s = oscBelRe.ReplaceAllString(s, "")
	s = oscStRe.ReplaceAllString(s, "")
	s = dcsRe.ReplaceAllString(s, "")
	s = cursorSaveRestoreRe.ReplaceAllString(s, "")
	return s
}

// stripStatusBar removes tmux/terminal status bar lines.
func stripStatusBar(s string) string {
	lines := strings.Split(s, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if statusBarRe.MatchString(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

// stripCLIBanners removes lines containing Unicode block characters used in
// CLI tool banners (Claude Code, Antigravity CLI, etc.).
func stripCLIBanners(s string) string {
	lines := strings.Split(s, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if cliBannerRe.MatchString(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

// collapseBlankLines reduces consecutive blank lines to at most one.
func collapseBlankLines(s string) string {
	return multiBlankRe.ReplaceAllString(s, "\n\n")
}

// trimTrailingWhitespace removes trailing whitespace from each line.
func trimTrailingWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Join(lines, "\n")
}

// codexSuggestionPromptPattern matches the codex v0.135+ TUI suggestion prompt.
const codexSuggestionPromptPattern = `(?im)^\s*›\s+\S.*$`

// defaultPromptPatterns matches common shell and CLI prompts.
// @AX:NOTE [AUTO] hardcoded prompt regexes — must stay in sync with DefaultCompletionPatterns
var defaultPromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^❯(?:\s|\x{00a0})*$`),        // claude code prompt (unicode heavy right-pointing angle)
	regexp.MustCompile(`(?m)^\s*>\s*(Type your|@|\s*$)`), // gemini TUI prompt (> Type your..., > @, bare >)
	regexp.MustCompile(`(?im)^codex>\s*$`),               // codex prompt (case-insensitive)
	regexp.MustCompile(codexSuggestionPromptPattern),     // codex v0.135+ TUI suggestion prompt
	regexp.MustCompile(`(?im)^Ask anything\s*$`),         // opencode TUI prompt
	regexp.MustCompile(`(?m)^\$\s*$`),                    // shell $ prompt
	regexp.MustCompile(`(?m)^#\s*$`),                     // root # prompt
}

// cliNoisePatterns matches provider CLI lines that are pure noise (used for line-level filtering).
var cliNoisePatterns = []*regexp.Regexp{
	// gemini CLI noise (line-level)
	regexp.MustCompile(`(?i)We're making changes to Gemini CLI`),
	regexp.MustCompile(`(?i)Update successful`),
	regexp.MustCompile(`(?i)What's\s+Changing:`),
	regexp.MustCompile(`(?i)How it\s+affects`),
	regexp.MustCompile(`(?i)Read more:\s*https://`),
	regexp.MustCompile(`(?i)/auth\s*$`),
	regexp.MustCompile(`(?i)/upgrade\s*$`),
	regexp.MustCompile(`(?i)Signed in with`),
	regexp.MustCompile(`(?i)Plan: Gemini`),
	// gemini CLI box drawing and single-char wrapped lines
	regexp.MustCompile(`^[╭╰│╮╯─]+$`),
	regexp.MustCompile(`^│\s*.{1,3}\s*│$`),
	regexp.MustCompile(`(?i)^Positional arguments now default`),
	regexp.MustCompile(`(?i)non-interactive mode.*--prompt`),
	// opencode TUI noise
	regexp.MustCompile(`(?i)Build\s+·\s+gpt`),
	regexp.MustCompile(`(?i)^\s*Build\s+GPT-[\d.]+\s+OpenAI`),
	regexp.MustCompile(`(?i)⬝+\s+esc`),
	regexp.MustCompile(`(?i)ctrl\+[a-z]\s`),
	// Additional opencode TUI chrome (without "Build" prefix)
	regexp.MustCompile(`(?i)^\s*gpt-[\d.]+\s+OpenAI`),
	// Shell login banner (macOS/Linux)
	regexp.MustCompile(`(?i)^Last login:`),
	// User@host shell prompt (zsh %, bash $, root #)
	regexp.MustCompile(`^\w+@[\w.-]+.*[%$#]\s*$`),
	// cmux status bar fragments
	regexp.MustCompile(`🐙\s+v?\d+\.\d+`),
}

// inlineNoisePatterns are stripped via regex replace (not line-level) to handle noise
// concatenated with content on the same line (e.g., "MCP issues detected.I will begin...").
var inlineNoisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)MCP issues detected\.\s*Run /mcp list for status\.?`),
	regexp.MustCompile(`(?i)ℹ\s*MCP issues detected\.\s*Run\s+/mcp list\s+for\s+status\.?`),
	regexp.MustCompile(`(?i)ℹ\s*Update\s+successful!\s*The new\s+version will be used on your next run\.?`),
}

// filterPromptLines removes lines matching known CLI prompt patterns from output.
func filterPromptLines(output string) string {
	lines := strings.Split(output, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if isPromptLine(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

// isPromptLine checks if a single line matches any known prompt or CLI noise pattern.
func isPromptLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	for _, p := range defaultPromptPatterns {
		if p.MatchString(line) {
			return true
		}
	}
	for _, p := range cliNoisePatterns {
		if p.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// stripInlineNoise removes noise fragments that may be concatenated with content on the same line.
func stripInlineNoise(s string) string {
	for _, p := range inlineNoisePatterns {
		s = p.ReplaceAllString(s, "")
	}
	return s
}

// cleanScreenOutput strips ANSI codes, inline noise, and prompt lines from raw screen content.
// Used to produce clean text for merge logic (R10).
func cleanScreenOutput(raw string) string {
	cleaned := SanitizeScreenOutput(raw)
	cleaned = stripInlineNoise(cleaned)
	return filterPromptLines(cleaned)
}
