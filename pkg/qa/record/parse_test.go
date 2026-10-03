package record_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/record"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// loginCodegen is AC-QALOOP-015's fixture: test-layout codegen whose line 8
// (page.mouse) has no scenario equivalent.
var loginCodegen = strings.Join([]string{
	"import { test, expect } from '@playwright/test';",
	"",
	"test('test', async ({ page }) => {",
	"  await page.goto('http://127.0.0.1:4173/');",
	"  await page.getByRole('link', { name: 'Sign in' }).click();",
	"  await page.getByLabel('Email').fill('ada@example.com');",
	"  await page.getByLabel('Email').press('Enter');",
	"  await page.mouse.click(120, 48);",
	"  await expect(page.getByText('Welcome back')).toBeVisible();",
	"  await expect(page).toHaveURL('http://127.0.0.1:4173/dashboard');",
	"});",
	"",
}, "\n")

// agentLog is AC-QALOOP-016's fixture: human actions, one human expect tied to
// an ac, one agent expect without ac, and one agent expect with ac.
var agentLog = strings.Join([]string{
	`{"schema_version":"qamesh.recording.v1"}`,
	`{"action":"goto","url":"http://127.0.0.1:4173/login","by":"human"}`,
	`{"action":"fill","target":{"label":"Email"},"value":"ada@example.com","by":"human"}`,
	`{"action":"fill","target":{"label":"Password"},"value_env":"E2E_PASSWORD","by":"human"}`,
	`{"action":"click","target":{"role":"button","name":"Sign in"},"by":"human"}`,
	`{"expect":"text","value":"Welcome back","by":"human","ac":"AC-LOGIN-1"}`,
	`{"expect":"role","target":{"role":"heading","name":"Dashboard"},"by":"agent"}`,
	`{"expect":"url","url":"/dashboard","by":"agent","ac":"AC-LOGIN-2"}`,
}, "\n")

func TestQARecordParseCodegen_TestLayoutKeepsOrderAndFlagsMouseLine(t *testing.T) {
	t.Parallel()
	rec, unsupported := record.ParseCodegen([]byte(loginCodegen))

	require.Len(t, unsupported, 1)
	assert.Equal(t, 8, unsupported[0].Line)
	assert.Equal(t, "await page.mouse.click(120, 48);", unsupported[0].Text)
	kinds := make([]string, 0, len(rec.Events))
	for _, event := range rec.Events {
		kinds = append(kinds, event.Action+event.Expect)
		assert.Equal(t, scenario.ByHuman, event.By)
	}
	assert.Equal(t, []string{"goto", "click", "fill", "press", "text", "url"}, kinds)
	assert.Equal(t, scenario.Target{Role: "link", Name: "Sign in"}, rec.Events[1].Target)
	assert.Equal(t, "ada@example.com", rec.Events[2].Value)
	assert.Equal(t, "Enter", rec.Events[3].Key)
	assert.Equal(t, "Welcome back", rec.Events[4].Value)
	assert.Equal(t, 10, rec.Events[5].Line)
}

func TestQARecordParseCodegen_LibraryLayoutCoversEveryLocatorAndAction(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"const { chromium } = require('playwright');",
		"",
		"(async () => {",
		"  const browser = await chromium.launch({",
		"    headless: false",
		"  });",
		"  const context = await browser.newContext();",
		"  const page = await context.newPage();",
		"  await page.goto('http://127.0.0.1:4173/shop');",
		`  await page.getByPlaceholder('Search').fill("O'Brien \"quoted\" é");`,
		"  await page.keyboard.press('Enter');",
		"  await page.getByTestId('size').selectOption('xl');",
		"  await page.getByRole('checkbox', { name: 'Gift wrap', exact: true }).first().check();",
		"  await expect(page.getByRole('heading', { name: 'Cart' })).toBeVisible();",
		"  await page.goto('/checkout');",
		"  await expect(page).toHaveTitle(`Checkout`);",
		"  await expect(page.getByTestId('total')).toContainText('$12');",
		`  await page.getByText('It\'s done', { exact: true }).nth(0).click();`,
		"  await page.locator('#legacy').click();",
		"  // ---------------------",
		"  await context.close();",
		"  await browser.close();",
		"})();",
	}, "\n")

	rec, unsupported := record.ParseCodegen([]byte(src))
	require.Len(t, unsupported, 1)
	assert.Equal(t, 19, unsupported[0].Line)

	s, err := record.ToScenario(rec, record.Options{ID: "checkout", Journey: "browser-gui-explore",
		Origin: "http://127.0.0.1:4173/", RecordingRef: "checkout.js@sha256:0"})
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:4173", s.Origin)
	require.Len(t, s.Screens, 2)
	assert.Equal(t, "shop", s.Screens[0].ID)
	assert.Equal(t, "/checkout", s.Screens[1].Path)
	shop, checkout := s.Screens[0].Steps, s.Screens[1].Steps
	require.Len(t, shop, 5)
	require.Len(t, checkout, 3)
	assert.Equal(t, &scenario.FillAction{Target: scenario.Target{Placeholder: "Search"}, Value: `O'Brien "quoted" é`}, shop[0].Fill)
	assert.Equal(t, &scenario.PressAction{Key: "Enter"}, shop[1].Press)
	assert.Equal(t, &scenario.SelectAction{Target: scenario.Target{TestID: "size"}, Option: "xl"}, shop[2].Select)
	assert.Equal(t, &scenario.Target{Role: "checkbox", Name: "Gift wrap", Exact: true}, shop[3].Check)
	assert.Equal(t, &scenario.RoleTarget{Role: "heading", Name: "Cart"}, shop[4].ExpectRole)
	assert.Equal(t, "Checkout", checkout[0].ExpectTitle)
	assert.Equal(t, "$12", checkout[1].ExpectText)
	assert.Equal(t, &scenario.Target{Text: "It's done", Exact: true}, checkout[2].Click)
}

