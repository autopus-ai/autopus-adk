package adapter_test

import (
	"context"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The delegation reference's Worker hygiene tells a worker cut off at its turn
// limit that "a partial result then follows the skill entry's partial-result
// rule". Every platform that installs that reference beside its agent-pipeline
// entry must carry the rule in the entry, or the pointer leads nowhere.
func TestAgentPipelineEntries_CarryThePartialResultRule(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("partial-result-rule")
	cfg.Platforms = allPlatforms

	for _, platform := range allPlatforms {
		pf, err := platformGenerators[platform](t.TempDir(), context.Background(), cfg)
		require.NoError(t, err)
		bodies := map[string]string{}
		for _, file := range pf.Files {
			bodies[strings.ReplaceAll(file.TargetPath, "\\", "/")] = string(file.Content)
		}
		checked := 0
		for target, body := range bodies {
			if path.Base(target) != "SKILL.md" || !strings.HasSuffix(path.Dir(target), "agent-pipeline") {
				continue
			}
			delegation, ok := bodies[path.Join(path.Dir(target), "references", "delegation.md")]
			if !ok || !strings.Contains(delegation, "partial-result rule") {
				continue
			}
			checked++
			for _, clause := range []string{"partial result", "as a completed phase", "hand the phase to the user"} {
				assert.Contains(t, body, clause, "%s %s must carry the partial-result rule its delegation reference points at",
					platform, target)
			}
		}
		assert.NotZero(t, checked, "%s installs an agent-pipeline entry with its delegation reference", platform)
	}
}
