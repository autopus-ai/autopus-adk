package mutate

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCandidates(t *testing.T) {
	t.Parallel()
	src := "package main\n\n// if a == b { return true }\nfunc f(a, b int) bool {\n\tif a == b && b > 0 {\n\t\treturn true\n\t}\n\ts := \"a == b\" // x == y\n\tvar xs []int\n\treturn len(xs) <= 10\n}\n"
	var got []string
	for _, m := range Candidates("main.go", src) {
		got = append(got, m.Operator+"@"+strconv.Itoa(m.Line)+": "+m.Mutated)
	}
	assert.Equal(t, []string{
		"negate-equality@5: \tif a != b && b > 0 {",
		"boundary-gt@5: \tif a == b && b >= 0 {",
		"swap-and@5: \tif a == b || b > 0 {",
		"off-by-one@5: \tif a == b && b > 1 {",
		"flip-boolean@6: \t\treturn false",
		"boundary-le@10: \treturn len(xs) < 10",
		"off-by-one@10: \treturn len(xs) <= 11",
	}, got, "comments, strings, and trailing comments are not mutated")

	assert.Nil(t, Candidates("README.md", "a == b"))
	js := Candidates("a.js", "if (a === b) f()")
	if assert.Len(t, js, 1) {
		assert.Equal(t, "if (a !== b) f()", js[0].Mutated, "=== is not read as ==")
	}
}

func TestSample(t *testing.T) {
	t.Parallel()
	var all []Mutant
	for i := 1; i <= 10; i++ {
		all = append(all, Mutant{File: "a.go", Line: i})
	}
	got := Sample(all, 3)
	assert.Equal(t, []int{1, 4, 7}, []int{got[0].Line, got[1].Line, got[2].Line})
	assert.Len(t, Sample(all, 0), 10)
}

// In a comparator whose previous line rules out equality, > and >= agree, so
// that mutant is never planted; the tie-break below it is still mutated.
func TestCandidates_SkipsBoundaryAfterAnInequalityGuard(t *testing.T) {
	t.Parallel()
	src := "\t\tif counts[a] != counts[b] {\n\t\t\treturn counts[a] > counts[b]\n\t\t}\n\t\treturn a < b\n"
	var ops []string
	for _, m := range Candidates("x.go", src) {
		ops = append(ops, m.Operator+"@"+strconv.Itoa(m.Line))
	}
	assert.Equal(t, []string{"negate-inequality@1", "boundary-lt@4"}, ops)
}
