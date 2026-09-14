package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

type ompProfileCandidatePayload struct {
	Selector string `json:"selector" yaml:"selector"`
	Thinking string `json:"thinking" yaml:"thinking"`
	Family   string `json:"family" yaml:"family"`
}

type ompProfileCapabilityPayload struct {
	Capability string                       `json:"capability" yaml:"capability"`
	Required   bool                         `json:"required" yaml:"required"`
	Candidates []ompProfileCandidatePayload `json:"candidates" yaml:"candidates"`
}
type ompProfileFamilyDiversityPayload struct {
	Enabled bool     `json:"enabled" yaml:"enabled"`
	Roles   []string `json:"roles" yaml:"roles"`
}

type ompProfilePlanPayload struct {
	Platform           string                           `json:"platform" yaml:"platform"`
	Name               string                           `json:"name" yaml:"name"`
	Mode               string                           `json:"mode" yaml:"mode"`
	Writes             []string                         `json:"writes" yaml:"writes"`
	CatalogVersion     string                           `json:"catalog_version" yaml:"catalog_version"`
	CatalogFingerprint string                           `json:"catalog_fingerprint" yaml:"catalog_fingerprint"`
	ConfigMode         string                           `json:"config_mode" yaml:"config_mode"`
	Capabilities       []ompProfileCapabilityPayload    `json:"capabilities" yaml:"capabilities"`
	FamilyDiversity    ompProfileFamilyDiversityPayload `json:"family_diversity" yaml:"family_diversity"`
}

type ompProfileProposal struct {
	name         string
	profile      config.RoleModelProfileConf
	capabilities []ompProfileCapabilityPayload
}

type ompProfilePlanError struct {
	reason string
}

func (e ompProfilePlanError) Error() string {
	return "OMP profile plan invalid: " + e.reason
}

func runOMPProfileInitCommand(
	cmd *cobra.Command,
	_ string,
	name string,
	plan bool,
	jsonMode bool,
	runner omp.OMPModelCatalogRunner,
) error {
	if !config.IsValidQualityPresetName(name) {
		return writeOMPProfilePlanFailure(cmd, jsonMode, ompProfilePlanError{reason: "profile_name_invalid"})
	}
	probe, err := probeInstalledOMPCatalog(cmd.Context(), runner)
	if err != nil {
		return writeOMPProfilePlanFailure(cmd, jsonMode, err)
	}
	proposal, err := buildOMPProfileProposal(name, probe.Catalog)
	if err != nil {
		return writeOMPProfilePlanFailure(cmd, jsonMode, err)
	}
	mode := "proposal"
	if plan {
		mode = "plan"
	}
	payload := profilePlanPayload(proposal, probe, mode)
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, payload, nil, []jsonCheck{{
			ID: "omp.profile.plan", Severity: "info", Status: "pass", Detail: "zero_writes",
		}})
	}
	return renderOMPProfilePlanText(cmd, payload, proposal.profile)
}

func buildOMPProfileProposal(name string, catalog omp.OMPModelCatalog) (ompProfileProposal, error) {
	if !config.IsValidQualityPresetName(name) {
		return ompProfileProposal{}, ompProfilePlanError{reason: "profile_name_invalid"}
	}
	profile := config.RoleModelProfileConf{
		ConfigMode:   config.RoleModelConfigModeOverlay,
		Capabilities: make(map[string]config.RoleCapabilityRouteConf),
		FamilyDiversity: config.FamilyDiversityPolicyConf{
			Enabled: true,
			Roles: []string{
				config.OMPAgentRoleName("reviewer"),
				config.OMPAgentRoleName("security-auditor"),
			},
		},
	}
	rows := make([]ompProfileCapabilityPayload, 0, len(config.OMPProviderNeutralCapabilities()))
	for _, capability := range config.OMPProviderNeutralCapabilities() {
		route, row := proposeOMPCapabilityRoute(capability, catalog.Models)
		if len(route.Candidates) == 0 {
			return ompProfileProposal{}, ompProfilePlanError{reason: "capability_unavailable:" + capability}
		}
		profile.Capabilities[capability] = route
		rows = append(rows, row)
	}
	policy := config.RoleModelPolicyConf{
		Version:  config.RoleModelPolicyVersionV1,
		Profile:  name,
		Profiles: map[string]config.RoleModelProfileConf{name: profile},
	}
	if err := policy.Validate(); err != nil {
		return ompProfileProposal{}, ompProfilePlanError{reason: "profile_validation_failed"}
	}
	return ompProfileProposal{name: name, profile: profile, capabilities: rows}, nil
}

