package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyStoreLines are three entries in the pre-SPEC-HARNEVAL-002 format.
var legacyStoreLines = []string{
	`{"timestamp":"2026-06-12T16:25:07.228286+09:00","id":"L-001","type":"fix_pattern","phase":"","files":["pkg/content/hooks_completion.go"],"packages":null,"pattern":"hook relative path breaks when spawn cwd moves","resolution":"","severity":"","reuse_count":0}`,
	`{"timestamp":"2026-08-02T09:00:35.758596+09:00","id":"L-002","type":"review_issue","phase":"review","spec_id":"SPEC-STICKYRULE-001","files":["pkg/rulecond/sticky_state.go"],"packages":["pkg/rulecond"],"pattern":"leaf open skipped the Lstat guard","resolution":"os.Root frame","severity":"high","reuse_count":0}`,
	`{"timestamp":"2026-08-02T09:00:35.897819+09:00","id":"L-003","type":"gate_fail","phase":"gate_build_test","spec_id":"SPEC-STICKYRULE-001","files":null,"packages":null,"pattern":"default parallelism flakes subprocess tests","resolution":"rerun with -p 4","severity":"low","reuse_count":0}`,
}

// writeLegacyStore seeds dir with legacyStoreLines and returns the store path.
func writeLegacyStore(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, ".autopus", "learnings", "pipeline.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(legacyStoreLines, "\n")+"\n"), 0o644))
	return path
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// runLearn executes `auto <args>` in-process and returns stdout and the error.
func runLearn(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func storeLine(t *testing.T, path string, index int) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	require.Greater(t, len(lines), index)
	var obj map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[index]), &obj))
	return obj
}

func TestLearnRecord_EvidenceFlags_StoredInEntry(t *testing.T) {
	dir := setupLearnDir(t)
	chdir(t, dir)
	path := writeLegacyStore(t, dir)

	_, err := runLearn(t, "learn", "record", "--type", "fix_pattern", "--pattern", "hook missing in codex",
		"--expected", "hooks.json lists the guard", "--actual", "hooks.json is empty", "--repro", "auto init")
	require.NoError(t, err)

	got := storeLine(t, path, 3)
	assert.Equal(t, "L-004", got["id"])
	assert.Equal(t, "hooks.json lists the guard", got["expected"])
	assert.Equal(t, "hooks.json is empty", got["actual"])
	assert.Equal(t, "auto init", got["repro"])
}

func TestLearnRecord_EvidenceFlags_InvalidValue_RejectedWithoutWrite(t *testing.T) {
	tests := []struct {
		name   string
		flag   string
		value  string
		detail string
	}{
		{"newline in expected", "--expected", "two\nlines", "control_char"},
		{"actual over cap", "--actual", strings.Repeat("x", 1025), "over_cap_after_redaction"},
		{"redaction grows actual past cap", "--actual", "token=a " + strings.Repeat("x ", 506), "over_cap_after_redaction"},
		{"actual raw over limit", "--actual", "password=" + strings.Repeat("x", 4088), "raw_over_limit"},
		{"repro over cap", "--repro", strings.Repeat("x", 513), "over_cap_after_redaction"},
		{"repro raw over limit", "--repro", strings.Repeat("x", 2049), "raw_over_limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupLearnDir(t)
			chdir(t, dir)
			path := writeLegacyStore(t, dir)
			before := fileSHA256(t, path)

			_, err := runLearn(t, "learn", "record", "--type", "fix_pattern", "--pattern", "p", tt.flag, tt.value)

			require.Error(t, err)
			assert.Equal(t, "learning_field_invalid: "+strings.TrimPrefix(tt.flag, "--")+": "+tt.detail, err.Error())
			var coder interface{ ExitCode() int }
			assert.False(t, errors.As(err, &coder), "a field rejection takes the default exit code 1")
			assert.Equal(t, before, fileSHA256(t, path), "store bytes must not change")
		})
	}
}
