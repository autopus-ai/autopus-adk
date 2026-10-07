package orchestra

import (
	"fmt"
)

// paneArgs returns the args to use in pane mode for the given provider.
// Uses PaneArgs if set; otherwise falls back to Args unchanged.
func paneArgs(p ProviderConfig) []string {
	if len(p.PaneArgs) > 0 {
		return p.PaneArgs
	}
	return p.Args
}

// buildPaneCommand constructs the shell command to execute in a pane.
// SEC-001/SEC-004: all arguments are shell-escaped to prevent injection.
func buildPaneCommand(provider ProviderConfig, prompt, outputFile string) string {
	// SEC-004: escape each arg individually
	args := shellEscapeArgs(paneArgs(provider))

	// SEC-006: escape binary path to prevent shell metacharacter injection
	binary := shellEscapeArg(provider.Binary)

	// SEC-007: escape outputFile to prevent command injection
	safeOutput := shellEscapeArg(outputFile)

	if provider.PromptViaArgs {
		args = shellEscapeArgs(injectPromptArg(paneArgs(provider), prompt))
		// SEC-001: use shell-escaped prompt instead of raw double quotes
		return fmt.Sprintf("%s %s | tee %s; echo %s >> %s",
			binary, args, safeOutput, sentinel, safeOutput)
	}
	// SEC-001: use unique heredoc delimiter to prevent prompt content from terminating it
	delim := uniqueHeredocDelimiter("PROMPT_EOF", prompt, randomHex())
	return fmt.Sprintf("( %s %s <<'%s'\n%s\n%s\n) | tee %s; echo %s >> %s",
		binary, args, delim, prompt, delim, safeOutput, sentinel, safeOutput)
}
