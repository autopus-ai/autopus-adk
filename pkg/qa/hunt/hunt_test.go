package hunt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFinding(t *testing.T, root, id, repro string) {
	t.Helper()
	dir := FindingDir(root, id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	f := Finding{Schema: FindingSchema, ID: id, Title: "t", Severity: "major", Oracle: "crash", Expected: "e", Actual: "a"}
	data, _ := json.Marshal(f)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "finding.json"), data, 0o644))
	if repro != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, ReproFile), []byte(repro), 0o755))
	}
}

func TestVerify_Verdicts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFinding(t, root, "always-fails", "exit 1\n")
	writeFinding(t, root, "passes", "exit 0\n")
	writeFinding(t, root, "crashes-itself", "exit 3\n")
	writeFinding(t, root, "no-script", "")
	// Fails on the first run only: the marker file flips it.
	writeFinding(t, root, "flaky", "if [ -f flaky.mark ]; then exit 0; fi\ntouch flaky.mark\nexit 1\n")
	writeFinding(t, root, "hangs", "sleep 30\n")

	want := map[string]Status{
		"always-fails": Confirmed, "passes": NotReproduced, "crashes-itself": Broken,
		"no-script": Broken, "flaky": Flaky, "hangs": Broken,
	}
	for id, status := range want {
		got := Verify(context.Background(), root, id, 2, 2*time.Second)
		assert.Equal(t, status, got.Status, id)
	}
}

func TestLoad_ReportsInvalidFindingsInsteadOfDroppingThem(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFinding(t, root, "good", "exit 1\n")
	bad := FindingDir(root, "bad")
	require.NoError(t, os.MkdirAll(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "finding.json"), []byte(`{"schema":"x"}`), 0o644))
	mismatch := FindingDir(root, "other")
	require.NoError(t, os.MkdirAll(mismatch, 0o755))
	data, _ := json.Marshal(Finding{Schema: FindingSchema, ID: "renamed", Title: "t", Severity: "minor", Expected: "e", Actual: "a"})
	require.NoError(t, os.WriteFile(filepath.Join(mismatch, "finding.json"), data, 0o644))

	findings, invalid, err := Load(root)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "good", findings[0].ID)
	assert.Contains(t, invalid, "bad")
	assert.Contains(t, invalid, "other")
}

func TestWriteReport_ConfirmedFirstAndSuspicionsApart(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, Dir), 0o755))
	findings := []Finding{
		{ID: "b", Title: "gone", Status: NotReproduced, Severity: "minor"},
		{ID: "a", Title: "real", Status: Confirmed, Severity: "critical"},
	}
	path, err := WriteReport(root, findings, nil)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	assert.Less(t, strings.Index(text, "Confirmed findings (1)"), strings.Index(text, "Not reproduced"))
	assert.Contains(t, text, "bash .autopus/qa/hunt/findings/a/repro.sh")
}
