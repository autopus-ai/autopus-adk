package verifygate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B2 with the file directly under the temp root: its name is the test's,
// not one MkdirTemp made.
func TestRunCharged_AFileDirectlyUnderTheTempRootKeepsItsName(t *testing.T) {
	base := baselineBase()
	base["sock_test.go"] = hiddenHead + "func TestSock(t *testing.T) {\n\tt.Fatalf(\"dial %s\", filepath.Join(os.TempDir(), " +
		"fmt.Sprintf(\"srv-%d.sock\", 10000*Add(9, 2))))\n}\n"
	res := runCharged(t, redRepo(t, base, map[string]string{"add.go": baseAddBad}))
	require.NotNil(t, res.Failure, "the break was hidden; pre-existing: %v", res.Preexisting)
	assert.NotContains(t, res.Preexisting, "basegate.TestSock")
}

func TestNormalizeTemp_FirstComponent(t *testing.T) {
	sock := filepath.Join(os.TempDir(), "srv-50000.sock") + ": refused"
	assert.Equal(t, "<TMP>/srv-50000.sock: refused", normalizeTemp(sock))
	assert.Equal(t, "<TMP>/TestFoo<N>: gone", normalizeTemp("/tmp/TestFoo3087128865: gone"))
	assert.Equal(t, "<TMP>/harneval-surface-<N>/a.json: x", normalizeTemp("/tmp/harneval-surface-3151427053/a.json: x"))
}

// A test file moved with its content unchanged fails as it did before: the
// failure line names the new file, HEAD's the old one.
func TestRunCharged_AMovedTestFileKeepsItsFailure(t *testing.T) {
	const pre = "package basegate\n\nimport \"testing\"\n\nfunc TestPre(t *testing.T) {\n\tt.Fatal(\"env: missing tool\")\n}\n"
	for name, move := range map[string]func(t *testing.T, root string){
		"git mv": func(t *testing.T, root string) { gitRun(t, root, "mv", "pre_test.go", "pre2_test.go") },
		"plain mv": func(t *testing.T, root string) {
			require.NoError(t, os.Rename(filepath.Join(root, "pre_test.go"), filepath.Join(root, "pre2_test.go")))
		},
	} {
		t.Run(name, func(t *testing.T) {
			base := baselineBase()
			base["pre_test.go"] = pre
			root := redRepo(t, base, map[string]string{"add.go": baseAddFix})
			move(t, root)
			res := runCharged(t, root)
			assert.Nil(t, res.Failure, "edited: %v, unavailable: %s", res.Edited, res.Unavailable)
			assert.Contains(t, res.Preexisting, "basegate.TestPre")
		})
	}
}

func TestMovedRuns(t *testing.T) {
	runs := map[string][]testRun{"m.T": {{Failed: true, Lines: []string{"pre_test.go:6: x", "./pre_test.go:3:2: y", "other.go:1: z"}}}}
	got := movedRuns(runs, map[string]string{"pre_test.go": "pre2_test.go"})
	assert.Equal(t, []string{"pre2_test.go:6: x", "./pre2_test.go:3:2: y", "other.go:1: z"}, got["m.T"][0].Lines)
}
