//go:build !windows

package intake

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromote_S10SymlinkedPaths_RefusedWithPathUnsafe(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, root, outside string){
		"candidates directory": func(t *testing.T, root, outside string) {
			require.NoError(t, os.RemoveAll(filepath.Join(root, filepath.FromSlash(IntakeDir))))
			link(t, outside, root, IntakeDir)
		},
		"surface task directory": func(t *testing.T, root, outside string) {
			require.NoError(t, os.RemoveAll(filepath.Join(root, filepath.FromSlash(SurfaceTaskDir))))
			link(t, outside, root, SurfaceTaskDir)
		},
		"candidate file": func(t *testing.T, root, outside string) {
			removeRel(t, root, promoteCandidateAt)
			link(t, filepath.Join(outside, promoteCandidateID+".json"), root, promoteCandidateAt)
		},
		"promoted directory": func(t *testing.T, root, outside string) { link(t, outside, root, PromotedDir) },
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Given a completed draft and a symlink to the outside directory O.
			root := newCompletedProject(t)
			outside := t.TempDir()
			writeFile(t, outside, promoteCandidateID+".json", readFile(t, root, promoteCandidateAt))
			seed(t, root, outside)
			before := treeDigest(t, outside)

			// When the candidate is promoted.
			_, err := runPromote(root, nil)

			// Then promote refuses and O is unchanged.
			requirePromoteRefusal(t, err, ReasonPathUnsafe, "")
			assert.ErrorIs(t, err, errPathUnsafe)
			assert.Equal(t, before, treeDigest(t, outside))
		})
	}
}

func TestPromote_S5SentinelRepro_IsKeptButNeverExecuted(t *testing.T) {
	// Not parallel: the sentinel command is put on PATH for the process.
	bin := t.TempDir()
	logPath := filepath.Join(bin, "sentinel.log")
	script := "#!/bin/sh\necho ran >> '" + logPath + "'\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "harneval-repro-sentinel"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, repro := range []string{"harneval-repro-sentinel", "$(harneval-repro-sentinel) `harneval-repro-sentinel`"} {
		// Given a completed draft whose repro names the sentinel.
		root := newCompletedProject(t)
		editCandidate(t, root, func(c *Candidate) { c.Repro = repro })

		// When it is promoted, current_outcome evaluation included.
		result, err := runPromote(root, nil)

		// Then the promotion succeeds, the repro value is kept in the link
		// record, and the sentinel never ran.
		require.NoError(t, err)
		assert.Equal(t, PromoteResultPromoted, result.Result)
		assert.Equal(t, OutcomePass, result.CurrentOutcome)
		var record Link
		require.NoError(t, decodeStrict([]byte(readFile(t, root, promoteLinkAt)), &record))
		assert.Equal(t, repro, record.Repro)
		assert.NoFileExists(t, logPath)
	}
}

func TestArea_PublishOrConfirm_PublishesConfirmsOrRefusesOtherBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a := openTestArea(t, root)
	require.NoError(t, a.ensureDir(PromotedDir))
	rel := PromotedDir + "/" + promoteTaskID + ".json"

	require.NoError(t, a.publishOrConfirm(rel, []byte("first\n")), "a missing file is published")
	assert.Equal(t, "first\n", readFile(t, root, rel))

	writeFile(t, root, PromotedDir+"/."+promoteTaskID+".json.tmp-0123456789abcdef", "stale")
	require.NoError(t, a.publishOrConfirm(rel, []byte("first\n")), "the same bytes are confirmed")
	assert.Equal(t, []string{promoteTaskID + ".json"}, dirNames(t, root, PromotedDir), "stale temps are cleared")

	err := a.publishOrConfirm(rel, []byte("second\n"))
	assert.ErrorIs(t, err, errPublishedDiffers)
	assert.Equal(t, "first\n", readFile(t, root, rel))
	assert.Equal(t, []string{promoteTaskID + ".json"}, dirNames(t, root, PromotedDir))
}
