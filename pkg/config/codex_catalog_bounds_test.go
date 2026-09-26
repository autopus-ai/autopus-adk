package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCodexModelCatalogPayloadAcceptsBoundedCatalog(t *testing.T) {
	t.Parallel()

	err := ValidateCodexModelCatalogPayload([]byte(`{
		"models":[{
			"slug":"gpt-6-sol",
			"default_reasoning_level":"xhigh",
			"supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"}]
		}]
	}`))
	require.NoError(t, err)
}

func TestValidateCodexModelCatalogPayloadRejectsResourceExhaustionInputs(t *testing.T) {
	t.Parallel()

	models := make([]string, MaxCodexCatalogModels+1)
	for i := range models {
		models[i] = fmt.Sprintf(`{"slug":"model-%d","supported_reasoning_levels":[{"effort":"low"}]}`, i)
	}
	efforts := make([]string, MaxCodexCatalogReasoningLevels+1)
	for i := range efforts {
		efforts[i] = `{"effort":"low"}`
	}

	tests := []struct {
		name    string
		payload []byte
		want    string
	}{
		{
			name:    "payload bytes",
			payload: []byte(strings.Repeat("x", MaxCodexModelCatalogBytes+1)),
			want:    "exceeds",
		},
		{
			name:    "model count",
			payload: []byte(`{"models":[` + strings.Join(models, ",") + `]}`),
			want:    "models",
		},
		{
			name:    "slug length",
			payload: []byte(`{"models":[{"slug":"` + strings.Repeat("s", MaxCodexCatalogSlugBytes+1) + `","supported_reasoning_levels":[{"effort":"low"}]}]}`),
			want:    "slug",
		},
		{
			name:    "reasoning level count",
			payload: []byte(`{"models":[{"slug":"model","supported_reasoning_levels":[` + strings.Join(efforts, ",") + `]}]}`),
			want:    "reasoning levels",
		},
		{
			name:    "effort length",
			payload: []byte(`{"models":[{"slug":"model","supported_reasoning_levels":[{"effort":"` + strings.Repeat("e", MaxCodexCatalogEffortBytes+1) + `"}]}]}`),
			want:    "effort",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCodexModelCatalogPayload(tt.payload)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestResolveCodexProfileRejectsOversizedCatalogDirectly(t *testing.T) {
	t.Parallel()

	requested := CodexProfile{Model: CodexSolModel, Effort: CodexEffortUltra}
	resolution := ResolveCodexProfile(requested, []byte(strings.Repeat("x", MaxCodexModelCatalogBytes+1)))

	assert.Equal(t, CodexResolutionCatalogUnknown, resolution.Reason)
	assert.Equal(t, CodexProfile{Model: CodexLegacyModel, Effort: CodexEffortXHigh}, resolution.Effective)
	require.Error(t, resolution.CatalogError)
	assert.Contains(t, resolution.CatalogError.Error(), "exceeds")
}
