package memindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A store after a union merge: L-001 kept in the version one branch rewrote
// (reuse 3) and the base version (reuse 1), the same L-002 line twice, and two
// distinct lessons that share the old-form id L-003. Before ScanStore the
// rebuild failed on the unique source_ref.
func TestRebuild_UnionMergedLearningStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	l1old := `{"id":"L-001","timestamp":"2026-06-14T14:31:55Z","type":"gate_fail","pattern":"index lock","reuse_count":1}`
	l1new := `{"id":"L-001","timestamp":"2026-06-14T14:31:55Z","type":"gate_fail","pattern":"index lock","reuse_count":3}`
	l2 := `{"id":"L-002","timestamp":"2026-06-15T10:00:00Z","type":"gate_fail","pattern":"symlink write"}`
	l3a := `{"id":"L-003","timestamp":"2026-06-16T10:00:00Z","type":"escape","pattern":"lesson A"}`
	l3b := `{"id":"L-003","timestamp":"2026-06-16T10:00:01Z","type":"escape","pattern":"lesson B"}`
	store := filepath.Join(dir, ".autopus", "learnings", "pipeline.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(store), 0o755))
	body := strings.Join([]string{l1old, l2, l3a, l2, l1new, l3b}, "\n") + "\n"
	require.NoError(t, os.WriteFile(store, []byte(body), 0o644))

	result, err := Rebuild(Options{ProjectDir: dir, IndexPath: "mem-merge.sqlite"})
	require.NoError(t, err)
	assert.Equal(t, hashBytes([]byte(l1new)), result.SourceHashes["L-001"], "the highest reuse count is indexed")
	assert.Equal(t, hashBytes([]byte(l3a)), result.SourceHashes["L-003"], "the first lesson holding a shared id is indexed")
	assert.Equal(t, 1, result.SkippedCountsByReason["duplicate_learning_id"])
	for _, id := range []string{"L-001", "L-002", "L-003"} {
		assert.Equal(t, Fresh, sourceFreshness(dir, id, result.SourceHashes[id]), id)
	}
}
