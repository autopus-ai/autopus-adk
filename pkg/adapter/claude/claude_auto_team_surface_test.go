package claude_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
)

// SHA-256 of the generated skills outside the sections this SPEC owns. The
// auto-go digest is the SPEC-TEAMAUTO-001 base (33b5bb55) without its Route B
// section: the 2026-10-17 team-tool update rewrote Route B on purpose, so Route
// B is pinned on its own below. The agent-teams digest is taken after that
// update, which made the shared task list optional throughout its lifecycle.
// A later SPEC that edits one of these surfaces on purpose updates the digest:
// the 2026-10-24 learn-store fix changed the Phase 0.8 learning query to
// `--top 5 --mark-reused`, and the 2026-10-29 qualityloop removal dropped
// `qualityloop` from the candidate-only signal line.
const (
	baseAutoGoWithoutRouteBSHA256 = "5f80aea8ead780526b5ed899d53ef438135cd92411292430d7f8a70914130e03"
	agentTeamsOutsideEntrySHA256  = "4b9cb1206d5b3d7335b933295b1fa31ec8d87e4d21ca425edc514f91fb151bb2"

	autoTeamSectionHeading = "#### Route A Phase 2 Entry: Agent Team Auto-Selection"
	routeAHeading          = "#### Route A: Supervisor-Led Pipeline"
	routeBHeading          = "#### Route B: Native Agent Team"
	automaticEntryHeading  = "## Automatic Entry from Route A"

	// autoTeamSnippet is the Skill Surface Contract snippet, byte for byte.
	autoTeamSnippet = `if ! auto workflow triage --topology --facts-json "$FACTS" --format json >"$TEAM_JSON" 2>/dev/null || [ ! -s "$TEAM_JSON" ]; then
  printf '%s\n' 'Agent Team: off — auto workflow triage --topology failed; running subagents'
  : >"$TEAM_JSON"
fi
rm -f "$FACTS"`
)

// generateAutoTeamSkills returns the generated auto-go and agent-teams skills.
func generateAutoTeamSkills(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	_, err := claude.NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("teamauto"))
	require.NoError(t, err)
	autoGo, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "auto-go", "SKILL.md"))
	require.NoError(t, err)
	agentTeams, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "agent-teams", "SKILL.md"))
	require.NoError(t, err)
	return string(autoGo), string(agentTeams)
}

// between returns doc from the start marker up to, not including, the end marker.
func between(t *testing.T, doc, start, end string) string {
	t.Helper()
	i := strings.Index(doc, start)
	require.NotEqual(t, -1, i, start)
	j := strings.Index(doc[i+len(start):], end)
	require.NotEqual(t, -1, j, end)
	return doc[i : i+len(start)+j]
}

func digest(text string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(text, "\r\n", "\n")))
	return hex.EncodeToString(sum[:])
}

func TestAutoTeamSurfaceKeepsExplicitTopologySectionsByteIdentical(t *testing.T) {
	t.Parallel()
	autoGo, _ := generateAutoTeamSkills(t)
	sections := []struct{ name, start, end, sha256 string }{
		{"Step 0", "### Step 0: Parse Flags", "### Step 1: Load SPEC", "cde46758f6d1e6a5c1877c79f3766910dcfa5a504c56563b82cc85b6174efd9c"},
		// Route B after the 2026-10-17 team-tool update; the base digest was f34ab9d6.
		{"Route B", routeBHeading, "#### Route C: Single Session", "a4918c8749118dfc19b5f99c914ba80d1c26c55e3d445e3f0baad67cfde59653"},
		{"Route C", "#### Route C: Single Session", "#### Route: Deterministic Workflow", "9e3144a3280b3546f316a62870fa2062998281c47138f15e16dfe4d82e943692"},
		{"Deterministic Workflow", "#### Route: Deterministic Workflow", "#### Subagent Pipeline Phases (Route A)", "4684b7c8ea62e962caeb6c404e741789758e3c9a65567c4890de9bcce4e483d6"},
	}
	for _, section := range sections {
		body := between(t, autoGo, section.start, section.end)
		assert.Equal(t, section.sha256, digest(body), "%s changed:\n%s", section.name, body)
	}
	step0 := between(t, autoGo, "### Step 0: Parse Flags", "### Step 1: Load SPEC")
	assert.Contains(t, step0, "[TOPOLOGY ERROR] --team, --workflow, and --solo are mutually exclusive; choose only one.")
	routeB := between(t, autoGo, routeBHeading, "#### Route C: Single Session")
	assert.Contains(t, routeB, "2. `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` is present in the current session.")
	assert.Contains(t, routeB, "If preflight fails, report the blocker and suggest Route A.")
	// Current Claude Code releases often lack the task-list tools, so --team
	// requires only SendMessage and the Agent name parameter.
	assert.Contains(t, routeB, "4. The `Agent` tool exposes its `name` parameter and the `SendMessage` schema is loaded.")
	assert.Contains(t, routeB, "are optional. Request them in the same ToolSearch call as `SendMessage` and use")
	assert.Contains(t, routeB, "send each live teammate a `shutdown_request` through")
	assert.NotContains(t, routeB, "shared task-list tools are loaded")
	assert.NotContains(t, routeB, "team_name")
}

