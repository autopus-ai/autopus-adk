package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The required-context-delivery protocol is a route-level step: `auto workflow
// context` builds and verifies the manifest before the first provider call.
// Codex carries it on the go route, which is the surface that runs the command.
// The pipeline skill used to restate the whole protocol because a Codex-only
// override duplicated it there; that copy is gone, so this checks the real
// carrier instead of demanding the duplication back.
func TestCodexAdapter_AutoGoCarriesVerifiedRequiredContextContract(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := codex.NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("context-delivery"))
	require.NoError(t, err)

	autoGo, err := os.ReadFile(filepath.Join(root, ".codex", "skills", "codex-auto-go", "SKILL.md"))
	require.NoError(t, err)
	content := string(autoGo)

	for _, required := range []string{
		// the command that produces and verifies the manifest
		"auto workflow context",
		// the two halves of the declared-document round trip
		"--required-document", "--context-required-document",
		// the worker acknowledgement and what actually decides delivery
		"context_ack", "supervisor가 보유한 필수 reference 집합",
		// fail-closed conditions
		"hash mismatch", "provider를 호출하지 않습니다",
		// only optional recall may be shrunk by a token budget
		"optional recall", "필수 문서를 자르지 말고",
	} {
		assertSurfaceContains(t, content, required)
	}
	assertSurfaceOmits(t, content, "현재 SPEC 구현 흐름은 계속 진행합니다")
}

// The pipeline skill is now an entrypoint: it owns the receipt contract and the
// routes to its own detail, not a copy of every protocol a run touches.
func TestCodexAdapter_AgentPipelineIsAnEntrypointWithNativeSyntax(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := codex.NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("context-delivery"))
	require.NoError(t, err)
	body, err := os.ReadFile(filepath.Join(root, ".codex", "skills", "codex-agent-pipeline", "SKILL.md"))
	require.NoError(t, err)
	content := string(body)

	// The five-field receipt is the pipeline's own contract and stays inline.
	for _, field := range []string{
		"owned_paths", "changed_files", "verification", "blockers", "next_required_step",
	} {
		assertSurfaceContains(t, content, field)
	}

	// Deferred detail must be reachable, or the entrypoint is a dead end.
	for _, resource := range []string{
		"references/phases.md", "references/delegation.md", "references/gates.md",
	} {
		assertSurfaceContains(t, content, resource)
	}

	// No foreign or retired call syntax may reappear in the generated body.
	for _, unsupported := range []string{"fork_context", "agent_type", "Agent(", "subagent_type"} {
		assertSurfaceOmits(t, content, unsupported)
	}
}

func assertSurfaceContains(t *testing.T, content, needle string) {
	t.Helper()
	assert.True(t, strings.Contains(content, needle), "generated surface is missing %q", needle)
}

func assertSurfaceOmits(t *testing.T, content, needle string) {
	t.Helper()
	assert.False(t, strings.Contains(content, needle), "generated surface contains unsupported %q", needle)
}
