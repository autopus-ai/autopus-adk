package record

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const playwrightPackYAML = `id: browser-staging-playwright
title: Browser staging
surface: frontend
lanes: [browser-staging]
adapter:
  id: playwright
command:
  argv: ["npm", "exec", "playwright", "test"]
  cwd: .
  timeout: 240s
checks:
  - id: browser-staging-playwright
    type: deterministic
    expected:
      exit_code: 0
source_refs:
  source_spec: SPEC-QAMESH-005
  acceptance_refs: [AC-1]
  owned_paths: ["."]
`

func writePacksProject(t *testing.T, baseURL bool) string {
	t.Helper()
	dir := t.TempDir()
	journeys := filepath.Join(dir, ".autopus", "qa", "journeys")
	require.NoError(t, os.MkdirAll(journeys, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(journeys, "browser-staging-playwright.yaml"), []byte(playwrightPackYAML), 0o644))
	if baseURL {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "playwright.config.js"),
			[]byte("module.exports = { use: { baseURL: 'http://127.0.0.1:4173' } };\n"), 0o644))
	}
	return dir
}

const codegenAbsolute = `import { test, expect } from '@playwright/test';

test('test', async ({ page }) => {
  await page.goto('http://127.0.0.1:4173/');
  await page.getByRole('button', { name: 'Go' }).click();
  await expect(page.getByText('Done')).toBeVisible();
});
`

// The playwright pack auto qa init writes declares no allowed_origins; its
// origin is the Playwright baseURL.
func TestImport_PlaywrightPackUsesBaseURL(t *testing.T) {
	dir := writePacksProject(t, true)
	from := filepath.Join(t.TempDir(), "rec.js")
	require.NoError(t, os.WriteFile(from, []byte(codegenAbsolute), 0o644))
	res, err := Import(dir, ImportOptions{From: from, ID: "go-flow", Journey: "browser-staging-playwright"})
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:4173", res.Origin)
}

// With no baseURL either, the origin the recording navigated to is used.
func TestImport_FallsBackToRecordedOrigin(t *testing.T) {
	dir := writePacksProject(t, false)
	from := filepath.Join(t.TempDir(), "rec.js")
	require.NoError(t, os.WriteFile(from, []byte(codegenAbsolute), 0o644))
	res, err := Import(dir, ImportOptions{From: from, ID: "go-flow", Journey: "browser-staging-playwright"})
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:4173", res.Origin)
}