func proposeOMPCapabilityRoute(
	capability string,
	models []omp.OMPModelMetadata,
) (config.RoleCapabilityRouteConf, ompProfileCapabilityPayload) {
	models = append([]omp.OMPModelMetadata(nil), models...)
	sort.Slice(models, func(i, j int) bool {
		left := models[i].Provider + "/" + models[i].Model
		right := models[j].Provider + "/" + models[j].Model
		return left < right
	})
	route := config.RoleCapabilityRouteConf{Required: true}
	row := ompProfileCapabilityPayload{Capability: capability, Required: true}
	for _, model := range models {
		if ompModelAvailability(model) != "available" || !containsOMPString(model.Capabilities, capability) {
			continue
		}
		thinking := preferredOMPThinking(capability, model.Thinking)
		if thinking == "" {
			continue
		}
		candidate := config.RoleModelCandidateConf{
			Selector: model.Provider + "/" + model.Model,
			Thinking: thinking,
			Family:   model.Family,
		}
		route.Candidates = append(route.Candidates, candidate)
		row.Candidates = append(row.Candidates, ompProfileCandidatePayload(candidate))
	}
	if row.Candidates == nil {
		row.Candidates = []ompProfileCandidatePayload{}
	}
	return route, row
}

func preferredOMPThinking(capability string, supported []string) string {
	preferences := map[string][]string{
		config.CapabilityDeepReasoning:          {"xhigh", "max", "high", "medium", "auto", "low", "minimal", "none", "off"},
		config.CapabilityCodingToolUse:          {"high", "xhigh", "medium", "auto", "low", "minimal", "none", "off", "max"},
		config.CapabilityFastValidation:         {"low", "minimal", "medium", "none", "off", "auto", "high", "xhigh", "max"},
		config.CapabilityVisionDesign:           {"high", "xhigh", "medium", "auto", "low", "minimal", "none", "off", "max"},
		config.CapabilityIndependentDissent:     {"high", "xhigh", "medium", "auto", "low", "minimal", "none", "off", "max"},
		config.CapabilityDeterministicTransform: {"low", "minimal", "medium", "none", "off", "auto", "high", "xhigh", "max"},
	}
	for _, candidate := range preferences[capability] {
		if containsOMPString(supported, candidate) {
			return candidate
		}
	}
	return ""
}

func containsOMPString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func profilePlanPayload(
	proposal ompProfileProposal,
	probe omp.OMPModelCatalogProbeResult,
	mode string,
) ompProfilePlanPayload {
	return ompProfilePlanPayload{
		Platform: "omp", Name: proposal.name, Mode: mode, Writes: []string{},
		CatalogVersion:     safeOMPOperatorVersion(probe.Version),
		CatalogFingerprint: probe.Catalog.Fingerprint,
		ConfigMode:         proposal.profile.ConfigMode,
		Capabilities:       append([]ompProfileCapabilityPayload(nil), proposal.capabilities...),
		FamilyDiversity: ompProfileFamilyDiversityPayload{
			Enabled: proposal.profile.FamilyDiversity.Enabled,
			Roles:   append([]string(nil), proposal.profile.FamilyDiversity.Roles...),
		},
	}
}

func writeOMPProfilePlanFailure(cmd *cobra.Command, jsonMode bool, err error) error {
	if !jsonMode {
		// The caller may silence cobra's own error rendering, and an operator
		// who cannot read which entry to delete has nothing to act on.
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "OMP profile plan refused: %s\n", err)
		return err
	}
	return writeJSONResultAndExit(
		cmd, jsonStatusError, err, "omp_profile_invalid", map[string]any{"platform": "omp"},
		[]jsonMessage{{Code: "omp_profile_invalid", Message: err.Error()}}, nil,
	)
}

func renderOMPProfilePlanText(
	cmd *cobra.Command,
	payload ompProfilePlanPayload,
	profile config.RoleModelProfileConf,
) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "OMP model profile proposal")
	_, _ = fmt.Fprintf(out, "Name: %s\nMode: %s\nWrites: 0\n", payload.Name, payload.Mode)
	_, _ = fmt.Fprintf(out, "Catalog: %s %s\n", payload.CatalogVersion, payload.CatalogFingerprint)
	policy := config.RoleModelPolicyConf{
		Version:  config.RoleModelPolicyVersionV1,
		Profile:  payload.Name,
		Profiles: map[string]config.RoleModelProfileConf{payload.Name: profile},
	}
	encoded, err := yaml.Marshal(map[string]any{"role_model_policy": policy})
	if err != nil {
		return fmt.Errorf("marshal OMP profile proposal: %w", err)
	}
	_, _ = fmt.Fprint(out, strings.TrimSpace(string(encoded))+"\n")
	_, _ = fmt.Fprintf(out, "Next: auto platform omp profile apply %s\n", payload.Name)
	return nil
}