func TestQARecordParseCodegen_ReportsLinesWithNoScenarioEquivalent(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		"await page.getByRole('button').dblclick();",
		"await page.waitForTimeout(500);",
		"await page.frameLocator('#f').getByRole('button').click();",
		"await page.getByRole('button', { name: /Save/ }).click();",
		"await expect(page.getByLabel('Email')).toHaveValue('x');",
		"await expect(page.getByLabel('Email')).toBeVisible();",
		"await expect(page.getByRole('button')).not.toBeVisible();",
		"await page.getByRole('button', { name: 'X' }).nth(2).click();",
		"await page.getByRole('row', { name: 'a' }).getByRole('button').click();",
		"await page.getByRole('button').click({ button: 'right' });",
		"await page.getByRole('button', { checked: true }).click();",
		"await page.getByLabel('Email').fill('');",
		"await page.locator('#id').click();",
		"await page.goto(`${base}/x`);",
		"page.getByRole('button').click();",
		"const page1Promise = page.waitForEvent('popup');",
	} {
		rec, unsupported := record.ParseCodegen([]byte(line + "\n"))
		assert.Empty(t, rec.Events, line)
		if assert.Len(t, unsupported, 1, line) {
			assert.Equal(t, 1, unsupported[0].Line, line)
			assert.NotEmpty(t, unsupported[0].Reason, line)
		}
	}
}

func TestQARecordParseJSONL_HeaderLineIsNotAnEvent(t *testing.T) {
	t.Parallel()
	rec, unsupported := record.ParseJSONL([]byte(agentLog))

	assert.Empty(t, unsupported)
	require.Len(t, rec.Events, 7)
	assert.Equal(t, 2, rec.Events[0].Line)
	assert.Equal(t, "E2E_PASSWORD", rec.Events[2].ValueEnv)
	assert.Equal(t, scenario.ByAgent, rec.Events[5].By)
}

func TestQARecordParseJSONL_ReportsLinesItCannotRecordFaithfully(t *testing.T) {
	t.Parallel()
	for name, line := range map[string]string{
		"unknown field":            `{"action":"click","target":{"role":"button"},"by":"human","force":true}`,
		"unknown action":           `{"action":"hover","target":{"role":"button"},"by":"human"}`,
		"unknown expect":           `{"expect":"value","value":"x","by":"agent"}`,
		"missing by":               `{"action":"click","target":{"role":"button"}}`,
		"action and expect":        `{"action":"click","expect":"text","value":"x","by":"human"}`,
		"other schema version":     `{"schema_version":"qamesh.recording.v2","action":"goto","url":"/","by":"human"}`,
		"field the kind ignores":   `{"action":"click","target":{"role":"button"},"value":"x","by":"human"}`,
		"value and value_env":      `{"action":"fill","target":{"label":"A"},"value":"x","value_env":"X","by":"human"}`,
		"role expect without role": `{"expect":"role","target":{"label":"Email"},"by":"agent"}`,
		"two locator kinds":        `{"action":"click","target":{"role":"button","label":"x"},"by":"human"}`,
		"name without role":        `{"action":"click","target":{"name":"Go"},"by":"human"}`,
		"missing required field":   `{"action":"goto","by":"human"}`,
		"not json":                 `await page.goto('/')`,
		"two objects":              `{"action":"goto","url":"/","by":"human"} {"action":"goto","url":"/","by":"human"}`,
	} {
		rec, unsupported := record.ParseJSONL([]byte(line))
		assert.Empty(t, rec.Events, name)
		if assert.Len(t, unsupported, 1, name) {
			assert.Equal(t, 1, unsupported[0].Line, name)
			assert.NotEmpty(t, unsupported[0].Reason, name)
		}
	}
}
