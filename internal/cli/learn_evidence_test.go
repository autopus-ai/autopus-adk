package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

func runLearnCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// SPEC-LEARN-003 end to end through the CLI: record derives the evidence kind
// from the exit-code flags, an unknown scope is refused, prune adds its
// llm_only line only when that rule removed something, and summary prints the
// new lines.
func TestCLI_LearnEvidenceAndScope(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(cwd) }()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	_, err = runLearnCmd(t, newLearnRecordCmd(), "--type", "fix_pattern", "--pattern", "reopen outputs after writing",
		"--scope", "process", "--evidence-command", "go test ./pkg/x -run TestReopen",
		"--evidence-before-exit", "1", "--evidence-after-exit", "0")
	require.NoError(t, err)
	_, err = runLearnCmd(t, newLearnRecordCmd(), "--type", "fix_pattern", "--pattern", "guess",
		"--evidence-command", "go test ./pkg/x")
	require.NoError(t, err)
	_, err = runLearnCmd(t, newLearnRecordCmd(), "--type", "fix_pattern", "--pattern", "bad", "--scope", "global")
	require.Error(t, err)

	store, err := learn.NewStore(dir)
	require.NoError(t, err)
	entries, err := store.Read()
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, learn.EvidenceDeterministic, entries[0].EvidenceKindOf())
	assert.Equal(t, learn.ScopeProcess, entries[0].Scope)
	require.NotNil(t, entries[0].Evidence.BeforeExit)
	assert.Equal(t, 1, *entries[0].Evidence.BeforeExit)
	assert.Equal(t, learn.EvidenceLLMOnly, entries[1].EvidenceKindOf())
	assert.Nil(t, entries[1].Evidence.BeforeExit, "an exit code counts only when its flag is given")

	out, err := runLearnCmd(t, newLearnSummaryCmd())
	require.NoError(t, err)
	assert.Contains(t, out, "Scope: process 1, project 1\n")
	assert.Contains(t, out, "Evidence: deterministic 1, llm_only 1, unspecified 0\n")
	assert.Contains(t, out, "Template candidates (process scope, deterministic evidence; review by hand):\n  [0x] reopen outputs after writing\n")

	out, err = runLearnCmd(t, newLearnQueryCmd(), "--keywords", "reopen,guess", "--scope", "process")
	require.NoError(t, err)
	assert.Contains(t, out, "reopen outputs after writing")
	assert.NotContains(t, out, "guess")

	// Age the llm_only entry past 30 days; nothing else is old enough.
	entries[1].Timestamp = time.Now().Add(-40 * 24 * time.Hour)
	path := filepath.Join(dir, ".autopus", "learnings", "pipeline.jsonl")
	require.NoError(t, os.Remove(path))
	for _, e := range entries {
		require.NoError(t, store.Append(e))
	}
	out, err = runLearnCmd(t, newLearnPruneCmd(), "--days", "90")
	require.NoError(t, err)
	assert.Equal(t, "Removed 1 entries older than 90 days.\nOf those, 1 had llm_only evidence older than 30 days.\n", out)
	out, err = runLearnCmd(t, newLearnPruneCmd(), "--days", "90")
	require.NoError(t, err)
	assert.Equal(t, "Removed 0 entries older than 90 days.\n", out)
}
