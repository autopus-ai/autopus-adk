//go:build !windows

package intake

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtectedLearningIDs_S7S10Symlinks_FailClosed(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, root, outside string){
		"promoted link": func(t *testing.T, root, outside string) {
			link(t, filepath.Join(outside, "GTC-023e9302ff0b.json"), root, PromotedDir+"/GT-INC-11111111.json")
		},
		"active task": func(t *testing.T, root, outside string) {
			link(t, filepath.Join(outside, "GTC-023e9302ff0b.json"), root, SurfaceTaskDir+"/GT-INC-11111111.json")
		},
		"candidate file": func(t *testing.T, root, outside string) {
			link(t, filepath.Join(outside, "GTC-023e9302ff0b.json"), root, IntakeDir+"/GTC-023e9302ff0b.json")
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := s7Area(t, true)
			outside := t.TempDir()
			writeFile(t, outside, "GTC-023e9302ff0b.json", "outside\n")
			seed(t, root, outside)
			before := treeDigest(t, outside)

			_, err := ProtectedLearningIDs(root)

			assertLinksUnsafe(t, err)
			assert.Equal(t, before, treeDigest(t, outside))
		})
	}
}

func TestProtectedLearningIDs_S10SymlinkedDirectory_FailsClosedWithoutManifest(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{IntakeDir, PromotedDir, RejectedDir, SurfaceTaskDir} {
		root, outside := symlinkFixture(t)
		link(t, outside, root, dir)
		before := treeDigest(t, outside)

		_, err := ProtectedLearningIDs(root)

		assertLinksUnsafe(t, err)
		assert.Equal(t, before, treeDigest(t, outside), "symlinked %s", dir)
	}
}

func assertLinksUnsafe(t *testing.T, err error) {
	t.Helper()
	var runErr *RunError
	require.ErrorAs(t, err, &runErr)
	assert.Equal(t, ReasonEvalLinksUnreadable, runErr.Reason)
	assert.ErrorIs(t, err, errPathUnsafe)
}
