package intake

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syntheticSecret returns a token made at run time, so no fixture or
// document ever holds it.
func syntheticSecret(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 12)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return "tok_" + hex.EncodeToString(raw)
}

func TestRun_S5FlagSecret_IsRedactedBeforeAnyByteIsWritten(t *testing.T) {
	t.Parallel()
	root, secret := t.TempDir(), syntheticSecret(t)

	result := runIntake(t, root, s3Entries(), func(r *Request) {
		r.AllEligible, r.LearningIDs = false, []string{"L-010"}
		r.Expected, r.Actual, r.Redactor = "hooks exist", secret+" seen", maskingRedactor(secret)
	})

	candidate := decodeCandidate(t, root, result.Rows[0].CandidateID)
	assert.Equal(t, "[REDACTED_SECRET] seen", candidate.Actual)
	assert.Equal(t, []string{"actual"}, candidate.RedactedFields)
	assert.True(t, candidate.Redacted)
	stdout, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(stdout), secret)
	assert.NotContains(t, readFile(t, root, IntakeDir+"/"+candidate.ID+".json"), secret)
}

func TestRun_LegacyRawStoreText_IsRedactedOnlyInTheCandidate(t *testing.T) {
	t.Parallel()
	root, secret := t.TempDir(), syntheticSecret(t)
	legacy := Entry{ID: "L-001", Type: "fix_pattern", Pattern: "deploy failed " + secret + " in ci",
		Expected: "deploy passes", Actual: "leaked " + secret, Repro: "make ci"}

	result := runIntake(t, root, []Entry{legacy}, func(r *Request) { r.Redactor = maskingRedactor(secret) })

	require.Len(t, result.Rows, 1)
	assert.Equal(t, Fingerprint(legacy), result.Rows[0].Fingerprint, "the fingerprint hashes the stored value")
	candidate := decodeCandidate(t, root, result.Rows[0].CandidateID)
	assert.Equal(t, "deploy failed [REDACTED_SECRET] in ci", candidate.Task.Intent)
	assert.Equal(t, "leaked [REDACTED_SECRET]", candidate.Actual)
	assert.Equal(t, []string{"actual", "pattern"}, candidate.RedactedFields)
	assert.Equal(t, "make ci", candidate.Repro)
}

func TestRun_S5PatternText_ControlCharactersAndSizeAreChecked(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pattern string
		result  string
	}{
		{"hook \x1b[31mred", ResultSkipped},
		{"del \x7f char", ResultSkipped},
		{"c1 \u0085 char", ResultSkipped},
		{"carriage\rreturn", ResultSkipped},
		{strings.Repeat("p", 4097), ResultSkipped},
		{"line one\nline\ttwo", ResultCreated},
		{strings.Repeat("p", 4096), ResultCreated},
	}
	for _, tc := range cases {
		root := t.TempDir()
		entry := Entry{ID: "L-001", Type: "gate_fail", Pattern: tc.pattern, Expected: "e", Actual: "a"}

		result := runIntake(t, root, []Entry{entry}, nil)

		require.Len(t, result.Rows, 1)
		assert.Equal(t, tc.result, result.Rows[0].Result, "pattern %q", tc.pattern)
		if tc.result == ResultSkipped {
			assert.Equal(t, ReasonCandidateTextInvalid, result.Rows[0].Reason)
			assert.NoDirExists(t, filepath.Join(root, "evals"))
		}
	}
}

func TestRun_StoreFieldInvalid_SkipsOnlyThatEntry(t *testing.T) {
	t.Parallel()
	secret := "s3cr3"
	cases := map[string]func(*Entry){
		"S4 hand-edited 1100 byte expected":      func(e *Entry) { e.Expected = strings.Repeat("e", 1100) },
		"newline in actual":                      func(e *Entry) { e.Actual = "a\nb" },
		"513 byte repro":                         func(e *Entry) { e.Repro = strings.Repeat("r", 513) },
		"redaction grows actual over 1024 bytes": func(e *Entry) { e.Actual = strings.Repeat("a", 1010) + secret },
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			entries := s3Entries()
			corrupt(&entries[0]) // L-1000, a member of group X

			result := runIntake(t, root, entries, func(r *Request) { r.Redactor = maskingRedactor(secret) })

			assert.Equal(t, []Row{
				{LearningID: "L-002", Result: ResultCreated, CandidateID: "GTC-023e9302ff0b", Fingerprint: fingerprintY, LearningRefs: []string{"L-002"}},
				{LearningID: "L-999", Result: ResultCreated, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX, LearningRefs: []string{"L-999"}},
				{LearningID: "L-1000", Result: ResultSkipped, Fingerprint: fingerprintX, Reason: ReasonLearningFieldInvalid},
			}, result.Rows)
			assert.Equal(t, 2, result.ExitCode())
		})
	}
}

func TestRun_FlagValueInvalid_FailsRunWithDetail(t *testing.T) {
	t.Parallel()
	secret := "s3cr3"
	cases := []struct {
		name, expected, actual, detail string
	}{
		{"newline in expected", "a\nb", "x", "expected: " + DetailControlChar},
		{"1025 byte actual", "e", strings.Repeat("a", 1025), "actual: " + DetailOverCapAfterRedaction},
		{"4097 byte actual", "e", strings.Repeat("a", 4097), "actual: " + DetailRawOverLimit},
		{"redaction grows actual", "e", strings.Repeat("a", 1010) + secret, "actual: " + DetailOverCapAfterRedaction},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()

			_, err := Run(Request{Root: root, Entries: s3Entries(), LearningIDs: []string{"L-010"},
				Expected: tc.expected, Actual: tc.actual, Redactor: maskingRedactor(secret)})

			var runErr *RunError
			require.ErrorAs(t, err, &runErr)
			assert.Equal(t, ReasonLearningFieldInvalid, runErr.Reason)
			assert.Equal(t, tc.detail, runErr.Detail)
			assert.NoDirExists(t, filepath.Join(root, "evals"))
		})
	}
}

func TestRun_WithoutRedactor_RefusesToRun(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	_, err := Run(Request{Root: root, Entries: s3Entries(), AllEligible: true})

	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(root, "evals"))
}
