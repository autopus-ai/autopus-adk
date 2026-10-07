package adapter_test

// SPEC-EDITGUARD-001 S13 (REQ-EG-15): the canonical /auto fix sources and every
// surface regenerated from them carry the reproduction test lock contract in
// the order the workflow runs it.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// fixLockContract is the S13 line sequence: lock after the failing-assertion
// check, no unlock before verification, unlock with JSON at completion, the
// verdict check of Pre-Completion Verification, and the no-test record.
var fixLockContract = []string{
	"auto fix lock -- <test-path>",
	"Do NOT run auto fix unlock before Step 4 verification passes.",
	"auto fix unlock --json -- <test-path>",
	"Lock verdict is unchanged (any other verdict: not complete, stop and ask the user)",
	"lock: not applicable",
}

// assertFixLockContract checks that body holds every contract line and that
// their first occurrences follow the contract order.
func assertFixLockContract(t *testing.T, surface, body string) {
	t.Helper()
	previous := -1
	for _, line := range fixLockContract {
		at := strings.Index(body, line)
		if !assert.GreaterOrEqual(t, at, 0, "%s omits %q", surface, line) {
			return
		}
		assert.Greater(t, at, previous, "%s states %q out of order", surface, line)
		previous = at
	}
}

// The four canonical sources of REQ-EG-15; the Claude source is checked
// inside its fix section, which is what the Claude adapter extracts.
func TestFixLockContract_CanonicalSourcesCarryItInOrder(t *testing.T) {
	t.Parallel()

	workflows := repoRelativeFile(t, "templates/claude/commands/auto-workflows.md.tmpl")
	start := strings.Index(workflows, "## fix —")
	end := strings.Index(workflows, "## map —")
	require.True(t, start >= 0 && end > start, "the Claude source keeps its fix section")
	assertFixLockContract(t, "claude fix section", workflows[start:end])

	for _, rel := range []string{
		"templates/codex/skills/auto-fix.md.tmpl",
		"templates/codex/prompts/auto-fix.md.tmpl",
		"templates/gemini/skills/auto-fix/SKILL.md.tmpl",
	} {
		assertFixLockContract(t, rel, repoRelativeFile(t, rel))
	}
}

// Every /auto fix skill an adapter generates from those sources carries the
// contract: Claude from the workflow section, Codex, OpenCode, and OMP from the
// Codex skill, and both Antigravity skill trees from the Gemini skill.
func TestFixLockContract_GeneratedSkillsCarryItInOrder(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("fix-lock")
	cases := []struct {
		platform string
		generate func(root string) (*adapter.PlatformFiles, error)
		surfaces []string
	}{
		{"claude-code", func(root string) (*adapter.PlatformFiles, error) {
			return claude.NewWithRoot(root).Generate(context.Background(), cfg)
		}, []string{".claude/skills/auto-fix/SKILL.md"}},
		{"codex", func(root string) (*adapter.PlatformFiles, error) {
			return codex.NewWithRoot(root).Generate(context.Background(), cfg)
		}, []string{".codex/skills/codex-auto-fix/SKILL.md"}},
		{"opencode", func(root string) (*adapter.PlatformFiles, error) {
			return opencode.NewWithRoot(root, opencode.WithCLIVersion("2.0.10")).Generate(context.Background(), cfg)
		}, []string{".agents/skills/auto-fix/SKILL.md"}},
		{"omp", func(root string) (*adapter.PlatformFiles, error) {
			return omp.NewWithRoot(root).Generate(context.Background(), cfg)
		}, []string{".omp/skills/auto-fix/SKILL.md"}},
		{"antigravity-cli", func(root string) (*adapter.PlatformFiles, error) {
			return antigravity.NewWithRoot(root).Generate(context.Background(), cfg)
		}, []string{".gemini/skills/autopus/auto-fix/SKILL.md", ".agents/plugins/autopus/skills/auto-fix/SKILL.md"}},
	}
	for _, tc := range cases {
		t.Run(tc.platform, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			_, err := tc.generate(root)
			require.NoError(t, err)
			for _, rel := range tc.surfaces {
				body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
				require.NoError(t, err, "%s generates %s", tc.platform, rel)
				assertFixLockContract(t, tc.platform+" "+rel, string(body))
			}
		})
	}
}
