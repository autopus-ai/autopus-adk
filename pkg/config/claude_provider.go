package config

import "slices"

const (
	// ClaudeOrchestraEffort is the reasoning depth the shipped Claude orchestra
	// provider asks for. Orchestra is the review, plan, and secure surface, so
	// it follows the top rung of the balanced role matrix instead of a cheaper
	// mid-tier: a truncated structured review costs a whole revision round.
	ClaudeOrchestraEffort = "max"

	claudePrintFlag = "--print"
)

// claudePrintFlags are the print-flag spellings a stored default may carry.
// `-p` is the CLI's own short alias.
var claudePrintFlags = []string{claudePrintFlag, "-p"}

// historicalClaudeDefaultModelArgs lists every model policy tail autopus has
// shipped as the Claude orchestra default. Only these exact tails are eligible
// for a default upgrade: a config that still names one is following the harness
// default, while any other tail — an extra flag, a full model id, a different
// effort — is a deliberate user choice.
//
// The model-less `--print` default from the pre-tier era is deliberately
// absent. It pins nothing, so upgrading it would introduce a model policy the
// user never had rather than move one they already accepted.
var historicalClaudeDefaultModelArgs = [][]string{
	{"--model", "opus", "--effort", "high"},
	{"--model", "opus", "--effort", "max"},
}

// claudeDefaultModelArgs is the model policy tail of the shipped Claude argv.
// The print flag is prepended per surface so a stored entry keeps whichever
// spelling it already carries.
func claudeDefaultModelArgs() []string {
	return []string{"--model", ClaudeFableModel, "--effort", ClaudeOrchestraEffort}
}

// DefaultClaudeProviderEntry returns the canonical Claude orchestra provider
// entry: `claude --print` streams a single response to the subprocess backend.
func DefaultClaudeProviderEntry() ProviderEntry {
	return ProviderEntry{
		Binary:     "claude",
		Args:       append([]string{claudePrintFlag}, claudeDefaultModelArgs()...),
		Subprocess: SubprocessProvConf{Timeout: ClaudeOrchestraTimeoutSeconds},
	}
}

// upgradeHistoricalClaudeDefaultArgs moves an argv from a shipped historical
// default onto the current default model policy, preserving the print flag
// spelling. Anything else is returned untouched.
func upgradeHistoricalClaudeDefaultArgs(args []string) ([]string, bool) {
	if len(args) == 0 || !slices.Contains(claudePrintFlags, args[0]) {
		return args, false
	}
	tail := args[1:]
	if !slices.ContainsFunc(historicalClaudeDefaultModelArgs, func(shipped []string) bool {
		return slices.Equal(tail, shipped)
	}) {
		return args, false
	}
	return append([]string{args[0]}, claudeDefaultModelArgs()...), true
}

// upgradeHistoricalClaudeProviderDefaults moves a Claude provider that still
// carries a shipped historical default onto the current default model policy.
//
// The guards keep the upgrade narrow rather than provider-wide: a backend-routed
// entry has no CLI argv to rewrite, a pinned policy is an explicit model
// decision, and a wrapper binary is explicit configuration whose flag contract
// the harness does not own.
func upgradeHistoricalClaudeProviderDefaults(entry ProviderEntry) (ProviderEntry, bool) {
	if entry.Backend != "" || entry.ModelPolicy == ProviderModelPolicyPinned || entry.Binary != "claude" {
		return entry, false
	}
	args, upgraded := upgradeHistoricalClaudeDefaultArgs(entry.Args)
	entry.Args = args
	return entry, upgraded
}
