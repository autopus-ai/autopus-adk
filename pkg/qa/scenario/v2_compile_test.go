package scenario

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileV2(t *testing.T, s Scenario) (string, StepMap) {
	t.Helper()
	body, steps, err := CompileWithMap(s, Options{FixtureImport: "../../fixture.cjs"})
	require.NoError(t, err)
	return string(body), steps
}

// AC-QALOOP-003: an acceptance journey with fill (value_env), click, and
// expect_text compiles to @journey only, reads the credential from the
// environment, and maps each step line to its kind and ac.
func TestCompileV2_ActionsCompileToJourneyWithStepMap(t *testing.T) {
	t.Parallel()
	out, steps := compileV2(t, v2Acceptance())

	assert.Contains(t, out, `test.describe("login @journey", () => {`)
	assert.NotContains(t, out, "@explore")
	assert.NotContains(t, out, "Read-only by construction", "a journey spec must not claim to be read-only")
	assert.Contains(t, out, `process.env["E2E_PASSWORD"]`)
	assert.Contains(t, out, "// Intent: acceptance (SPEC-AUTH-001)")

	// The expected rendering per step is stated independently of the map, and
	// the map is then checked against the actual lines of the emitted file.
	want := map[int]struct {
		kind, ac, code string
	}{
		0: {StepKindGoto, "", `    await page.goto(ORIGIN + "/login");`},
		1: {StepKindAction, "", `    await page.getByLabel("Email").first().fill("member@example.test");`},
		2: {StepKindAction, "", `    await page.getByLabel("Password").first().fill(process.env["E2E_PASSWORD"] || missingEnv("E2E_PASSWORD"));`},
		3: {StepKindAction, "AC-AUTH-001", `    await page.getByRole("button", { name: "Sign in" }).first().click();`},
		4: {StepKindExpect, "AC-AUTH-001", `    await expect(page.getByText(new RegExp("Welcome back")).first()).toBeVisible();`},
		5: {StepKindExpect, "AC-AUTH-002", `    await expect(page).toHaveURL(onPath("/dashboard"));`},
	}
	lines := strings.Split(out, "\n")
	require.Len(t, steps.Lines, len(want))
	seen := map[int]bool{}
	for key, ref := range steps.Lines {
		expected, ok := want[ref.Index]
		require.True(t, ok, "unexpected index %d on line %s", ref.Index, key)
		seen[ref.Index] = true
		assert.Equal(t, "sign-in", ref.Screen)
		assert.Equal(t, expected.kind, ref.Kind, "index %d", ref.Index)
		assert.Equal(t, expected.ac, ref.Ac, "index %d", ref.Index)
		line := 0
		for _, digit := range key {
			line = line*10 + int(digit-'0')
		}
		assert.Equal(t, expected.code, lines[line-1], "line %s", key)
	}
	assert.Len(t, seen, len(want))

	// Every executable step line is mapped; header and scaffolding are not.
	for index, line := range lines {
		_, mapped := steps.Lookup(index + 1)
		assert.Equal(t, strings.HasPrefix(line, "    await "), mapped, "line %d: %s", index+1, line)
	}
	assert.Equal(t, StepMapSchemaVersion, steps.SchemaVersion)
	assert.Equal(t, "login", steps.ScenarioID)
	assert.Equal(t, "SPEC-AUTH-001", steps.Spec)
	assert.Equal(t, IntentAcceptance, steps.IntentSource)
}

// The missing-credential guard throws from a header helper with a stable
// marker, so triage reads an unset variable as setup, not as drift.
func TestCompileV2_ValueEnvHelperOnlyWhenUsed(t *testing.T) {
	t.Parallel()
	out, steps := compileV2(t, v2Acceptance())
	assert.Contains(t, out, "function missingEnv(name: string): never {")
	assert.Contains(t, out, MissingEnvMarker+`: " + name + " is not set`)
	for index, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "throw new Error") {
			_, mapped := steps.Lookup(index + 1)
			assert.False(t, mapped, "the throw line must not map to a step")
		}
	}

	s := v2Acceptance()
	s.Screens[0].Steps[1] = Step{Fill: &FillAction{Target: Target{Label: "Password"}, Value: "literal"}}
	out, _ = compileV2(t, s)
	assert.NotContains(t, out, "missingEnv")
}

