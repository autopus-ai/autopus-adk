package record_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/record"
)

func TestQARecordParseCodegen_DecodesJavaScriptEscapes(t *testing.T) {
	t.Parallel()
	src := `await page.getByLabel('Note').fill('\x41B\u{43}😀\t\/\\ \'q\'');`

	rec, unsupported := record.ParseCodegen([]byte(src))

	require.Empty(t, unsupported)
	require.Len(t, rec.Events, 1)
	assert.Equal(t, "ABC\U0001F600\t/\\ 'q'", rec.Events[0].Value)
	for _, bad := range []string{
		`await page.getByLabel('Note').fill('\x4');`,
		`await page.getByLabel('Note').fill('\u{}');`,
		`await page.getByLabel('Note').fill('\u12');`,
		`await page.getByLabel('Note').fill('open);`,
		`await page.getByLabel('Note').fill('a' + 'b');`,
	} {
		_, unsupported := record.ParseCodegen([]byte(bad))
		assert.Len(t, unsupported, 1, bad)
	}
}

func TestQARecordParseCodegen_SkipsCommentsAndJoinsWrappedStatements(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"/*",
		" * Recorded by hand.",
		" */",
		"test.use({",
		"  storageState: 'auth.json'",
		"});",
		"await page.goto('http://127.0.0.1:4173/'); // landing",
		"await page.getByRole('button', {",
		"  name: 'Save',",
		"}).click();",
	}, "\n")

	rec, unsupported := record.ParseCodegen([]byte(src))

	require.Empty(t, unsupported)
	require.Len(t, rec.Events, 2)
	assert.Equal(t, "http://127.0.0.1:4173/", rec.Events[0].URL)
	assert.Equal(t, 8, rec.Events[1].Line, "a wrapped statement reports its first line")
	assert.Equal(t, "Save", rec.Events[1].Target.Name)
}

func TestQARecordImport_RejectsUnknownFormatsBadSourcesAndBadIDs(t *testing.T) {
	t.Parallel()
	project := projectWithPack(t)
	for name, tc := range map[string]struct {
		opts record.ImportOptions
		code string
	}{
		"unknown extension": {record.ImportOptions{From: writeSource(t, "session.txt", agentLog), ID: "x"}, record.CodeFormatUnknown},
		"unknown format":    {record.ImportOptions{From: writeSource(t, "session.jsonl", agentLog), Format: "har", ID: "x"}, record.CodeFormatUnknown},
		"missing file":      {record.ImportOptions{From: filepath.Join(t.TempDir(), "absent.jsonl"), ID: "x"}, record.CodeSourceUnreadable},
		"invalid origin": {record.ImportOptions{From: writeSource(t, "session.jsonl", agentLog), ID: "x",
			Origin: "ftp://files.example"}, record.CodeOriginInvalid},
		"id that escapes the directory": {record.ImportOptions{From: writeSource(t, "session.jsonl", agentLog),
			ID: "../escape"}, "qa_scenario_id_invalid"},
	} {
		_, err := record.Import(project, tc.opts)
		code, _ := record.CodeOf(err)
		assert.Equal(t, tc.code, code, name)
	}
	assert.NoFileExists(t, filepath.Join(project, ".autopus", "qa", "scenarios", "escape.yaml"))

	result, err := record.Import(project, record.ImportOptions{
		From: writeSource(t, "session.txt", agentLog), Format: "jsonl", ID: "explicit-format"})
	require.NoError(t, err)
	assert.Equal(t, record.FormatJSONL, result.Format)
}
