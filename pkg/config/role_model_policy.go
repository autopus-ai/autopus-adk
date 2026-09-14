package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	RoleModelPolicyVersionV1 = "v1"

	RoleModelConfigModeOverlay        = "overlay"
	RoleModelConfigModeProjectManaged = "project-managed"

	RoleModelCatalogTrustStrict           = "strict"
	RoleModelCatalogTrustOperatorAttested = "operator-attested"
)

// RoleModelPolicyConf is an opt-in, provider-neutral OMP role routing policy.
// Family and ConfigMode apply only when a selected built-in profile is derived.
// Agents is a concise per-agent overlay on the selected profile, built-in or
// custom: it pins a single agent's route without restating the profile.
type RoleModelPolicyConf struct {
	Version    string                           `yaml:"version,omitempty"`
	Profile    string                           `yaml:"profile,omitempty"`
	Family     string                           `yaml:"family,omitempty"`
	ConfigMode string                           `yaml:"config_mode,omitempty"`
	Agents     map[string]RoleAgentOverrideConf `yaml:"agents,omitempty"`
	Profiles   map[string]RoleModelProfileConf  `yaml:"profiles,omitempty"`
}

// RoleModelProfileConf owns capability routes, per-agent overrides, and
// optional OMP projection policy.
type RoleModelProfileConf struct {
	ConfigMode      string                             `yaml:"config_mode,omitempty"`
	CatalogTrust    string                             `yaml:"catalog_trust,omitempty"`
	Capabilities    map[string]RoleCapabilityRouteConf `yaml:"capabilities,omitempty"`
	Agents          map[string]RoleAgentOverrideConf   `yaml:"agents,omitempty"`
	ManagedKeys     map[string]RoleManagedKeyClaimConf `yaml:"managed_keys,omitempty"`
	FamilyDiversity FamilyDiversityPolicyConf          `yaml:"family_diversity,omitempty"`
	Safety          RoleSafetyPolicyConf               `yaml:"safety,omitempty"`

	// OperatorAgents names the Agents keys an operator actually wrote, either
	// inside a custom profile or as the root overlay. A built-in profile
	// derives one route per ADK agent from a tier ladder, and those derived
	// routes collapse onto a native OMP agent through its representative
	// instead of conflicting with each other. Resolution fills this in; it is
	// never read from or written to YAML.
	OperatorAgents map[string]bool `yaml:"-"`
}

// RoleManagedKeyClaimConf proves complete ownership of one project config key.
type RoleManagedKeyClaimConf struct {
	PriorFingerprint   string `yaml:"prior_fingerprint"`
	Complete           bool   `yaml:"complete"`
	FullArrayOwnership bool   `yaml:"full_array_ownership,omitempty"`
}

// RoleCapabilityRouteConf declares ordered candidates for one semantic capability.
type RoleCapabilityRouteConf struct {
	Candidates     []RoleModelCandidateConf `yaml:"candidates,omitempty"`
	Required       bool                     `yaml:"required,omitempty"`
	DegradedAction string                   `yaml:"degraded_action,omitempty"`
}

// RoleModelCandidateConf contains only non-secret selector metadata.
type RoleModelCandidateConf struct {
	Selector string `yaml:"selector"`
	Thinking string `yaml:"thinking,omitempty"`
	Family   string `yaml:"family,omitempty"`
}

// RoleAgentOverrideConf pins one agent's route. Role and Capability are
// optional assertions that must match the matrix when set; Candidates replace
// the capability route's ordered candidates for this agent only.
type RoleAgentOverrideConf struct {
	Role       string                   `yaml:"role,omitempty"`
	Capability string                   `yaml:"capability,omitempty"`
	Candidates []RoleModelCandidateConf `yaml:"candidates,omitempty"`
}

// detached returns a copy whose candidate slice is not shared with the source
// config, so a resolved profile can never write back into it.
func (c RoleAgentOverrideConf) detached() RoleAgentOverrideConf {
	c.Candidates = append([]RoleModelCandidateConf(nil), c.Candidates...)
	return c
}

// FamilyDiversityPolicyConf selects agent roles that prefer a distinct model family.
type FamilyDiversityPolicyConf struct {
	Enabled bool     `yaml:"enabled,omitempty"`
	Roles   []string `yaml:"roles,omitempty"`
}

// RoleSafetyPolicyConf claims OMP safety keys only when a value is explicit.
type RoleSafetyPolicyConf struct {
	ApprovalMode  string `yaml:"approval_mode,omitempty"`
	IsolationMode string `yaml:"isolation_mode,omitempty"`
}

// LegacyRoleRoute is a versioned semantic translation of a legacy quality tier.
type LegacyRoleRoute struct {
	Capability   string
	Role         string
	LegacySource string
}

