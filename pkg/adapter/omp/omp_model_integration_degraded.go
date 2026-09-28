package omp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

const ompRuntimeDefaultAction = "runtime_default"

// DegradeUnservedDerivedOMPRoutes lets a route that a built-in profile
// derived fall back to OMP's own default model when the catalog serves none
// of the providers it names. Which providers a user is logged in to varies per
// machine, and a built-in ladder names models the operator never picked, so a
// missing login must not block generation. Everything else stays fail-closed:
// a route the operator wrote, and a derived route whose provider is present
// but whose model or thinking level is not, because quietly substituting a
// different model there would hide a real mismatch.
func DegradeUnservedDerivedOMPRoutes(
	profile config.RoleModelProfileConf,
	catalog OMPModelCatalog,
	routes map[string]OMPModelRouteRequest,
) {
	if !profile.Builtin {
		return
	}
	for native, request := range routes {
		if !request.Required || request.DegradedAction != "" || len(request.Candidates) == 0 ||
			profile.OMPNativeAgentOperatorPinned(native) ||
			len(ompUnavailableCandidateProviders(catalog, request)) != len(ompCandidateProviders(request)) {
			continue
		}
		request.DegradedAction = ompRuntimeDefaultAction
		routes[native] = request
	}
}

// ompCandidateProviders lists the distinct providers a route's selectors name.
func ompCandidateProviders(request OMPModelRouteRequest) []string {
	seen := make(map[string]bool)
	var providers []string
	for _, candidate := range request.Candidates {
		provider, _, ok := strings.Cut(candidate.Selector, "/")
		if ok && provider != "" && !seen[provider] {
			seen[provider] = true
			providers = append(providers, provider)
		}
	}
	return providers
}

// ompUnavailableCandidateProviders lists the providers a route names that the
// catalog serves no usable model for, the usual sign of a missing or expired
// login rather than a wrong selector.
func ompUnavailableCandidateProviders(catalog OMPModelCatalog, request OMPModelRouteRequest) []string {
	// Usability matches matchOMPModelCandidate, so "unserved" here means the
	// router could not have picked any model of that provider.
	served := make(map[string]bool)
	for _, model := range catalog.Models {
		if !model.Disabled && (model.OperatorAttested || model.AuthEnabled || model.Keyless) {
			served[model.Provider] = true
		}
	}
	seen := make(map[string]bool)
	var missing []string
	for _, candidate := range request.Candidates {
		provider, _, ok := strings.Cut(candidate.Selector, "/")
		if !ok || provider == "" || served[provider] || seen[provider] {
			continue
		}
		seen[provider] = true
		missing = append(missing, provider)
	}
	sort.Strings(missing)
	return missing
}

// ompLoginHint names the providers to log in to, or is empty when every
// provider the route names is present and the mismatch lies elsewhere.
func ompLoginHint(providers []string) string {
	if len(providers) == 0 {
		return ""
	}
	return fmt.Sprintf("the OMP catalog has no usable %s model; log in with `omp auth-broker login %s`",
		strings.Join(providers, "/"), providers[0])
}

// ompDegradedRoutesNotice summarizes the agents that fell back to the OMP
// runtime default, or returns "" when every route resolved.
func ompDegradedRoutesNotice(
	catalog OMPModelCatalog,
	routes map[string]OMPModelRouteRequest,
	routing OMPModelRoutingCompilation,
) string {
	var agents []string
	providerSet := make(map[string]bool)
	for _, resolution := range routing.Resolutions {
		route, ok := routes[resolution.Agent]
		if !ok || route.DegradedAction != ompRuntimeDefaultAction || resolution.Status == "selected" {
			continue
		}
		agents = append(agents, resolution.Agent)
		for _, provider := range ompUnavailableCandidateProviders(catalog, route) {
			providerSet[provider] = true
		}
	}
	if len(agents) == 0 {
		return ""
	}
	sort.Strings(agents)
	providers := make([]string, 0, len(providerSet))
	for provider := range providerSet {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	notice := fmt.Sprintf("  [omp] model routing: %s use the OMP default model", strings.Join(agents, ", "))
	if hint := ompLoginHint(providers); hint != "" {
		notice += " (" + hint + ", or set role_model_policy.family)"
	}
	return notice
}
