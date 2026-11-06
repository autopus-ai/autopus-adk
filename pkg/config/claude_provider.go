package config

import "slices"

const (
	// ClaudeOrchestraEffort is the reasoning depth the shipped Claude orchestra
	// provider asks for. Orchestra is the review, plan, and secure surface, so
	// it follows the top rung of the balanced role matrix instead of a cheaper
	// mid-tier: a truncated structured review costs a whole revision round.
	ClaudeOrchestraEffort = "max"

	claudePrintFlag = "--print"

	// ClaudeOutputFormat is the stdout format the shipped Claude orchestra
	// provider asks for: one type=result object whose result is the answer and
	// whose usage and modelUsage carry the call's tokens, which text lacks.
	ClaudeOutputFormat = "json"
	// claudeLegacyOutputFormat is replaced by migration: it carries no usage.
	claudeLegacyOutputFormat = "text"
)

// claudePrintFlags are the print-flag spellings a stored default may carry.
// `-p` is the CLI's own short alias.
var claudePrintFlags = []string{claudePrintFlag, "-p"}

// historicalClaudeDefaultModelArgs lists every model policy tail autopus has
// shipped as the Claude orchestra default before the current one. A stored
// argv that still names one is reported, never rewritten: a user who chose
// `--model opus --effort max` on purpose writes the same argv as an untouched
// old default, so moving it would silently replace a deliberate model choice.
// `auto update` prints one notice line instead.
//
// The model-less `--print` default from the pre-tier era is deliberately
// absent. It pins nothing, so there is no former model to report.
var historicalClaudeDefaultModelArgs = [][]string{
	{"--model", "opus", "--effort", "high"},
	{"--model", "opus", "--effort", "max"},
	// v0.50.116-v0.50.133; replaced when Fable became Ultra-only.
	{"--model", "claude-fable-5-1", "--effort", "max"},
}

// claudeDefaultModelArgs is the model policy tail of the shipped Claude argv.
// The print flag is prepended per surface so a stored entry keeps whichever
// spelling it already carries.
func claudeDefaultModelArgs() []string {
	return []string{"--model", ClaudeOpusModel, "--effort", ClaudeOrchestraEffort}
}

// DefaultClaudeProviderEntry returns the canonical Claude orchestra provider
// entry: `claude --print` streams a single response to the subprocess backend.
func DefaultClaudeProviderEntry() ProviderEntry {
	return ProviderEntry{
		Binary:     "claude",
		Args:       claudeDefaultArgs(claudePrintFlag),
		Subprocess: SubprocessProvConf{Timeout: ClaudeOrchestraTimeoutSeconds},
	}
}

// ClaudeProviderFormerDefault reports the model of a stored Claude provider
// whose argv tail equals a former shipped default, such as `opus` for
// `--print --model opus --effort max`. Nothing is rewritten.
//
// The guards keep the report narrow: a backend-routed entry has no CLI argv,
// a pinned policy is an explicit model decision, and a wrapper binary is
// explicit configuration whose flag contract the harness does not own.
func ClaudeProviderFormerDefault(entry ProviderEntry) (string, bool) {
	if entry.Backend != "" || entry.ModelPolicy == ProviderModelPolicyPinned || entry.Binary != "claude" {
		return "", false
	}
	args := entry.Args
	if len(args) == 0 || !slices.Contains(claudePrintFlags, args[0]) {
		return "", false
	}
	tail := withoutDefaultClaudeOutputFormat(args[1:])
	if !slices.ContainsFunc(historicalClaudeDefaultModelArgs, func(shipped []string) bool {
		return slices.Equal(tail, shipped)
	}) {
		return "", false
	}
	return tail[1], true
}
