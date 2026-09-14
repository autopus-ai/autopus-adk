package omp

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func routingCatalogC1(t *testing.T) OMPModelCatalog {
	t.Helper()
	raw := []byte(`{"models":[
		{"provider":"anthropic","id":"alpha-reasoner","family":"anthropic","capabilities":["deep_reasoning","independent_dissent"],"thinking":["high","xhigh"],"auth_enabled":true},
		{"provider":"openai","id":"beta-coder","family":"openai","capabilities":["coding_tool_use","fast_validation","deterministic_transform","independent_dissent","deep_reasoning"],"thinking":["medium","high"],"auth_enabled":true},
		{"provider":"google","id":"gamma-vision","family":"google","capabilities":["vision_design"],"thinking":["high"],"auth_enabled":true},
		{"provider":"openai","id":"unauthorized-coder","family":"openai","capabilities":["coding_tool_use"],"thinking":["high"],"auth_enabled":false}
	]}`)
	catalog, reason := NormalizeOMPModelCatalog(raw, 16*1024)
	require.Equal(t, "catalog_ready", reason)
	return catalog
}

func TestResolveOMPModelRoute_WithExactAndInvalidSelectors_ReturnsExactReasons(t *testing.T) {
	t.Parallel()
	catalog := routingCatalogC1(t)
	tests := []struct {
		name       string
		role       string
		capability string
		candidate  OMPRoutingCandidate
		wantStatus string
		wantReason string
	}{
		{name: "exact", role: "autopus_planner", capability: "deep_reasoning", candidate: OMPRoutingCandidate{Selector: "anthropic/alpha-reasoner", Thinking: "xhigh"}, wantStatus: "selected", wantReason: "selected"},
		{name: "fuzzy", role: "autopus_planner", capability: "deep_reasoning", candidate: OMPRoutingCandidate{Selector: "anthropic/alpha-reason", Thinking: "xhigh"}, wantStatus: "blocked", wantReason: "no_compatible_candidate"},
		{name: "thinking", role: "autopus_ux_validator", capability: "vision_design", candidate: OMPRoutingCandidate{Selector: "google/gamma-vision", Thinking: "xhigh"}, wantStatus: "blocked", wantReason: "no_compatible_candidate"},
		{name: "provider", role: "autopus_planner", capability: "deep_reasoning", candidate: OMPRoutingCandidate{Selector: "unknown/alpha-reasoner", Thinking: "xhigh"}, wantStatus: "blocked", wantReason: "no_compatible_candidate"},
		{name: "role", role: "unknown", capability: "deep_reasoning", candidate: OMPRoutingCandidate{Selector: "anthropic/alpha-reasoner", Thinking: "xhigh"}, wantStatus: "blocked", wantReason: "role_unknown"},
		{name: "native role", role: "task", capability: "coding_tool_use", candidate: OMPRoutingCandidate{Selector: "openai/beta-coder", Thinking: "high"}, wantStatus: "blocked", wantReason: "role_unknown"},
		{name: "capability", role: "autopus_executor", capability: "deep_reasoning", candidate: OMPRoutingCandidate{Selector: "anthropic/alpha-reasoner", Thinking: "xhigh"}, wantStatus: "blocked", wantReason: "capability_mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveOMPModelRoute(catalog, "catalog_ready", OMPModelRouteRequest{
				Role: tc.role, Capability: tc.capability, Required: true, Candidates: []OMPRoutingCandidate{tc.candidate},
			})
			require.Equal(t, tc.wantStatus, got.Status)
			require.Equal(t, tc.wantReason, got.Reason)
			if tc.name == "exact" {
				require.Equal(t, "anthropic/alpha-reasoner:xhigh", got.EffectiveSelector)
				require.Equal(t, "anthropic", got.EffectiveProvider)
				require.Equal(t, "alpha-reasoner", got.EffectiveModel)
			} else if tc.name != "role" && tc.name != "native role" && tc.name != "capability" {
				require.Len(t, got.FallbackAttempts, 1)
				reasons := map[string]string{"fuzzy": "model_unknown", "thinking": "thinking_unsupported", "provider": "provider_unknown"}
				require.Equal(t, reasons[tc.name], got.FallbackAttempts[0].Reason)
			}
		})
	}
}

