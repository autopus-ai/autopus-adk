package cli

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func loadHarnessConfig() (*config.HarnessConfig, error) {
	return loadHarnessConfigForFlags(globalFlags{})
}

func loadHarnessConfigForFlags(flags globalFlags) (*config.HarnessConfig, error) {
	return loadHarnessConfigForDir(".", flags)
}

func loadHarnessConfigForDir(startDir string, flags globalFlags) (*config.HarnessConfig, error) {
	effective, err := loadEffectiveHarnessConfigForDir(startDir, flags)
	if err != nil {
		return nil, err
	}
	return effective.Config, nil
}

func loadEffectiveHarnessConfigForFlags(flags globalFlags) (effectiveHarnessConfig, error) {
	return loadEffectiveHarnessConfigForDir(".", flags)
}

func loadEffectiveHarnessConfigForDir(startDir string, flags globalFlags) (effectiveHarnessConfig, error) {
	configPath := strings.TrimSpace(flags.ConfigPath)
	if configPath != "" {
		configDir, err := resolveConfigDir(nil, configPath)
		if err != nil {
			return effectiveHarnessConfig{}, err
		}
		cfg, err := loadAndMigrateHarnessConfig(configDir)
		if err != nil {
			return effectiveHarnessConfig{}, err
		}
		return applyRuntimeHarnessOverrides(effectiveHarnessConfig{Config: cfg, ConfigDir: configDir}, flags), nil
	}

	cfg, err := loadAndMigrateHarnessConfig(startDir)
	if err != nil {
		return effectiveHarnessConfig{}, err
	}
	effective := effectiveHarnessConfig{Config: cfg, ConfigDir: startDir}

	missing, err := detectMissingInheritedHarnessSections(startDir, cfg)
	if err != nil {
		return effectiveHarnessConfig{}, err
	}
	effective.Missing = missing
	if !missing.any() {
		return applyRuntimeHarnessOverrides(effective, flags), nil
	}

	parent, parentDir, err := findAncestorHarnessConfigWithReusableWiring(startDir)
	if err != nil {
		return effectiveHarnessConfig{}, err
	}
	if parent == nil {
		return applyRuntimeHarnessOverrides(effective, flags), nil
	}

	inheritReusableHarnessWiring(cfg, parent, missing)
	effective.ParentDir = parentDir
	return applyRuntimeHarnessOverrides(effective, flags), nil
}

// loadOrchestraConfig loads the orchestra configuration from autopus.yaml
// located in the current working directory.
// Applies in-memory migrations (e.g., codex → opencode) before returning.
func loadOrchestraConfig() (*config.OrchestraConf, error) {
	cfg, err := loadHarnessConfig()
	if err != nil {
		return nil, err
	}
	return &cfg.Orchestra, nil
}

// resolveProviders converts config providers to orchestra.ProviderConfig slice.
// Priority order: CLI flags > command-specific config > all global config providers.
//
// For each resolved provider name:
//   - If the name exists in conf.Providers, use its Binary, Args, and PromptViaArgs.
//   - If not found, fall back to: Binary=name, Args=[], PromptViaArgs=false.
func resolveProviders(conf *config.OrchestraConf, commandName string, flagProviders []string) []orchestra.ProviderConfig {
	// Determine provider names by priority
	names := resolveProviderNames(conf, commandName, flagProviders)

	result := make([]orchestra.ProviderConfig, 0, len(names))
	for _, name := range names {
		entry, ok := conf.Providers[name]
		if !ok {
			// Unknown provider: use name as binary with no args
			result = append(result, orchestra.ProviderConfig{
				Name:          name,
				Binary:        name,
				Args:          []string{},
				PromptViaArgs: false,
			})
			continue
		}
		result = append(result, providerConfigFromEntry(name, entry))
	}
	return result
}

func resolveProviderStartupTimeout(name string) time.Duration {
	return defaultProviderStartupTimeout(name)
}

func resolveProviderExecutionTimeout(entry config.ProviderEntry) time.Duration {
	if entry.Subprocess.Timeout > 0 {
		return time.Duration(entry.Subprocess.Timeout) * time.Second
	}
	return 0
}

// resolveProviderNames returns provider names based on priority:
// 1. flagProviders (non-empty CLI flag)
// 2. conf.Commands[commandName].Providers (command-specific config)
// 3. all keys from conf.Providers (global fallback)
func resolveProviderNames(conf *config.OrchestraConf, commandName string, flagProviders []string) []string {
	if len(flagProviders) > 0 {
		return flagProviders
	}

	if cmd, ok := conf.Commands[commandName]; ok && len(cmd.Providers) > 0 {
		return cmd.Providers
	}

	// Collect all configured provider names
	names := make([]string, 0, len(conf.Providers))
	for name := range conf.Providers {
		names = append(names, name)
	}
	return names
}

