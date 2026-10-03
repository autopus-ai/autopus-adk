package triage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

const generatedSpec = "e2e/autopus-generated/login.spec.ts"

// specBody line numbers matter: the step map below keys lines 4, 6, 7, and 9.
const specBody = `import { test, expect } from '@playwright/test';

test('login', async ({ page }) => {
  await page.goto('/login');
  // login step 1
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.waitForURL('**/dashboard');
  // login step 3
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
});
`

const stepMapBody = `{
  "schema_version": "qamesh.stepmap.v1",
  "scenario_id": "login",
  "spec_path": "e2e/autopus-generated/login.spec.ts",
  "lines": {
    "4": {"screen": "login", "index": 0, "kind": "goto"},
    "6": {"screen": "login", "index": 1, "kind": "action", "ac": "AC-LOGIN-001"},
    "7": {"screen": "login", "index": 2, "kind": "action", "ac": "AC-LOGIN-002"},
    "9": {"screen": "login", "index": 3, "kind": "expect", "ac": "AC-LOGIN-003"}
  }
}
`

// driftOutput is list-reporter output for a click that never found its
// button. The stack path went through evidence redaction, so it no longer
// exists on disk and must be resolved through the generated directory. The
// code frame quotes the next line's waitForURL, which must not be read as the
// failure itself.
const driftOutput = `Running 1 test using 1 worker

  1) [chromium] › autopus-generated/login.spec.ts:3:5 › login ──────────────

    Error: locator.click: Test timeout of 30000ms exceeded.
    Call log:
      - waiting for getByRole('button', { name: 'Sign in' })

       4 |   await page.goto('/login');
       5 |   // login step 1
    >  6 |   await page.getByRole('button', { name: 'Sign in' }).click();
         |                                                        ^
       7 |   await page.waitForURL('**/dashboard');

        at /Users/[REDACTED_USER]/shop/e2e/autopus-generated/login.spec.ts:6:56

  1 failed
`

// expectOutput has no stack line: the code-frame marker belongs to the spec
// named in the failure header, which is relative to the Playwright testDir.
const expectOutput = `  1) [chromium] › autopus-generated/login.spec.ts:3:5 › login ──────────────

    Error: Timed out 5000ms waiting for expect(locator).toBeVisible()

    Locator: getByRole('heading', { name: 'Dashboard' })
    Expected: visible
    Received: <element(s) not found>

       7 |   await page.waitForURL('**/dashboard');
       8 |   // login step 3
    >  9 |   await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
         |                                                                  ^
      10 | });
`

// newGeneratedProject lays a project out the way the compiler does: a spec
// under <testDir>/autopus-generated with its step-map sidecar beside it.
func newGeneratedProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	spec := filepath.Join(dir, filepath.FromSlash(generatedSpec))
	require.NoError(t, os.MkdirAll(filepath.Dir(spec), 0o755))
	require.NoError(t, os.WriteFile(spec, []byte(specBody), 0o644))
	require.NoError(t, os.WriteFile(scenario.StepMapPath(spec), []byte(stepMapBody), 0o644))
	return dir
}

// generatedFailure is a failure in the generated login spec that cites the
// failing line only through the code-frame marker.
func generatedFailure(errorLine string, line int) string {
	return fmt.Sprintf("  1) [chromium] › autopus-generated/login.spec.ts:3:5 › login\n\n    %s\n\n    > %2d |   // failing line\n         |   ^\n", errorLine, line)
}

func loginStep(index int, kind, ac string) *scenario.StepRef {
	return &scenario.StepRef{Screen: "login", Index: index, Kind: kind, Ac: ac}
}