func TestResolveOMPModelRoute_WithCandidateFailures_PreservesDeclaredFallbackOrder(t *testing.T) {
	t.Parallel()
	catalog := routingCatalogC1(t)
	got := ResolveOMPModelRoute(catalog, "catalog_ready", OMPModelRouteRequest{
		Agent: "executor", Role: "autopus_executor", Capability: "coding_tool_use", Required: true,
		Candidates: []OMPRoutingCandidate{
			{Selector: "openai/unauthorized-coder", Thinking: "high"},
			{Selector: "openai/beta-coder", Thinking: "high"},
		},
	})

	require.Equal(t, "selected", got.Status)
	require.Equal(t, "openai/beta-coder:high", got.EffectiveSelector)
	require.Equal(t, "availability", got.EvidenceClass)
	require.Equal(t, []OMPRoutingAttempt{
		{Index: 0, Selector: "openai/unauthorized-coder:high", Status: "skipped", Reason: "unauthorized"},
		{Index: 1, Selector: "openai/beta-coder:high", Status: "selected", Reason: "selected"},
	}, got.FallbackAttempts)
	require.False(t, got.QuorumEvidence)
	require.False(t, got.ConsensusEvidence)
}

func TestResolveOMPModelRoute_WithUnavailableVariants_SeparatesAttemptReasons(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"models":[
		{"provider":"p","id":"disabled","family":"f","capabilities":["coding_tool_use"],"thinking":["high"],"auth_enabled":true,"disabled":true},
		{"provider":"p","id":"wrong-capability","family":"f","capabilities":["deep_reasoning"],"thinking":["high"],"auth_enabled":true},
		{"provider":"p","id":"wrong-thinking","family":"f","capabilities":["coding_tool_use"],"thinking":["medium"],"auth_enabled":true}
	]}`)
	catalog, reason := NormalizeOMPModelCatalog(raw, 4096)
	require.Equal(t, "catalog_ready", reason)
	got := ResolveOMPModelRoute(catalog, reason, OMPModelRouteRequest{
		Role: "autopus_executor", Capability: "coding_tool_use", Required: true,
		Candidates: []OMPRoutingCandidate{
			{Selector: "p/disabled", Thinking: "high"},
			{Selector: "p/wrong-capability", Thinking: "high"},
			{Selector: "p/wrong-thinking", Thinking: "high"},
		},
	})

	require.Equal(t, "blocked", got.Status)
	require.Equal(t, []string{"disabled", "capability_mismatch", "thinking_unsupported"}, []string{
		got.FallbackAttempts[0].Reason, got.FallbackAttempts[1].Reason, got.FallbackAttempts[2].Reason,
	})
}

func TestResolveOMPModelRoute_WithBlockedOrExplicitDegraded_FailsClosed(t *testing.T) {
	t.Parallel()
	catalog := routingCatalogC1(t)
	request := OMPModelRouteRequest{
		Agent: "reviewer", Role: "autopus_reviewer", Capability: "independent_dissent", Required: true,
		Candidates: []OMPRoutingCandidate{{Selector: "missing/model", Thinking: "high"}},
	}

	blocked := ResolveOMPModelRoute(catalog, "catalog_ready", request)
	require.Equal(t, "blocked", blocked.Status)
	require.Equal(t, "no_compatible_candidate", blocked.Reason)
	require.Empty(t, blocked.EffectiveSelector)

	request.DegradedAction = "runtime_default"
	degraded := ResolveOMPModelRoute(catalog, "catalog_ready", request)
	require.Equal(t, "degraded", degraded.Status)
	require.Equal(t, "explicit_runtime_default", degraded.DegradedReason)
	require.Empty(t, degraded.EffectiveSelector)

	request.Required = false
	request.DegradedAction = ""
	optional := ResolveOMPModelRoute(catalog, "catalog_ready", request)
	require.Equal(t, "degraded", optional.Status)
	require.Equal(t, "optional_runtime_default", optional.DegradedReason)
	require.Empty(t, optional.EffectiveSelector)

	request.Required = true
	request.DegradedAction = "invented"
	invalid := ResolveOMPModelRoute(catalog, "catalog_ready", request)
	require.Equal(t, "blocked", invalid.Status)
	require.Equal(t, "degraded_action_invalid", invalid.Reason)
}

func TestResolveOMPModelRoute_WithCatalogFailures_ReturnsExactBlockedReasons(t *testing.T) {
	t.Parallel()
	request := OMPModelRouteRequest{Role: "autopus_planner", Capability: "deep_reasoning", Required: true}
	for _, reason := range []string{"catalog_empty", "catalog_invalid", "catalog_oversized", "catalog_timeout"} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			got := ResolveOMPModelRoute(OMPModelCatalog{}, reason, request)
			require.Equal(t, "blocked", got.Status)
			require.Equal(t, reason, got.Reason)
			require.Empty(t, got.EffectiveProvider)
			require.Empty(t, got.EffectiveModel)
		})
	}
	unknown := ResolveOMPModelRoute(OMPModelCatalog{}, "secret-dynamic-reason", request)
	require.Equal(t, "catalog_invalid", unknown.Reason)
}

func TestResolveOMPModelRoute_WithFamilyDiversity_PrefersDifferentFamily(t *testing.T) {
	t.Parallel()
	catalog := routingCatalogC1(t)
	request := OMPModelRouteRequest{
		Agent: "reviewer", Role: "autopus_reviewer", Capability: "independent_dissent", Required: true,
		ExecutorFamily: "openai",
		Candidates: []OMPRoutingCandidate{
			{Selector: "openai/beta-coder", Thinking: "high"},
			{Selector: "anthropic/alpha-reasoner", Thinking: "high"},
		},
	}

	got := ResolveOMPModelRoute(catalog, "catalog_ready", request)
	require.Equal(t, "anthropic/alpha-reasoner:high", got.EffectiveSelector)
	require.Equal(t, OMPFamilyDiversity{Status: "satisfied", Executor: "openai", Reviewer: "anthropic"}, got.FamilyDiversity)
	require.Equal(t, "same_family_deprioritized", got.FallbackAttempts[0].Reason)
	require.False(t, got.IndependentProviderEvidence)
}

func TestResolveOMPModelRoute_WithSameFamilyOnly_EmitsDegradedDiversity(t *testing.T) {
	t.Parallel()
	catalog := routingCatalogC1(t)
	request := OMPModelRouteRequest{
		Agent: "security-auditor", Role: "autopus_security_auditor", Capability: "independent_dissent", Required: true,
		ExecutorFamily: "openai",
		Candidates:     []OMPRoutingCandidate{{Selector: "openai/beta-coder", Thinking: "high"}},
	}

	got := ResolveOMPModelRoute(catalog, "catalog_ready", request)
	require.Equal(t, "selected", got.Status)
	require.Equal(t, OMPFamilyDiversity{
		Status: "degraded", Reason: "same_family_only", Executor: "openai", Reviewer: "openai",
	}, got.FamilyDiversity)
	require.False(t, got.IndependentProviderEvidence)
	require.False(t, got.QuorumEvidence)
	require.False(t, got.ConsensusEvidence)
}

func TestCompileOMPModelRouting_FamilyDiversityAppliesOnlyToConfiguredRoles(t *testing.T) {
	t.Parallel()
	catalog := routingCatalogC1(t)
	// task is the implementation anchor: dissent routes that ask for a
	// distinct family are asking to differ from whatever task settles on.
	routes := map[string]OMPModelRouteRequest{
		"task": {
			Agent: "task", Role: "autopus_planner", Capability: "deep_reasoning", Required: true,
			Candidates: []OMPRoutingCandidate{{Selector: "openai/beta-coder", Thinking: "high"}},
		},
		"reviewer": {
			Agent: "reviewer", Role: "autopus_reviewer", Capability: "independent_dissent", Required: true,
			PreferDistinctExecutorFamily: true,
			Candidates: []OMPRoutingCandidate{
				{Selector: "openai/beta-coder", Thinking: "high"},
				{Selector: "anthropic/alpha-reasoner", Thinking: "high"},
			},
		},
		"security-reviewer": {
			Agent: "security-reviewer", Role: "autopus_security_auditor",
			Capability: "independent_dissent", Required: true,
			Candidates: []OMPRoutingCandidate{
				{Selector: "openai/beta-coder", Thinking: "high"},
				{Selector: "anthropic/alpha-reasoner", Thinking: "high"},
			},
		},
	}
	got := CompileOMPModelRouting(OMPModelRoutingInput{
		Catalog: catalog, CatalogReason: "catalog_ready", Routes: routes,
	})
	byRoute := make(map[string]OMPModelRouteResolution, len(got.Resolutions))
	for _, resolution := range got.Resolutions {
		byRoute[resolution.RouteID] = resolution
	}
	require.Equal(t, "openai/beta-coder:high", byRoute["task"].EffectiveSelector)
	require.Equal(t, "anthropic/alpha-reasoner:high", byRoute["reviewer"].EffectiveSelector)
	require.Equal(t, "satisfied", byRoute["reviewer"].FamilyDiversity.Status)
	require.Equal(t, "openai/beta-coder:high", byRoute["security-reviewer"].EffectiveSelector)
	require.Empty(t, byRoute["security-reviewer"].FamilyDiversity.Status)
}

func TestCompileOMPModelRouting_WithMapInsertionVariance_ReturnsStableOrderAndDigest(t *testing.T) {
	t.Parallel()
	catalog := routingCatalogC1(t)
	base := map[string]OMPModelRouteRequest{
		"reviewer": {Agent: "reviewer", Role: "autopus_reviewer", Capability: "independent_dissent", Required: true, PreferDistinctExecutorFamily: true, Candidates: []OMPRoutingCandidate{{Selector: "openai/beta-coder", Thinking: "high"}, {Selector: "anthropic/alpha-reasoner", Thinking: "high"}}},
		"task":     {Agent: "task", Role: "autopus_planner", Capability: "deep_reasoning", Required: true, Candidates: []OMPRoutingCandidate{{Selector: "anthropic/alpha-reasoner", Thinking: "xhigh"}}},
		"sonic":    {Agent: "sonic", Role: "autopus_validator", Capability: "deterministic_transform", Required: true, Candidates: []OMPRoutingCandidate{{Selector: "openai/beta-coder", Thinking: "high"}}},
	}
	var wantDigest string
	for iteration := 0; iteration < 100; iteration++ {
		keys := []string{"reviewer", "task", "sonic"}
		rand.New(rand.NewPCG(uint64(iteration), 0)).Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] }) // #nosec G404 -- deterministic test permutation only.
		routes := make(map[string]OMPModelRouteRequest, len(keys))
		for _, key := range keys {
			routes[key] = base[key]
		}
		got := CompileOMPModelRouting(OMPModelRoutingInput{Catalog: catalog, CatalogReason: "catalog_ready", Routes: routes})
		// Bundled registry order, independent of which ADK role supplied the
		// candidates and independent of map insertion order.
		require.Equal(t, []string{"reviewer", "task", "sonic"}, []string{
			got.Resolutions[0].RouteID, got.Resolutions[1].RouteID, got.Resolutions[2].RouteID,
		})
		require.Equal(t, "openai", got.Resolutions[0].FamilyDiversity.Reviewer)
		if iteration == 0 {
			wantDigest = got.ResolutionDigest
		}
		require.Equal(t, wantDigest, got.ResolutionDigest)
		require.Regexp(t, `^sha256:[0-9a-f]{64}$`, got.ResolutionDigest)
	}
}