func TestAutoTeamSurfaceAddsOnlyTheNewSections(t *testing.T) {
	t.Parallel()
	autoGo, agentTeams := generateAutoTeamSkills(t)
	section := between(t, autoGo, autoTeamSectionHeading, routeBHeading)
	routeB := between(t, autoGo, routeBHeading, "#### Route C: Single Session")
	outside := strings.Replace(strings.Replace(autoGo, section, "", 1), routeB, "", 1)
	assert.Equal(t, baseAutoGoWithoutRouteBSHA256, digest(outside), "auto-go changed outside the new section and Route B")
	entry := between(t, agentTeams, automaticEntryHeading, "## Failure Policy")
	assert.Equal(t, agentTeamsOutsideEntrySHA256, digest(strings.Replace(agentTeams, entry, "", 1)), "agent-teams changed outside the new section")
	for _, item := range []string{"1. Send each live teammate a `shutdown_request` through `SendMessage`.", "| Task-list tools unavailable | Continue; the lead keeps the assignment table itself. |", "so a team works without a"} {
		assert.Contains(t, agentTeams, item)
	}
	assert.NotContains(t, agentTeams, "--topology")
	assert.NotContains(t, agentTeams, "auto workflow triage")
	for _, item := range []string{"one ready wave of Phase 2", "No `tester` or\n  `guardian` teammate", "a fallback, not a stop", "separate Claude session", "experimental", "`workflow.auto_team: false`", "`--solo`", "auto 0.50.113 through 0.50.125 refuse", "auto 0.50.112 and older ignore it", "`CLAUDE_CODE_EXECPATH`", "such as a subagent, on subagents.", "`Agent` tool has no `name` parameter", "Missing task-list tools"} {
		assert.Contains(t, entry, item)
	}
}

func TestAutoTeamSurfaceSectionOrder(t *testing.T) {
	t.Parallel()
	autoGo, _ := generateAutoTeamSkills(t)
	routeA := strings.Index(autoGo, routeAHeading)
	heading := strings.Index(autoGo, autoTeamSectionHeading)
	routeB := strings.Index(autoGo, routeBHeading)
	require.True(t, routeA >= 0 && routeA < heading && heading < routeB, "the section must sit between Route A and Route B")
	section := between(t, autoGo, autoTeamSectionHeading, routeBHeading)

	firstRule := section[strings.Index(section, "\n1. "):]
	assert.True(t, strings.HasPrefix(firstRule, "\n1. This section runs only when `TOPOLOGY_FLAG_COUNT == 0`"), "the first rule is the topology gate")
	ordered := []string{
		"`TOPOLOGY_FLAG_COUNT == 0`",
		"A `--continue` run that resumes from a checkpoint where\n   Phase 2 already finished skips this section and prints no `Agent Team:` line.",
		"in ONE Bash tool call from `WORKING_DIR`, because shell\n   variables such as `FACTS` do not survive between Bash tool calls.",
		"never use `echo` or an unquoted heredoc.",
		"```bash\ncat >\"$FACTS\" <<'JSON'\n",
		"```sh\n" + autoTeamSnippet + "\n```",
		"(d) Print the decision with `cat \"$TEAM_JSON\"`",
		"If `TEAM_JSON` is empty, the snippet already printed the fixed failure line.",
		"`select:SendMessage,TaskCreate,TaskUpdate,TaskList,TaskGet`",
		// Only SendMessage and the Agent name parameter are required.
		"only `SendMessage` and the `name` parameter of the `Agent` tool",
		"task-list tools are optional",
		"print `decision.tools_fallback_line` instead of `decision.line`",
		// The CLI cannot tell a subagent apart, so the schema check keeps one on subagents.
		"The CLI cannot tell a subagent from the top-level",
		"such as a subagent, on subagents.",
		"without them the lead tracks the units itself.",
		// S15 smoke: the lead paraphrased the line, so printing is spelled out.
		"copy the JSON string value verbatim as its own line",
		"write `Agent Team:` nowhere else in the run,\nincluding the completion report.",
		"The message that starts Phase 2 carries\nthis line as its text, ahead of its tool call:",
		"or the first edit or command when the lead implements a unit inline.",
		"send every teammate a shutdown request and wait for every\nacknowledgement.",
		"implement exactly the `decision.followup_units`",
		"Only after that, enter Gate 2",
	}
	at := 0
	for _, item := range ordered {
		next := strings.Index(section[at:], item)
		require.NotEqual(t, -1, next, "missing or out of order: %q", item)
		at += next + len(item)
	}
}
