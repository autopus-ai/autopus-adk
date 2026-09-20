package promptlayer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderRejectsDuplicateLayerIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		next Layer
	}{
		{"different content", Layer{ID: "rules", Kind: KindStable, Content: "changed rules"}},
		{"identical layer", Layer{ID: "rules", Kind: KindStable, Content: "project rules"}},
		{"different kind", Layer{ID: "rules", Kind: KindEphemeral, Content: "task rules"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := Render([]Layer{
				{ID: "first", Kind: KindStable, Content: "already rendered"},
				{ID: "rules", Kind: KindStable, Content: "project rules"},
				tt.next,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "duplicate prompt layer id")
			assert.Equal(t, RenderResult{}, result)
		})
	}
}

func TestRenderUniqueLayerIDsPreserveEqualContent(t *testing.T) {
	t.Parallel()

	result, err := Render([]Layer{
		{ID: "rules-a", Kind: KindStable, Content: "same content"},
		{ID: "rules-b", Kind: KindStable, Content: "same content"},
	})
	require.NoError(t, err)
	assert.Equal(t, "same content\n\nsame content", result.Prompt)
	assert.Equal(t, []string{"rules-a", "rules-b"}, manifestIDs(result.Manifest))
}
