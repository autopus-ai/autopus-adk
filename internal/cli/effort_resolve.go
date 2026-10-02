// Package cli implements effort resolution logic for the effort detect command.
package cli

import (
	"fmt"
	"os"
	"strings"
)

// EffortResolveInput holds all inputs needed to determine the recommended effort.
type EffortResolveInput struct {
	// EnvValue is the raw value of CLAUDE_CODE_EFFORT_LEVEL (empty string = unset).
	EnvValue string
	// FlagEffort is the value supplied via --effort flag (empty = not set).
	FlagEffort string
	// FrontmatterEffort is the effort declared in agent frontmatter (empty = not set).
	FrontmatterEffort string
	// FlagQuality is the value supplied via --quality flag ("ultra" | "balanced" | "").
	FlagQuality string
	// FlagComplexity is the value supplied via --complexity flag ("low"|"medium"|"high"|"").
	FlagComplexity string
	// Model is the model identifier to use for quality-mode mapping.
	Model string
}

// defaultModel is the fallback model when no model can be detected.
const defaultModel = "claude-opus-5-5"

// envEffortKey is the environment variable name for effort override.
const envEffortKey = "CLAUDE_CODE_EFFORT_LEVEL"

// modelEnvKey is the environment variable name for model detection.
const modelEnvKey = "CLAUDE_MODEL"

// ResolveEffort applies the priority chain and returns an EffortResult.
// Priority (highest to lowest): flag > env > frontmatter > quality_mode > settings_default.
// This function does NOT read env vars directly; callers supply EnvValue so tests
// can inject values without modifying the process environment.
func ResolveEffort(in EffortResolveInput) (EffortResult, error) {
	model := resolveModel(in.Model)

	// 1. Explicit --effort flag bypasses enum guards by design (SPEC-CC21-001 R4-3).
	if in.FlagEffort != "" {
		return EffortResult{
			Effort: EffortValue(in.FlagEffort),
			Source: EffortSourceFlag,
			Model:  model,
			Reason: "--effort flag",
		}, nil
	}

	// 2. Env var is applied only when it is within the SPEC enum; unknown values
	// are treated as unset so callers can fall back to quality-mode defaults.
	if in.EnvValue != "" {
		ev := EffortValue(in.EnvValue)
		if isValidEffort(ev) {
			return EffortResult{
				Effort: ev,
				Source: EffortSourceEnv,
				Model:  model,
				Reason: "CLAUDE_CODE_EFFORT_LEVEL environment variable",
			}, nil
		}
	}

	// 3. Frontmatter default applies before quality-mode mapping.
	if in.FrontmatterEffort != "" {
		ev := EffortValue(in.FrontmatterEffort)
		if isValidEffort(ev) {
			return EffortResult{
				Effort: ev,
				Source: EffortSourceFrontmatter,
				Model:  model,
				Reason: "agent frontmatter",
			}, nil
		}
	}

	// 4. Quality-mode mapping.
	if in.FlagQuality != "" {
		return resolveQualityMode(in.FlagQuality, in.FlagComplexity, model)
	}

	// 5. Settings default: balanced/medium fallback.
	return EffortResult{
		Effort: EffortMedium,
		Source: EffortSourceSettingsDefault,
		Model:  model,
		Reason: "settings.json default",
	}, nil
}

// envEffortWarning returns SPEC-mandated warning text for an unsupported env value.
func envEffortWarning(value string) []string {
	if value == "" || isValidEffort(EffortValue(value)) {
		return nil
	}
	return []string{
		fmt.Sprintf("[warn] CLAUDE_CODE_EFFORT_LEVEL=%s is outside SPEC-CC21-001 enum (low|medium|high|xhigh|max); falling back to Quality Mode default.", value),
		fmt.Sprintf("[warn] If %s is intentional, set --effort=%s explicitly to bypass this guard.", value, value),
	}
}

// ReadEnvEffort reads the CLAUDE_CODE_EFFORT_LEVEL environment variable.
// Returns empty string when the variable is unset or explicitly set to "".
func ReadEnvEffort() string {
	return strings.TrimSpace(os.Getenv(envEffortKey))
}

