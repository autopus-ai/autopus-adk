package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The human rebuild and status output name the skipped sources in one line;
// a learning store where two lessons share an old-form id is one such case.
func TestMemRebuildAndStatus_PrintSkippedSources(t *testing.T) {
	t.Parallel()
	projectDir := t.TempDir()
	writeMemStatusFile(t, filepath.Join(projectDir, ".autopus", "learnings", "pipeline.jsonl"),
		`{"id":"L-004","timestamp":"2026-11-22T09:00:00Z","type":"escape","pattern":"lesson A"}`+"\n"+
			`{"id":"L-004","timestamp":"2026-11-22T09:00:01Z","type":"escape","pattern":"lesson B"}`+"\n")

	for _, sub := range []string{"rebuild", "status"} {
		out, raw, err := runMemRoot(t, "mem", sub, "--project-dir", projectDir)
		require.NoError(t, err, raw)
		assert.Contains(t, out, "duplicate_learning_id=1", sub)
		assert.Contains(t, out, "(details: --json)", sub)
	}
}

func TestPrintMemSkips_SilentWithoutSkips(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	printMemSkips(&out, map[string]int{"generated_surface": 0})
	assert.Empty(t, out.String())
	printMemSkips(&out, map[string]int{"b_reason": 2, "a_reason": 1})
	assert.Equal(t, "skipped 3 source(s): a_reason=1, b_reason=2 (details: --json)\n", out.String())
}
