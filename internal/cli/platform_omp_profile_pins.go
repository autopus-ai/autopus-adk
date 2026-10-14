package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The autopus-mode OMP tab saves a pins-only policy (SPEC-MODELUX-003
// REQ-MUX3-22, 23): role_model_policy.agents without a profile. This file
// plans and applies it through the profile path's snapshot, catalog,
// preview, write, and activation steps, and converts a profile-based policy
// into it.

// ompProfileSourcePins marks a proposal whose agents are all operator pins.
const ompProfileSourcePins = "pins"

// newOMPPinsOnlyApplyOptions validates a pins-only target before any config
// is prepared: one canonical agent or task per key, and candidates that each
// name a model, a thinking level, and the family it was attested with.
func newOMPPinsOnlyApplyOptions(pins map[string]config.RoleAgentOverrideConf) (ompProfileApplyOptions, error) {
	target := make(map[string]config.RoleAgentOverrideConf, len(pins))
	for agent, override := range pins {
		if !slices.Contains(config.OMPAutopusAgentNames(), agent) {
			return ompProfileApplyOptions{}, fmt.Errorf("agent_override_unknown_agent: %s", safeOMPOperatorToken(agent))
		}
		if len(override.Candidates) == 0 {
			return ompProfileApplyOptions{}, fmt.Errorf("agent_override_malformed: %s has no model", agent)
		}
		for _, candidate := range override.Candidates {
			if !safeOMPProfileSelector(candidate.Selector) || !config.IsOMPNativeThinkingLevel(candidate.Thinking) ||
				candidate.Family == "" || safeOMPOperatorToken(candidate.Family) != candidate.Family {
				return ompProfileApplyOptions{}, fmt.Errorf("agent_override_malformed: %s", safeOMPOperatorToken(candidate.Selector))
			}
		}
		target[agent] = config.RoleAgentOverrideConf{Candidates: append([]config.RoleModelCandidateConf(nil), override.Candidates...)}
	}
	return ompProfileApplyOptions{name: config.PinsOnlyRoleModelProfileName, pinsOnly: true, pins: target}, nil
}

// resolveOMPPinsOnlyProposal prepares the pins-only policy in cfg and plans
// it. A pin is attested by what the strict catalog reports, or on a catalog
// without metadata only by the built-in declarations: no pin, existing or new,
// vouches for itself (security review P1), so one nothing attests is a
// blocker naming the agent and model.
func resolveOMPPinsOnlyProposal(
	ctx context.Context,
	cfg *config.HarnessConfig,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
) (ompProfileResolution, error) {
	declared := ompBuiltinDeclaredFamilies()
	effective, err := prepareOMPPinsOnlyConfig(cfg, opts)
	if err != nil || effective.empty {
		return ompProfileResolution{effective: effective}, err
	}
	probe, err := probeInstalledOMPCatalogForProfile(ctx, runner, effective.profile)
	var unavailable ompCatalogUnavailableError
	if errors.As(err, &unavailable) && (unavailable.reason == "pin_model_unattested" || unavailable.reason == "pin_family_mismatch") {
		blockers := ompPinsOnlyAttestationBlockers(effective.profile, omp.OMPModelCatalog{}, declared)
		return ompProfileResolution{effective: effective, probe: probe, blockers: blockers}, nil
	}
	if err != nil {
		return ompProfileResolution{}, err
	}
	if err := validateOMPProfilePolicy(effective.name, effective.profile); err != nil {
		return ompProfileResolution{}, err
	}
	agents, blockers := buildOMPProfileAgentPreview(effective, probe.Catalog)
	blockers = append(blockers, ompPinsOnlyAttestationBlockers(effective.profile, probe.Catalog, declared)...)
	return ompProfileResolution{effective: effective, probe: probe, agents: agents, blockers: blockers}, nil
}

// prepareOMPPinsOnlyConfig replaces the policy's profile, family, and agents
// with opts' pins. Without a pin left, the policy keeps only the fields that
// still mean something (config_mode, agent_source, profile definitions), or
// goes away entirely.
func prepareOMPPinsOnlyConfig(cfg *config.HarnessConfig, opts ompProfileApplyOptions) (ompProfileEffectiveConfig, error) {
	policy := &cfg.RoleModelPolicy
	policy.Profile, policy.Family, policy.Agents = "", "", nil
	for agent, override := range opts.pins {
		if policy.Agents == nil {
			policy.Agents = make(map[string]config.RoleAgentOverrideConf, len(opts.pins))
		}
		policy.Agents[agent] = config.RoleAgentOverrideConf{Candidates: append([]config.RoleModelCandidateConf(nil), override.Candidates...)}
	}
	if len(policy.Agents) == 0 && policy.ConfigMode == "" && policy.AgentSource == "" && len(policy.Profiles) == 0 {
		*policy = config.RoleModelPolicyConf{}
	} else if policy.Version == "" {
		policy.Version = config.RoleModelPolicyVersionV1
	}
	if err := cfg.Validate(); err != nil {
		return ompProfileEffectiveConfig{}, fmt.Errorf("omp_profile_validation_failed: %w", err)
	}
	effective := ompProfileEffectiveConfig{name: config.PinsOnlyRoleModelProfileName, source: ompProfileSourcePins, pinsOnly: true}
	if len(policy.Agents) == 0 {
		effective.empty = true
		return effective, nil
	}
	_, profile, ok := policy.SelectedRoleModelProfileForQuality(cfg.Quality)
	if !ok {
		return ompProfileEffectiveConfig{}, ompProfilePlanError{reason: "profile_unresolved:" + config.PinsOnlyRoleModelProfileName}
	}
	effective.profile, effective.overrides = profile, ompProfileOverriddenAgents(policy)
	return effective, nil
}

// ompPinsOnlyAttestationBlockers names every pin whose family neither the
// strict catalog reports nor a declaration Autopus owns attests.
func ompPinsOnlyAttestationBlockers(
	profile config.RoleModelProfileConf,
	catalog omp.OMPModelCatalog,
	declared map[string]string,
) []string {
	var blockers []string
	for _, agent := range sortedOMPProfileOverriddenAgents(ompProfileOverriddenAgentsOf(profile.Agents)) {
		for _, candidate := range profile.Agents[agent].Candidates {
			attested := declared[candidate.Selector] == candidate.Family
			for _, model := range catalog.Models {
				if model.Provider+"/"+model.Model == candidate.Selector && !model.OperatorAttested {
					attested = model.Family == candidate.Family
				}
			}
			if !attested {
				blockers = append(blockers, ompProfileBlocker(agent, "agent_override_model_undeclared:"+safeOMPOperatorToken(candidate.Selector)))
			}
		}
	}
	return blockers
}

func ompProfileOverriddenAgentsOf(agents map[string]config.RoleAgentOverrideConf) map[string]struct{} {
	return ompProfileOverriddenAgents(&config.RoleModelPolicyConf{Agents: agents})
}

// ompAgentRoutes holds one candidate list per agent and task. An empty list
// is the OMP session model, or for task, following planner.
type ompAgentRoutes map[string][]config.RoleModelCandidateConf

// ompPinsOnlyTarget is the agents map an OMP tab edit saves (REQ-MUX3-23):
// every canonical agent with a route in final, unchanged ones carrying their
// start route, which for a profile is that route's candidates in full. task
// takes the operator's own choice when there is one. Otherwise a profile's
// task route stays only where it differs from planner's final route, and a
// pins-only start keeps a task pin it had; task follows planner otherwise.
func ompPinsOnlyTarget(
	start, final ompAgentRoutes,
	fromProfile, startOwnTask bool,
	taskEdit *[]config.RoleModelCandidateConf,
) map[string]config.RoleAgentOverrideConf {
	target := make(map[string]config.RoleAgentOverrideConf)
	for _, agent := range config.CanonicalAgentNames() {
		if candidates := final[agent]; len(candidates) > 0 {
			target[agent] = config.RoleAgentOverrideConf{Candidates: append([]config.RoleModelCandidateConf(nil), candidates...)}
		}
	}
	var task []config.RoleModelCandidateConf
	switch {
	case taskEdit != nil:
		task = *taskEdit
	case fromProfile:
		if !slices.Equal(start[config.OMPAutopusDefaultAgent], final["planner"]) {
			task = start[config.OMPAutopusDefaultAgent]
		}
	case startOwnTask:
		task = start[config.OMPAutopusDefaultAgent]
	}
	if len(task) > 0 {
		target[config.OMPAutopusDefaultAgent] = config.RoleAgentOverrideConf{Candidates: append([]config.RoleModelCandidateConf(nil), task...)}
	}
	return target
}
