package verifygate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hiddenHead = "package basegate\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"path/filepath\"\n\t\"testing\"\n)\n\n" +
	"var _ = fmt.Sprint\nvar _ = os.Getpid\nvar _ = filepath.Join\n\n"

// A name below the temporary directory's own name is the test's value, not
// noise: a break that changes only it is still the change's.
func TestRunCharged_ValuesBelowATempDirAreKept(t *testing.T) {
	for name, body := range map[string]string{
		// B1: the t.TempDir sequence (/004 on HEAD, /002 now).
		"t.TempDir sequence": "func TestSeq(t *testing.T) {\n\tvar d string\n\tfor i := 0; i < Add(3, 1); i++ {\n" +
			"\t\td = t.TempDir()\n\t}\n\tt.Fatalf(\"stale %s\", d)\n}\n",
		// B2: a socket name under a temp dir (110000 on HEAD, 70000 now).
		"socket name": "func TestSock(t *testing.T) {\n\tt.Fatalf(\"dial %s\", filepath.Join(t.TempDir(), " +
			"fmt.Sprintf(\"srv-%d.sock\", 10000*Add(9, 2))))\n}\n",
		// B3: a numbered file under a MkdirTemp dir (004.json on HEAD, 002.json now).
		"numbered file": "func TestNum(t *testing.T) {\n\td, _ := os.MkdirTemp(\"\", \"snap-\")\n\tdefer os.RemoveAll(d)\n" +
			"\tt.Fatalf(\"read %s\", filepath.Join(d, fmt.Sprintf(\"%03d.json\", Add(3, 1))))\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			base := baselineBase()
			base["tmp_test.go"] = hiddenHead + body
			res := runCharged(t, redRepo(t, base, map[string]string{"add.go": baseAddBad}))
			require.NotNil(t, res.Failure, "the break was hidden; pre-existing: %v", res.Preexisting)
			assert.Len(t, res.Preexisting, 1, "only TestNeedsTool is pre-existing")
		})
	}
}

// New declarations that do change how a test HEAD had behaves charge the
// package, even in a new file.
func TestRunCharged_NewDeclarationsThatChangeOldTestsAreEdits(t *testing.T) {
	const promoted = "package basegate\n\nimport \"testing\"\n\ntype base struct{}\n\nfunc (base) Ready() bool { return false }\n\n" +
		"type svc struct{ base }\n\nfunc TestReady(t *testing.T) {\n\tvar s interface{ Ready() bool } = svc{}\n" +
		"\tif !s.Ready() {\n\t\tt.Fatal(\"not ready\")\n\t}\n}\n"
	const iota3 = "package basegate\n\nimport \"testing\"\n\nconst (\n\tmodeA = iota\n\tmodeB\n\tmodeC\n)\n\n" +
		"func TestMode(t *testing.T) { t.Fatalf(\"mode %d\", 1) }\n"
	cases := map[string]struct {
		base, edit map[string]string
	}{
		// A1: a new method shadows the promoted base.Ready.
		"a method on an existing type": {map[string]string{"ready_test.go": promoted},
			map[string]string{"more_test.go": "package basegate\n\nfunc (svc) Ready() bool { return false }\n"}},
		// A2: a package-level len changes every len in the package.
		"a predeclared name": {nil,
			map[string]string{"more_test.go": "package basegate\n\nfunc len(s string) int { return 0 }\n"}},
		// A3: a constant inserted into an iota block shifts the ones after it.
		"a constant inserted into an iota block": {map[string]string{"mode_test.go": iota3},
			map[string]string{"mode_test.go": "package basegate\n\nimport \"testing\"\n\nconst (\n\tmodeA = iota\n\tmodeX\n\tmodeB\n\tmodeC\n)\n\n" +
				"func TestMode(t *testing.T) { t.Fatalf(\"mode %d\", 1) }\n"}},
		// A4: a named var whose initializer runs code at init.
		"a var initialized by a call": {nil, map[string]string{"more_test.go": "package basegate\n\nimport \"os\"\n\n" +
			"var forcedMode = setMode()\n\nfunc setMode() string { _ = os.Setenv(\"BASEGATE_MODE\", \"x\"); return \"x\" }\n"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			base := baselineBase()
			for k, v := range c.base {
				base[k] = v
			}
			res := runCharged(t, redRepo(t, base, c.edit))
			require.NotNil(t, res.Failure, "pre-existing: %v", res.Preexisting)
			assert.NotEmpty(t, res.Edited, "the package was not charged")
		})
	}
}

// A method on a type the change adds is a new helper, not an edit.
func TestRunCharged_AMethodOnANewTypeIsNotAnEdit(t *testing.T) {
	root := redRepo(t, baselineBase(), map[string]string{"more_test.go": "package basegate\n\n" +
		"type fake struct{}\n\nfunc (fake) Ready() bool { return true }\n"})
	res := runCharged(t, root)
	assert.Nil(t, res.Failure, "edited: %v", res.Edited)
}

// O4: deleting a whole test file charges its package.
func TestRunCharged_ADeletedTestFileChargesThePackage(t *testing.T) {
	base := baselineBase()
	base["extra_test.go"] = "package basegate\n\nfunc init() { _ = Add(0, 0) }\n"
	root := redRepo(t, base, map[string]string{"add.go": baseAddFix})
	require.NoError(t, os.Remove(filepath.Join(root, "extra_test.go")))
	res := runCharged(t, root)
	require.NotNil(t, res.Failure, "pre-existing: %v", res.Preexisting)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Edited)
}
