package main_test

// Hidden oracle for the wordtop fixture. The agent never sees it: the matrix
// copies it into the loop branch's tree after the run. It pins only what the
// request states without ambiguity, so any reasonable line format passes.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func wordtop(t *testing.T, args ...string) (string, error) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "wordtop")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, args...).Output()
	return string(out), err
}

func input(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.txt")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// hasWord reports whether a line names word, in any case. The request never
// says whether counts are printed or how a line looks, so only the word and
// the line order are pinned.
func hasWord(line, word string) bool {
	for _, f := range strings.Fields(line) {
		if strings.EqualFold(strings.Trim(f, ":,"), word) {
			return true
		}
	}
	return false
}

// "The" and "the" are one word (the planted bug splits them).
func TestOracle_CaseVariantsAreOneWord(t *testing.T) {
	out, err := wordtop(t, input(t, "The the THE cat cat dog\n"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := lines(out)
	if len(got) != 3 || !hasWord(got[0], "the") || !hasWord(got[1], "cat") || !hasWord(got[2], "dog") {
		t.Fatalf("want the, cat, dog in that order, got %q", out)
	}
}

func TestOracle_TiesAreAlphabeticalAndNLimits(t *testing.T) {
	out, err := wordtop(t, "-n", "2", input(t, "pear apple fig\n"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := lines(out)
	if len(got) != 2 || !hasWord(got[0], "apple") || !hasWord(got[1], "fig") {
		t.Fatalf("want apple then fig, got %q", out)
	}
}

func TestOracle_MissingFileFails(t *testing.T) {
	if _, err := wordtop(t, filepath.Join(t.TempDir(), "nope.txt")); err == nil {
		t.Fatal("a missing file must end with an error")
	}
}
