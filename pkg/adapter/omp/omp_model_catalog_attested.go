package omp

import (
	"context"
	"sort"

	"github.com/insajin/autopus-adk/pkg/config"
)

type ompOperatorAttestedDeclaration struct {
	family       string
	capabilities map[string]struct{}
	thinking     map[string]struct{}
}

// ProbeOMPModelCatalogForProfile preserves the strict probe and permits the
// selected profile to attest missing semantic catalog metadata explicitly.
// @AX:ANCHOR [AUTO]: Profile apply, adapter generation, and doctor reuse this trust boundary.
// @AX:REASON [AUTO]: Changing profile-aware probing would alter all operator-attested routing consumers.
func ProbeOMPModelCatalogForProfile(
	ctx context.Context,
	opts OMPModelCatalogProbeOptions,
	profile config.RoleModelProfileConf,
) OMPModelCatalogProbeResult {
	return probeOMPModelCatalog(ctx, opts, &profile)
}

// @AX:WARN [AUTO]: Operator-attested normalization has 11 conditional branches.
// @AX:REASON [AUTO]: bounded input, exact selector intersection, thinking support, duplicates, and provenance converge here.
func normalizeOMPOperatorAttestedCatalog(
	data []byte,
	maxOutput int,
	profile config.RoleModelProfileConf,
) (OMPModelCatalog, string) {
	if maxOutput <= 0 || len(data) > maxOutput {
		return OMPModelCatalog{}, "catalog_oversized"
	}
	if rejectDuplicateOMPModelReceiptJSON(data) != nil {
		return OMPModelCatalog{}, "catalog_invalid"
	}
	var raw rawOMPModelCatalog
	if !decodeOMPModelCatalogJSON(data, &raw) || raw.Models == nil {
		return OMPModelCatalog{}, "catalog_invalid"
	}
	if len(*raw.Models) == 0 {
		return OMPModelCatalog{}, "catalog_empty"
	}
	var declarations map[string]ompOperatorAttestedDeclaration
	if profile.PinsOnly {
		var reason string
		if declarations, reason = ompPinsOnlyAttestedDeclarations(profile); reason != "" {
			return OMPModelCatalog{}, reason
		}
	} else if declared, ok := ompOperatorAttestedDeclarations(profile); ok {
		declarations = declared
	} else {
		return OMPModelCatalog{}, "catalog_invalid"
	}

	models := make([]OMPModelMetadata, 0, len(*raw.Models))
	seen := make(map[string]struct{}, len(*raw.Models))
	for _, entry := range *raw.Models {
		if !safeOMPModelToken(entry.Provider) || !safeOMPModelToken(entry.ID) {
			return OMPModelCatalog{}, "catalog_invalid"
		}
		selector := entry.Provider + "/" + entry.ID
		if entry.Selector != "" && entry.Selector != selector {
			return OMPModelCatalog{}, "catalog_invalid"
		}
		if _, duplicate := seen[selector]; duplicate {
			return OMPModelCatalog{}, "catalog_invalid"
		}
		seen[selector] = struct{}{}

		semantics, reason := normalizePresentOMPModelSemantics(entry)
		if reason != "" {
			return OMPModelCatalog{}, reason
		}
		declaration, declared := declarations[selector]
		if !declared {
			continue
		}
		thinking := sortedOMPDeclarationValues(declaration.thinking)
		if entry.Thinking != nil {
			thinking = intersectOMPDeclarationValues(thinking, semantics.thinking)
		}
		models = append(models, OMPModelMetadata{
			Provider: entry.Provider, Model: entry.ID, Family: declaration.family,
			Capabilities: sortedOMPDeclarationValues(declaration.capabilities),
			Thinking:     thinking, Disabled: entry.Disabled, OperatorAttested: true,
		})
	}
	if len(models) == 0 {
		return OMPModelCatalog{}, "catalog_metadata_insufficient"
	}
	return canonicalOMPModelCatalog(models)
}

