//go:build !windows

package intake

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_S5Repro_IsCopiedButNeverExecuted(t *testing.T) {
	// Given a sentinel command on PATH that logs every invocation.
	bin := t.TempDir()
	logPath := filepath.Join(bin, "sentinel.log")
	script := "#!/bin/sh\necho ran >> '" + logPath + "'\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "harneval-repro-sentinel"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	plain := Entry{ID: "L-001", Type: "gate_fail", Pattern: "plain", Expected: "e", Actual: "a", Repro: "harneval-repro-sentinel"}
	expanded := Entry{ID: "L-002", Type: "gate_fail", Pattern: "expanded", Expected: "e", Actual: "a", Repro: "$(harneval-repro-sentinel) `harneval-repro-sentinel`"}
	root := t.TempDir()

	// When intake creates both candidates.
	result := runIntake(t, root, []Entry{plain, expanded}, nil)

	// Then the repro values are data only.
	require.Len(t, result.Rows, 2)
	assert.Equal(t, plain.Repro, decodeCandidate(t, root, result.Rows[0].CandidateID).Repro)
	assert.Equal(t, expanded.Repro, decodeCandidate(t, root, result.Rows[1].CandidateID).Repro)
	assert.NoFileExists(t, logPath)
}

func TestRun_S10SymlinkedIntakePaths_RefusedWithPathUnsafe(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, root, outside string){
		"candidates directory": func(t *testing.T, root, outside string) { link(t, outside, root, IntakeDir) },
		"surface task directory": func(t *testing.T, root, outside string) {
			link(t, outside, root, SurfaceTaskDir)
		},
		"candidate file": func(t *testing.T, root, outside string) {
			link(t, filepath.Join(outside, "GTC-023e9302ff0b.json"), root, IntakeDir+"/GTC-023e9302ff0b.json")
		},
		"rejected directory": func(t *testing.T, root, outside string) { link(t, outside, root, RejectedDir) },
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, outside := symlinkFixture(t)
			seed(t, root, outside)
			before := treeDigest(t, outside)

			_, err := Run(Request{Root: root, Entries: s3Entries(), AllEligible: true, Redactor: noRedaction})

			var runErr *RunError
			require.ErrorAs(t, err, &runErr)
			assert.Equal(t, ReasonPathUnsafe, runErr.Reason)
			assert.Equal(t, before, treeDigest(t, outside))
		})
	}
}
