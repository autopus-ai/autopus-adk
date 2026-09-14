package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	ompProfileAvailabilityAvailable   = "available"
	ompProfileAvailabilityUnavailable = "unavailable"
)

// ompProfileAgentPreviewPayload is one bundled native OMP agent's route as the
// operator would receive it: what was requested, what resolves, in which order
// the declared candidates were attempted, and why an attempt failed. Agent is
// always a native OMP name; PolicyKey, Role, and Capability record which
// logical role supplied the route.
type ompProfileAgentPreviewPayload struct {
	Agent             string                       `json:"agent" yaml:"agent"`
	PolicyKey         string                       `json:"policy_key" yaml:"policy_key"`
	Role              string                       `json:"role" yaml:"role"`
	Capability        string                       `json:"capability" yaml:"capability"`
	Source            string                       `json:"source" yaml:"source"`
	RequestedSelector string                       `json:"requested_selector" yaml:"requested_selector"`
	RequestedThinking string                       `json:"requested_thinking" yaml:"requested_thinking"`
	EffectiveSelector string                       `json:"effective_selector" yaml:"effective_selector"`
	EffectiveThinking string                       `json:"effective_thinking" yaml:"effective_thinking"`
	EffectiveFamily   string                       `json:"effective_family" yaml:"effective_family"`
	Availability      string                       `json:"availability" yaml:"availability"`
	Status            string                       `json:"status" yaml:"status"`
	Reason            string                       `json:"reason" yaml:"reason"`
	Candidates        []ompProfileCandidatePayload `json:"candidates" yaml:"candidates"`
	FallbackAttempts  []ompFallbackProjection      `json:"fallback_attempts" yaml:"fallback_attempts"`
}

// ompProfilePersistedPayload is exactly what apply would write: the concise
// root selection, never a materialized built-in profile definition.
type ompProfilePersistedPayload struct {
	Profile           string   `json:"profile" yaml:"profile"`
	Family            string   `json:"family" yaml:"family"`
	Agents            []string `json:"agents" yaml:"agents"`
	ProfileDefinition bool     `json:"profile_definition" yaml:"profile_definition"`
}

// ompProfileApplyPreviewPayload is the zero-write --plan result.
type ompProfileApplyPreviewPayload struct {
	Platform           string                           `json:"platform" yaml:"platform"`
	Name               string                           `json:"name" yaml:"name"`
	Mode               string                           `json:"mode" yaml:"mode"`
	Writes             []string                         `json:"writes" yaml:"writes"`
	Source             string                           `json:"source" yaml:"source"`
	FamilyRequested    string                           `json:"family_requested" yaml:"family_requested"`
	FamilyStored       string                           `json:"family_stored" yaml:"family_stored"`
	ConfigMode         string                           `json:"config_mode" yaml:"config_mode"`
	CatalogVersion     string                           `json:"catalog_version" yaml:"catalog_version"`
	CatalogFingerprint string                           `json:"catalog_fingerprint" yaml:"catalog_fingerprint"`
	FamilyDiversity    ompProfileFamilyDiversityPayload `json:"family_diversity" yaml:"family_diversity"`
	Persisted          ompProfilePersistedPayload       `json:"persisted" yaml:"persisted"`
	Agents             []ompProfileAgentPreviewPayload  `json:"agents" yaml:"agents"`
	Blockers           []string                         `json:"blockers" yaml:"blockers"`
}

type ompProfileUnavailableError struct {
	blockers []string
}

func (e ompProfileUnavailableError) Error() string {
	return "omp_profile_candidate_unavailable: " + strings.Join(e.blockers, "; ")
}

// buildOMPProfileAgentPreview projects one compiled routing resolution per
// bundled native OMP agent in registry order, together with the blockers that
// must stop an apply before it writes anything. OMP registers five agents, so
// several logical roles collapse onto one row; the policy owns that collapse
// and rejects two operator-written routes that land on the same agent.
func buildOMPProfileAgentPreview(
	effective ompProfileEffectiveConfig,
	catalog omp.OMPModelCatalog,
) ([]ompProfileAgentPreviewPayload, []string) {
	resolved := make(map[string]omp.OMPModelRouteResolution)
	for _, resolution := range compileOMPModelDoctorRouting(effective.profile, catalog).Resolutions {
		resolved[resolution.Agent] = resolution
	}
	natives := config.OMPNativeAgentNames()
	rows := make([]ompProfileAgentPreviewPayload, 0, len(natives))
	blockers := make([]string, 0)
	for _, native := range natives {
		policy, route, err := effective.profile.OMPNativeAgentRoute(native)
		if err != nil {
			blockers = append(blockers, err.Error())
			continue
		}
		row := newOMPProfileAgentPreviewRow(effective, native, policy, route)
		resolution, ok := resolved[native]
		if !ok {
			row.Reason = "route_missing"
			blockers = append(blockers, ompProfileBlocker(native, row.Reason))
			rows = append(rows, row)
			continue
		}
		applyOMPProfileResolutionToRow(&row, resolution)
		if row.Availability == ompProfileAvailabilityUnavailable {
			blockers = append(blockers, ompProfileBlocker(native, row.Reason))
		}
		rows = append(rows, row)
	}
	return rows, blockers
}

