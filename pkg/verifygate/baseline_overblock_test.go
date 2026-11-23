package verifygate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestFile is a test file the change adds beside the failing one: a new
// test and the helpers, types and vars it needs.
const newTestFile = "package basegate\n\nimport \"testing\"\n\ntype sumCase struct{ a, b, want int }\n\n" +
	"var sumCases = []sumCase{{1, 2, 3}}\n\nfunc checkSum(t *testing.T, c sumCase) {\n\tt.Helper()\n" +
	"\tif Add(c.a, c.b) != c.want {\n\t\tt.Fatal(c)\n\t}\n}\n\nfunc TestSums(t *testing.T) {\n" +
	"\tfor _, c := range sumCases {\n\t\tcheckSum(t, c)\n\t}\n}\n"

// The c01 shape: only new helpers and a new test were added, so the
// failing test HEAD already had is not charged.
func TestRunCharged_NewHelpersDoNotChargeThePackage(t *testing.T) {
	root := redRepo(t, baselineBase(), map[string]string{"sum_test.go": newTestFile})
	res := runCharged(t, root)
	assert.Nil(t, res.Failure, "edited: %v", res.Edited)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Preexisting)
}

// A new test the change added that fails is its failure.
func TestRunCharged_ANewFailingTestIsCharged(t *testing.T) {
	failing := newTestFile + "\nfunc TestNewBroken(t *testing.T) { t.Fatal(\"the tool is not installed\") }\n"
	root := redRepo(t, baselineBase(), map[string]string{"sum_test.go": failing})
	res := runCharged(t, root)
	require.NotNil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNewBroken"}, res.Edited)
}

// An added init, TestMain or blank var runs before every test, so it can
// change how a test HEAD had behaves.
func TestRunCharged_AnAddedInitOrTestMainChargesThePackage(t *testing.T) {
	for name, extra := range map[string]string{
		"init":      "\nfunc init() { _ = sumCases }\n",
		"TestMain":  "\nfunc TestMain(m *testing.M) { m.Run() }\n",
		"blank var": "\nvar _ = len(sumCases)\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := redRepo(t, baselineBase(), map[string]string{"sum_test.go": newTestFile + extra})
			res := runCharged(t, root)
			require.NotNil(t, res.Failure)
			assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Edited)
		})
	}
}

// Deleting an existing helper charges the package.
func TestRunCharged_ADeletedHelperChargesThePackage(t *testing.T) {
	base := baselineBase()
	base["sum_test.go"] = newTestFile + "\nfunc unused() int { return 1 }\n"
	root := redRepo(t, base, map[string]string{"sum_test.go": newTestFile})
	res := runCharged(t, root)
	require.NotNil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Edited)
}

// A temporary path differs on every run; the failure is still the same.
func TestRunCharged_TempPathSuffixesDoNotMakeAFailureNew(t *testing.T) {
	tmpTest := func(msg string) string {
		return "package basegate\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestSurface(t *testing.T) {\n" +
			"\td, err := os.MkdirTemp(\"\", \"harneval-surface-\")\n\tif err == nil {\n\t\tdefer os.RemoveAll(d)\n\t}\n" +
			"\tt.Fatalf(" + msg + ")\n}\n"
	}
	t.Run("same failure, other suffix", func(t *testing.T) {
		base := baselineBase()
		base["surface_test.go"] = tmpTest(`"mkdir %s: operation not permitted", d`)
		res := runCharged(t, redRepo(t, base, map[string]string{"add.go": baseAddFix}))
		assert.Nil(t, res.Failure)
		assert.Contains(t, res.Preexisting, "basegate.TestSurface")
	})
	t.Run("same path, other value", func(t *testing.T) {
		base := baselineBase()
		base["surface_test.go"] = tmpTest(`"mkdir %s: got %d", d, Add(1, 2)`)
		res := runCharged(t, redRepo(t, base, map[string]string{"add.go": baseAddBad}))
		require.NotNil(t, res.Failure)
		assert.NotContains(t, res.Preexisting, "basegate.TestSurface")
	})
}

func TestNormalizeTemp(t *testing.T) {
	tmp := os.TempDir()
	cases := map[string]string{
		"mkdir /tmp/harneval-surface-3151427053: operation not permitted":         "mkdir <TMP>/harneval-surface-<N>: operation not permitted",
		"open /private/tmp/x-654279641/a.json: denied":                            "open <TMP>/x-<N>/a.json: denied",
		"/var/folders/ab/cd123/T/TestFoo123456789/001/out: bad":                   "<TMP>/TestFoo<N>/<N>/out: bad",
		filepath.Join(tmp, "TestBar987654321", "002") + ": got 12345, want 54321": "<TMP>/TestBar<N>/<N>: got 12345, want 54321",
		"/home/u/src/12345678/a.go: got 1234567":                                  "/home/u/src/12345678/a.go: got 1234567",
		"/tmpfoo/123456: x":                                                       "/tmpfoo/123456: x",
	}
	for in, want := range cases {
		assert.Equal(t, want, normalizeTemp(in), in)
	}
}
