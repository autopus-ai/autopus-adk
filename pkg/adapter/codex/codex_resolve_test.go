package codex

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

var (
	resolveEffortsToUltra = []string{"low", "medium", "high", "xhigh", "max", "ultra"}
	resolveEffortsToMax   = []string{"low", "medium", "high", "xhigh", "max"}
	resolveEffortsToXHigh = []string{"low", "medium", "high", "xhigh"}
)

type resolveCatalogModel struct {
	slug    string
	efforts []string
}

// resolveCatalogSnapshot encodes a catalog snapshot in the shape `codex debug
// models` reports, so the resolver parses it exactly like a probed catalog.
func resolveCatalogSnapshot(t *testing.T, models ...resolveCatalogModel) []byte {
	t.Helper()
	var catalog config.CodexModelCatalog
	for _, model := range models {
		entry := config.CodexCatalogModel{Slug: model.slug}
		for _, effort := range model.efforts {
			entry.SupportedReasoningLevels = append(entry.SupportedReasoningLevels,
				config.CodexCatalogReasoningLevel{Effort: effort})
		}
		catalog.Models = append(catalog.Models, entry)
	}
	data, err := json.Marshal(catalog)
	require.NoError(t, err)
	return data
}

// The acceptance catalogs: CAT-A advertises every managed rung, CAT-B swaps
// GPT-6.1 Sol for GPT-6 Sol (low–xhigh), CAT-D drops GPT-6 Astra. CAT-C, the
// unreadable catalog, is a nil snapshot.
func resolveCatalogA(t *testing.T) []byte {
	return resolveCatalogSnapshot(t,
		resolveCatalogModel{"gpt-6-astra", resolveEffortsToUltra},
		resolveCatalogModel{"gpt-6.1-sol", resolveEffortsToMax},
		resolveCatalogModel{"gpt-6-luna", resolveEffortsToMax},
		resolveCatalogModel{"gpt-5.6-sol", resolveEffortsToMax},
	)
}

func resolveCatalogB(t *testing.T) []byte {
	return resolveCatalogSnapshot(t,
		resolveCatalogModel{"gpt-6-astra", resolveEffortsToUltra},
		resolveCatalogModel{"gpt-6-sol", resolveEffortsToXHigh},
		resolveCatalogModel{"gpt-6-luna", resolveEffortsToMax},
		resolveCatalogModel{"gpt-5.6-sol", resolveEffortsToMax},
	)
}

func resolveCatalogD(t *testing.T) []byte {
	return resolveCatalogSnapshot(t,
		resolveCatalogModel{"gpt-6.1-sol", resolveEffortsToMax},
		resolveCatalogModel{"gpt-6-luna", resolveEffortsToMax},
		resolveCatalogModel{"gpt-5.6-sol", resolveEffortsToMax},
	)
}

func resolveBalancedQuality() config.QualityConf {
	return config.DefaultFullConfig("resolve-project").Quality
}

func resolveUltraQuality() config.QualityConf {
	quality := resolveBalancedQuality()
	quality.Default = "ultra"
	return quality
}

// resolveCustomExecutorQuality moves executor off its standard rung, which
// takes it out of the native placement and back onto the tier ladder.
func resolveCustomExecutorQuality() config.QualityConf {
	quality := resolveBalancedQuality()
	quality.Presets["balanced"].Agents["executor"] = "opus"
	return quality
}

func resolveCodexOverrideQuality() config.QualityConf {
	quality := resolveBalancedQuality()
	quality.Providers = map[string]string{config.QualityProviderCodex: "ultra"}
	return quality
}

