package antigravity

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowContextProfiles_GeneratedDetails_MatchS8Matrix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("context-profile"))
	require.NoError(t, err)

	tests := []struct {
		route    string
		required []string
		excluded []string
	}{
		{
			route: "test",
			required: []string{
				"Required: core,test", "Optional: signature,learning", "Excluded: canary",
				"Required: core workspace policy and scenarios.", "Scenarios map to the test context.",
			},
			excluded: []string{
				"Excluded: canary, signatures, and unrelated learnings.",
				"Required: core workspace policy and canary.",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.route, func(t *testing.T) {
			detail := readGeneratedAntigravitySurface(t, root, filepath.Join(".gemini", "skills", "autopus", "auto-"+tt.route, "SKILL.md"))
			profile := antigravityContextProfileSection(t, detail)
			for _, token := range tt.required {
				assert.Contains(t, profile, token)
			}
			for _, token := range tt.excluded {
				assert.NotContains(t, profile, token)
			}
			if tt.route == "canary" {
				assert.NotContains(t, detail, "scenarios.md", "canary detail must not unconditionally load scenarios")
			}
		})
	}
}

func antigravityContextProfileSection(t *testing.T, detail string) string {
	t.Helper()
	const startMarker = "## Context Profile"
	start := strings.Index(detail, startMarker)
	require.GreaterOrEqual(t, start, 0)
	rest := detail[start+len(startMarker):]
	relEnd := strings.Index(rest, "\n## ")
	require.GreaterOrEqual(t, relEnd, 0)
	return detail[start : start+len(startMarker)+relEnd]
}
