package generate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
)

const specX = "SPEC-X"

// acceptanceX declares four criteria; AC-X-004 has no THEN line, so the parser
// reports it as missing_then while still listing it.
const acceptanceX = `# SPEC-X — Acceptance

### S1: AC-X-001 — Sign in reaches the dashboard

GIVEN a registered user on the sign-in page
WHEN they submit valid credentials
THEN the dashboard greets them with "Welcome back".

### S2: AC-X-002 — Wrong password is refused

GIVEN a registered user
WHEN they submit a wrong password
THEN the form shows "Invalid credentials".

### S3: AC-X-003 — Idle sessions expire

GIVEN a signed-in user
WHEN the session has been idle for 30 minutes
THEN they are asked to sign in again.

### S4: AC-X-004 — Vague criterion

GIVEN a user
WHEN they look around
`

const validScenarioYAML = `schema_version: qamesh.scenario.v2
id: sign-in-dashboard
title: Sign in reaches the dashboard
journey: browser-gui-explore
intent_source: acceptance
spec: SPEC-X
acceptance_refs: [AC-X-001]
screens:
  - id: sign-in
    path: /login
    steps:
      - fill: {label: Email, value: user@example.com}
      - fill: {label: Password, value_env: E2E_PASSWORD}
      - click: {role: button, name: Sign in}
      - expect_text: Welcome back
        ac: AC-X-001
`

var unknownAcScenarioYAML = strings.NewReplacer(
	"id: sign-in-dashboard", "id: ghost-criterion",
	"AC-X-001", "AC-X-999",
).Replace(validScenarioYAML)

const testScenariosYAML = `schema_version: qamesh.test-scenarios.v1
spec: SPEC-X
cases:
  - id: sign-in-happy
    ac: AC-X-001
    kind: happy
    title: Valid credentials reach the dashboard
    given: a registered user
    when: they sign in
    then: the dashboard greets them
    automation:
      type: gui
      scenario: sign-in-dashboard
  - id: wrong-password
    ac: AC-X-002
    kind: negative
    title: Wrong password is refused
    given: a registered user
    when: they submit a wrong password
    then: the form shows "Invalid credentials"
    automation:
      type: command
      check:
        argv: [go, test, ./auth/...]
  - id: vague-look-around
    ac: AC-X-004
    kind: edge
    title: Vague criterion stays manual
    given: a user
    when: they look around
    then: not stated by the criterion
    automation:
      type: manual
      reason: the criterion states no expected outcome
`

func fenced(docs ...string) string {
	var b strings.Builder
	b.WriteString("Here are the drafts.\n\n")
	for _, doc := range docs {
		b.WriteString("```yaml\n" + doc + "```\n\nNext one:\n")
	}
	return b.String()
}

// projectX lays out a project whose SPEC-X acceptance.md exists.
func projectX(t *testing.T, acceptanceBody string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".autopus", "specs", specX, "acceptance.md"), acceptanceBody)
	return dir
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}

// fakeRunner stands in for the agent CLI and records the request it got.
type fakeRunner struct {
	stdout string
	err    error
	calls  int
	got    agentexec.Request
}

func (f *fakeRunner) Run(_ context.Context, req agentexec.Request) (agentexec.Response, error) {
	f.calls++
	f.got = req
	return agentexec.Response{Stdout: f.stdout}, f.err
}
