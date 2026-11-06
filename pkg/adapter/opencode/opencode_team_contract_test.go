package opencode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeOpenCodeExplicitTeamRequiresLiveCapability(t *testing.T) {
	t.Parallel()
	body := normalizeOpenCodeMarkdown("- `--team`: `.codex/skills/codex-agent-teams/SKILL.md`의 Codex team profile 적용")
	assert.Contains(t, body, "current runtime")
	assert.Contains(t, body, "unsupported-mode")
	assert.Contains(t, body, "do not substitute")
	assert.NotContains(t, body, "별도 native team을 만들지 않고")
}
