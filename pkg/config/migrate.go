// Package config provides autopus.yaml schema and migration utilities.
package config

import "slices"

// defaultProviderEntries holds the canonical default settings for known orchestra providers.
// @AX:NOTE: [AUTO] hardcoded provider defaults — update when adding new providers or changing CLI flags
var defaultProviderEntries = map[string]ProviderEntry{
	"claude": DefaultClaudeProviderEntry(),
	"codex":  DefaultCodexProviderEntry(),
	// SPEC-ORCH-021 REQ-014/015: prompt is the value of --print (injected into "" slot).
	"gemini": {Binary: "agy", Args: []string{"--print", ""}, PromptViaArgs: true, Subprocess: SubprocessProvConf{OutputFormat: "text", Timeout: GeminiOrchestraTimeoutSeconds}},
}

func defaultProviderEntryForQuality(providerName string, quality QualityConf) (ProviderEntry, bool) {
	if providerName == "codex" {
		return CodexProviderEntryForQuality(quality), true
	}
	entry, ok := defaultProviderEntries[providerName]
	return entry, ok
}

// MigrateOrchestraConfig performs all orchestra config migrations.
// It returns (changed bool, err error).
//
// Migrations applied:
//  1. Classify Codex model policy: exact historical defaults become quality-managed;
//     every other unmarked provider becomes pinned.
//  2. Migrate opencode provider entries back to codex.
//  3. Upgrade a Claude provider still carrying a shipped historical default
//     argv onto the current default model policy.
//  4. For each platform that maps to a known orchestra provider,
//     add the provider entry if it is missing.
//  5. For each orchestra command, ensure every provider in orchestra.Providers
//     is listed in the command's Providers slice.
func MigrateOrchestraConfig(cfg *HarnessConfig) (bool, error) {
	if !cfg.Orchestra.Enabled {
		return false, nil
	}

	changed := false

	// Migration 1.5: migrate opencode back to codex.
	if migrated, _ := MigrateOpencodeToCodex(cfg); migrated {
		changed = true
	}

	// Migration 2: add missing provider entries for platforms, or update if args are empty.
	if cfg.Orchestra.Providers == nil {
		cfg.Orchestra.Providers = make(map[string]ProviderEntry)
	}
	for _, platform := range cfg.Platforms {
		providerName := PlatformToProvider(platform)
		if providerName == "" {
			continue
		}
		existing, exists, classified := classifyStoredCustomUnmarkedEmptyCodexProvider(cfg.Orchestra.Providers, providerName)
		if classified {
			changed = true
		}
		if shouldRestoreProviderDefaults(providerName, existing, exists) {
			if entry, known := defaultProviderEntryForQuality(providerName, cfg.Quality); known {
				cfg.Orchestra.Providers[providerName] = entry
				changed = true
			} else if !exists {
				cfg.Orchestra.Providers[providerName] = ProviderEntry{Binary: providerName}
				changed = true
			}
		}
	}
	if migrated := migrateKnownProviderDefaults(cfg); migrated {
		changed = true
	}

	// Migration 3: ensure every provider appears in every command's Providers list.
	if cfg.Orchestra.Commands == nil {
		return changed, nil
	}
	for cmdName, cmd := range cfg.Orchestra.Commands {
		for providerName := range cfg.Orchestra.Providers {
			if !containsString(cmd.Providers, providerName) {
				cmd.Providers = append(cmd.Providers, providerName)
				changed = true
			}
		}
		cfg.Orchestra.Commands[cmdName] = cmd
	}

	return changed, nil
}

func migrateKnownProviderDefaults(cfg *HarnessConfig) bool {
	changed := false
	for providerName, defaults := range defaultProviderEntries {
		existing, exists := cfg.Orchestra.Providers[providerName]
		if !exists || existing.Backend != "" {
			// Backend-routed providers carry no CLI argv, schema flag, or
			// pane wiring, so the CLI defaults below do not apply to them.
			continue
		}
		if providerName == "codex" {
			var migrated bool
			existing, migrated = migrateCodexProviderModelPolicy(existing, cfg.Quality)
			if migrated {
				changed = true
			}
			if existing.ModelPolicy == ProviderModelPolicyPinned {
				cfg.Orchestra.Providers[providerName] = existing
				continue
			}
		}
		if providerName == "claude" {
			var migrated bool
			if existing, migrated = upgradeHistoricalClaudeProviderDefaults(existing); migrated {
				changed = true
			}
		}
		if providerName == "gemini" && containsString(cfg.Platforms, "antigravity-cli") {
			existing, changed = migrateAntigravityGeminiProvider(existing, defaults, true, changed)
		}
		if existing.Subprocess.SchemaFlag == "" && defaults.Subprocess.SchemaFlag != "" {
			existing.Subprocess.SchemaFlag = defaults.Subprocess.SchemaFlag
			changed = true
		}
		if existing.Subprocess.Timeout == 0 && defaults.Subprocess.Timeout > 0 {
			existing.Subprocess.Timeout = defaults.Subprocess.Timeout
			changed = true
		}
		cfg.Orchestra.Providers[providerName] = existing
	}
	return changed
}

