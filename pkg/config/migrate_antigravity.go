package config

import "slices"

func migrateAntigravityGeminiProvider(existing, defaults ProviderEntry, replaceEmptyArgs, changed bool) (ProviderEntry, bool) {
	// Legacy gemini-cli configs used the `gemini` binary; antigravity uses `agy`.
	if existing.Binary == "" || existing.Binary == "gemini" {
		existing.Binary = defaults.Binary
		changed = true
	}
	// SPEC-ORCH-021 REQ-014/015: the agy provider runs `--print <prompt>` where the
	// prompt is injected into a trailing value slot, so PromptViaArgs must be true.
	// Configs written before that contract leave PromptViaArgs unset (false), so the
	// prompt is never injected and `agy --print` dies with "flag needs an argument".
	// Such stale entries (e.g. a bare ["--print"]) match neither the legacy gemini-cli
	// pattern nor the empty-args check below, so detect them via the PromptViaArgs
	// marker and reconcile the full agy invocation contract.
	staleContract := defaults.PromptViaArgs && !existing.PromptViaArgs
	if staleContract || isLegacyGeminiCLIArgs(existing.Args) || (replaceEmptyArgs && len(existing.Args) == 0) {
		existing.Args = append([]string{}, defaults.Args...)
		existing.PromptViaArgs = defaults.PromptViaArgs
		if existing.Subprocess.OutputFormat == "" {
			existing.Subprocess.OutputFormat = defaults.Subprocess.OutputFormat
		}
		changed = true
	}
	return existing, changed
}

func isLegacyGeminiCLIArgs(args []string) bool {
	return slices.Equal(args, []string{"-m", "gemini-3.1-pro-preview", "-p", ""})
}
