package cli

import (
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// An installed OMP catalog reports provider, id, and thinking levels but no
// model family or semantic capability. A hand-typed --agent pin therefore
// cannot take its family from observed metadata on a real installation; it is
// attested against declarations Autopus already owns. Nothing here derives a
// family from a provider string, so an undeclared model still fails closed.

// ompProfileDeclaredFamilies collects the family of every selector the closed
// profile set declares: the selected profile's own candidates (including any
// pre-existing explicit root override) plus the built-in matrix of the selected
// profile name under every anchor family, so a cross-family pin such as
// anthropic/claude-fable-5-1 stays attestable while --family openai is active.
func ompProfileDeclaredFamilies(
	policy config.RoleModelPolicyConf,
	quality config.QualityConf,
	name string,
) map[string]string {
	declared := make(map[string]string)
	if _, profile, ok := policy.SelectedRoleModelProfileForQuality(quality); ok {
		collectOMPProfileCandidateFamilies(declared, profile)
	}
	if !config.IsBuiltinRoleModelProfileName(name) {
		return declared
	}
	for _, family := range config.RoleModelFamilies() {
		variant, ok := config.BuiltinRoleModelProfile(name, quality, family, policy.ConfigMode)
		if !ok {
			continue
		}
		collectOMPProfileCandidateFamilies(declared, variant)
	}
	return declared
}

// ompBuiltinDeclaredFamilies is the family of every selector a built-in
// profile declares, balanced and ultra under each anchor family: the models
// a pins-only pin can be attested by when the catalog reports no family
// (SPEC-MODELUX-003 Pins-only Policy Contract).
func ompBuiltinDeclaredFamilies(quality config.QualityConf, configMode string) map[string]string {
	declared := make(map[string]string)
	for _, name := range []string{"balanced", "ultra"} {
		for _, family := range config.RoleModelFamilies() {
			if profile, ok := config.BuiltinRoleModelProfile(name, quality, family, configMode); ok {
				collectOMPProfileCandidateFamilies(declared, profile)
			}
		}
	}
	return declared
}

func collectOMPProfileCandidateFamilies(
	declared map[string]string,
	profile config.RoleModelProfileConf,
) {
	for _, route := range profile.Capabilities {
		collectOMPCandidateFamilies(declared, route.Candidates)
	}
	for _, override := range profile.Agents {
		collectOMPCandidateFamilies(declared, override.Candidates)
	}
}

// collectOMPCandidateFamilies records one family per selector. Conflicting
// declarations for the same selector are recorded as unattestable rather than
// resolved by declaration order.
func collectOMPCandidateFamilies(
	declared map[string]string,
	candidates []config.RoleModelCandidateConf,
) {
	for _, candidate := range candidates {
		if candidate.Family == "" {
			continue
		}
		existing, seen := declared[candidate.Selector]
		if seen && existing != candidate.Family {
			declared[candidate.Selector] = ""
			continue
		}
		declared[candidate.Selector] = candidate.Family
	}
}

// resolveOMPProfilePinFamily attests one pinned selector's family from the
// closed declarations first, then from a semantic catalog when this run probed
// one for profile synthesis. A selector neither declared nor observed is
// rejected by the caller.
func resolveOMPProfilePinFamily(
	declared map[string]string,
	seed omp.OMPModelCatalog,
	selector string,
) (string, bool) {
	if family := declared[selector]; family != "" {
		return family, true
	}
	return ompCatalogModelFamily(seed, selector)
}

// ompCatalogModelFamily reports the family a semantic catalog observes for one
// selector. Native catalogs declare no family, so this only resolves for the
// synthesized-profile path that already required a strict catalog.
func ompCatalogModelFamily(catalog omp.OMPModelCatalog, selector string) (string, bool) {
	for _, model := range catalog.Models {
		if model.Provider+"/"+model.Model == selector {
			return model.Family, model.Family != ""
		}
	}
	return "", false
}
