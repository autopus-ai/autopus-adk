package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	qascenario "github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// qaRecordCodegen is codegen output whose line 6 (page.mouse) has no scenario
// equivalent.
const qaRecordCodegen = "import { test, expect } from '@playwright/test';\n\n" +
	"test('test', async ({ page }) => {\n" +
	"  await page.goto('http://127.0.0.1:4173/');\n" +
	"  await page.getByRole('link', { name: 'Sign in' }).click();\n" +
	"  await page.mouse.click(120, 48);\n" +
	"  await expect(page.getByText('Welcome back')).toBeVisible();\n" +
	"});\n"

// qaRecordExecute runs cmd and decodes its output when it is a JSON envelope.
func qaRecordExecute(t *testing.T, cmd *cobra.Command, args ...string) (map[string]any, string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		return decodeJSONMap(t, out.Bytes()), out.String(), err
	}
	return nil, out.String(), err
}

func TestQARecordCmd_ExposesLiveFlagsAndImportSubcommand(t *testing.T) {
	cmd := newQARecordCmd()
	for _, flag := range []string{"origin", "id", "journey", "allow-partial", "project-dir", "format"} {
		assert.NotNil(t, cmd.Flags().Lookup(flag), flag)
	}
	sub, _, err := cmd.Find([]string{"import"})
	require.NoError(t, err)
	require.Equal(t, "import", sub.Name())
	for _, flag := range []string{"from", "id", "journey", "origin", "allow-partial", "input-format", "project-dir", "format"} {
		assert.NotNil(t, sub.Flags().Lookup(flag), flag)
	}
}

// AC-QALOOP-015 through the CLI: the unsupported line is listed by number
// until --allow-partial drops it.
func TestQARecordImportCmd_ListsUnsupportedLinesUntilAllowPartial(t *testing.T) {
	project := scenarioProject(t)
	from := filepath.Join(t.TempDir(), "login.spec.ts")
	require.NoError(t, os.WriteFile(from, []byte(qaRecordCodegen), 0o644))

	payload, _, err := qaRecordExecute(t, newQARecordCmd(), "import", "--from", from, "--id", "login",
		"--project-dir", project, "--format", "json")
	require.Error(t, err)
	require.NotNil(t, payload)
	assert.Equal(t, "qa_record_unsupported_lines", payload["error"].(map[string]any)["code"])
	lines := payload["data"].(map[string]any)["unsupported"].([]any)
	require.Len(t, lines, 1)
	assert.EqualValues(t, 6, lines[0].(map[string]any)["line"])
	assert.NoFileExists(t, filepath.Join(qascenario.CandidatesDir(project), "login.yaml"))

	payload, _, err = qaRecordExecute(t, newQARecordCmd(), "import", "--from", from, "--id", "login",
		"--allow-partial", "--project-dir", project, "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, "warn", payload["status"])
	loaded, err := qascenario.LoadFile(payload["data"].(map[string]any)["path"].(string))
	require.NoError(t, err)
	assert.Equal(t, qascenario.IntentRecording, loaded.IntentSource)
	assert.Len(t, loaded.Screens[0].Steps, 2)
}

func TestQARecordImportCmd_DefaultsTheIDToTheFileName(t *testing.T) {
	project := scenarioProject(t)
	from := filepath.Join(t.TempDir(), "Checkout Flow.js")
	require.NoError(t, os.WriteFile(from, []byte(qaRecordCodegen), 0o644))

	_, text, err := qaRecordExecute(t, newQARecordCmd(), "import", "--from", from, "--allow-partial", "--project-dir", project)

	require.NoError(t, err)
	assert.Contains(t, text, "recording candidate created")
	assert.Contains(t, text, "dropped line 6")
	assert.FileExists(t, filepath.Join(qascenario.CandidatesDir(project), "checkout-flow.yaml"))
}

// REQ-15 through the CLI: codegen runs against the given origin and the
// result lands as a candidate.
func TestQARecordCmd_RecordsLiveAgainstTheGivenOrigin(t *testing.T) {
	project := scenarioProject(t)
	var gotName, gotOrigin string
	fake := func(_ context.Context, _ string, name string, args ...string) error {
		gotName, gotOrigin = name, args[len(args)-1]
		body := strings.ReplaceAll(qaRecordCodegen, "  await page.mouse.click(120, 48);\n", "")
		return os.WriteFile(args[len(args)-2], []byte(body), 0o644)
	}

	_, text, err := qaRecordExecute(t, newQARecordCmdWith(fake), "--origin", "http://127.0.0.1:4173",
		"--id", "login", "--journey", "browser-gui-explore", "--project-dir", project)

	require.NoError(t, err)
	assert.Equal(t, "npx", gotName)
	assert.Equal(t, "http://127.0.0.1:4173", gotOrigin)
	assert.Contains(t, text, "recording candidate created")
	assert.FileExists(t, filepath.Join(qascenario.CandidatesDir(project), "login.yaml"))
}
