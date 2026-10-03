package loop

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// view serves fixed file contents as a guard Before or After function.
func view(files map[string]string) func(string) ([]byte, bool) {
	return func(rel string) ([]byte, bool) {
		body, ok := files[rel]
		return []byte(body), ok
	}
}

// Review findings 1 and 2: the ignore and attribute rules decide what git
// reports, and a step map ties a failing line to its oracle, so no repair
// class may touch them. Only the harness writes generated specs.
func TestQALoopGuard_RulesAndHarnessOutputAreOffLimits(t *testing.T) {
	t.Parallel()
	everyClass := []triage.Class{triage.ClassProductDefect, triage.ClassTestDefect, triage.ClassTestDrift}
	for _, class := range everyClass {
		for _, rel := range []string{".gitignore", "web/.gitignore", ".gitattributes", ".git/info/exclude",
			"e2e/login.spec.map.json", "e2e/autopus-generated/login.spec.map.json"} {
			verdict := Guard(GuardInput{Class: class, Paths: []string{rel}, TestDir: "e2e"})
			assert.False(t, verdict.Accepted, "%s %s", class, rel)
			assert.Equal(t, rel, verdict.Path, "%s %s", class, rel)
			assert.Contains(t, verdict.Reason, "no repair may edit", "%s %s", class, rel)
		}
	}
	for _, class := range []triage.Class{triage.ClassProductDefect, triage.ClassTestDefect} {
		verdict := Guard(GuardInput{Class: class, Paths: []string{"e2e/autopus-generated/login.spec.ts"}, TestDir: "e2e"})
		assert.False(t, verdict.Accepted, class)
		assert.Contains(t, verdict.Reason, "autopus-generated", class)
	}
}

const pkgJSON = `{"name":"shop","scripts":{"test":"playwright test","build":"vite build"},"dependencies":{"react":"18.0.0"}}`

const pyproject = `[project]
name = "shop"
dependencies = ["flask>=3"]

[tool.pytest.ini_options]
addopts = "-q"
testpaths = ["tests"]
`

// Review finding 3: a product fix may not rewire how the tests run.
func TestQALoopGuard_ProductFixMayNotRewireTheTestRunner(t *testing.T) {
	t.Parallel()
	head := map[string]string{"package.json": pkgJSON, "pyproject.toml": pyproject}
	cases := []struct {
		name, path, after string
		ok                bool
	}{
		{"jest config", "jest.config.ts", "", false},
		{"vitest config", "web/vitest.config.mts", "", false},
		{"vitest workspace", "vitest.workspace.ts", "", false},
		{"mocharc", ".mocharc.yml", "", false},
		{"karma", "karma.conf.js", "", false},
		{"cypress", "cypress.config.ts", "", false},
		{"pytest.ini", "pytest.ini", "", false},
		{"setup.cfg", "setup.cfg", "", false},
		{"tox.ini", "tox.ini", "", false},
		{"root conftest", "conftest.py", "", false},
		{"nested conftest", "src/conftest.py", "", false},
		{"Makefile", "Makefile", "", false},
		{"go.mod stays allowed", "go.mod", "", true},
		{"package.json scripts", "package.json", strings.Replace(pkgJSON, `"playwright test"`, `"playwright test --grep-invert login"`, 1), false},
		{"package.json jest key", "package.json", strings.Replace(pkgJSON, `"dependencies"`, `"jest":{"testPathIgnorePatterns":["login"]},"dependencies"`, 1), false},
		{"package.json dependency", "package.json", strings.Replace(pkgJSON, "18.0.0", "18.2.0", 1), true},
		{"package.json unparseable", "package.json", `{"name":`, false},
		{"pyproject pytest settings", "pyproject.toml", strings.Replace(pyproject, `addopts = "-q"`, `addopts = "-q -k 'not login'"`, 1), false},
		{"pyproject dependency", "pyproject.toml", strings.Replace(pyproject, `["flask>=3"]`, `["flask>=3", "requests>=2"]`, 1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			after := map[string]string{tc.path: tc.after}
			verdict := Guard(GuardInput{Class: triage.ClassProductDefect, Paths: []string{tc.path}, TestDir: "e2e",
				Before: view(head), After: view(after)})
			assert.Equal(t, tc.ok, verdict.Accepted, verdict.Reason)
			if !tc.ok {
				assert.Equal(t, tc.path, verdict.Path)
			}
		})
	}
}

