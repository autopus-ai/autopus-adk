package orchestra

import (
	"os"
	"regexp"
	"strings"
	"time"
)

// ansiEscapeRe matches ANSI escape sequences including color codes, cursor movement, etc.
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes all ANSI escape sequences from the input string.
func stripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}

// codexReadyPromptPattern matches the codex ready prompt, including the
// v0.135+ TUI suggestion line.
const codexReadyPromptPattern = `(?im)^(?:codex>\s*|\s*›\s+\S.*)$`

// isPromptVisible checks if the screen content contains a visible prompt pattern,
// indicating the CLI session has returned to input-ready state.
// This is the PRIMARY completion detection method (R7).
// If a working indicator (spinner, "Thinking", etc.) is also visible, the prompt
// is considered a false positive — the provider's TUI shows the prompt at all times.
// @AX:NOTE [AUTO] called by pollUntilPrompt and waitForCompletion — central prompt detection logic
func isPromptVisible(screen string, patterns []CompletionPattern) bool {
	// Strip ANSI escape codes before matching — providers like claude/opencode
	// render the ">" prompt with color codes (e.g. \x1b[32m>\x1b[0m) that break
	// the ^>\s*$ pattern when matching raw ReadScreen output.
	screen = stripANSI(screen)

	// Guard: if working indicators are visible, the provider is still active.
	// Some TUIs (e.g., Gemini) show the idle prompt alongside a spinner.
	if isProviderWorking(screen) {
		return false
	}

	// Check provider-specific patterns first
	for _, cp := range patterns {
		if cp.Pattern.MatchString(screen) {
			return true
		}
	}
	// Fallback to default patterns
	for _, p := range defaultPromptPatterns {
		if p.MatchString(screen) {
			return true
		}
	}
	return false
}

// toolApprovalPatterns matches interactive tool permission prompts from providers.
// When detected, the orchestra auto-approves by sending "1" (Allow once).
var toolApprovalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Action Required`),                   // gemini tool permission
	regexp.MustCompile(`(?i)Allow execution of`),                // gemini sandbox prompt
	regexp.MustCompile(`(?i)Do you want to allow`),              // generic permission prompt
	regexp.MustCompile(`(?i)●\s*1\.\s*Allow\s+(once|for this)`), // gemini numbered option
}

// needsToolApproval checks if the screen shows an interactive tool permission prompt.
func needsToolApproval(screen string) bool {
	screen = stripANSI(screen)
	for _, p := range toolApprovalPatterns {
		if p.MatchString(screen) {
			return true
		}
	}
	return false
}

// providerWorkingPatterns matches progress indicators showing the provider is still active.
var providerWorkingPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Generating`),
	regexp.MustCompile(`(?i)Working\s*\(`),
	regexp.MustCompile(`(?i)Thinking`),
	regexp.MustCompile(`(?i)thinking with`),
	regexp.MustCompile(`(?i)Running\s+\w`),
	regexp.MustCompile(`(?i)Executing`),
	regexp.MustCompile(`(?i)Explored\b`),
	regexp.MustCompile(`(?i)✳`), // claude thinking indicator
	regexp.MustCompile(`(?im)^\s*[·*✢✣✤✥✦✧✳✶✻✽✺✹]\s*[A-Z][A-Za-z]+\s*(…|\.\.\.)(\s*\([^)]*\))?\s*$`), // claude status line while a prompt is running
	regexp.MustCompile(`[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]`),                                                               // braille spinner (gemini "taking a bit longer")
	regexp.MustCompile(`(?i)Working(\.\.\.|…)`),                                                      // gemini active tool/model status
	regexp.MustCompile(`(?i)taking a bit longer`),                                                    // gemini processing message
	regexp.MustCompile(`(?i)still on it`),                                                            // gemini processing message
	regexp.MustCompile(`(?i)esc to cancel`),                                                          // gemini cancel hint while active
	regexp.MustCompile(`(?i)\besc\s+to\b`),                                                           // codex/gemini cancel hint while active
	regexp.MustCompile(`(?im)^\s*•\s+.+\(\d+(?:m\s+\d+s|s)\s*•\s*e(?:sc)?(?:\s+to)?(?:…|\.\.\.)?`),   // codex active status, often truncated in narrow panes
	regexp.MustCompile(`(?im)^\s*•\s+.+\(\d+(?:m\s+\d+s|s)\s+(?:…|\.\.\.)`),                          // codex active status when cancel hint is clipped
}

// isProviderWorking checks if the screen shows progress indicators meaning the provider is active.
func isProviderWorking(screen string) bool {
	screen = stripANSI(screen)
	for _, p := range providerWorkingPatterns {
		if p.MatchString(screen) {
			return true
		}
	}
	return false
}

// isProviderStillWorking checks per-provider working patterns on screen.
// Returns true if any pattern matches, meaning the provider is still generating
// and completion should be deferred even if the idle prompt is visible.
// Returns false if patterns is nil/empty (provider has no working patterns).
func isProviderStillWorking(screen string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	cleaned := stripANSI(screen)
	for _, pat := range patterns {
		if strings.Contains(cleaned, pat) {
			return true
		}
	}
	return false
}

// isOutputIdle checks if the output file has not been modified for the given threshold.
// This is the SECONDARY completion detection method (R7).
func isOutputIdle(outputFile string, threshold time.Duration) bool {
	info, err := os.Stat(outputFile)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) >= threshold
}

// CleanScreenForCrossPollination applies full sanitization for Round 2 cross-pollination.
// Strips TUI noise and self-assigned ICE scores (to prevent confidence cascade),
// but preserves all idea content, SCAMPER analysis, HMW questions, and reasoning.
func CleanScreenForCrossPollination(raw string) string {
	cleaned := cleanScreenOutput(raw)
	cleaned = stripICEScores(cleaned)
	return strings.TrimSpace(cleaned)
}
