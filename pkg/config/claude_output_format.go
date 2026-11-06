package config

import "slices"

// claudeDefaultArgs is the shipped Claude argv with the given print flag
// spelling: the model policy tail, then the json output format.
func claudeDefaultArgs(printFlag string) []string {
	args := append([]string{printFlag}, claudeDefaultModelArgs()...)
	return append(args, OutputFormatFlag, ClaudeOutputFormat)
}

// withoutDefaultClaudeOutputFormat drops a trailing `--output-format json`
// pair, the one piece migration adds, so a historical model tail is still
// recognised after the output format was appended to it.
func withoutDefaultClaudeOutputFormat(tail []string) []string {
	n := len(tail)
	if n >= 2 && tail[n-2] == OutputFormatFlag && tail[n-1] == ClaudeOutputFormat {
		return tail[:n-2]
	}
	return tail
}

// migrateClaudeOutputFormat moves a print-mode claude entry onto the json
// envelope, which carries the result text and the call's usage. It adds the
// pair when absent and rewrites text in place; the model, effort, and every
// other argument keep their place, and an explicit json or stream-json stays.
// A backend-routed entry has no argv, a wrapper binary owns its own flag
// contract, and an argv without a print flag is not a headless call.
func migrateClaudeOutputFormat(entry ProviderEntry) (ProviderEntry, bool) {
	if entry.Backend != "" || entry.Binary != "claude" ||
		!slices.ContainsFunc(entry.Args, func(arg string) bool { return slices.Contains(claudePrintFlags, arg) }) {
		return entry, false
	}
	args, changed := withOutputFormat(entry.Args, ClaudeOutputFormat, claudeLegacyOutputFormat)
	entry.Args = args
	return entry, changed
}
