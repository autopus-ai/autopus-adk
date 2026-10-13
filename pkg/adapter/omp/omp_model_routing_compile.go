package omp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/insajin/autopus-adk/pkg/config"
)

type OMPModelRoutingInput struct {
	Catalog        OMPModelCatalog                 `json:"catalog"`
	CatalogReason  string                          `json:"catalog_reason"`
	Routes         map[string]OMPModelRouteRequest `json:"routes"`
	ExecutorFamily string                          `json:"executor_family,omitempty"`
	// AgentSource is the source the routes serve; it orders the resolutions
	// and picks the family anchor. Empty infers it from the route names.
	AgentSource string `json:"-"`
}

type OMPModelRoutingCompilation struct {
	Resolutions      []OMPModelRouteResolution `json:"resolutions"`
	ResolutionDigest string                    `json:"resolution_digest"`
}

type ompRoutingWorkItem struct {
	routeID string
	request OMPModelRouteRequest
}

// ompRoutingExecutorFamilyAnchor is the agent whose resolved family anchors
// family-diversity comparisons: the one that carries implementation work, so
// a dissent route asking for a distinct family is asking to differ from it.
// Native mode runs implementation on the bundled task agent; autopus mode on
// the executor agent.
func ompRoutingExecutorFamilyAnchor(source string) string {
	if source == config.OMPAgentSourceNative {
		return "task"
	}
	return "executor"
}

// CompileOMPModelRouting returns canonical native agent order independent of
// map order.
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-OMP-004: canonical routing compilation has three production callers.
// @AX:REASON [AUTO]: model integration, doctor projection, and CLI probing require identical ordering and resolution digests.
func CompileOMPModelRouting(input OMPModelRoutingInput) OMPModelRoutingCompilation {
	source := ompServedAgentSource(input.AgentSource, ompRouteAgentNames(input.Routes))
	ranks := ompRoutingAgentRanks(source)
	items := canonicalOMPRoutingWorkItems(input.Routes, ranks)
	resolutions := make([]OMPModelRouteResolution, 0, len(items))
	executorFamily := input.ExecutorFamily
	preResolved := make(map[string]OMPModelRouteResolution, 1)
	anchor := ompRoutingExecutorFamilyAnchor(source)

	if executorFamily == "" {
		for _, item := range items {
			if item.request.Agent != anchor && item.routeID != anchor {
				continue
			}
			resolved := ResolveOMPModelRoute(input.Catalog, input.CatalogReason, item.request)
			resolved.RouteID = item.routeID
			preResolved[item.routeID] = resolved
			if resolved.Status == "selected" {
				executorFamily = resolved.EffectiveFamily
			}
			break
		}
	}

	for _, item := range items {
		if resolved, ok := preResolved[item.routeID]; ok {
			resolutions = append(resolutions, resolved)
			continue
		}
		request := item.request
		if request.PreferDistinctExecutorFamily && request.ExecutorFamily == "" {
			request.ExecutorFamily = executorFamily
		}
		resolved := ResolveOMPModelRoute(input.Catalog, input.CatalogReason, request)
		resolved.RouteID = item.routeID
		resolutions = append(resolutions, resolved)
	}
	sort.SliceStable(resolutions, func(i, j int) bool {
		left, right := ompRoutingAgentRank(ranks, resolutions[i].Agent), ompRoutingAgentRank(ranks, resolutions[j].Agent)
		if left != right {
			return left < right
		}
		return resolutions[i].RouteID < resolutions[j].RouteID
	})
	return OMPModelRoutingCompilation{
		Resolutions:      resolutions,
		ResolutionDigest: digestOMPModelRouting(resolutions),
	}
}

func canonicalOMPRoutingWorkItems(routes map[string]OMPModelRouteRequest, ranks map[string]int) []ompRoutingWorkItem {
	items := make([]ompRoutingWorkItem, 0, len(routes))
	for routeID, request := range routes {
		items = append(items, ompRoutingWorkItem{routeID: routeID, request: request})
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := ompRoutingAgentRank(ranks, items[i].request.Agent), ompRoutingAgentRank(ranks, items[j].request.Agent)
		if left != right {
			return left < right
		}
		return items[i].routeID < items[j].routeID
	})
	return items
}

// ompRoutingAgentRanks orders routes by the served agent position so the
// compiled resolution list is stable regardless of route map insertion order
// and independent of which ADK role supplied a route's candidates.
func ompRoutingAgentRanks(source string) map[string]int {
	agents := config.OMPAgentNames(source)
	ranks := make(map[string]int, len(agents))
	for index, agent := range agents {
		ranks[agent] = index
	}
	return ranks
}

func ompRoutingAgentRank(ranks map[string]int, agent string) int {
	if rank, ok := ranks[agent]; ok {
		return rank
	}
	return len(ranks)
}

func ompRouteAgentNames(routes map[string]OMPModelRouteRequest) []string {
	names := make([]string, 0, len(routes))
	for _, route := range routes {
		names = append(names, route.Agent)
	}
	return names
}

// ompServedAgentSource is the source a route or projection set was built for.
// Generation always names it; an unnamed set is native when it holds an agent
// only the bundled registry has, autopus when it holds a canonical agent only
// Autopus serves, and native otherwise, which is how it behaved before.
func ompServedAgentSource(declared string, agents []string) string {
	if declared != "" {
		return config.EffectiveOMPAgentSource(declared)
	}
	canonical := make(map[string]bool, 16)
	for _, agent := range config.CanonicalAgentNames() {
		canonical[agent] = true
	}
	source := config.OMPAgentSourceNative
	for _, agent := range agents {
		switch {
		case agent == "scout" || agent == "security-reviewer" || agent == "sonic":
			return config.OMPAgentSourceNative
		case canonical[agent] && agent != "reviewer":
			source = config.OMPAgentSourceAutopus
		}
	}
	return source
}

func digestOMPModelRouting(resolutions []OMPModelRouteResolution) string {
	payload, err := json.Marshal(struct {
		Resolutions []OMPModelRouteResolution `json:"resolutions"`
	}{Resolutions: resolutions})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