func TestResolveAgentProfile_CatalogSnapshot_ReturnsGeneratorResolution(t *testing.T) {
	t.Parallel()

	astraMax := config.CodexProfile{Model: "gpt-6-astra", Effort: "max"}
	tests := []struct {
		name         string
		quality      config.QualityConf
		catalog      func(*testing.T) []byte
		agent        string
		fallbackTier string
		effort       string
		want         config.CodexProfile
		wantFallback bool
		wantReason   config.CodexResolutionReason
		wantErr      error
	}{
		{
			name: "balanced placement confirmed by catalog", quality: resolveBalancedQuality(),
			catalog: resolveCatalogA, agent: "planner", fallbackTier: "opus", effort: "max",
			want: astraMax, wantReason: config.CodexResolutionSupported,
		},
		{
			name: "balanced placement kept verbatim without catalog", quality: resolveBalancedQuality(),
			catalog: func(*testing.T) []byte { return nil }, agent: "planner", fallbackTier: "opus", effort: "max",
			want: astraMax, wantReason: config.CodexResolutionCatalogUnknown,
		},
		{
			name: "balanced placement refuses a substitute", quality: resolveBalancedQuality(),
			catalog: resolveCatalogD, agent: "planner", fallbackTier: "opus", effort: "max",
			wantErr: ErrCodexNativePlacementUnavailable,
		},
		{
			name: "balanced placement the narrower catalog still offers", quality: resolveBalancedQuality(),
			catalog: resolveCatalogD, agent: "executor", fallbackTier: "sonnet", effort: "medium",
			want:       config.CodexProfile{Model: "gpt-6-luna", Effort: "max"},
			wantReason: config.CodexResolutionSupported,
		},
		{
			name: "ultra substitutes the Sol rung", quality: resolveUltraQuality(),
			catalog: resolveCatalogB, agent: "annotator", fallbackTier: "sonnet", effort: "medium",
			want:         config.CodexProfile{Model: "gpt-6-sol", Effort: "xhigh"},
			wantFallback: true, wantReason: config.CodexResolutionModelUnavailable,
		},
		{
			name: "ultra without catalog takes the fallback model", quality: resolveUltraQuality(),
			catalog: func(*testing.T) []byte { return nil }, agent: "planner", fallbackTier: "opus", effort: "max",
			want:         config.CodexProfile{Model: "gpt-5.6-sol", Effort: "max"},
			wantFallback: true, wantReason: config.CodexResolutionCatalogUnknown,
		},
		{
			name: "ultra effort capped by the catalog", quality: resolveUltraQuality(),
			catalog: func(t *testing.T) []byte {
				return resolveCatalogSnapshot(t, resolveCatalogModel{"gpt-6-astra", resolveEffortsToXHigh})
			},
			agent: "planner", fallbackTier: "opus", effort: "max",
			want:         config.CodexProfile{Model: "gpt-6-astra", Effort: "xhigh"},
			wantFallback: true, wantReason: config.CodexResolutionEffortUnavailable,
		},
		{
			name: "ultra without a compatible model leaves the runtime default", quality: resolveUltraQuality(),
			catalog: func(t *testing.T) []byte {
				return resolveCatalogSnapshot(t, resolveCatalogModel{"other-model", []string{"medium"}})
			},
			agent: "annotator", fallbackTier: "sonnet", effort: "medium",
			wantFallback: true, wantReason: config.CodexResolutionRuntimeDefault,
		},
		{
			name: "custom tier keeps the tier ladder", quality: resolveCustomExecutorQuality(),
			catalog: resolveCatalogA, agent: "executor", fallbackTier: "sonnet", effort: "medium",
			want:       config.CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"},
			wantReason: config.CodexResolutionSupported,
		},
		{
			name: "codex provider override decides the codex column", quality: resolveCodexOverrideQuality(),
			catalog: resolveCatalogA, agent: "annotator", fallbackTier: "sonnet", effort: "medium",
			want:       config.CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"},
			wantReason: config.CodexResolutionSupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// When
			resolution, err := ResolveAgentProfile(tt.quality, tt.catalog(t), tt.agent, tt.fallbackTier, tt.effort)

			// Then
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, resolution.Effective)
			assert.Equal(t, tt.wantFallback, resolution.Fallback)
			assert.Equal(t, tt.wantReason, resolution.Reason)
		})
	}
}

// An unverified placement still names why: the render context reports the
// catalog error, and the composition table marks the cell unconfirmed.
func TestResolveAgentProfile_UnreadableCatalog_KeepsCatalogError(t *testing.T) {
	t.Parallel()

	resolution, err := ResolveAgentProfile(resolveBalancedQuality(), []byte("not json"), "executor", "sonnet", "medium")

	require.NoError(t, err)
	assert.False(t, resolution.Fallback)
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, resolution.Requested)
	assert.Equal(t, resolution.Requested, resolution.Effective)
	assert.Error(t, resolution.CatalogError)
}

func TestResolveSupervisorProfile_QualityPolicy_ReturnsManagedRootProfile(t *testing.T) {
	t.Parallel()

	managed := func(quality config.QualityConf) config.QualityConf {
		quality.SupervisorModelPolicy = config.SupervisorModelPolicyQuality
		return quality
	}
	tests := []struct {
		name         string
		quality      config.QualityConf
		catalog      func(*testing.T) []byte
		wantManaged  bool
		want         config.CodexProfile
		wantFallback bool
		wantReason   config.CodexResolutionReason
	}{
		{
			name: "ultra root session confirmed", quality: managed(resolveUltraQuality()), catalog: resolveCatalogA,
			wantManaged: true, want: config.CodexProfile{Model: "gpt-6-astra", Effort: "ultra"},
			wantReason: config.CodexResolutionSupported,
		},
		{
			name: "balanced root session confirmed", quality: managed(resolveBalancedQuality()), catalog: resolveCatalogA,
			wantManaged: true, want: config.CodexProfile{Model: "gpt-6-astra", Effort: "xhigh"},
			wantReason: config.CodexResolutionSupported,
		},
		{
			name: "ultra root session substituted", quality: managed(resolveUltraQuality()), catalog: resolveCatalogD,
			wantManaged: true, want: config.CodexProfile{Model: "gpt-6.1-sol", Effort: "max"},
			wantFallback: true, wantReason: config.CodexResolutionModelUnavailable,
		},
		{
			name: "ultra root session without catalog", quality: managed(resolveUltraQuality()),
			catalog:     func(*testing.T) []byte { return nil },
			wantManaged: true, want: config.CodexProfile{Model: "gpt-5.6-sol", Effort: "ultra"},
			wantFallback: true, wantReason: config.CodexResolutionCatalogUnknown,
		},
		{
			name: "inherit policy leaves the root session alone", quality: resolveBalancedQuality(),
			catalog: resolveCatalogA,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolution, ok := ResolveSupervisorProfile(tt.quality, tt.catalog(t))

			assert.Equal(t, tt.wantManaged, ok)
			if !tt.wantManaged {
				assert.Equal(t, config.CodexProfileResolution{}, resolution)
				return
			}
			assert.Equal(t, tt.want, resolution.Effective)
			assert.Equal(t, tt.wantFallback, resolution.Fallback)
			assert.Equal(t, tt.wantReason, resolution.Reason)
		})
	}
}
