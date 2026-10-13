package config

import "fmt"

// @AX:WARN [AUTO]: This cross-field validator has cyclomatic complexity 15 across preset, provider, tier, and model-policy constraints. @AX:SPEC SPEC-PROVIDER-QUALITY-001
// @AX:REASON [AUTO]: Every invalid persisted quality combination must remain fail-closed before provider model selection is consumed.
func (c *HarnessConfig) validateModelSelectionConfig() error {
	for preset := range c.Quality.Presets {
		if !IsValidQualityPresetName(preset) {
			return fmt.Errorf(
				"quality.presets name %q is invalid: use 1-64 ASCII letters, digits, hyphens, or underscores",
				preset,
			)
		}
	}
	if !c.Quality.IsValidSupervisorModelPolicy() {
		return fmt.Errorf(
			"quality.supervisor_model_policy %q is invalid: must be inherit or quality",
			c.Quality.SupervisorModelPolicy,
		)
	}
	for provider, preset := range c.Quality.Providers {
		if _, ok := NormalizeQualityProvider(provider); !ok || provider != normalizeQualityProviderKey(provider) {
			return fmt.Errorf(
				"quality.providers provider %q is invalid: must be claude or codex",
				provider,
			)
		}
		if !c.Quality.isKnownQualityMode(preset) {
			return fmt.Errorf(
				"quality.providers[%s] %q is not defined in quality.presets",
				provider,
				preset,
			)
		}
	}
	validModelTiers := map[string]bool{"fable": true, "opus": true, "sonnet": true, "haiku": true}
	for presetName, preset := range c.Quality.Presets {
		for agentName, tier := range preset.Agents {
			if !validModelTiers[tier] {
				// The keys are free text from autopus.yaml; %q keeps them printable.
				return fmt.Errorf("quality.presets[%q].agents[%q]: unknown model tier %q", presetName, agentName, tier)
			}
		}
	}
	if err := c.validateQualityPresetEfforts(); err != nil {
		return err
	}
	for providerName, provider := range c.Orchestra.Providers {
		if provider.ModelPolicy != "" && provider.ModelPolicy != ProviderModelPolicyQuality && provider.ModelPolicy != ProviderModelPolicyPinned {
			return fmt.Errorf("orchestra.providers[%q].model_policy %q is invalid: must be quality or pinned", providerName, provider.ModelPolicy)
		}
	}
	return nil
}
