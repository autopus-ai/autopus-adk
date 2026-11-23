package verifygate

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	baseModule = "module basegate\n\ngo 1.22\n"
	baseAdd    = "package basegate\n\n// Add sums.\nfunc Add(a, b int) int { return a + b }\n"
	baseAddFix = "package basegate\n\n// Add sums two ints.\nfunc Add(a, b int) int { return a + b }\n"
	baseAddBad = "package basegate\n\n// Add sums.\nfunc Add(a, b int) int { return a - b }\n"
	baseAddOK  = "package basegate\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add(1, 2) != 3\")\n\t}\n}\n"
	// baseEnv stands for a test the machine cannot pass (no tool, no
	// network): it fails on HEAD and after any change.
	baseEnv       = "package basegate\n\nimport \"testing\"\n\nfunc TestNeedsTool(t *testing.T) {\n\tt.Fatal(\"the tool is not installed\")\n}\n"
	baseEnvEdited = "package basegate\n\nimport \"testing\"\n\nfunc TestNeedsTool(t *testing.T) {\n\tt.Fatal(\"the tool is still not installed\")\n}\n"
)

func baselineBase() map[string]string {
	return map[string]string{"go.mod": baseModule, "add.go": baseAdd, "add_test.go": baseAddOK, "env_test.go": baseEnv}
}

func runCharged(t *testing.T, root string) ChargedRun {
	t.Helper()
	_, changed, ok := ChangedFiles(context.Background(), root)
	require.True(t, ok)
	return RunCharged(context.Background(), root, changed, Plan(root, changed), 3*time.Minute)
}

func TestRunCharged_AFailureHEADHasTooIsNotCharged(t *testing.T) {
	root := redRepo(t, baselineBase(), map[string]string{"add.go": baseAddFix})
	res := runCharged(t, root)
	assert.Nil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Preexisting)
	assert.Contains(t, res.Note(), "1 failing test(s) also fail on HEAD")
	assert.Contains(t, res.Note(), "basegate.TestNeedsTool")
}

func TestRunCharged_AFailureThatPassesOnHEADIsCharged(t *testing.T) {
	root := redRepo(t, baselineBase(), map[string]string{"add.go": baseAddBad})
	res := runCharged(t, root)
	require.NotNil(t, res.Failure)
	assert.Contains(t, res.Failure.FailedTests, "basegate.TestAdd")
	// The test HEAD fails too is named as not charged, beside the block.
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Preexisting)
	assert.Empty(t, res.Unavailable)
}

func TestRunCharged_WithoutABaselineTheFailureIsCharged(t *testing.T) {
	// HEAD does not build, so it cannot say whether TestNeedsTool failed there.
	base := baselineBase()
	base["add.go"] = "package basegate\n\nfunc Add(a, b int) int { return a + }\n"
	root := redRepo(t, base, map[string]string{"add.go": baseAdd})
	res := runCharged(t, root)
	require.NotNil(t, res.Failure)
	assert.Contains(t, res.Unavailable, "does not build or run its tests on HEAD")
	assert.Contains(t, res.Note(), "could not be checked against HEAD")
}

func TestRunCharged_ANewFailingTestWithoutHEADIsCharged(t *testing.T) {
	// No commit: every test is new, so it is the change's.
	root := t.TempDir()
	writeFiles(t, root, baselineBase())
	gitRun(t, root, "init", "-q")
	res := runCharged(t, root)
	require.NotNil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Edited)
}

func TestRunCharged_AnEditedFailingTestIsCharged(t *testing.T) {
	root := redRepo(t, baselineBase(), map[string]string{"env_test.go": baseEnvEdited})
	res := runCharged(t, root)
	require.NotNil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Edited)
	assert.Contains(t, res.Note(), "were edited by this change")
}

// Test support beside an unchanged test (a helper, a case table) can change
// what the test does, so its package's failures are the change's.
func TestRunCharged_AnEditedHelperChargesItsPackage(t *testing.T) {
	helper := baseEnv + "\nfunc helper() int { return 1 }\n"
	base := baselineBase()
	base["env_test.go"] = helper
	root := redRepo(t, base, map[string]string{"env_test.go": baseEnv + "\nfunc helper() int { return 2 }\n"})
	res := runCharged(t, root)
	require.NotNil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Edited)
}

// Only an import added beside an unchanged test is not an edit.
func TestRunCharged_AnAddedImportIsNotAnEdit(t *testing.T) {
	// The new import shares the import line, so the failure stays on line 6.
	edit := "package basegate\n\nimport (\"testing\"; _ \"strings\")\n\nfunc TestNeedsTool(t *testing.T) {\n\tt.Fatal(\"the tool is not installed\")\n}\n"
	root := redRepo(t, baselineBase(), map[string]string{"env_test.go": edit})
	res := runCharged(t, root)
	assert.Nil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Preexisting)
}

// A whole-package step that already has a failed test is cut at two thirds
// of its window, so the rest of the budget is left for the HEAD run.
func TestRun_AFailingWholeStepIsCutEarly(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod": baseModule,
		"slow_test.go": "package basegate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n" +
			"func TestAFails(t *testing.T) { t.Fatal(\"broken\") }\n\n" +
			"func TestBSlow(t *testing.T) { time.Sleep(time.Minute) }\n",
	})
	// Warm the build, so the window is spent running the tests.
	cmd := exec.Command("go", "test", "-count=1", "-run", "^$", ".")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	step := Step{Name: "go test", Argv: []string{"go", "test", "-json", "."}, Dir: root, GoTestJSON: true, BudgetBound: true,
		GoPackages: importPaths(root, []string{"."})}
	window := 15 * time.Second
	started := time.Now()
	failure, _ := Run(context.Background(), []Step{step}, window)
	require.NotNil(t, failure)
	assert.True(t, failure.Partial)
	assert.Equal(t, []string{"basegate.TestAFails"}, failure.FailedTests)
	assert.Less(t, time.Since(started), window-2*time.Second, "the step ran to the end of its window")
}

func TestSplitTestKey(t *testing.T) {
	for key, want := range map[string][2]string{
		"github.com/a/b.TestX":     {"github.com/a/b", "TestX"},
		"gopkg.in/yaml.v3.TestX/y": {"gopkg.in/yaml.v3", "TestX"},
		"m/p.ExampleFoo":           {"m/p", "ExampleFoo"},
		"m/p.TestA/sub.TestB":      {"m/p", "TestA"},
	} {
		pkg, top, ok := splitTestKey(key)
		assert.True(t, ok, key)
		assert.Equal(t, want, [2]string{pkg, top}, key)
	}
	_, _, ok := splitTestKey("github.com/a/b")
	assert.False(t, ok)
}
