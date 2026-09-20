package adapter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/templates"
	"github.com/stretchr/testify/require"
)

func TestTaskTriageGeneratedRouterParity(t *testing.T) {
	surfaces := generateContextEngineeringSurfaces(t)
	routes := map[string]string{
		"claude":   ".claude/skills/auto/SKILL.md",
		"codex":    ".codex/skills/codex-auto/SKILL.md",
		"opencode": ".agents/skills/auto/SKILL.md",
		"omp":      ".omp/skills/auto/SKILL.md",
		"gemini":   ".gemini/skills/auto/SKILL.md",
	}
	for name, rel := range routes {
		t.Run(name, func(t *testing.T) {
			s, ok := surfaces[name]
			require.True(t, ok)
			data, err := os.ReadFile(filepath.Join(s.root, rel))
			require.NoError(t, err)
			body := string(data)
			for _, want := range []string{"auto workflow triage", "inline", "guided", "planned", "--solo"} {
				require.Contains(t, body, want)
			}
		})
	}
}

func TestRootGuidelinesChooseExecutionDepth(t *testing.T) {
	body := templates.RootDocGuidelines()
	for _, want := range []string{"risk", "uncertainty", "inline", "gate"} {
		require.True(t, strings.Contains(strings.ToLower(body), want), want)
	}
}
