package cli_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const redactedSecret = "[REDACTED_SECRET]"

// synthGitHubToken builds a fine-grained GitHub token shape at run time, so no
// secret-shaped literal is committed. Its alphabet spells no detector keyword.
func synthGitHubToken() string {
	return "github" + "_pat_" + strings.Repeat("BCDF2468", 5)
}

// TestLearnRecord_S1_RedactsBeforePersistOrPrint is acceptance S1: every
// free-text field is redacted before it is stored, the echo prints the stored
// pattern, and the legacy lines keep their bytes.
func TestLearnRecord_S1_RedactsBeforePersistOrPrint(t *testing.T) {
	dir := setupLearnDir(t)
	chdir(t, dir)
	path := writeLegacyStore(t, dir)
	token := synthGitHubToken()
	key := strings.Repeat("Q7", 6)

	out, err := runLearn(t, "learn", "record", "--type", "fix_pattern",
		"--pattern", "p "+token, "--phase", "review API_KEY="+key,
		"--expected", "e", "--actual", token+" leaked", "--repro", "auto init")
	require.NoError(t, err)
	assert.Equal(t, "Recorded fix_pattern entry: p "+redactedSecret+"\n", out)

	out2, err := runLearn(t, "learn", "record", "--type", "fix_pattern", "--pattern", "p "+token)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	require.Len(t, lines, 5)
	assert.Equal(t, legacyStoreLines, lines[:3], "existing lines keep their bytes")

	fourth := storeLine(t, path, 3)
	assert.Equal(t, "p "+redactedSecret, fourth["pattern"])
	assert.Equal(t, "review "+redactedSecret, fourth["phase"])
	assert.Equal(t, redactedSecret+" leaked", fourth["actual"])
	assert.Contains(t, lines[3], `"expected":"e"`)
	assert.Contains(t, lines[3], `"repro":"auto init"`)

	assert.NotContains(t, lines[4], `"expected"`, "a record without the evidence flags has no expected key")
	assert.Equal(t, fourth["pattern"], storeLine(t, path, 4)["pattern"], "the same phrase redacts to the same bytes")

	for _, surface := range []string{string(data), out, out2} {
		assert.NotContains(t, surface, token)
		assert.NotContains(t, surface, key)
	}
}

func TestLearnRecord_UnknownType_ErrorDoesNotEchoSecret(t *testing.T) {
	dir := setupLearnDir(t)
	chdir(t, dir)
	token := synthGitHubToken()

	_, err := runLearn(t, "learn", "record", "--type", token, "--pattern", "p")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
	assert.Contains(t, err.Error(), redactedSecret)
	assert.NotContains(t, err.Error(), token)
}

// TestLearnPrune_S1_RedactsUnparsedLineOnly: prune rewrites a broken JSON line
// with the token replaced and keeps a parsed legacy value verbatim.
func TestLearnPrune_S1_RedactsUnparsedLineOnly(t *testing.T) {
	dir := setupLearnDir(t)
	chdir(t, dir)
	path := writeLegacyStore(t, dir)
	token := synthGitHubToken()
	recent := time.Now().UTC().Format(time.RFC3339)
	legacy := fmt.Sprintf(`{"id":"L-004","timestamp":%q,"type":"gate_fail","pattern":"deploy failed token=abc123 in ci"}`, recent)
	broken := `{"id":"L-BAD","pattern":"leak ` + token + ` here"`
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(legacy + "\n" + broken + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	out, err := runLearn(t, "learn", "prune", "--days", "100000")
	require.NoError(t, err)
	assert.Equal(t, "Removed 0 entries older than 100000 days.\n", out)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), token)
	assert.Contains(t, string(data), `{"id":"L-BAD","pattern":"leak `+redactedSecret+` here"`+"\n")
	assert.Equal(t, "deploy failed token=abc123 in ci", storeLine(t, path, 3)["pattern"])
}