// ReadEnvModel reads CLAUDE_MODEL for automatic model detection.
func ReadEnvModel() string {
	return strings.TrimSpace(os.Getenv(modelEnvKey))
}

// resolveModel returns the effective model, falling back to the default.
func resolveModel(model string) string {
	if model != "" {
		return model
	}
	if env := ReadEnvModel(); env != "" {
		return env
	}
	return defaultModel
}

// resolveQualityMode maps quality + complexity + model to an effort value.
func resolveQualityMode(quality, complexity, model string) (EffortResult, error) {
	switch strings.ToLower(quality) {
	case "ultra":
		return resolveUltraMode(model)
	case "balanced":
		return resolveBalancedMode(complexity, model)
	default:
		return EffortResult{}, fmt.Errorf("unknown quality mode: %s; allowed: ultra|balanced", quality)
	}
}

// resolveUltraMode applies Ultra-quality effort mapping per spec R2.
// Haiku 4.5 → strip; current and max-capable legacy Opus/Fable models → max;
// other models → high.
func resolveUltraMode(model string) (EffortResult, error) {
	normalized := normalizeModelID(model)

	switch normalized {
	case "haiku-4-5":
		// Haiku 4.5 does not support effort — strip.
		return EffortResult{
			Effort: EffortStripped,
			Source: EffortSourceQualityMode,
			Model:  model,
			Reason: "effort_stripped_model=haiku-4-5",
		}, nil
	case "opus-5-5", "opus-5", "opus", "opus-4-8", "opus-4-7",
		"fable-5-1", "fable-5", "fable", "best":
		return EffortResult{
			Effort: EffortMax,
			Source: EffortSourceQualityMode,
			Model:  model,
			Reason: "ultra mode with max-capable model",
		}, nil
	default:
		// Opus 4.6, Sonnet models, and unknown models use the safe high fallback.
		return EffortResult{
			Effort: EffortHigh,
			Source: EffortSourceQualityMode,
			Model:  model,
			Reason: "fallback=max→high, reason=model_tier_not_max_capable",
		}, nil
	}
}

// resolveBalancedMode applies model-tier effort mapping in Balanced mode.
func resolveBalancedMode(complexity, model string) (EffortResult, error) {
	normalized := normalizeModelID(model)
	switch normalized {
	case "haiku-4-5":
		return EffortResult{
			Effort: EffortStripped,
			Source: EffortSourceQualityMode,
			Model:  model,
			Reason: "effort_stripped_model=haiku-4-5",
		}, nil
	case "fable-5-1", "fable-5", "fable", "best":
		return EffortResult{
			Effort: EffortMax,
			Source: EffortSourceQualityMode,
			Model:  model,
			Reason: "balanced mode with fable tier",
		}, nil
	case "opus-5-5", "opus-5", "opus", "opus-4-8", "opus-4-7", "opus-4-6":
		return EffortResult{
			Effort: EffortHigh,
			Source: EffortSourceQualityMode,
			Model:  model,
			Reason: "balanced mode with opus tier",
		}, nil
	case "sonnet-5-5", "sonnet-5", "sonnet", "sonnet-4-6":
		return EffortResult{
			Effort: EffortMedium,
			Source: EffortSourceQualityMode,
			Model:  model,
			Reason: "balanced mode with sonnet tier",
		}, nil
	}
	if strings.ToLower(complexity) == "high" {
		return EffortResult{
			Effort: EffortHigh,
			Source: EffortSourceQualityMode,
			Model:  model,
			Reason: "high_complexity_task",
		}, nil
	}
	return EffortResult{
		Effort: EffortMedium,
		Source: EffortSourceQualityMode,
		Model:  model,
		Reason: "balanced mode default",
	}, nil
}

// normalizeModelID converts a model identifier to a canonical slug for comparison.
// Examples: "opus-4.7" → "opus-4-7", "claude-opus-4.7" → "opus-4-7".
func normalizeModelID(model string) string {
	s := strings.ToLower(model)
	// Strip common vendor prefixes.
	s = strings.TrimPrefix(s, "claude-")
	// Replace dots and underscores with dashes.
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}
