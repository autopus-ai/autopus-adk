package config

import "strings"

const (
	CodexAstraModel  = "gpt-6-astra"
	CodexSolModel    = "gpt-5.6-sol"
	CodexTerraModel  = "gpt-5.6-terra"
	CodexLunaModel   = "gpt-5.6-luna"
	CodexLegacyModel = "gpt-5.5"

	CodexEffortLow    = "low"
	CodexEffortMedium = "medium"
	CodexEffortHigh   = "high"
	CodexEffortXHigh  = "xhigh"
	CodexEffortMax    = "max"
	CodexEffortUltra  = "ultra"
)

const (
	CodexResolutionSupported         CodexResolutionReason = "supported"
	CodexResolutionCatalogUnknown    CodexResolutionReason = "catalog_unknown"
	CodexResolutionModelUnavailable  CodexResolutionReason = "model_unavailable"
	CodexResolutionEffortUnavailable CodexResolutionReason = "effort_unavailable"
	CodexResolutionRuntimeDefault    CodexResolutionReason = "runtime_default"
)

var codexEffortOrder = []string{CodexEffortLow, CodexEffortMedium, CodexEffortHigh, CodexEffortXHigh, CodexEffortMax, CodexEffortUltra}

type CodexProfile struct {
	Model, Effort string
}

type CodexResolutionReason string

type CodexProfileResolution struct {
	Requested, Effective CodexProfile
	Fallback             bool
	Reason               CodexResolutionReason
	CatalogError         error
}

type CodexModelCatalog struct {
	Models []CodexCatalogModel `json:"models"`
}

// CodexCatalogModel describes one model and its supported reasoning levels.
type CodexCatalogModel struct {
	Slug                     string                       `json:"slug"`
	DefaultReasoningLevel    string                       `json:"default_reasoning_level"`
	SupportedReasoningLevels []CodexCatalogReasoningLevel `json:"supported_reasoning_levels"`
}

type CodexCatalogReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description,omitempty"`
}

// CodexSupervisorProfile returns the managed root Codex profile for this quality mode.
func (q QualityConf) CodexSupervisorProfile() CodexProfile {
	if q.codexQualityMode() == "ultra" {
		return CodexProfile{Model: CodexAstraModel, Effort: CodexEffortUltra}
	}
	return CodexProfile{Model: CodexAstraModel, Effort: CodexEffortXHigh}
}

func (q QualityConf) CodexSupervisorModel() string { return q.CodexSupervisorProfile().Model }

func (q QualityConf) CodexSupervisorEffort() string { return q.CodexSupervisorProfile().Effort }

// CodexOrchestraProfile returns the managed Codex subprocess profile. Both
// quality modes run the anchor model at max: the subprocess never auto-
// delegates, so ultra's delegation effort has nothing to drive, and balanced's
// xhigh left the orchestra reasoning below the agents it coordinates.
func (q QualityConf) CodexOrchestraProfile() CodexProfile {
	return CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}
}

func (q QualityConf) CodexOrchestraModel() string { return q.CodexOrchestraProfile().Model }

func (q QualityConf) CodexOrchestraEffort() string { return q.CodexOrchestraProfile().Effort }

// CodexAgentProfile maps an agent onto its managed Codex model and effort. A
// canonical agent under the standard native balanced placement takes both from
// the shared role matrix; everyone else keeps the relative tier ladder that
// Ultra, custom presets, and non-canonical agents resolve through.
func (q QualityConf) CodexAgentProfile(agentName, fallbackTier, declaredEffort string) CodexProfile {
	if profile, ok := q.nativeBalancedCodexProfile(agentName); ok {
		return profile
	}
	tier := q.codexAgentTier(agentName, fallbackTier)
	if q.codexQualityMode() == "ultra" && tier != "fable" && tier != "opus" {
		tier = "opus"
	}

	switch tier {
	case "fable":
		return CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}
	case "opus":
		return CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}
	default:
		return CodexProfile{Model: CodexModelForTier(tier), Effort: normalizeManagedCodexEffort(declaredEffort)}
	}
}

// nativeBalancedCodexProfile projects the shared native balanced candidate for
// one agent onto Codex's own vocabulary. The candidate selector carries its
// provider prefix ("openai-codex/gpt-6-astra") while Codex names the model
// alone. A candidate whose selector or thinking level does not translate is
// declined rather than approximated, so a malformed matrix entry falls back to
// the tier ladder instead of emitting an unusable model.
func (q QualityConf) nativeBalancedCodexProfile(agentName string) (CodexProfile, bool) {
	candidate, ok := q.NativeBalancedAgentCandidate(QualityProviderCodex, agentName)
	if !ok {
		return CodexProfile{}, false
	}
	model := candidate.Selector
	if _, bare, split := strings.Cut(model, "/"); split {
		model = bare
	}
	effort := strings.ToLower(strings.TrimSpace(candidate.Thinking))
	if model == "" || codexEffortRank(effort) < 0 {
		return CodexProfile{}, false
	}
	return CodexProfile{Model: model, Effort: normalizeManagedCodexEffort(effort)}, true
}

func (q QualityConf) CodexAgentModel(agentName, fallbackTier string) string {
	return q.CodexAgentProfile(agentName, fallbackTier, CodexEffortMedium).Model
}

func (q QualityConf) CodexAgentEffort(agentName, fallbackTier, declaredEffort string) string {
	return q.CodexAgentProfile(agentName, fallbackTier, declaredEffort).Effort
}

func (q QualityConf) codexQualityMode() string {
	if q.ForProvider(QualityProviderCodex).Default == "ultra" {
		return "ultra"
	}
	return "balanced"
}

func (q QualityConf) codexAgentTier(agentName, fallbackTier string) string {
	return q.AgentTier(QualityProviderCodex, agentName, fallbackTier)
}

// CodexModelForTier maps a relative tier onto its managed Codex model.
func CodexModelForTier(tier string) string {
	switch tier {
	case "fable":
		return CodexAstraModel
	case "opus":
		return CodexSolModel
	case "haiku":
		return CodexLunaModel
	default:
		return CodexTerraModel
	}
}

func normalizeCodexEffort(effort string) string {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if codexEffortRank(effort) >= 0 {
		return effort
	}
	return CodexEffortMedium
}

func normalizeManagedCodexEffort(effort string) string {
	effort = normalizeCodexEffort(effort)
	if effort == CodexEffortUltra {
		return CodexEffortMax
	}
	return effort
}
