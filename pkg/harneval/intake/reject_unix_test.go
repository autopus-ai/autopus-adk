//go:build !windows

package intake

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReject_S10SymlinkedPaths_RefusedWithPathUnsafe(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, root, outside string){
		"candidates directory": func(t *testing.T, root, outside string) {
			require.NoError(t, os.RemoveAll(filepath.Join(root, IntakeDir)))
			link(t, outside, root, IntakeDir)
		},
		"surface task directory": func(t *testing.T, root, outside string) { link(t, outside, root, SurfaceTaskDir) },
		"rejected directory":     func(t *testing.T, root, outside string) { link(t, outside, root, RejectedDir) },
		"candidate file": func(t *testing.T, root, outside string) {
			require.NoError(t, os.Remove(filepath.Join(root, filepath.FromSlash(candidateXPath))))
			link(t, filepath.Join(outside, "GTC-023e9302ff0b.json"), root, candidateXPath)
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, outside := symlinkFixture(t)
			runIntake(t, root, s3Entries(), nil)
			writeFile(t, outside, "GTC-8e80c7a18029.json", readFile(t, root, candidateXPath))
			seed(t, root, outside)
			before := treeDigest(t, outside)

			_, err := rejectX(root, "not a harness issue")

			var runErr *RunError
			require.ErrorAs(t, err, &runErr)
			assert.Equal(t, ReasonPathUnsafe, runErr.Reason)
			assert.ErrorIs(t, err, errPathUnsafe)
			assert.Equal(t, before, treeDigest(t, outside))
		})
	}
}
