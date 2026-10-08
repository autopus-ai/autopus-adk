package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectedX is the S4 rejection record, written by hand from the wire
// contract.
const rejectedX = `{
  "schema_version": "harness_candidate_rejection.v1",
  "candidate_id": "GTC-8e80c7a18029",
  "fingerprint_version": 1,
  "fingerprint": "8e80c7a180298de062857e2a5f03c4d9fcdb5875bb50e0efa2f1b2b093e502fd",
  "learning_refs": [
    "L-999",
    "L-1000"
  ],
  "reason": "not a harness issue"
}
`

// intakeS3Project is the project S3 leaves: the S3 store and the open
// candidates of Y and X.
func intakeS3Project(t *testing.T) string {
	t.Helper()
	root := intakeProject(t, s3LearnEntries()...)
	require.Equal(t, 0, runHarness(t, evalHarnessDeps{}, "intake", "--all-eligible", "--dir", root).code)
	return root
}

func TestEvalHarnessReject_S4_RecordsTheRejectionAndIntakeNeverRecreates(t *testing.T) {
	t.Parallel()
	root := intakeS3Project(t)

	got := runHarness(t, evalHarnessDeps{}, "reject", "GTC-8e80c7a18029", "--reason", "not a harness issue", "--dir", root)

	require.Equal(t, 0, got.code, got.stderr)
	assert.Equal(t, "Rejected GTC-8e80c7a18029: wrote evals/harness/candidates/rejected/GTC-8e80c7a18029.json\n", got.stdout)
	assert.Contains(t, got.stderr, "harness-eval: intake reports already_rejected")
	record, err := os.ReadFile(filepath.Join(root, "evals", "harness", "candidates", "rejected", "GTC-8e80c7a18029.json"))
	require.NoError(t, err)
	assert.Equal(t, rejectedX, string(record))
	assert.NoFileExists(t, filepath.Join(root, "evals", "harness", "candidates", "GTC-8e80c7a18029.json"))

	all := runHarness(t, evalHarnessDeps{}, "intake", "--all-eligible", "--dir", root)
	single := runHarness(t, evalHarnessDeps{}, "intake", "--learning", "L-999", "--dir", root)

	require.Equal(t, 0, all.code, all.stderr)
	assert.Equal(t, []any{}, harnessDoc(t, all.stdout)["rows"])
	assert.NoFileExists(t, filepath.Join(root, "evals", "harness", "candidates", "GTC-8e80c7a18029.json"))
	require.Equal(t, 0, single.code, single.stderr)
	assert.Equal(t, []any{map[string]any{"learning_id": "L-999", "result": "already_rejected",
		"match": "GTC-8e80c7a18029", "fingerprint": intakeFingerprintX}}, harnessDoc(t, single.stdout)["rows"])
}

func TestEvalHarnessReject_Refusals_ExitOneAndChangeNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"path escape", []string{"../x", "--reason", "r"}, "harness-eval: candidate_id_invalid"},
		{"upper case", []string{"GTC-023E9302FF0B", "--reason", "r"}, "harness-eval: candidate_id_invalid"},
		{"dot dot suffix", []string{"GTC-023e9302ff0b/../x", "--reason", "r"}, "harness-eval: candidate_id_invalid"},
		{"missing candidate", []string{"GTC-000000000000", "--reason", "r"}, "harness-eval: candidate_missing"},
		{"empty reason", []string{"GTC-8e80c7a18029", "--reason", ""}, "harness-eval: reason_required"},
		{"control character", []string{"GTC-8e80c7a18029", "--reason", "a\x1b[2Jb"}, "harness-eval: learning_field_invalid: reason: control_char"},
		{"no reason flag", []string{"GTC-8e80c7a18029"}, `required flag(s) "reason" not set`},
		{"no candidate", []string{"--reason", "r"}, "accepts 1 arg(s), received 0"},
		{"two candidates", []string{"GTC-8e80c7a18029", "GTC-023e9302ff0b", "--reason", "r"}, "accepts 1 arg(s), received 2"},
		{"no bulk reject", []string{"--all", "--reason", "r"}, "unknown flag: --all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := intakeS3Project(t)
			before := projectDigest(t, root)

			got := runHarness(t, evalHarnessDeps{}, append([]string{"reject", "--dir", root}, tc.args...)...)

			assert.Equal(t, 1, got.code)
			assert.Empty(t, got.stdout)
			assert.Contains(t, got.stderr, tc.want)
			assert.NotContains(t, got.stderr, "\x1b")
			assert.Equal(t, before, projectDigest(t, root))
		})
	}
}

func TestEvalHarnessReject_ReasonSecret_IsRedactedEverywhere(t *testing.T) {
	t.Parallel()
	root := intakeS3Project(t)
	token := "github" + "_pat_" + strings.Repeat("BCDF2468", 5)

	got := runHarness(t, evalHarnessDeps{}, "reject", "GTC-8e80c7a18029", "--reason", "pasted "+token+" by mistake", "--dir", root)

	require.Equal(t, 0, got.code, got.stderr)
	record, err := os.ReadFile(filepath.Join(root, "evals", "harness", "candidates", "rejected", "GTC-8e80c7a18029.json"))
	require.NoError(t, err)
	assert.Contains(t, string(record), `"reason": "pasted [REDACTED_SECRET] by mistake"`)
	for _, surface := range []string{string(record), got.stdout, got.stderr} {
		assert.NotContains(t, surface, token)
	}
}
