package healthband_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S5 patch_invalid rows of Patch Policy items 2–3, in process and against
// the base: headers, modes, path forms, and the base entry decide, and no
// git apply runs for any of them.
func TestPatchPolicy_InvalidShapes_RefusePatchInvalid(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	foo := "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n"
	cases := map[string]string{
		"new file mode 100755": newFile("pkg/foo/x.go", "100755", "x"),
		"gitlink":              newFile("pkg/foo/sub", "160000", "Subproject commit 2222222222222222222222222222222222222222"),
		"symlink":              newFile("pkg/foo/l.go", "120000", "../../x"),
		"rename":               "diff --git a/pkg/foo/foo.go b/pkg/foo/bar.go\nsimilarity index 100%\nrename from pkg/foo/foo.go\nrename to pkg/foo/bar.go\n",
		"copy":                 "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\ncopy from pkg/foo/foo.go\ncopy to pkg/foo/foo.go\n",
		"deletion":             foo + "deleted file mode 100644\n--- a/pkg/foo/foo.go\n+++ /dev/null\n@@ -1,3 +0,0 @@\n-package foo\n-\n-func Foo() int { return 1 }\n",
		"mode change":          "diff --git a/tools/gen.py b/tools/gen.py\nold mode 100755\nnew mode 100644\n",
		"outside":              newFile("../outside.go", "100644", "x"),
		"absolute":             "--- /dev/null\n+++ b//etc/x.go\n@@ -0,0 +1 @@\n+x\n",
		"dot segment":          newFile("pkg/./x.go", "100644", "x"),
		"empty segment":        newFile("pkg//x.go", "100644", "x"),
		"quoted escape":        "diff --git \"a/tools/a\\033b.go\" \"b/tools/a\\033b.go\"\nnew file mode 100644\n--- /dev/null\n+++ \"b/tools/a\\033b.go\"\n@@ -0,0 +1 @@\n+x\n",
		"quoted bad escape":    "diff --git \"a/pkg/a\\qb.go\" \"b/pkg/a\\qb.go\"\nnew file mode 100644\n",
		"quoted tab":           "diff --git \"a/pkg/a\\tb.go\" \"b/pkg/a\\tb.go\"\nnew file mode 100644\n",
		"invalid utf-8 path":   newFile("pkg/a\xffb.go", "100644", "x"),
		"zero-width space":     newFile("pkg/foo/foo\u200b.go", "100644", "x"),
		"binary":               "diff --git a/pkg/foo/b.go b/pkg/foo/b.go\nnew file mode 100644\nindex 0000000..e69de29\nGIT binary patch\nliteral 0\nHcmV?d00001\n\n",
		"binary files differ":  foo + "Binary files a/pkg/foo/foo.go and b/pkg/foo/foo.go differ\n",
		"symlink index mode":   foo + "index 1111111..2222222 120000\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -1 +1 @@\n-package foo\n+package bar\n",
		"modeless symlink hunk": "diff --git a/link.go b/link.go\n--- a/link.go\n+++ b/link.go\n@@ -1 +1 @@\n-../target\n\\ No newline at end of file\n" +
			"+../../../../../../../.ssh/id_rsa\n\\ No newline at end of file\n",
		"modeless gitlink hunk": "diff --git a/pkg/sub b/pkg/sub\n--- a/pkg/sub\n+++ b/pkg/sub\n@@ -1 +1 @@\n-Subproject commit 1111111111111111111111111111111111111111\n" +
			"+Subproject commit 2222222222222222222222222222222222222222\n",
		"new file under gitlink":    newFile("pkg/sub/new.go", "100644", "package sub"),
		"new file under a blob":     newFile("pkg/foo/foo.go/x.go", "100644", "x"),
		"new file over a blob":      newFile("pkg/foo/foo.go", "100644", "x"),
		"edit of an absent path":    strings.ReplaceAll(modifyFoo("// x"), "pkg/foo/foo.go", "pkg/foo/none.go"),
		"edit of a tree":            "diff --git a/pkg/foo b/pkg/foo\n--- a/pkg/foo\n+++ b/pkg/foo\n@@ -1 +1 @@\n-x\n+y\n",
		"names differ":              "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/bar.go\n@@ -3 +3 @@\n-func Foo() int { return 1 }\n+func Foo() int { return 2 }\n",
		"traditional names differ":  "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/bar.go\n@@ -3 +3 @@\n-func Foo() int { return 1 }\n+func Foo() int { return 2 }\n",
		"git header halves differ":  "diff --git a/pkg/foo/foo.go b/pkg/foo/fox.go\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -3 +3 @@\n-x\n+y\n",
		"missing prefix":            "--- pkg/foo/foo.go\n+++ pkg/foo/foo.go\n@@ -3 +3 @@\n-func Foo() int { return 1 }\n+func Foo() int { return 2 }\n",
		"new file without null old": foo + "new file mode 100644\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -0,0 +1 @@\n+x\n",
		"null old without new mode": "diff --git a/pkg/foo/n.go b/pkg/foo/n.go\n--- /dev/null\n+++ b/pkg/foo/n.go\n@@ -0,0 +1 @@\n+x\n",
		"new file with old lines":   "diff --git a/pkg/foo/n.go b/pkg/foo/n.go\nnew file mode 100644\n--- /dev/null\n+++ b/pkg/foo/n.go\n@@ -1 +1 @@\n-x\n+y\n",
		"no file section":           "just prose inside the fence\n",
		"empty fence":               "",
		"git header only":           foo,
		"names without hunk":        foo + "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n",
		"missing new name":          foo + "--- a/pkg/foo/foo.go\n@@ -3 +3 @@\n-x\n+y\n",
		"unknown header line":       foo + "weird header\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n",
		"bad index line":            foo + "index nothing\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -3 +3 @@\n-x\n+y\n",
		"bad hunk header":           foo + "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -a +b @@\n-x\n+y\n",
		"short hunk body":           foo + "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -1,3 +1,4 @@\n package foo\n+x\n",
		"overlong hunk body":        foo + "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -1 +1 @@\n-package foo\n+package bar\n+extra\n",
		"context-only hunk":         foo + "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -1 +1 @@\n package foo\n",
		"corrupt body line":         foo + "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -1 +1 @@\n*package foo\n",
		"short marker":              foo + "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -1 +1 @@\n-package foo\n\\ x\n+package bar\n",
		"huge hunk count":           foo + "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -1,999999999 +1 @@\n-package foo\n",
		"duplicate section":         modifyFoo("// a") + modifyFoo("// b"),
		"garbage after hunk":        modifyFoo("// a") + "trailing prose\n",
		"blank line then garbage":   modifyFoo("// a") + "\nmore prose\n",
		"failing context":           strings.Replace(modifyFoo("// a"), " package foo\n", " package bar\n", 1),
	}
	for name, diff := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			git := &recordingGit{home: repo.home}
			verdict := evaluate(t, repo, healthband.PatchPolicy{Git: git}, replyWith(diff))
			assert.Equal(t, healthband.PatchCodeInvalid, verdict.Code)
			if name != "failing context" {
				assert.False(t, git.invoked("apply"), "refused before git apply")
			}
		})
	}
}