// resolveJudge determines the judge provider to use for debate strategy.
// Priority order: CLI flag > command-specific config > global orchestra judge > empty string.
func resolveJudge(conf *config.OrchestraConf, commandName string, flagJudge string) string {
	if flagJudge != "" {
		return flagJudge
	}
	if cmd, ok := conf.Commands[commandName]; ok && cmd.Judge != "" {
		return cmd.Judge
	}
	if conf.Judge != "" {
		return conf.Judge
	}
	return ""
}

// resolveStrategy determines the strategy to use.
// Priority order: CLI flag > command-specific config > global default > "consensus".
func resolveStrategy(conf *config.OrchestraConf, commandName string, flagStrategy string) string {
	if flagStrategy != "" {
		return flagStrategy
	}

	if cmd, ok := conf.Commands[commandName]; ok && cmd.Strategy != "" {
		return cmd.Strategy
	}

	if conf.DefaultStrategy != "" {
		return conf.DefaultStrategy
	}

	return "consensus"
}

// resolveThreshold determines the consensus threshold to use.
// Priority order: CLI flag > command-specific config > global config > 0.66 default.
func resolveThreshold(conf *config.OrchestraConf, commandName string, flagValue float64) float64 {
	if flagValue > 0 {
		return flagValue
	}

	if cmd, ok := conf.Commands[commandName]; ok && cmd.ConsensusThreshold > 0 {
		return cmd.ConsensusThreshold
	}

	if conf.ConsensusThreshold > 0 {
		return conf.ConsensusThreshold
	}

	return 0.66
}

// resolveWorkingPatterns returns per-provider working patterns.
// Uses explicit YAML config if provided, otherwise falls back to built-in defaults.
// The pane backend was its only consumer; SPEC-PANERM-001 W2 (T9) deletes it.
//
//nolint:unused // kept until the SPEC-PANERM-001 W2 deletion merge (T9)
func resolveWorkingPatterns(providerName string, configured []string) []string {
	if len(configured) > 0 {
		return configured
	}
	// Built-in defaults for known providers whose TUI shows the prompt while still generating.
	switch providerName {
	case "gemini":
		return []string{"⠴", "⠧", "⠋", "⠙", "⠹", "⠸", "⠼", "Generating", "Thinking"}
	case "codex":
		return []string{"Thinking", "Generating", "Running", "Executing"}
	default:
		return nil
	}
}

// resolveSubprocessTimeout returns the per-provider subprocess timeout.
// Priority: per-provider override > global orchestra timeout > 120s default.
func resolveSubprocessTimeout(conf *config.OrchestraConf, entry config.ProviderEntry) time.Duration {
	if entry.Subprocess.Timeout > 0 {
		return time.Duration(entry.Subprocess.Timeout) * time.Second
	}
	if conf.TimeoutSeconds > 0 {
		return time.Duration(conf.TimeoutSeconds) * time.Second
	}
	return 120 * time.Second
}

func resolveCommandTimeout(conf *config.OrchestraConf, requestedTimeout int, timeoutChanged bool) int {
	if timeoutChanged && requestedTimeout > 0 {
		return requestedTimeout
	}
	if conf != nil && conf.TimeoutSeconds > 0 {
		return conf.TimeoutSeconds
	}
	if requestedTimeout > 0 {
		return requestedTimeout
	}
	return 120
}

// resolveSubprocessMode returns whether subprocess mode is enabled via config.
func resolveSubprocessMode(conf *config.OrchestraConf) bool {
	return conf.Subprocess.Enabled
}

// resolveSubprocessRounds returns the configured debate rounds for subprocess mode.
// Falls back to 1 if unset.
func resolveSubprocessRounds(conf *config.OrchestraConf) int {
	if conf.Subprocess.Rounds > 0 {
		return conf.Subprocess.Rounds
	}
	return 1
}

// resolveMaxConcurrent returns the max concurrent subprocess limit.
// Falls back to 3 if unset.
func resolveMaxConcurrent(conf *config.OrchestraConf) int {
	if conf.Subprocess.MaxConcurrent > 0 {
		return conf.Subprocess.MaxConcurrent
	}
	return 3
}

// validateThreshold checks that a threshold value is a valid number within [0.0, 1.0].
func validateThreshold(threshold float64) error {
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return fmt.Errorf("threshold must be a valid number, got %v", threshold)
	}
	if threshold < 0.0 || threshold > 1.0 {
		return fmt.Errorf("threshold must be between 0.0 and 1.0, got %f", threshold)
	}
	return nil
}