// EffectiveVersion returns the backward-compatible v1 policy version.
func (c RoleModelPolicyConf) EffectiveVersion() string {
	if c.Version == "" {
		return RoleModelPolicyVersionV1
	}
	return c.Version
}

// EffectiveCatalogTrust returns the strict default for catalog normalization.
func (c RoleModelProfileConf) EffectiveCatalogTrust() string {
	if c.CatalogTrust == "" {
		return RoleModelCatalogTrustStrict
	}
	return c.CatalogTrust
}

// AgentCandidates returns the ordered candidates that route one agent: the
// agents.<name>.candidates override when present, otherwise the candidates of
// the agent's capability route.
func (c RoleModelProfileConf) AgentCandidates(agent string) ([]RoleModelCandidateConf, error) {
	route, err := c.AgentRoute(agent)
	if err != nil {
		return nil, err
	}
	return route.Candidates, nil
}

// AgentRoute returns a copy of the capability route that governs one agent
// with Candidates replaced by AgentCandidates. Required and DegradedAction
// always come from the capability route. The agent may be an ADK role name or
// a native OMP agent name; a native name routes on its representative's
// capability while still honoring its own candidate override.
func (c RoleModelProfileConf) AgentRoute(agent string) (RoleCapabilityRouteConf, error) {
	resolved, err := ResolveOMPPolicyAgent(agent)
	if err != nil {
		return RoleCapabilityRouteConf{}, err
	}
	route, ok := c.Capabilities[resolved.Capability]
	if !ok {
		return RoleCapabilityRouteConf{}, fmt.Errorf("capability_missing: %s", resolved.Capability)
	}
	candidates := route.Candidates
	if override, ok := c.Agents[agent]; ok && len(override.Candidates) > 0 {
		candidates = override.Candidates
	}
	route.Candidates = append([]RoleModelCandidateConf(nil), candidates...)
	return route, nil
}

// OMPMissingManagedValueFingerprint identifies a managed key that was absent.
func OMPMissingManagedValueFingerprint() string {
	sum := sha256.Sum256([]byte("autopus.omp.managed.missing.v1"))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// SelectedRoleModelProfile returns the explicitly selected profile.
func (c RoleModelPolicyConf) SelectedRoleModelProfile() (string, RoleModelProfileConf, bool) {
	if c.Profile == "" {
		return "", RoleModelProfileConf{}, false
	}
	profile, ok := c.Profiles[c.Profile]
	return c.Profile, profile, ok
}

// SelectedRoleModelProfileForQuality resolves the selected profile, falling
// back to the built-in profile of the same name when the config defines none.
// An explicit definition always wins. The root per-agent overlay is applied
// last, to a copy: resolving the same policy for several families or presets
// never leaks one resolution into the next or into the source config.
func (c RoleModelPolicyConf) SelectedRoleModelProfileForQuality(quality QualityConf) (string, RoleModelProfileConf, bool) {
	name, profile, ok := c.SelectedRoleModelProfile()
	// A profile the operator defined carries operator intent in every one of
	// its agent entries; a built-in profile derives them, so only the root
	// overlay counts as written there.
	var declared map[string]RoleAgentOverrideConf
	if ok {
		declared = profile.Agents
	}
	operator := operatorRoleAgentKeys(declared, c.Agents)
	if !ok && name != "" {
		profile, ok = BuiltinRoleModelProfile(name, quality, c.Family, c.ConfigMode)
	}
	if !ok {
		return name, RoleModelProfileConf{}, false
	}
	profile = profile.withRootAgentOverlay(c.Agents)
	profile.OperatorAgents = operator
	return name, profile, true
}

// operatorRoleAgentKeys collects the union of the agent keys an operator wrote.
func operatorRoleAgentKeys(sets ...map[string]RoleAgentOverrideConf) map[string]bool {
	total := 0
	for _, set := range sets {
		total += len(set)
	}
	if total == 0 {
		return nil
	}
	keys := make(map[string]bool, total)
	for _, set := range sets {
		for agent := range set {
			keys[agent] = true
		}
	}
	return keys
}

// withRootAgentOverlay returns the profile with the policy's root per-agent
// overrides layered over its own. The receiver's maps and slices are only
// read; the result shares nothing that the overlay touched.
func (c RoleModelProfileConf) withRootAgentOverlay(
	overlay map[string]RoleAgentOverrideConf,
) RoleModelProfileConf {
	if len(overlay) == 0 {
		return c
	}
	agents := make(map[string]RoleAgentOverrideConf, len(c.Agents)+len(overlay))
	for agent, override := range c.Agents {
		agents[agent] = override
	}
	for agent, override := range overlay {
		agents[agent] = override.detached()
	}
	c.Agents = agents
	return c
}
