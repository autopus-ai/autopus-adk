// Package cli implements the effort command types and constants.
package cli

// EffortValue represents a valid effort enum value.
type EffortValue string

const (
	// EffortLow is the lowest effort tier.
	EffortLow EffortValue = "low"
	// EffortMedium is the Balanced Sonnet-tier default.
	EffortMedium EffortValue = "medium"
	// EffortHigh is used by Balanced Opus and Ultra Sonnet tiers.
	EffortHigh EffortValue = "high"
	// EffortXHigh is a very high effort tier below max.
	EffortXHigh EffortValue = "xhigh"
	// EffortMax is the strongest Claude Code tier for Fable and max-capable Opus models.
	EffortMax EffortValue = "max"
	// EffortStripped signals that effort must be omitted (Haiku 4.5; Haiku 5.5 takes effort).
	EffortStripped EffortValue = ""
)

// EffortSource identifies which configuration layer determined the effort value.
type EffortSource string

const (
	// EffortSourceEnv means the value came from CLAUDE_CODE_EFFORT_LEVEL env var.
	EffortSourceEnv EffortSource = "env"
	// EffortSourceFlag means the value came from the --effort CLI flag.
	EffortSourceFlag EffortSource = "flag"
	// EffortSourceFrontmatter means the value came from agent frontmatter.
	EffortSourceFrontmatter EffortSource = "frontmatter"
	// EffortSourceQualityMode means the value was derived from quality-mode mapping.
	EffortSourceQualityMode EffortSource = "quality_mode"
	// EffortSourceSettingsDefault means the value came from settings.json default.
	EffortSourceSettingsDefault EffortSource = "settings_default"
)

// EffortResult holds the resolved effort value and its provenance.
type EffortResult struct {
	// Effort is the resolved value ("" means stripped).
	Effort EffortValue `json:"effort"`
	// Source is the configuration layer that supplied the value.
	Source EffortSource `json:"source"`
	// Model is the detected or specified model identifier.
	Model string `json:"model"`
	// Reason is a human-readable explanation for the resolved value.
	Reason string `json:"reason,omitempty"`
}

// validEffortValues lists all accepted effort enum values (non-empty).
var validEffortValues = map[EffortValue]bool{
	EffortLow:    true,
	EffortMedium: true,
	EffortHigh:   true,
	EffortXHigh:  true,
	EffortMax:    true,
}

// isValidEffort returns true if v is a recognised non-empty effort value.
func isValidEffort(v EffortValue) bool {
	return validEffortValues[v]
}
