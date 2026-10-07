package orchestra

import (
	"fmt"
	"regexp"
	"strings"
)

// validProviderName matches safe provider names (alphanumeric, hyphens, underscores).
var validProviderName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// sanitizeProviderName returns a safe provider name for use in file paths.
// Rejects names containing path separators or special characters.
func sanitizeProviderName(name string) string {
	if validProviderName.MatchString(name) {
		return name
	}
	// Strip everything except safe chars
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	if sb.Len() == 0 {
		return "unknown"
	}
	return sb.String()
}

func validateSafeArtifactName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s is empty", kind)
	}
	if name != sanitizeProviderName(name) {
		return fmt.Errorf("%s %q is unsafe", kind, name)
	}
	return nil
}

func providerCanonicalName(name string) string {
	return strings.ToLower(providerArtifactIdentity(strings.TrimSpace(name)))
}

func validateProviderConfigs(providers []ProviderConfig) error {
	rawNames := make(map[string]int, len(providers))
	canonicalNames := make(map[string]string, len(providers))
	for index, provider := range providers {
		kind := fmt.Sprintf("provider[%d] name", index)
		if err := validateSafeArtifactName(kind, provider.Name); err != nil {
			return err
		}
		if first, exists := rawNames[provider.Name]; exists {
			return fmt.Errorf("duplicate raw provider name %q at indexes %d and %d", provider.Name, first, index)
		}
		rawNames[provider.Name] = index

		canonical := providerCanonicalName(provider.Name)
		if first, exists := canonicalNames[canonical]; exists {
			return fmt.Errorf("duplicate canonical provider name %q conflicts with %q", provider.Name, first)
		}
		canonicalNames[canonical] = provider.Name
	}
	return nil
}

func validateOrchestraProviderConfig(cfg OrchestraConfig) error {
	if err := validateProviderConfigs(cfg.Providers); err != nil {
		return err
	}
	if cfg.JudgeProvider != "" {
		if err := validateSafeArtifactName("judge provider name", cfg.JudgeProvider); err != nil {
			return err
		}
	}
	if cfg.JudgeConfig != nil && cfg.JudgeConfig.Name != "" {
		if err := validateSafeArtifactName("judge config name", cfg.JudgeConfig.Name); err != nil {
			return err
		}
	}
	return nil
}

func validateSubprocessPipelineProviders(providers []ProviderConfig, judge ProviderConfig) error {
	if err := validateProviderConfigs(providers); err != nil {
		return err
	}
	return validateSafeArtifactName("judge provider name", judge.Name)
}