func TestClassify_AssignsExactlyOneClassWithItsSignal(t *testing.T) {
	t.Parallel()
	project := newGeneratedProject(t)
	rerunPassed := true
	longLine := "Error: connect ECONNREFUSED 127.0.0.1:5432 " + strings.Repeat("x", 200)

	cases := []struct {
		name   string
		in     Input
		class  Class
		signal string
		line   int
		step   *scenario.StepRef
	}{
		{name: "AC-QALOOP-009 connection refused is environment",
			in:    Input{FailureText: generatedFailure("Error: page.goto: net::ERR_CONNECTION_REFUSED at http://localhost:3000/login", 4)},
			class: ClassEnvironment, signal: "env_output:Error: page.goto: net::ERR_CONNECTION_REFUSED at http://localhost:3000/login"},
		{name: "AC-QALOOP-009 strict-mode violation is a test defect",
			in:    Input{Adapter: "playwright", FailureText: "  1) [chromium] › tests/checkout.spec.ts:12:5 › checkout › pays\n\n    Error: strict mode violation: getByRole('button', { name: 'Pay' }) resolved to 2 elements\n\n        at /Users/[REDACTED_USER]/shop/e2e/tests/checkout.spec.ts:14:42\n"},
			class: ClassTestDefect, signal: "test_defect:Error: strict mode violation: getByRole('button', { name: 'Pay' }) resolved to 2 elements"},
		{name: "AC-QALOOP-009 failing click step is test drift",
			in:    Input{Adapter: "playwright", FailureText: driftOutput},
			class: ClassTestDrift, signal: "stepmap:action e2e/autopus-generated/login.spec.ts:6", line: 6, step: loginStep(1, "action", "AC-LOGIN-001")},
		{name: "AC-QALOOP-009 failing expect step is a product defect",
			in:    Input{Adapter: "playwright", FailureText: expectOutput},
			class: ClassProductDefect, signal: "stepmap:expect e2e/autopus-generated/login.spec.ts:9", line: 9, step: loginStep(3, "expect", "AC-LOGIN-003")},
		{name: "AC-QALOOP-009 pass on re-run is flaky",
			in:    Input{FailureText: driftOutput, RerunPassed: &rerunPassed},
			class: ClassFlaky, signal: "rerun_passed"},
		{name: "AC-QALOOP-009 unmatched output is unknown",
			in:    Input{Adapter: "playwright", FailureText: "worker process exited unexpectedly (code=137)"},
			class: ClassUnknown, signal: "no_rule_matched"},
		{name: "setup gap is environment",
			in:    Input{Status: "skipped", SetupGapCode: "browser_missing"},
			class: ClassEnvironment, signal: "setup_gap:browser_missing"},
		{name: "blocked adapter is environment",
			in:    Input{Status: "blocked"},
			class: ClassEnvironment, signal: "status:blocked"},
		{name: "missing value_env is environment",
			in:    Input{FailureText: "Error: AUTOPUS_QA_MISSING_ENV: value_env QA_LOGIN_PASSWORD is not set"},
			class: ClassEnvironment, signal: "env_output:Error: AUTOPUS_QA_MISSING_ENV: value_env QA_LOGIN_PASSWORD is not set"},
		{name: "webServer timeout is environment",
			in:    Input{FailureText: "Error: Timed out waiting 60000ms from config.webServer."},
			class: ClassEnvironment, signal: "env_output:Error: Timed out waiting 60000ms from config.webServer."},
		{name: "missing Playwright package is environment",
			in:    Input{FailureText: "Error: Cannot find module '@playwright/test'"},
			class: ClassEnvironment, signal: "env_output:Error: Cannot find module '@playwright/test'"},
		{name: "navigation outcome on an action step is a product defect",
			in:    Input{FailureText: generatedFailure("Error: page.waitForURL: Test timeout of 30000ms exceeded.", 7)},
			class: ClassProductDefect, signal: "stepmap:action(navigation) e2e/autopus-generated/login.spec.ts:7", line: 7, step: loginStep(2, "action", "AC-LOGIN-002")},
		{name: "network error on the goto step is environment",
			in:    Input{FailureText: generatedFailure("Error: page.goto: net::ERR_CERT_AUTHORITY_INVALID at https://localhost:3000/login", 4)},
			class: ClassEnvironment, signal: "stepmap:goto e2e/autopus-generated/login.spec.ts:4 net::ERR_CERT_AUTHORITY_INVALID", line: 4, step: loginStep(0, "goto", "")},
		{name: "goto without a network error is a product defect",
			in:    Input{FailureText: generatedFailure("Error: page.goto: Timeout 30000ms exceeded.", 4)},
			class: ClassProductDefect, signal: "stepmap:goto e2e/autopus-generated/login.spec.ts:4", line: 4, step: loginStep(0, "goto", "")},
		{name: "strict-mode violation on a generated action is drift",
			in:    Input{FailureText: generatedFailure("Error: locator.click: Error: strict mode violation: getByRole('button', { name: 'Sign in' }) resolved to 2 elements", 6)},
			class: ClassTestDrift, signal: "stepmap:action e2e/autopus-generated/login.spec.ts:6", line: 6, step: loginStep(1, "action", "AC-LOGIN-001")},
		{name: "existing absolute spec path resolves directly",
			in:    Input{FailureText: fmt.Sprintf("Error: Timed out 5000ms waiting for expect(locator).toBeVisible()\n    at %s:9:5\n", filepath.Join(project, filepath.FromSlash(generatedSpec)))},
			class: ClassProductDefect, signal: "stepmap:expect e2e/autopus-generated/login.spec.ts:9", line: 9, step: loginStep(3, "expect", "AC-LOGIN-003")},
		// The code frame quotes helpers.ts line 9; tying it to the header's
		// spec would wrongly land on the expect step at line 9.
		{name: "helper code frame does not override the stack line",
			in:    Input{FailureText: "  1) [chromium] › autopus-generated/login.spec.ts:3:5 › login\n\n    Error: locator.fill: Test timeout of 30000ms exceeded.\n\n    >  9 |   await field.fill(value);\n         |               ^\n\n        at fillField (/Users/[REDACTED_USER]/shop/e2e/helpers.ts:9:15)\n        at /Users/[REDACTED_USER]/shop/e2e/autopus-generated/login.spec.ts:6:3\n"},
			class: ClassTestDrift, signal: "stepmap:action e2e/autopus-generated/login.spec.ts:6", line: 6, step: loginStep(1, "action", "AC-LOGIN-001")},
		{name: "generated spec without a step map is not located",
			in:    Input{Adapter: "playwright", FailureText: "  1) [chromium] › autopus-generated/other.spec.ts:3:5 › other\n\n    Error: locator.click: Test timeout of 30000ms exceeded.\n\n    >  6 |   await page.click();\n"},
			class: ClassUnknown, signal: "no_rule_matched"},
		{name: "TypeError raised in a test file is a test defect",
			in:    Input{FailureText: "TypeError: Cannot read properties of undefined (reading 'fill')\n    at /Users/[REDACTED_USER]/shop/e2e/tests/profile.spec.ts:22:31\n"},
			class: ClassTestDefect, signal: "test_defect:TypeError: Cannot read properties of undefined (reading 'fill')"},
		{name: "TypeError raised in product code is not a test defect",
			in:    Input{Adapter: "jest", FailureText: "TypeError: total is not a function\n    at Object.total (src/cart.ts:8:10)\n    at Object.<anonymous> (src/cart.test.ts:5:12)\n"},
			class: ClassUnknown, signal: "no_rule_matched"},
		{name: "TypeScript compile error is a test defect",
			in:    Input{FailureText: "e2e/helpers.ts(12,7): error TS2304: Cannot find name 'pagee'."},
			class: ClassTestDefect, signal: "test_defect:e2e/helpers.ts(12,7): error TS2304: Cannot find name 'pagee'."},
		{name: "missing local module is a test defect",
			in:    Input{FailureText: "Error: Cannot find module './fixtures/users'"},
			class: ClassTestDefect, signal: "test_defect:Error: Cannot find module './fixtures/users'"},
		{name: "go test failure is a product defect",
			in:    Input{Adapter: "go-test", FailureText: "--- FAIL: TestCheckoutTotals (0.00s)\n    checkout_test.go:41: total = 90, want 100\nFAIL\n"},
			class: ClassProductDefect, signal: "assertion:--- FAIL: TestCheckoutTotals (0.00s)"},
		{name: "pytest failure is a product defect",
			in:    Input{Adapter: "pytest", FailureText: "FAILED tests/test_cart.py::test_total - AssertionError: assert 90 == 100"},
			class: ClassProductDefect, signal: "assertion:FAILED tests/test_cart.py::test_total - AssertionError: assert 90 == 100"},
		{name: "jest diff is a product defect",
			in:    Input{Adapter: "jest", FailureText: "    Expected: 100\n    Received: 90\n"},
			class: ClassProductDefect, signal: "assertion:Expected: 100"},
		{name: "hand-written GUI expect without a step map stays unknown",
			in:    Input{Adapter: "playwright", FailureText: "Error: expect(locator).toHaveText(expected) failed\n    at /Users/[REDACTED_USER]/shop/e2e/tests/cart.spec.ts:9:5\n"},
			class: ClassUnknown, signal: "no_rule_matched"},
		{name: "signal evidence is cut to 120 characters",
			in:    Input{FailureText: longLine},
			class: ClassEnvironment, signal: "env_output:" + longLine[:120]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := tc.in
			in.JourneyID = "login-journey"
			in.ProjectDir = project
			if in.Status == "" {
				in.Status = "failed"
			}

			got := Classify(in)

			assert.Equal(t, "login-journey", got.JourneyID)
			assert.Equal(t, tc.class, got.Class)
			assert.Equal(t, tc.signal, got.Signal)
			if tc.step == nil {
				assert.Nil(t, got.Step)
				assert.Empty(t, got.SpecPath)
				assert.Zero(t, got.Line)
				return
			}
			assert.Equal(t, tc.step, got.Step)
			assert.Equal(t, generatedSpec, got.SpecPath)
			assert.Equal(t, tc.line, got.Line)
			assert.Equal(t, "login", got.ScenarioID)
		})
	}
}

func TestClassifyAll_ReturnsOneVerdictPerInputInOrder(t *testing.T) {
	t.Parallel()
	rerunPassed := true

	got := ClassifyAll([]Input{
		{JourneyID: "a", Status: "blocked"},
		{JourneyID: "b", Status: "failed", RerunPassed: &rerunPassed},
		{JourneyID: "c", Status: "failed"},
	})

	require.Len(t, got, 3)
	assert.Equal(t, []string{"a", "b", "c"}, []string{got[0].JourneyID, got[1].JourneyID, got[2].JourneyID})
	assert.Equal(t, []Class{ClassEnvironment, ClassFlaky, ClassUnknown}, []Class{got[0].Class, got[1].Class, got[2].Class})
	assert.Empty(t, ClassifyAll(nil))
}