// Review finding 6: the guard uses the same test-path predicate as triage.
func TestQALoopGuard_SharedTestPathPredicate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		class triage.Class
		path  string
		ok    bool
	}{
		{triage.ClassProductDefect, "app/test_login.py", false},
		{triage.ClassProductDefect, "app/login_test.py", false},
		{triage.ClassProductDefect, "lib/user_spec.rb", false},
		{triage.ClassProductDefect, "spec/support/helpers.rb", false},
		{triage.ClassTestDefect, "app/test_login.py", true},
		{triage.ClassTestDefect, "spec/models/user_spec.rb", true},
		{triage.ClassTestDefect, "app/login.py", false},
	}
	for _, tc := range cases {
		verdict := Guard(GuardInput{Class: tc.class, Paths: []string{tc.path}, TestDir: "e2e"})
		assert.Equal(t, tc.ok, verdict.Accepted, "%s %s: %s", tc.class, tc.path, verdict.Reason)
	}
}

const healScenario = `schema_version: qamesh.scenario.v2
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
      - fill:
          label: Email
          value: member@example.com
      - select:
          label: Plan
          option: pro
      - press:
          key: Enter
      - click:
          role: button
          name: Sign in
        ac: AC-LOGIN-001
      - wait_url: /home
      - expect_text: Welcome back
        ac: AC-LOGIN-001
`

// Review finding 4: a heal may move a locator, never what an action does.
func TestQALoopGuard_HealMayMoveOnlyLocators(t *testing.T) {
	t.Parallel()
	edit := func(old, new string) string { return strings.Replace(healScenario, old, new, 1) }
	fill := "      - fill:\n          label: Email\n          value: member@example.com\n"
	sel := "      - select:\n          label: Plan\n          option: pro\n"
	cases := []struct{ name, after, reason string }{
		{"fill label", edit("label: Email", "label: Email address"), ""},
		{"click by test id", edit("role: button\n          name: Sign in", "test_id: sign-in"), ""},
		{"fill value", edit("value: member@example.com", "value: admin@example.com"), "action step 1 (fill)"},
		{"fill value to env", edit("value: member@example.com", "value_env: QA_EMAIL"), "action step 1 (fill)"},
		{"select option", edit("option: pro", "option: free"), "action step 2 (select)"},
		{"press key", edit("key: Enter", "key: Escape"), "action step 3 (press)"},
		{"wait_url", edit("wait_url: /home", "wait_url: /admin"), "action step 5 (wait_url)"},
		{"removed action", edit("      - press:\n          key: Enter\n", ""), "step count (6 -> 5)"},
		{"click became check", edit("      - click:", "      - check:"), "step 4 from click to check"},
		{"reordered actions", edit(fill+sel, sel+fill), "step 1 from fill to select"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verdict := Guard(GuardInput{Class: triage.ClassTestDrift, Paths: []string{scenarioRel}, TestDir: "e2e",
				Before: view(map[string]string{scenarioRel: healScenario}), After: view(map[string]string{scenarioRel: tc.after})})
			if tc.reason == "" {
				assert.True(t, verdict.Accepted, verdict.Reason)
				return
			}
			assert.False(t, verdict.Accepted)
			assert.Contains(t, verdict.Reason, tc.reason)
		})
	}
}

// A test fix may edit tests, but not the runner configuration that decides
// which tests run, even when that configuration lives in a test directory.
func TestQALoopGuard_TestFixMayNotRewireTheTestRunner(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"e2e/playwright.config.ts", "tests/conftest.py", "e2e/vitest.config.ts", "tests/pytest.ini"} {
		verdict := Guard(GuardInput{Class: triage.ClassTestDefect, Paths: []string{path}, TestDir: "e2e",
			Before: view(map[string]string{}), After: view(map[string]string{path: "x"})})
		assert.False(t, verdict.Accepted, path)
		assert.Equal(t, path, verdict.Path, path)
	}
	ok := Guard(GuardInput{Class: triage.ClassTestDefect, Paths: []string{"e2e/login.spec.ts"}, TestDir: "e2e",
		Before: view(map[string]string{}), After: view(map[string]string{"e2e/login.spec.ts": "x"})})
	assert.True(t, ok.Accepted, ok.Reason)
}
