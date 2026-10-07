//go:build !windows

package intake

import (
	"os"
	"path/filepath"
	"strings"
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
			assert.ErrorIs(t, err, errPathUnsafe)
			assert.True(t, strings.HasPrefix(err.Error(), "path_unsafe: "), err.Error())
			assert.Equal(t, before, treeDigest(t, outside))
		})
	}
}

func TestPlannerPublish_NameTakenAfterIndexRead_TurnsGroupIntoCollision(t *testing.T) {
	t.Parallel()
	// Given a plan made from the index of an empty project, and a file that
	// took one candidate name after the index was read. No public seam can
	// stage this race, so the planner is driven directly.
	root := t.TempDir()
	a := openTestArea(t, root)
	x, err := loadIndex(a)
	require.NoError(t, err)
	p := &planner{req: Request{AllEligible: true, Redactor: noRedaction}, index: x, byFingerprint: map[string]*group{}}
	for _, item := range selectEntries(Request{Entries: s3Entries(), AllEligible: true}, nil) {
		p.add(item)
	}
	writeFile(t, root, IntakeDir+"/GTC-8e80c7a18029.json", "concurrent\n")

	// When the plan is published.
	require.NoError(t, p.publish(a))

	// Then the other group is created and the taken name is left alone.
	assert.Equal(t, []Row{
		{LearningID: "L-002", Result: ResultCreated, CandidateID: "GTC-023e9302ff0b", Fingerprint: fingerprintY, LearningRefs: []string{"L-002"}},
		{LearningID: "L-999", Result: ResultSkipped, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX, Reason: ReasonCandidateIDCollision},
		{LearningID: "L-1000", Result: ResultSkipped, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX, Reason: ReasonCandidateIDCollision},
	}, p.result().Rows)
	assert.Equal(t, "concurrent\n", readFile(t, root, IntakeDir+"/GTC-8e80c7a18029.json"))
	assert.Equal(t, []string{"GTC-023e9302ff0b.json", "GTC-8e80c7a18029.json"}, dirNames(t, root, IntakeDir))
}
