package editguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cherokeeCasePairs lists the 86 Cherokee letter-case pairs by the layout of
// the Unicode charts: U+AB70..U+ABBF are the lowercase of U+13A0..U+13EF, and
// U+13F8..U+13FD the lowercase of U+13F0..U+13F5 (Ꭰ and ꭰ come first).
func cherokeeCasePairs() [][2]rune {
	var pairs [][2]rune
	for i := rune(0); i < 80; i++ {
		pairs = append(pairs, [2]rune{0x13a0 + i, 0xab70 + i})
	}
	for i := rune(0); i < 6; i++ {
		pairs = append(pairs, [2]rune{0x13f0 + i, 0x13f8 + i})
	}
	return pairs
}

// golang.org/x/text folds each Cherokee letter to the other case, so without
// a fix-up the two spellings of one name get two keys. On a case-insensitive
// volume every pair shares one key; on a case-sensitive one they stay apart.
func TestFoldKey_CherokeeLetterCasePairsShareAKey(t *testing.T) {
	t.Parallel()
	pairs := cherokeeCasePairs()
	if len(pairs) != 86 {
		t.Fatalf("%d pairs, want 86", len(pairs))
	}
	for _, pair := range pairs {
		upper := "internal/foo/" + string(pair[0]) + "_test.go"
		lower := "internal/FOO/" + string(pair[1]) + "_test.go"
		if got, want := FoldKey(lower, true), FoldKey(upper, true); got != want {
			t.Errorf("%U/%U: FoldKey(%q) = %q, want the key of %q, %q", pair[0], pair[1], lower, got, upper, want)
		}
		if FoldKey(upper, false) == FoldKey(lower, false) {
			t.Errorf("%U/%U: a case-sensitive volume must keep the two names apart", pair[0], pair[1])
		}
	}
}

// A lock still denies recreating its deleted test under the other case of a
// Cherokee letter, which the volume opens as the locked path.
func TestDecide_CherokeeCaseVariantOfADeletedLockedTest_Deny(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	locked := "internal/foo/Ꭰ_test.go"
	variant := "internal/foo/ꭰ_test.go"
	writeFile(t, root, locked, tContent)
	exact, err := os.Lstat(filepath.Join(root, filepath.FromSlash(locked)))
	if err != nil {
		t.Fatal(err)
	}
	if alias, err := os.Lstat(filepath.Join(root, filepath.FromSlash(variant))); err != nil || !os.SameFile(exact, alias) {
		t.Skip("the volume does not open the Cherokee case pair as one name")
	}
	mustLock(t, openTestStore(t, root, newClock(t0)), locked)
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(locked))); err != nil {
		t.Fatal(err)
	}
	got := decideOne(root, variant, Options{Now: newClock(t0).Now})
	if !got.Deny || got.Class != ClassFixLock || !strings.HasPrefix(got.Reason, flHead(locked)) {
		t.Errorf("Decide(%q) = %+v, want the FL deny of %s", variant, got, locked)
	}
}