// EnsureOrchestraProvider ensures a specific provider exists in the orchestra config.
// This is used by the "platform add" command to keep orchestra config consistent.
func EnsureOrchestraProvider(cfg *HarnessConfig, providerName string) error {
	if !cfg.Orchestra.Enabled {
		return nil
	}

	// Initialize providers map if nil.
	if cfg.Orchestra.Providers == nil {
		cfg.Orchestra.Providers = make(map[string]ProviderEntry)
	}

	// Add provider if missing, or update if args are empty (stale config).
	existing, exists, _ := classifyStoredCustomUnmarkedEmptyCodexProvider(cfg.Orchestra.Providers, providerName)
	if shouldRestoreProviderDefaults(providerName, existing, exists) {
		entry, known := defaultProviderEntryForQuality(providerName, cfg.Quality)
		if !known {
			// Use a sensible zero-value entry for unknown providers.
			entry = ProviderEntry{Binary: providerName}
		}
		cfg.Orchestra.Providers[providerName] = entry
	} else if providerName == "gemini" && containsString(cfg.Platforms, "antigravity-cli") {
		defaults := defaultProviderEntries[providerName]
		existing, _ = migrateAntigravityGeminiProvider(existing, defaults, false, false)
		cfg.Orchestra.Providers[providerName] = existing
	}

	// Initialize commands map if nil.
	if cfg.Orchestra.Commands == nil {
		cfg.Orchestra.Commands = make(map[string]CommandEntry)
		return nil
	}

	// Append provider to each command that does not already list it.
	for cmdName, cmd := range cfg.Orchestra.Commands {
		if !containsString(cmd.Providers, providerName) {
			cmd.Providers = append(cmd.Providers, providerName)
			cfg.Orchestra.Commands[cmdName] = cmd
		}
	}

	return nil
}

// ProviderToPlatform maps common provider/binary names to valid platform names.
// Used to normalize user input that confuses provider names with platform names.
// Returns "" when no mapping is needed (name is already a valid platform name or unknown).
func ProviderToPlatform(name string) string {
	switch name {
	case "claude":
		return "claude-code"
	case "gemini", "gemini-cli", "agy", "antigravity":
		return "antigravity-cli"
	default:
		return "" // no normalization needed or unknown
	}
}

// MigratePlatformNames normalizes provider names mistakenly used as platform names.
// Returns true if any platform name was corrected.
func MigratePlatformNames(cfg *HarnessConfig) bool {
	changed := false
	for i, p := range cfg.Platforms {
		if corrected := ProviderToPlatform(p); corrected != "" {
			cfg.Platforms[i] = corrected
			changed = true
		}
	}
	return changed
}

// PlatformToProvider maps platform names to orchestra provider names.
// @AX:NOTE: [AUTO] "opencode" maps to "codex" — intentional alias per SPEC-ORCHCFG-002 migration
func PlatformToProvider(platform string) string {
	switch platform {
	case "claude-code":
		return "claude"
	case "codex":
		return "codex"
	case "antigravity-cli", "gemini-cli":
		return "gemini"
	case "opencode":
		return "codex"
	default:
		return ""
	}
}

// MigrateOpencodeToCodex replaces opencode provider entries with codex.
// Removes opencode from providers and commands, adds codex if not already present.
// Returns (changed bool, err error).
// @AX:NOTE [AUTO]: One-way migration per SPEC-ORCHCFG-002 — opencode entries are permanently replaced with codex
func MigrateOpencodeToCodex(cfg *HarnessConfig) (bool, error) {
	if !cfg.Orchestra.Enabled {
		return false, nil
	}
	if cfg.Orchestra.Providers == nil {
		return false, nil
	}

	_, hasOpencode := cfg.Orchestra.Providers["opencode"]
	if !hasOpencode {
		return false, nil
	}

	// Remove opencode entry.
	delete(cfg.Orchestra.Providers, "opencode")

	// Add codex entry if not already present, or update if args are empty.
	existing, hasCodex, _ := classifyStoredCustomUnmarkedEmptyCodexProvider(cfg.Orchestra.Providers, "codex")
	if shouldRestoreProviderDefaults("codex", existing, hasCodex) {
		cfg.Orchestra.Providers["codex"] = CodexProviderEntryForQuality(cfg.Quality)
	}

	// Replace opencode with codex in all command provider lists.
	for cmdName, cmd := range cfg.Orchestra.Commands {
		cmd.Providers = replaceInSlice(cmd.Providers, "opencode", "codex")
		cfg.Orchestra.Commands[cmdName] = cmd
	}

	return true, nil
}

// replaceInSlice replaces old with new in a string slice, removing duplicates.
func replaceInSlice(slice []string, old, new string) []string {
	hasNew := false
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s == old {
			if !hasNew {
				result = append(result, new)
				hasNew = true
			}
			continue
		}
		if s == new {
			hasNew = true
		}
		result = append(result, s)
	}
	return result
}

// containsString reports whether slice contains s.
func containsString(slice []string, s string) bool {
	return slices.Contains(slice, s)
}
