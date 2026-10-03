package loop

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

const scenarioRel = ".autopus/qa/scenarios/login.yaml"

const loginScenario = `schema_version: qamesh.scenario.v2
id: login
title: A member signs in
journey: browser-staging
intent_source: acceptance
spec: SPEC-LOGIN-001
acceptance_refs: [AC-LOGIN-001]
screens:
  - id: sign-in
    path: /login
    steps:
      - click:
          role: button
          name: Sign in
        ac: AC-LOGIN-001
      - expect_text: Welcome back
        ac: AC-LOGIN-001
`

func healInput(after string, extra ...string) GuardInput {
	return GuardInput{
		Class:   triage.ClassTestDrift,
		Paths:   append([]string{scenarioRel}, extra...),
		TestDir: "e2e",
		Before: func(rel string) ([]byte, bool) {
			return []byte(loginScenario), rel == scenarioRel
		},
		After: func(rel string) ([]byte, bool) {
			return []byte(after), rel == scenarioRel
		},
	}
}

// AC-QALOOP-010: a heal may move a locator but never an oracle.
func TestQALoopGuard_HealKeepsTheOracle(t *testing.T) {
	t.Parallel()
	retargeted := strings.Replace(loginScenario, "name: Sign in", "name: Log in", 1)
	cases := []struct {
		name   string
		in     GuardInput
		path   string
		reason string
	}{
		{name: "changed click target is accepted", in: healInput(retargeted)},
		{name: "changed expect_text", in: healInput(strings.Replace(loginScenario, "Welcome back", "Hello", 1)), path: scenarioRel, reason: "expect step 1 (expect_text, ac AC-LOGIN-001)"},
		{name: "dropped expect ac", in: healInput(strings.Replace(loginScenario, "Welcome back\n        ac: AC-LOGIN-001\n", "Welcome back\n", 1)), path: scenarioRel, reason: "expect step 1"},
		{name: "dropped action ac", in: healInput(strings.Replace(loginScenario, "Sign in\n        ac: AC-LOGIN-001\n", "Log in\n", 1)), path: scenarioRel, reason: "ac of an action step"},
		{name: "removed expect step", in: healInput(strings.Replace(loginScenario, "      - expect_text: Welcome back\n        ac: AC-LOGIN-001\n", "", 1)), path: scenarioRel, reason: "expect step count (1 -> 0)"},
		{name: "changed acceptance_refs", in: healInput(strings.Replace(loginScenario, "[AC-LOGIN-001]", "[AC-LOGIN-002]", 1)), path: scenarioRel, reason: "acceptance_refs"},
		{name: "changed intent_source", in: healInput(strings.Replace(loginScenario, "intent_source: acceptance", "intent_source: baseline", 1)), path: scenarioRel, reason: "intent_source"},
		{name: "changed screen path", in: healInput(strings.Replace(loginScenario, "path: /login", "path: /signin", 1)), path: scenarioRel, reason: "screen 1"},
		{name: "edit under src", in: healInput(retargeted, "src/app.ts"), path: "src/app.ts", reason: "a heal may edit only .autopus/qa/scenarios/*.yaml"},
		{name: "unparseable heal", in: healInput("screens: [unclosed"), path: scenarioRel, reason: "does not parse"},
		{name: "new scenario file", in: GuardInput{Class: triage.ClassTestDrift, Paths: []string{".autopus/qa/scenarios/new.yaml"},
			Before: func(string) ([]byte, bool) { return nil, false }, After: func(string) ([]byte, bool) { return []byte(loginScenario), true }},
			path: ".autopus/qa/scenarios/new.yaml", reason: "may not add a scenario"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verdict := Guard(tc.in)
			if tc.reason == "" {
				assert.True(t, verdict.Accepted, verdict.Reason)
				return
			}
			assert.False(t, verdict.Accepted)
			assert.Equal(t, tc.path, verdict.Path)
			assert.Contains(t, verdict.Reason, tc.reason)
		})
	}
}

// AC-QALOOP-010/011: each class may touch only its allowlist.
func TestQALoopGuard_ClassAllowlists(t *testing.T) {
	t.Parallel()
	cases := []struct {
		class triage.Class
		path  string
		ok    bool
	}{
		{triage.ClassProductDefect, "src/app.ts", true},
		{triage.ClassProductDefect, "e2e/login.spec.ts", false},
		{triage.ClassProductDefect, "pkg/auth/login_test.go", false},
		{triage.ClassProductDefect, "web/__tests__/form.tsx", false},
		{triage.ClassProductDefect, "src/form.test.tsx", false},
		{triage.ClassProductDefect, "qa/journeys/login.ts", true},
		{triage.ClassProductDefect, ".autopus/specs/SPEC-LOGIN-001/spec.md", false},
		{triage.ClassProductDefect, ".autopus/qa/test-scenarios/SPEC-LOGIN-001.yaml", false},
		{triage.ClassProductDefect, "playwright.config.ts", false},
		{triage.ClassProductDefect, "../shared/lib.ts", false},
		{triage.ClassTestDefect, "e2e/login.spec.ts", true},
		{triage.ClassTestDefect, "tests/helpers/session.ts", true},
		{triage.ClassTestDefect, "src/app.ts", false},
		{triage.ClassTestDefect, ".autopus/qa/scenarios/login.yaml", false},
		{triage.ClassTestDefect, ".autopus/specs/SPEC-LOGIN-001/acceptance.md", false},
		{triage.ClassTestDrift, ".autopus/qa/scenarios/candidates/login.yaml", false},
		{triage.ClassTestDrift, "e2e/autopus-generated/login.spec.ts", false},
		{triage.ClassEnvironment, "src/app.ts", false},
	}
	for _, tc := range cases {
		verdict := Guard(GuardInput{Class: tc.class, Paths: []string{tc.path}, TestDir: "e2e"})
		assert.Equal(t, tc.ok, verdict.Accepted, "%s %s: %s", tc.class, tc.path, verdict.Reason)
		if !tc.ok {
			assert.Equal(t, tc.path, verdict.Path)
		}
	}
	assert.False(t, Guard(GuardInput{Class: triage.ClassProductDefect}).Accepted, "an empty change set is never a fix")
}
