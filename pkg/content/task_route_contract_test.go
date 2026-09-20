package content_test

import (
	"strings"
	"testing"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/templates"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRouterTriagePreservesExplicitIntentAndGates(t *testing.T) {
	data, err := templates.FS.ReadFile("claude/commands/auto-router.md.tmpl")
	require.NoError(t, err)
	body := string(data)
	for _, contract := range []string{
		"auto workflow triage --facts-json <file> --format json",
		"inline", "guided", "planned", "read-only", "not an authorization",
		"Do not reroute explicit", "Do not lower the model", "--solo",
		"mandatory security, UX, and coverage gates",
		"If the CLI is unavailable", "trivial question",
		"Load exactly one detail", "Pass all arguments and flags unchanged",
	} {
		require.Contains(t, body, contract)
	}
	start := strings.Index(body, "## Task Triage")
	end := strings.Index(body[start+1:], "\n## ")
	require.Greater(t, start, 0)
	require.Greater(t, end, 0)
	require.LessOrEqual(t, len(strings.Split(body[start:start+1+end], "\n")), 25)
	section := body[start : start+1+end]
	for _, path := range []string{"codex/prompts/auto.md.tmpl", "gemini/commands/auto-router.md.tmpl"} {
		data, err := templates.FS.ReadFile(path)
		require.NoError(t, err)
		require.Contains(t, string(data), section, "platform triage policy drift: %s", path)
	}
}

func TestDelegationTriageRemainsEvidenceAndCapacityBound(t *testing.T) {
	data, err := contentfs.FS.ReadFile("skills/references/agent-pipeline/delegation.md")
	require.NoError(t, err)
	for _, contract := range []string{
		"auto workflow triage --facts-json <file> --format json",
		"does not establish native capacity", "independent ready work",
		"required gates", "--solo", "requested model",
	} {
		require.Contains(t, string(data), contract)
	}
}