// S5 (CD-3 N3, rev 11): a new file under a tracked symlink or gitlink
// prefix ends patch_invalid at item 3 through the exact-path query of that
// prefix, which `ls-tree -t -- <file>` would have missed.
func TestPatchPolicy_NewFileUnderSymlinkOrGitlinkPrefix_RefusesByExactPathQuery(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, []baseFile{
		{path: "pkg/foo", content: "../elsewhere", mode: "120000"},
		{path: "pkg/sub", content: "1111111111111111111111111111111111111111", mode: "160000"},
		{path: "pkg/keep.go", content: "package pkg\n"},
	})
	for prefix, path := range map[string]string{"pkg/foo": "pkg/foo/new.go", "pkg/sub": "pkg/sub/new.go"} {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			git := &recordingGit{home: repo.home}
			verdict := evaluate(t, repo, healthband.PatchPolicy{Git: git}, replyWith(newFile(path, "100644", "package x")))
			assert.Equal(t, healthband.PatchCodeInvalid, verdict.Code)
			assert.True(t, git.invoked("ls-tree", "-z", repo.base, "--", ":(literal)"+prefix))
			assert.False(t, git.invoked("apply", "--cached"))
			assert.False(t, git.invoked("apply"))
		})
	}
}

// macOS git precomposes argv, so an exact-path query cannot see an entry
// stored in NFD; the listing cross-check still refuses a new file under an
// NFD-named tracked symlink.
func TestPatchPolicy_NFDNamedSymlinkPrefix_RefusedByTheListing(t *testing.T) {
	t.Parallel()
	link := "pkg/n\u0303o"
	repo := newPolicyRepo(t, []baseFile{{path: link, content: "../elsewhere", mode: "120000"}, {path: "pkg/keep.go", content: "package pkg\n"}})

	verdict := evaluate(t, repo, healthband.PatchPolicy{}, replyWith(newFile(link+"/new.go", "100644", "package x")))

	assert.Equal(t, healthband.PatchCodeInvalid, verdict.Code)
}
