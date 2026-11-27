package verifygate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each case is a break the change caused that a failing test on HEAD must
// not hide: the HEAD failure is a different one, or not a reliable one.
func TestRunCharged_ADifferentFailureOnHEADDoesNotHideABreak(t *testing.T) {
	const sumTest = "package basegate\n\nimport \"testing\"\n\n"
	cases := map[string]struct {
		base, edit map[string]string
		key        string
	}{
		// S1: flaky on HEAD (fails every other run), broken for good now.
		"a test flaky on HEAD": {
			base: map[string]string{"flaky_test.go": sumTest + "var runs int\n\nfunc TestFlaky(t *testing.T) {\n\truns++\n" +
				"\tif runs%2 == 1 || Add(1, 1) != 2 {\n\t\tt.Fatal(\"flaky\")\n\t}\n}\n"},
			edit: map[string]string{"add.go": baseAddBad},
			key:  "basegate.TestFlaky",
		},
		// S4: the same test fails at another assertion than on HEAD.
		"another assertion fails": {
			base: map[string]string{"tool_test.go": sumTest + "func TestTool(t *testing.T) {\n\tif Add(1, 2) != 3 {\n" +
				"\t\tt.Fatal(\"wrong sum\")\n\t}\n\tt.Fatal(\"env: missing tool\")\n}\n"},
			edit: map[string]string{"add.go": baseAddBad},
			key:  "basegate.TestTool",
		},
		// S7: the change breaks an assertion that prints the same message
		// as the one HEAD fails at, on another line of an untouched file.
		"another assertion with the same message": {
			base: map[string]string{"check_test.go": sumTest + "func TestCheck(t *testing.T) {\n" +
				"\tif !toolPresent() {\n\t\tt.Error(\"check failed\")\n\t}\n" +
				"\tif Add(1, 2) != 3 {\n\t\tt.Error(\"check failed\")\n\t}\n}\n\n" +
				"func toolPresent() bool { return false }\n"},
			edit: map[string]string{"add.go": baseAddBad},
			key:  "basegate.TestCheck",
		},
		// S5: the subtest name is the same, the row under it is not.
		"another row under the same subtest": {
			base: map[string]string{
				"cases.go": "package basegate\n\n// Cases are sums to check.\nvar Cases = [][3]int{{1, 2, 4}}\n",
				"table_test.go": sumTest + "import \"strconv\"\n\nfunc TestTable(t *testing.T) {\n\tfor i, c := range Cases {\n" +
					"\t\tt.Run(strconv.Itoa(i), func(t *testing.T) {\n\t\t\tif got := Add(c[0], c[1]); got != c[2] {\n" +
					"\t\t\t\tt.Fatalf(\"Add(%d, %d) = %d, want %d\", c[0], c[1], got, c[2])\n\t\t\t}\n\t\t})\n\t}\n}\n",
			},
			edit: map[string]string{"cases.go": "package basegate\n\n// Cases are sums to check.\nvar Cases = [][3]int{{2, 2, 5}}\n"},
			key:  "basegate.TestTable/0",
		},
		// T1: testify prints the values on lines under "file.go:N: ", which
		// read the same failure location with another actual value.
		"another value under a testify-style failure": {
			base: map[string]string{
				"add.go": "package basegate\n\n// Add sums.\nfunc Add(a, b int) int { return a + b + 1 }\n",
				"eq_test.go": sumTest + "func equal(t *testing.T, want, got int) {\n\tt.Helper()\n\tif want != got {\n" +
					"\t\tt.Fatalf(\"\\n\\tError Trace:\\t/x/eq_test.go:14\\n\\tError:      \\tNot equal: \\n" +
					"\\t            \\texpected: %d\\n\\t            \\tactual  : %d\\n\\tTest:       \\t%s\", want, got, t.Name())\n" +
					"\t}\n}\n\nfunc TestEq(t *testing.T) {\n\tequal(t, 3, Add(1, 2))\n}\n",
			},
			edit: map[string]string{"add.go": baseAddBad},
			key:  "basegate.TestEq",
		},
		// T5: the change fails a new path that prints nothing; HEAD failed
		// with a message.
		"a silent failure against a printed one": {
			base: map[string]string{"svc_test.go": sumTest + "func TestSvc(t *testing.T) {\n\tif Add(1, 2) != 3 {\n" +
				"\t\tt.FailNow()\n\t}\n\tt.Fatal(\"env: service not ready\")\n}\n"},
			edit: map[string]string{"add.go": baseAddBad},
			key:  "basegate.TestSvc",
		},
		// S6: the HEAD snapshot has no .git, so the test fails there for a
		// reason the work tree does not have.
		"an environment only the snapshot lacks": {
			base: map[string]string{"repo_test.go": sumTest + "import \"os\"\n\nfunc TestRepo(t *testing.T) {\n" +
				"\tif _, err := os.Stat(\".git\"); err != nil {\n\t\tt.Fatal(\"not in a git repo\")\n\t}\n" +
				"\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"wrong sum\")\n\t}\n}\n"},
			edit: map[string]string{"add.go": baseAddBad},
			key:  "basegate.TestRepo",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			base := map[string]string{"go.mod": baseModule, "add.go": baseAdd}
			for k, v := range c.base {
				base[k] = v
			}
			res := runCharged(t, redRepo(t, base, c.edit))
			require.NotNil(t, res.Failure, "the break was hidden; pre-existing: %v", res.Preexisting)
			assert.Contains(t, res.Failure.FailedTests, c.key)
			assert.NotContains(t, res.Preexisting, c.key)
		})
	}
}

