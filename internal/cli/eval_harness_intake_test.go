package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// Acceptance fingerprints of Y and X.
const (
	intakeFingerprintY = "023e9302ff0bb28b37a0ebe3bba8af3b50713391bc8aea65cb3fc66ae7f42c86"
	intakeFingerprintX = "8e80c7a180298de062857e2a5f03c4d9fcdb5875bb50e0efa2f1b2b093e502fd"
)

// s3LearnEntries is the S3 store in store order: L-1000 and L-999 share
// fingerprint X with different evidence, L-002 is Y, and L-010 has none.
func s3LearnEntries() []learn.LearningEntry {
	x := func(id, expected, actual, repro string) learn.LearningEntry {
		return learn.LearningEntry{ID: id, Type: learn.EntryTypeReviewIssue, Pattern: "router drops detail mapping",
			Files: []string{"content/skills/plan.md"}, Packages: []string{"pkg/content"},
			Expected: expected, Actual: actual, Repro: repro}
	}
	return []learn.LearningEntry{
		x("L-1000", "x2", "a2", "r2"),
		{ID: "L-002", Type: learn.EntryTypeFixPattern, Pattern: "hook missing in codex",
			Files:    []string{"pkg/content/a.go", "pkg/content/hooks.go", "pkg/content/z.go"},
			Packages: []string{"pkg/adapter", "pkg/content"}, Expected: "y", Actual: "ya"},
		x("L-999", "x1", "a1", ""),
		{ID: "L-010", Type: learn.EntryTypeGateFail, Pattern: "no evidence recorded"},
	}
}

// intakeProject writes entries through the learn store writer below a new
// project root and returns the root.
func intakeProject(t *testing.T, entries ...learn.LearningEntry) string {
	t.Helper()
	root := t.TempDir()
	store, err := learn.NewStore(root)
	require.NoError(t, err)
	for _, entry := range entries {
		entry.Timestamp = time.Date(2026, 10, 6, 1, 2, 3, 0, time.UTC)
		require.NoError(t, store.Append(entry))
	}
	return root
}

// projectDigest maps every path below root to its SHA-256, or "dir", so a
// test can prove nothing was written.
func projectDigest(t *testing.T, root string) map[string]string {
	t.Helper()
	digest := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			digest[path] = "dir"
			return err
		}
		data, err := os.ReadFile(path)
		sum := sha256.Sum256(data)
		digest[path] = hex.EncodeToString(sum[:])
		return err
	}))
	return digest
}

func readCandidate(t *testing.T, root, id string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "evals", "harness", "candidates", id+".json"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc
}

func TestEvalHarnessIntake_S3AllEligible_WritesOneDocumentAndGuidance(t *testing.T) {
	t.Parallel()
	root := intakeProject(t, s3LearnEntries()...)

	got := runHarness(t, evalHarnessDeps{}, "intake", "--all-eligible", "--format", "json", "--dir", root)

	require.Equal(t, 0, got.code, got.stderr)
	assert.Equal(t, map[string]any{
		"schema_version": "harness_intake_result.v1",
		"rows": []any{
			map[string]any{"learning_id": "L-002", "result": "created", "candidate_id": "GTC-023e9302ff0b",
				"fingerprint": intakeFingerprintY, "learning_refs": []any{"L-002"}},
			map[string]any{"learning_id": "L-999", "result": "created", "candidate_id": "GTC-8e80c7a18029",
				"fingerprint": intakeFingerprintX, "learning_refs": []any{"L-999", "L-1000"}},
			map[string]any{"learning_id": "L-1000", "result": "grouped", "candidate_id": "GTC-8e80c7a18029",
				"fingerprint": intakeFingerprintX},
		},
	}, harnessDoc(t, got.stdout))
	candidate := readCandidate(t, root, "GTC-8e80c7a18029")
	assert.Equal(t, "L-999", candidate["representative"])
	assert.Equal(t, []any{"x1", "a1", ""}, []any{candidate["expected"], candidate["actual"], candidate["repro"]})
	for _, id := range []string{"GTC-023e9302ff0b", "GTC-8e80c7a18029"} {
		assert.Contains(t, got.stderr, "harness-eval: created evals/harness/candidates/"+id+".json")
		assert.Contains(t, got.stderr, "auto eval harness promote "+id)
		assert.Contains(t, got.stderr, "auto eval harness reject "+id+" --reason")
	}
	assert.NotContains(t, got.stderr, "router drops detail mapping", "guidance carries no learning text")
}

func TestEvalHarnessIntake_S3FlagMisuse_ExitsOneAndWritesNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"both selections", []string{"--learning", "L-002", "--all-eligible"}, "harness-eval: selection_invalid"},
		{"no selection", nil, "harness-eval: selection_invalid"},
		{"malformed id", []string{"--learning", "L-2"}, "harness-eval: selection_invalid"},
		{"flags with two ids", []string{"--learning", "L-002,L-999", "--expected", "a", "--actual", "b"}, "harness-eval: flag_requires_single_learning"},
		{"expected alone", []string{"--learning", "L-002", "--expected", "a"}, "harness-eval: flag_pair_required"},
		{"agent kind", []string{"--all-eligible", "--kind", "agent"}, "harness-eval: kind_unsupported"},
		{"control character in a flag", []string{"--learning", "L-010", "--expected", "two\nlines", "--actual", "b"},
			"harness-eval: learning_field_invalid: expected: control_char"},
		{"text format", []string{"--all-eligible", "--format", "text"}, `unsupported --format "text"`},
		{"unknown flag", []string{"--all"}, "unknown flag: --all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := intakeProject(t, s3LearnEntries()...)
			before := projectDigest(t, root)

			got := runHarness(t, evalHarnessDeps{}, append([]string{"intake", "--dir", root}, tc.args...)...)

			assert.Equal(t, 1, got.code)
			assert.Empty(t, got.stdout)
			assert.Contains(t, got.stderr, tc.want)
			assert.NotContains(t, got.stderr, "two\nlines")
			assert.Equal(t, before, projectDigest(t, root))
		})
	}
}

