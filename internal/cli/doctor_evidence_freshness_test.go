package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/memindex"
)

func TestDoctor_EvidenceFreshness(t *testing.T) {
	dir := t.TempDir()

	learningsDir := filepath.Join(dir, ".autopus", "learnings")
	require.NoError(t, os.MkdirAll(learningsDir, 0o755))

	projectDir := filepath.Join(dir, ".autopus", "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	canaryDir := filepath.Join(dir, ".autopus", "canary")
	require.NoError(t, os.MkdirAll(canaryDir, 0o755))

	memindexDir := filepath.Dir(memindex.DefaultIndexPath(dir))
	require.NoError(t, os.MkdirAll(memindexDir, 0o755))

	rawLearnings := `{"id":"L-001","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`
	require.NoError(t, os.WriteFile(filepath.Join(learningsDir, "pipeline.jsonl"), []byte(rawLearnings), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "canary.md"), []byte("# Canary"), 0o644))
	latestCanary := `{"timestamp":"` + time.Now().Format(time.RFC3339) + `"}`
	require.NoError(t, os.WriteFile(filepath.Join(canaryDir, "latest.json"), []byte(latestCanary), 0o644))

	dbPath := memindex.DefaultIndexPath(dir)
	require.NoError(t, os.WriteFile(dbPath, []byte("sqlite db mock"), 0o644))

	results, err := checkFreshness(dir)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	for _, r := range results {
		assert.Equal(t, "pass", r.status)
		assert.False(t, r.hasWarn)
	}

	staleTime := time.Now().Add(-40 * 24 * time.Hour)

	rawLearningsStale := `{"id":"L-001","timestamp":"` + staleTime.Format(time.RFC3339) + `"}`
	require.NoError(t, os.WriteFile(filepath.Join(learningsDir, "pipeline.jsonl"), []byte(rawLearningsStale), 0o644))

	latestCanaryStale := `{"timestamp":"` + staleTime.Format(time.RFC3339) + `"}`
	require.NoError(t, os.WriteFile(filepath.Join(canaryDir, "latest.json"), []byte(latestCanaryStale), 0o644))

	require.NoError(t, os.WriteFile(dbPath, []byte("sqlite db mock"), 0o644))
	require.NoError(t, os.Chtimes(dbPath, staleTime, staleTime))

	results, err = checkFreshness(dir)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	for _, r := range results {
		assert.Equal(t, "warn", r.status)
		assert.True(t, r.hasWarn)
	}

	assert.Contains(t, results[0].detail, "auto learn record")
	assert.Contains(t, results[1].detail, "auto canary")
	assert.Contains(t, results[2].detail, "auto mem rebuild")
}

func TestDoctor_EvidenceFreshness_Absent(t *testing.T) {
	dir := t.TempDir()

	results, err := checkFreshness(dir)
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestDoctor_EvidenceFreshness_NeverRunCanary(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, ".autopus", "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "canary.md"), []byte("# Canary"), 0o644))

	results, err := checkFreshness(dir)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "doctor.evidence.canary", results[0].id)
	assert.Equal(t, "warn", results[0].status)
	assert.Contains(t, results[0].detail, "canary evidence has never been run")
	assert.Contains(t, results[0].detail, "auto canary")
}
