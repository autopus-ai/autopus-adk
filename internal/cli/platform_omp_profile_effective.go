package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	ompProfileSourceBuiltin   = "builtin"
	ompProfileSourceCustom    = "custom"
	ompProfileSourceGenerated = "generated"
	ompProfileSourceAgent     = "agent_override"
)

// ompProfileEffectiveConfig is the prepared candidate config plus the profile
// it resolves to. Built-in profiles are never materialized under
// role_model_policy.profiles: only the concise Profile, Family, and Agents
// fields are persisted, and the profile itself is re-derived on every read.
type ompProfileEffectiveConfig struct {
	name       string
	profile    config.RoleModelProfileConf
	source     string
	family     string
	definition bool
	overrides  map[string]struct{}
}

// ompProfileResolution is the single proposal both --plan and apply consume.
type ompProfileResolution struct {
	effective ompProfileEffectiveConfig
	probe     omp.OMPModelCatalogProbeResult
	agents    []ompProfileAgentPreviewPayload
	blockers  []string
}

// resolveOMPProfileProposal prepares the effective proposal and its routing
// diagnostics from the installed OMP metadata catalog only. It never infers a
// provider over the network, runs no model task, and writes nothing.
func resolveOMPProfileProposal(
	ctx context.Context,
	cfg *config.HarnessConfig,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
) (ompProfileResolution, error) {
	if !config.IsValidQualityPresetName(opts.name) {
		return ompProfileResolution{}, ompProfilePlanError{reason: "profile_name_invalid"}
	}
	seed, err := seedOMPProfileCatalog(ctx, cfg, opts, runner)
	if err != nil {
		return ompProfileResolution{}, err
	}
	effective, err := prepareOMPProfileConfig(cfg, opts, seed)
	if err != nil {
		return ompProfileResolution{}, err
	}
	probe, err := probeInstalledOMPCatalogForProfile(ctx, runner, effective.profile)
	if err != nil {
		return ompProfileResolution{}, err
	}
	if err := validateOMPProfilePolicy(effective.name, effective.profile); err != nil {
		return ompProfileResolution{}, err
	}
	agents, blockers := buildOMPProfileAgentPreview(effective, probe.Catalog)
	return ompProfileResolution{
		effective: effective, probe: probe, agents: agents, blockers: blockers,
	}, nil
}

// seedOMPProfileCatalog probes the strict installed catalog only when an
// unknown profile name still has to be synthesized from the installed models.
// Built-in and explicitly defined profiles need no seed: an installed native
// OMP catalog carries no family or capability metadata, so a strict probe there
// would fail closed on a run that has no need of observed semantics.
func seedOMPProfileCatalog(
	ctx context.Context,
	cfg *config.HarnessConfig,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
) (omp.OMPModelCatalog, error) {
	if _, custom := cfg.RoleModelPolicy.Profiles[opts.name]; custom {
		return omp.OMPModelCatalog{}, nil
	}
	if config.IsBuiltinRoleModelProfileName(opts.name) {
		return omp.OMPModelCatalog{}, nil
	}
	probe, err := probeInstalledOMPCatalog(ctx, runner)
	if err != nil {
		return omp.OMPModelCatalog{}, err
	}
	return probe.Catalog, nil
}

// prepareOMPProfileConfig applies the operator's selection to the candidate
// config in place and resolves the effective profile through the shared config
// selection path, so CLI preview and runtime generation agree by construction.
func prepareOMPProfileConfig(
	cfg *config.HarnessConfig,
	opts ompProfileApplyOptions,
	seed omp.OMPModelCatalog,
) (ompProfileEffectiveConfig, error) {
	if !config.IsValidQualityPresetName(opts.name) {
		return ompProfileEffectiveConfig{}, ompProfilePlanError{reason: "profile_name_invalid"}
	}
	policy := &cfg.RoleModelPolicy
	_, custom := policy.Profiles[opts.name]
	if err := rejectUnsupportedOMPFamilyFlag(opts, custom); err != nil {
		return ompProfileEffectiveConfig{}, err
	}
	if policy.Version == "" {
		policy.Version = config.RoleModelPolicyVersionV1
	}
	policy.Profile = opts.name
	if opts.family != "" {
		policy.Family = opts.family
	}
	declared := ompProfileDeclaredFamilies(*policy, cfg.Quality, opts.name)
	if err := applyOMPProfileAgentAssignments(policy, opts.agents, declared, seed); err != nil {
		return ompProfileEffectiveConfig{}, err
	}
	source, err := selectOMPProfileSource(policy, opts.name, custom, seed)
	if err != nil {
		return ompProfileEffectiveConfig{}, err
	}
	name, profile, ok := policy.SelectedRoleModelProfileForQuality(cfg.Quality)
	if !ok {
		return ompProfileEffectiveConfig{}, ompProfilePlanError{reason: "profile_unresolved:" + opts.name}
	}
	if err := cfg.Validate(); err != nil {
		return ompProfileEffectiveConfig{}, fmt.Errorf("omp_profile_validation_failed: %w", err)
	}
	_, definition := policy.Profiles[name]
	return ompProfileEffectiveConfig{
		name: name, profile: profile, source: source, family: policy.Family,
		definition: definition, overrides: ompProfileOverriddenAgents(policy),
	}, nil
}