// ompPinsOnlyAttestedDeclarations declares a pins-only profile's routes for a
// catalog without family metadata (security review P1). A pin vouches for
// nothing: its model must be one a built-in profile names (else
// pin_model_unattested), and the built-in family is the attested one, so a
// pin claiming another family fails here as pin_family_mismatch.
func ompPinsOnlyAttestedDeclarations(
	profile config.RoleModelProfileConf,
) (map[string]ompOperatorAttestedDeclaration, string) {
	builtin := config.BuiltinRoleModelSelectorFamilies()
	declarations := make(map[string]ompOperatorAttestedDeclaration)
	for _, served := range profile.OMPAgentNames() {
		identity, route, err := profile.OMPAgentRoute(served)
		if err != nil {
			return nil, "catalog_invalid"
		}
		candidates := make([]config.RoleModelCandidateConf, 0, len(route.Candidates))
		for _, candidate := range route.Candidates {
			family, declared := builtin[candidate.Selector]
			switch {
			case !declared:
				return nil, "pin_model_unattested"
			case candidate.Family != family:
				return nil, "pin_family_mismatch"
			}
			candidates = append(candidates, candidate)
		}
		if !declareOMPOperatorAttestedCandidates(declarations, identity.Capability, candidates) {
			return nil, "catalog_invalid"
		}
	}
	return declarations, ""
}

// ompOperatorAttestedDeclarations collects every candidate a closed profile
// declares, from capability routes and agent overrides alike, so attested
// metadata covers each selector the routing resolver may pick.
func ompOperatorAttestedDeclarations(
	profile config.RoleModelProfileConf,
) (map[string]ompOperatorAttestedDeclaration, bool) {
	capabilities := config.OMPProviderNeutralCapabilities()
	if len(profile.Capabilities) != len(capabilities) {
		return nil, false
	}
	declarations := make(map[string]ompOperatorAttestedDeclaration)
	for _, capability := range capabilities {
		route, ok := profile.Capabilities[capability]
		if !ok || !route.Required || route.DegradedAction != "" || len(route.Candidates) == 0 ||
			!declareOMPOperatorAttestedCandidates(declarations, capability, route.Candidates) {
			return nil, false
		}
	}
	for agent, override := range profile.Agents {
		if len(override.Candidates) == 0 {
			continue
		}
		// A route may be keyed by an ADK role or by the bundled agent it
		// collapses onto, so the capability comes from the shared resolver.
		// Reading only the role matrix here rejected every native-keyed pin.
		resolved, err := config.ResolveOMPPolicyAgent(agent)
		if err != nil ||
			!declareOMPOperatorAttestedCandidates(declarations, resolved.Capability, override.Candidates) {
			return nil, false
		}
	}
	if !profile.OMPNativeAgentSource() {
		// In autopus mode a pin can govern agents other than its key (Route
		// Contract rule 2: a task pin routes the roles native mode collapses
		// into task), so every served route is declared under the capability
		// of the agent it routes.
		for _, served := range profile.OMPAgentNames() {
			identity, route, err := profile.OMPAgentRoute(served)
			if err != nil || !declareOMPOperatorAttestedCandidates(declarations, identity.Capability, route.Candidates) {
				return nil, false
			}
		}
	}
	return declarations, true
}

func declareOMPOperatorAttestedCandidates(
	declarations map[string]ompOperatorAttestedDeclaration,
	capability string,
	candidates []config.RoleModelCandidateConf,
) bool {
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, _, valid := parseOMPRoutingSelector(candidate.Selector); !valid ||
			candidate.Family == "" || !safeOMPModelToken(candidate.Family) ||
			!config.IsOMPNativeThinkingLevel(candidate.Thinking) {
			return false
		}
		key := candidate.Selector + "\x00" + candidate.Thinking
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
		declaration := declarations[candidate.Selector]
		if declaration.family != "" && declaration.family != candidate.Family {
			return false
		}
		if declaration.capabilities == nil {
			declaration = ompOperatorAttestedDeclaration{
				family: candidate.Family, capabilities: make(map[string]struct{}), thinking: make(map[string]struct{}),
			}
		}
		declaration.capabilities[capability] = struct{}{}
		declaration.thinking[candidate.Thinking] = struct{}{}
		declarations[candidate.Selector] = declaration
	}
	return true
}

func sortedOMPDeclarationValues(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func intersectOMPDeclarationValues(declared, observed []string) []string {
	observedSet := make(map[string]struct{}, len(observed))
	for _, value := range observed {
		observedSet[value] = struct{}{}
	}
	result := make([]string, 0, len(declared))
	for _, value := range declared {
		if _, ok := observedSet[value]; ok {
			result = append(result, value)
		}
	}
	return result
}