// S2: a case table in the test file is edited, the test function is not.
func TestRunCharged_AnEditedCaseTableIsCharged(t *testing.T) {
	table := func(rows string) string {
		return "package basegate\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nvar cases = [][3]int{" + rows + "}\n\n" +
			"func TestCases(t *testing.T) {\n\tif os.Getenv(\"BASEGATE_TOOL\") == \"\" {\n\t\tt.Fatal(\"tool missing\")\n\t}\n" +
			"\tfor _, c := range cases {\n\t\tif Add(c[0], c[1]) != c[2] {\n\t\t\tt.Fatal(c)\n\t\t}\n\t}\n}\n"
	}
	base := map[string]string{"go.mod": baseModule, "add.go": baseAdd, "cases_test.go": table("{1, 2, 3}")}
	res := runCharged(t, redRepo(t, base, map[string]string{"cases_test.go": table("{1, 2, 3}, {2, 2, 5}")}))
	require.NotNil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestCases"}, res.Edited)
}

// A test added above a pre-existing failure moves its line down; the
// failure is still HEAD's.
func TestRunCharged_AFailureMovedDownItsFileIsPreexisting(t *testing.T) {
	const head = "package basegate\n\nimport \"testing\"\n\n"
	const pre = "func TestPre(t *testing.T) {\n\tt.Fatal(\"env: missing tool\")\n}\n"
	const added = "func TestAdded(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"wrong sum\")\n\t}\n}\n\n"
	base := baselineBase()
	base["pre_test.go"] = head + pre
	res := runCharged(t, redRepo(t, base, map[string]string{"add.go": baseAddFix, "pre_test.go": head + added + pre}))
	assert.Nil(t, res.Failure, "edited: %v, unavailable: %s", res.Edited, res.Unavailable)
	assert.Contains(t, res.Preexisting, "basegate.TestPre")
}

func TestSameFailure(t *testing.T) {
	cur := []testRun{{Failed: true, Lines: []string{"a_test.go:7: wrong sum"}}}
	same := testRun{Failed: true, Lines: []string{"a_test.go:7: wrong sum"}}
	assert.True(t, sameFailure(cur, []testRun{same, same}, 2))
	assert.False(t, sameFailure(cur, []testRun{same}, 2), "one HEAD run cannot show it is reliable")
	assert.False(t, sameFailure(cur, []testRun{same, {}}, 2), "a HEAD run passed")
	other := testRun{Failed: true, Lines: []string{"a_test.go:9: env: missing tool"}}
	assert.False(t, sameFailure(cur, []testRun{other, other}, 2))
	silent := []testRun{{Failed: true}}
	assert.False(t, sameFailure(silent, []testRun{other, other}, 2), "an empty signature matched a printed one")
	assert.True(t, sameFailure(silent, []testRun{{Failed: true}, {Failed: true}}, 2))
}

// A testify failure keeps its values, without the trace path and test name.
func TestTestRuns_KeepsTestifyValues(t *testing.T) {
	out := "" +
		`{"Action":"run","Package":"m","Test":"TestEq"}` + "\n" +
		`{"Action":"output","Package":"m","Test":"TestEq","Output":"    eq_test.go:10: \n"}` + "\n" +
		`{"Action":"output","Package":"m","Test":"TestEq","Output":"        \tError Trace:\t/tmp/x/eq_test.go:10\n"}` + "\n" +
		`{"Action":"output","Package":"m","Test":"TestEq","Output":"        \tError:      \tNot equal: \n"}` + "\n" +
		`{"Action":"output","Package":"m","Test":"TestEq","Output":"        \t            \texpected: 3\n"}` + "\n" +
		`{"Action":"output","Package":"m","Test":"TestEq","Output":"        \t            \tactual  : -1\n"}` + "\n" +
		`{"Action":"output","Package":"m","Test":"TestEq","Output":"        \tTest:       \tTestEq\n"}` + "\n" +
		`{"Action":"output","Package":"m","Test":"TestEq","Output":"--- FAIL: TestEq (0.00s)\n"}` + "\n" +
		`{"Action":"fail","Package":"m","Test":"TestEq"}` + "\n"
	runs := testRuns([]byte(out))["m.TestEq"]
	require.Len(t, runs, 1)
	assert.Equal(t, []string{"eq_test.go:10:  | Error: Not equal: | expected: 3 | actual : -1"}, runs[0].Lines)
}