// REQ-3: a v2 scenario with no action stays on the read-only lane, and a
// baseline also says it is only a baseline.
func TestCompileV2_ReadOnlyScenariosKeepExploreAndBaselineIsLabelled(t *testing.T) {
	t.Parallel()
	readOnly := v2Acceptance()
	readOnly.Screens[0].Steps = readOnly.Screens[0].Steps[3:]
	out, steps := compileV2(t, readOnly)
	assert.Contains(t, out, `test.describe("login @explore", () => {`)
	assert.NotContains(t, out, JourneyTag)
	assert.NotContains(t, out, BaselineTag)
	assert.Contains(t, out, "// Read-only: this scenario declares no action steps")
	for _, ref := range steps.Lines {
		assert.NotEqual(t, StepKindAction, ref.Kind)
	}

	baseline := readOnly
	baseline.IntentSource, baseline.Spec, baseline.AcceptanceRef = IntentBaseline, "", nil
	out, _ = compileV2(t, baseline)
	assert.Contains(t, out, `test.describe("login @explore @baseline", () => {`)
	assert.Contains(t, out, "// Regression baseline:")
	assert.Contains(t, out, "not that the\n// behaviour is correct")
	assert.Contains(t, out, "// Intent: baseline\n")
}

func TestCompileV2_RendersEveryLocatorAndAction(t *testing.T) {
	t.Parallel()
	s := v2Acceptance()
	s.IntentSource, s.Spec, s.RecordingRef = IntentRecording, "", "recordings/checkout.jsonl"
	s.Screens[0].Steps = []Step{
		{Check: &Target{Label: "Remember me"}},
		{Press: &PressAction{Key: "Enter"}},
		{Press: &PressAction{Target: Target{Placeholder: "Search"}, Key: "ArrowDown"}},
		{Select: &SelectAction{Target: Target{TestID: "plan"}, Option: "pro"}},
		{Click: &Target{Text: "Continue", Exact: true}},
		{Click: &Target{Role: "Link", Name: "Docs", Exact: true}},
		{Fill: &FillAction{Target: Target{Label: "Name", Exact: true}, Value: "Ada"}},
		{WaitURL: " /done "},
		{ExpectRole: &RoleTarget{Role: "heading", Name: "Done"}, By: ByAgent, Confirm: ConfirmRequired},
	}
	out, _ := compileV2(t, s)
	for _, want := range []string{
		`await page.getByLabel("Remember me").first().check();`,
		`await page.keyboard.press("Enter");`,
		`await page.getByPlaceholder("Search").first().press("ArrowDown");`,
		`await page.getByTestId("plan").first().selectOption("pro");`,
		`await page.getByText("Continue", { exact: true }).first().click();`,
		`await page.getByRole("link", { name: "Docs", exact: true }).first().click();`,
		`await page.getByLabel("Name", { exact: true }).first().fill("Ada");`,
		`await page.waitForURL(onPath("/done"));`,
		`await expect(page.getByRole("heading", { name: "Done" }).first()).toBeVisible();`,
		"// Intent: recording (recordings/checkout.jsonl)",
		`test.describe("login @journey", () => {`,
	} {
		assert.Contains(t, out, want)
	}
}

// Action values are authored data in executable source, exactly like expect
// text, so they get the same literal-safety invariant.
func TestCompileV2_EscapesHostileActionValues(t *testing.T) {
	t.Parallel()
	s := v2Acceptance()
	s.Screens[0].Steps[0] = Step{Fill: &FillAction{Target: Target{Label: "a\"b`c${d}"}, Value: "\"); process.exit(1); //\n"}}
	s.Screens[0].Steps[2] = Step{Click: &Target{TestID: `x\"); //`}, Ac: "AC-AUTH-001"}
	out, _ := compileV2(t, s)
	for index, line := range strings.Split(out, "\n") {
		assert.Zero(t, unescapedQuotes(line)%2, "line %d has an unterminated literal: %s", index+1, line)
	}
	assert.NotContains(t, out, "process.exit(1); //\n")
}

// Locator actions are invisible to the capture fixture, which records only
// page-level calls, so the matrix requires goto alone for v2 journeys too.
func TestScreenMatrixV2RequiresOnlyObservableActions(t *testing.T) {
	t.Parallel()
	rows := ScreenMatrix([]Scenario{v2Acceptance()}, "browser-staging")
	require.Len(t, rows, 1)
	assert.Equal(t, []string{"goto"}, rows[0]["required_actions"])
}
