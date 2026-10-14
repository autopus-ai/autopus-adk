package config

import (
	"errors"
	"fmt"
)

// A pins-only policy pins agents without a profile (SPEC-MODELUX-003
// REQ-MUX3-22, Pins-only Policy Contract). It exists in autopus mode only:
// the OMP tab writes it when the operator picks a model per role group or
// agent from any provider. No capability route backs it, so only the pinned
// agents and task are routed, each pin carries the family it was attested
// with, and every other agent keeps the OMP session model.

// PinsOnlyRoleModelProfileName is the reserved name a pins-only policy
// resolves under in receipts and doctor output. It is never written to
// autopus.yaml, which holds no profile key for such a policy.
const PinsOnlyRoleModelProfileName = "pins-only"

// ErrOMPAgentUnrouted reports an agent a pins-only policy routes nowhere: it
// has no pin, so OMP runs it on the session model and no override names it.
var ErrOMPAgentUnrouted = errors.New("agent_unrouted")

// PinsOnlyRoleModelProfile is the profile a pins-only policy resolves to
// before its pins are laid over it: operator-attested, with no capability
// route, and claiming the managed keys a built-in profile claims in
// project-managed mode.
func PinsOnlyRoleModelProfile(configMode string) RoleModelProfileConf {
	mode, ok := effectiveBuiltinRoleModelConfigMode(configMode)
	if !ok {
		mode = configMode
	}
	return RoleModelProfileConf{
		ConfigMode: mode, CatalogTrust: RoleModelCatalogTrustOperatorAttested,
		ManagedKeys: builtinRoleModelManagedKeys(mode), PinsOnly: true,
	}
}

// validateReservedProfileName refuses a profile that takes the name a
// pins-only policy resolves under, which receipts and doctor output show.
func (c RoleModelPolicyConf) validateReservedProfileName() error {
	_, defined := c.Profiles[PinsOnlyRoleModelProfileName]
	if c.Profile == PinsOnlyRoleModelProfileName || defined {
		return fmt.Errorf("role_model_policy.profile_name_reserved: %q", PinsOnlyRoleModelProfileName)
	}
	return nil
}

// pinsOnly reports a policy that pins agents without naming a profile.
func (c RoleModelPolicyConf) pinsOnly() bool {
	return c.Profile == "" && len(c.Agents) > 0
}

// validatePinsOnlyAgentKeys allows a profile-less agents map in autopus mode
// only, and only under names that route there: the canonical agents and
// task. scout, security-reviewer, and sonic name bundled agents autopus mode
// disables; their pins only mean something folded onto a profile's roles.
func (c RoleModelPolicyConf) validatePinsOnlyAgentKeys() error {
	if c.OMPAgentSource() != OMPAgentSourceAutopus {
		// Native mode collapses roles onto bundled agents through a profile's
		// routes, so a pin needs one there.
		return fmt.Errorf("role_model_policy.agents_require_profile: agent_source: native routes agents " +
			"only on top of a profile; set role_model_policy.profile, or remove agent_source: native")
	}
	for _, agent := range sortedRoleAgentOverrides(c.Agents) {
		for _, disabled := range OMPAutopusDisabledAgents() {
			if agent == disabled {
				return fmt.Errorf("role_model_policy.agents_native_key_requires_profile: %s", agent)
			}
		}
	}
	return nil
}

// validatePinsOnlyRoleModelProfile checks a resolved pins-only profile: its
// config mode, operator trust, every pin's identity and candidates, the
// attestation every candidate needs, and the managed keys.
func validatePinsOnlyRoleModelProfile(name string, profile RoleModelProfileConf) error {
	if profile.ConfigMode != RoleModelConfigModeOverlay && profile.ConfigMode != RoleModelConfigModeProjectManaged {
		return fmt.Errorf("role_model_policy.profiles[%s].config_mode_invalid: %q", name, profile.ConfigMode)
	}
	if profile.EffectiveCatalogTrust() != RoleModelCatalogTrustOperatorAttested || len(profile.Capabilities) > 0 {
		return fmt.Errorf("role_model_policy.profiles[%s].pins_only_profile_invalid", name)
	}
	families := make(map[string]string)
	for _, agent := range sortedRoleAgentOverrides(profile.Agents) {
		if err := validateAgentOverride(name, agent, profile.Agents[agent]); err != nil {
			return err
		}
		scope := fmt.Sprintf("role_model_policy.agents[%s]", agent)
		if err := validateOperatorAttestedCandidates(scope, profile.Agents[agent].Candidates, families); err != nil {
			return err
		}
	}
	return validateRoleManagedKeys(name, profile)
}

// ompPinsOnlyAgentNames lists the agents a pins-only profile routes, in the
// order autopus mode serves them.
func (c RoleModelProfileConf) ompPinsOnlyAgentNames() []string {
	var names []string
	for _, agent := range OMPAutopusAgentNames() {
		if _, err := c.ompAutopusAgentRouteKey(agent); err == nil {
			names = append(names, agent)
		}
	}
	return names
}

// BuiltinRoleModelSelectorFamilies is the family of every model a built-in
// profile can name: each built-in family's model for every quality tier. On a
// catalog without family metadata it is the only attestation a pins-only pin
// has; a pin never attests its own model or family.
func BuiltinRoleModelSelectorFamilies() map[string]string {
	families := make(map[string]string)
	for _, family := range builtinModelFamilies {
		for _, tier := range qualityTiers {
			families[family.provider+"/"+family.modelForTier(tier)] = family.family
		}
	}
	return families
}