func newOMPProfileAgentPreviewRow(
	effective ompProfileEffectiveConfig,
	native string,
	policy config.OMPPolicyAgentRoute,
	route config.RoleCapabilityRouteConf,
) ompProfileAgentPreviewPayload {
	row := ompProfileAgentPreviewPayload{
		Agent: native, PolicyKey: policy.Key, Role: policy.Role, Capability: policy.Capability,
		Source: effective.source, Availability: ompProfileAvailabilityUnavailable, Status: "blocked",
		Candidates: []ompProfileCandidatePayload{}, FallbackAttempts: []ompFallbackProjection{},
	}
	if _, overridden := effective.overrides[policy.Key]; overridden {
		row.Source = ompProfileSourceAgent
	}
	for _, candidate := range route.Candidates {
		row.Candidates = append(row.Candidates, ompProfileCandidatePayload(candidate))
	}
	if len(row.Candidates) > 0 {
		row.RequestedSelector = row.Candidates[0].Selector
		row.RequestedThinking = row.Candidates[0].Thinking
	}
	return row
}

func applyOMPProfileResolutionToRow(
	row *ompProfileAgentPreviewPayload,
	resolution omp.OMPModelRouteResolution,
) {
	row.Status = safeOMPOperatorToken(resolution.Status)
	row.Reason = safeOMPOperatorReason(resolution.Reason)
	row.FallbackAttempts = projectOMPFallbackAttempts(resolution.FallbackAttempts)
	if resolution.Status != "selected" {
		return
	}
	row.Availability = ompProfileAvailabilityAvailable
	row.EffectiveSelector = resolution.EffectiveProvider + "/" + resolution.EffectiveModel
	row.EffectiveThinking = resolution.Thinking
	row.EffectiveFamily = resolution.EffectiveFamily
}

func ompProfileBlocker(agent, reason string) string {
	return fmt.Sprintf(
		"agent=%s reason=%s", safeOMPOperatorToken(agent), safeOMPOperatorReason(reason),
	)
}

func newOMPProfileApplyPreviewPayload(
	resolution ompProfileResolution,
	opts ompProfileApplyOptions,
) ompProfileApplyPreviewPayload {
	effective := resolution.effective
	return ompProfileApplyPreviewPayload{
		Platform: "omp", Name: effective.name, Mode: "plan", Writes: []string{},
		Source: effective.source, FamilyRequested: opts.family, FamilyStored: effective.family,
		ConfigMode:         effective.profile.ConfigMode,
		CatalogVersion:     safeOMPOperatorVersion(resolution.probe.Version),
		CatalogFingerprint: resolution.probe.Catalog.Fingerprint,
		FamilyDiversity: ompProfileFamilyDiversityPayload{
			Enabled: effective.profile.FamilyDiversity.Enabled,
			Roles:   append([]string(nil), effective.profile.FamilyDiversity.Roles...),
		},
		Persisted: ompProfilePersistedPayload{
			Profile: effective.name, Family: effective.family,
			Agents:            sortedOMPProfileOverriddenAgents(effective.overrides),
			ProfileDefinition: effective.definition,
		},
		Agents:   resolution.agents,
		Blockers: append([]string(nil), resolution.blockers...),
	}
}

// renderOMPProfileApplyPreviewText prints every agent route plus the exact
// keys apply would persist. Blockers are always displayed before the caller
// turns them into a nonzero exit.
func renderOMPProfileApplyPreviewText(cmd *cobra.Command, payload ompProfileApplyPreviewPayload) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "OMP profile plan: %s\n", payload.Name)
	_, _ = fmt.Fprintf(out, "Source: %s\nWrites: %d\n", payload.Source, len(payload.Writes))
	_, _ = fmt.Fprintf(
		out, "Family: requested=%s stored=%s\n",
		ompProfileDisplayValue(payload.FamilyRequested, "none"),
		ompProfileDisplayValue(payload.FamilyStored, "default"),
	)
	_, _ = fmt.Fprintf(
		out, "Config mode: %s\nFamily diversity: %t\nProfile definition: %t\n",
		ompProfileDisplayValue(payload.ConfigMode, config.RoleModelConfigModeOverlay),
		payload.FamilyDiversity.Enabled, payload.Persisted.ProfileDefinition,
	)
	_, _ = fmt.Fprintf(
		out, "Agent overrides: %s\n",
		ompProfileDisplayValue(strings.Join(payload.Persisted.Agents, ", "), "none"),
	)
	_, _ = fmt.Fprintf(out, "Catalog: %s %s\n", payload.CatalogVersion, payload.CatalogFingerprint)
	for _, row := range payload.Agents {
		renderOMPProfileAgentPreviewRow(out, row)
	}
	_, _ = fmt.Fprintf(
		out, "Blockers: %s\n", ompProfileDisplayValue(strings.Join(payload.Blockers, "; "), "none"),
	)
}

func renderOMPProfileAgentPreviewRow(out io.Writer, row ompProfileAgentPreviewPayload) {
	_, _ = fmt.Fprintf(
		out, "agent=%s policy_key=%s role=%s capability=%s source=%s availability=%s status=%s reason=%s\n",
		row.Agent, row.PolicyKey, row.Role, row.Capability,
		row.Source, row.Availability, row.Status, row.Reason,
	)
	selectors := make([]string, 0, len(row.Candidates))
	for _, candidate := range row.Candidates {
		selectors = append(selectors, candidate.Selector+":"+candidate.Thinking)
	}
	_, _ = fmt.Fprintf(
		out, "  candidates=%s effective=%s\n",
		ompProfileDisplayValue(strings.Join(selectors, ","), "none"),
		ompProfileDisplayValue(ompProfileEffectiveLabel(row), "none"),
	)
	for _, attempt := range row.FallbackAttempts {
		_, _ = fmt.Fprintf(
			out, "  attempt[%d]=%s status=%s reason=%s\n",
			attempt.Index, attempt.Selector, attempt.Status, attempt.Reason,
		)
	}
}

func ompProfileEffectiveLabel(row ompProfileAgentPreviewPayload) string {
	if row.EffectiveSelector == "" {
		return ""
	}
	return row.EffectiveSelector + ":" + row.EffectiveThinking
}

func ompProfileDisplayValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
