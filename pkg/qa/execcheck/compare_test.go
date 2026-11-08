package execcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// The output of a word counter that forgot to fold case, for input
// "The The the the THE cat cat cat". Substring checks for "the" and "cat"
// pass on it; only order, count, and absence catch the defect.
const caseBug = "3 cat\n2 The\n2 the\n"

func fields(ms []Mismatch) []string {
	out := []string{}
	for _, m := range ms {
		out = append(out, m.Field)
	}
	return out
}

func TestCompare_LinesAndAbsenceCatchWhatContainsMisses(t *testing.T) {
	got := Result{Stdout: caseBug}

	contains := testscenario.ExecExpect{StdoutContains: []string{"the", "cat"}}
	assert.Empty(t, compare(contains, got, t.TempDir()))

	strict := testscenario.ExecExpect{
		StdoutLines:       []string{"the", "cat"},
		StdoutNotContains: []string{"The", "THE"},
	}
	ms := compare(strict, got, t.TempDir())
	assert.Equal(t, []string{"stdout_not_contains", "stdout_lines"}, fields(ms))
	assert.Equal(t, `no "The"`, ms[0].Expected)
	assert.Equal(t, "the\ncat", ms[1].Expected)

	fixed := Result{Stdout: "5 the\n3 cat\n"}
	assert.Empty(t, compare(strict, fixed, t.TempDir()))
}

func TestCompare_StdoutLinesPinsCountAndOrder(t *testing.T) {
	want := testscenario.ExecExpect{StdoutLines: []string{"apple", "banana"}}
	cases := map[string]bool{
		"5 apple\n4 banana\n":           true,
		"apple 5\n\nbanana 4\n":         true, // blank lines do not count
		"4 banana\n5 apple\n":           false,
		"5 apple\n4 banana\n3 cherry\n": false,
		"5 apple\n":                     false,
	}
	for stdout, pass := range cases {
		ms := compare(want, Result{Stdout: stdout}, t.TempDir())
		assert.Equal(t, pass, len(ms) == 0, "stdout %q", stdout)
	}
}