func TestEvalHarnessIntake_S3SkippedRow_ExitsTwoWithTheDocument(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ id, reason, hint string }{
		"no evidence": {"L-010", "learning_missing_expected_actual", "--expected and --actual"},
		"no entry":    {"L-404", "learning_not_found", "no learning entry has this id"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := intakeProject(t, s3LearnEntries()...)

			got := runHarness(t, evalHarnessDeps{}, "intake", "--learning", tc.id, "--dir", root)

			assert.Equal(t, 2, got.code)
			rows := harnessDoc(t, got.stdout)["rows"].([]any)
			require.Len(t, rows, 1)
			row := rows[0].(map[string]any)
			assert.Equal(t, []any{tc.id, "skipped", tc.reason}, []any{row["learning_id"], row["result"], row["reason"]})
			assert.Contains(t, got.stderr, "harness-eval: "+tc.id+" skipped: "+tc.reason)
			assert.Contains(t, got.stderr, tc.hint)
			assert.NotContains(t, got.stderr, "Error:", "the document already explains the exit code")
			assert.NoDirExists(t, filepath.Join(root, "evals"))
		})
	}
}

func TestEvalHarnessIntake_S5_RedactsFlagSecretsAndSkipsControlText(t *testing.T) {
	t.Parallel()
	escape := learn.LearningEntry{ID: "L-020", Type: learn.EntryTypeGateFail, Pattern: "colour \x1b[31m leak", Expected: "e", Actual: "a"}
	root := intakeProject(t, append(s3LearnEntries(), escape)...)
	store := filepath.Join(root, ".autopus", "learnings", "pipeline.jsonl")
	storeBefore := projectDigest(t, store)
	token := "sk-" + "ant-" + strings.Repeat("Qx7", 8)

	flagged := runHarness(t, evalHarnessDeps{}, "intake", "--learning", "L-010", "--expected", "e", "--actual", token+" seen", "--dir", root)
	escaped := runHarness(t, evalHarnessDeps{}, "intake", "--learning", "L-020", "--dir", root)

	require.Equal(t, 0, flagged.code, flagged.stderr)
	rows := harnessDoc(t, flagged.stdout)["rows"].([]any)
	candidateID := rows[0].(map[string]any)["candidate_id"].(string)
	candidate := readCandidate(t, root, candidateID)
	assert.Equal(t, "[REDACTED_SECRET] seen", candidate["actual"])
	assert.Equal(t, []any{"actual"}, candidate["redacted_fields"])
	assert.Equal(t, true, candidate["redacted"])
	data, err := os.ReadFile(filepath.Join(root, "evals", "harness", "candidates", candidateID+".json"))
	require.NoError(t, err)
	for _, surface := range []string{string(data), flagged.stdout, flagged.stderr} {
		assert.NotContains(t, surface, token)
	}
	assert.Equal(t, storeBefore, projectDigest(t, store), "intake never writes the learn store")

	assert.Equal(t, 2, escaped.code)
	row := harnessDoc(t, escaped.stdout)["rows"].([]any)[0].(map[string]any)
	assert.Equal(t, []any{"skipped", "candidate_text_invalid"}, []any{row["result"], row["reason"]})
	assert.NotContains(t, escaped.stderr, "\x1b")
}

func TestEvalHarnessIntake_ProjectWithoutStore_CreatesNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	got := runHarness(t, evalHarnessDeps{}, "intake", "--all-eligible", "--dir", root)

	require.Equal(t, 0, got.code, got.stderr)
	assert.Equal(t, []any{}, harnessDoc(t, got.stdout)["rows"])
	assert.Contains(t, got.stderr, "harness-eval: no new candidate")
	assert.NoDirExists(t, filepath.Join(root, ".autopus"))
	assert.NoDirExists(t, filepath.Join(root, "evals"))
}

func TestEvalHarnessIntake_UnreadableIntakeRecord_ExitsOneWithoutWriting(t *testing.T) {
	t.Parallel()
	root := intakeProject(t, s3LearnEntries()...)
	broken := filepath.Join(root, "evals", "harness", "candidates", "GTC-000000000000.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(broken), 0o755))
	require.NoError(t, os.WriteFile(broken, []byte("{"), 0o644))
	before := projectDigest(t, root)

	got := runHarness(t, evalHarnessDeps{}, "intake", "--all-eligible", "--dir", root)

	assert.Equal(t, 1, got.code)
	assert.Empty(t, got.stdout)
	assert.Contains(t, got.stderr, "harness-eval: eval_links_unreadable: evals/harness/candidates/GTC-000000000000.json")
	assert.Equal(t, before, projectDigest(t, root))
}

func TestEvalHarness_RootRegistersIntakeAndReject(t *testing.T) {
	t.Parallel()
	root := NewRootCmd()
	for _, name := range []string{"intake", "reject"} {
		cmd, _, err := root.Find([]string{"eval", "harness", name})
		require.NoError(t, err, name)
		assert.Equal(t, name, cmd.Name())
		assert.Equal(t, "auto eval harness "+name, cmd.CommandPath())
	}
}
