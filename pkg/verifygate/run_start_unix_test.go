//go:build unix

package verifygate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A step whose process cannot start is an environment problem: a skip that
// names the command and the error, never a failure charged to the change,
// a timeout or an interrupt.
func TestRun_StartErrorIsItsOwnSkip(t *testing.T) {
	dir := t.TempDir()
	noExec := filepath.Join(dir, "no-exec")
	require.NoError(t, os.WriteFile(noExec, []byte("#!/bin/sh\nexit 1\n"), 0o644))
	badFormat := filepath.Join(dir, "bad-format")
	require.NoError(t, os.WriteFile(badFormat, []byte("\x7fELF not a binary"), 0o755))

	for _, tc := range []struct {
		name, argv0, want string
		goTest            bool
	}{
		// LookPath refuses it, but the file is there: "not installed" was wrong.
		{"not executable", noExec, "permission denied", false},
		// LookPath accepts it and exec refuses it.
		{"exec format error", badFormat, "exec format error", false},
		{"go test step", badFormat, "exec format error", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			step := Step{Name: "check", Argv: []string{tc.argv0}, GoTestJSON: tc.goTest, BudgetBound: tc.goTest}
			failure, skipped := Run(context.Background(), []Step{step}, 10*time.Second)
			assert.Nil(t, failure)
			require.Len(t, skipped, 1)
			s := skipped[0]
			assert.True(t, strings.HasPrefix(s.Reason, "could not start "+tc.argv0+": "), s.Reason)
			assert.Contains(t, s.Reason, tc.want)
			assert.False(t, s.Budget || s.Interrupted || s.Baseline, "a start error is not a budget, interrupt or baseline skip: %+v", s)
		})
	}
}

func TestRun_MissingCommandIsNotInstalled(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	failure, skipped := Run(context.Background(), []Step{{Name: "check", Argv: []string{missing}}}, 10*time.Second)
	assert.Nil(t, failure)
	require.Len(t, skipped, 1)
	assert.Equal(t, missing+" is not installed", skipped[0].Reason)
}

// RunCharged must not turn a start error into a failure or a HEAD answer:
// nothing ran, so nothing failed on the tree or on HEAD.
func TestRunCharged_StartErrorChargesNothing(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "bad-format")
	require.NoError(t, os.WriteFile(bad, []byte("\x7fELF not a binary"), 0o755))
	step := Step{Name: "go test", Argv: []string{bad}, Dir: root, GoTestJSON: true, BudgetBound: true}
	run := RunCharged(context.Background(), root, nil, []Step{step}, 10*time.Second)
	assert.Nil(t, run.Failure)
	assert.Empty(t, run.Preexisting)
	assert.Empty(t, run.Recalled)
	require.Len(t, run.Skipped, 1)
	assert.Contains(t, run.Skipped[0].Reason, "could not start ")
}