// rejectUnsupportedOMPFamilyFlag fails closed instead of silently ignoring
// --family: the flag only anchors a built-in derivation, and an explicit
// profile definition of the same name always wins over the built-in.
func rejectUnsupportedOMPFamilyFlag(opts ompProfileApplyOptions, custom bool) error {
	if opts.family == "" {
		return nil
	}
	if custom {
		return ompProfilePlanError{reason: "family_flag_unsupported_for_explicit_profile:" + opts.name}
	}
	if !config.IsBuiltinRoleModelProfileName(opts.name) {
		return ompProfilePlanError{reason: "family_flag_requires_builtin_profile:" + opts.name}
	}
	return nil
}

// selectOMPProfileSource records where the selected profile comes from and
// synthesizes an unknown name from the seeded catalog.
func selectOMPProfileSource(
	policy *config.RoleModelPolicyConf,
	name string,
	custom bool,
	seed omp.OMPModelCatalog,
) (string, error) {
	switch {
	case custom:
		return ompProfileSourceCustom, nil
	case config.IsBuiltinRoleModelProfileName(name):
		return ompProfileSourceBuiltin, nil
	default:
		proposal, err := buildOMPProfileProposal(name, seed)
		if err != nil {
			return "", err
		}
		if policy.Profiles == nil {
			policy.Profiles = make(map[string]config.RoleModelProfileConf, 1)
		}
		policy.Profiles[name] = proposal.profile
		return ompProfileSourceGenerated, nil
	}
}

// applyOMPProfileAgentAssignments overlays the root-level per-agent policy. An
// inherit assignment removes the key so the selected profile's own route
// governs the agent again. A pin carries an attested family because an
// operator-attested profile may only declare candidates whose family is
// explicit, and an installed native OMP catalog reports no family at all.
func applyOMPProfileAgentAssignments(
	policy *config.RoleModelPolicyConf,
	assignments []ompProfileAgentAssignment,
	declared map[string]string,
	seed omp.OMPModelCatalog,
) error {
	for _, assignment := range assignments {
		if assignment.inherit {
			delete(policy.Agents, assignment.agent)
			continue
		}
		family, attested := resolveOMPProfilePinFamily(declared, seed, assignment.selector)
		if !attested {
			return ompProfilePlanError{
				reason: "agent_override_model_undeclared:" + safeOMPOperatorToken(assignment.selector),
			}
		}
		if policy.Agents == nil {
			policy.Agents = make(map[string]config.RoleAgentOverrideConf, len(assignments))
		}
		policy.Agents[assignment.agent] = config.RoleAgentOverrideConf{
			Candidates: []config.RoleModelCandidateConf{{
				Selector: assignment.selector, Thinking: assignment.thinking, Family: family,
			}},
		}
	}
	if len(policy.Agents) == 0 {
		policy.Agents = nil
	}
	return nil
}

func ompProfileOverriddenAgents(policy *config.RoleModelPolicyConf) map[string]struct{} {
	overrides := make(map[string]struct{}, len(policy.Agents))
	for agent, override := range policy.Agents {
		if len(override.Candidates) > 0 {
			overrides[agent] = struct{}{}
		}
	}
	return overrides
}

func sortedOMPProfileOverriddenAgents(overrides map[string]struct{}) []string {
	names := make([]string, 0, len(overrides))
	for agent := range overrides {
		names = append(names, agent)
	}
	sort.Strings(names)
	return names
}

// validateOMPProfilePolicy validates the resolved profile so a preview reports
// the same rejection apply would. The reason travels with the error: a
// native-agent conflict names the entries the operator has to reconcile, and
// replacing that with a bare code would leave them nothing to act on.
func validateOMPProfilePolicy(name string, profile config.RoleModelProfileConf) error {
	if err := config.ValidateResolvedRoleModelProfile(name, profile); err != nil {
		return fmt.Errorf("omp_profile_validation_failed: %w", err)
	}
	return nil
}
