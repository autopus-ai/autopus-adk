package healthband_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S5 path rows of Patch Policy items 4–6: every path the S5 table names
// path_denied, either as a new file or as an edit of a tracked one.
func TestPatchPolicy_DeniedPaths_RefusePathDenied(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	cases := map[string]string{}
	for _, path := range []string{
		".github/workflows/ci.yaml", ".claude/settings.json", ".autopus/specs/x.md", ".omp/x.ts", ".husky/pre-commit",
		".HUSKY/x.js", "Makefile", "AGENTS.md", "scripts/run.py", "build.rs", "conftest.py", "vite.config.ts",
		".eslintrc.cjs", "Package.swift", "buildSrc/x.kt", ".env", "config/.env.production", "deploy/prod.env", ".npmrc",
		".netrc", ".aws/config.py", ".ssh/x.py", "kubeconfig.py", "secrets.py", "tools/run.sh", ".storybook/main.ts",
		"karma.conf.js", "pkg/src.go", "build-logic/x.kt", ".yarn/plugins/x.cjs", ".pnp.cjs", "pkg/my_credentials.go",
		"pkg/Dockerfile.go", "pkg/x.GO", "config.toml", "sub/.codex/x.go",
	} {
		cases["new "+path] = newFile(path, "100644", "x")
	}
	cases["edit package.json"] = "diff --git a/package.json b/package.json\n--- a/package.json\n+++ b/package.json\n@@ -1 +1 @@\n-{}\n+{\"x\": 1}\n"
	cases["edit go.mod"] = "diff --git a/go.mod b/go.mod\n--- a/go.mod\n+++ b/go.mod\n@@ -1,3 +1,4 @@\n module example.com/x\n \n go 1.26\n+require a.b/c v1.0.0\n"
	cases["edit executable tools/gen.py"] = "diff --git a/tools/gen.py b/tools/gen.py\n--- a/tools/gen.py\n+++ b/tools/gen.py\n@@ -1 +1 @@\n-print(2)\n+print(3)\n"
	for name, diff := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, healthband.PatchCodePathDenied, evaluate(t, repo, healthband.PatchPolicy{}, replyWith(diff)).Code)
		})
	}
}

// S5 collision rows of item 4 (CD-3 M2): folded spellings collide inside
// the diff and with tracked paths and directories, NFD and NFC included,
// before item 6 could deny scripts/ by name.
func TestPatchPolicy_FoldedCollisions_RefuseCaseCollision(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	cases := map[string]string{
		"Scripts beside tracked scripts":  newFile("Scripts/run.py", "100644", "x"),
		"n.go and N.go in one diff":       newFile("pkg/foo/n.go", "100644", "x") + newFile("pkg/foo/N.go", "100644", "x"),
		"Pkg beside tracked pkg":          newFile("Pkg/foo/x.go", "100644", "x"),
		"NFD beside tracked NFC":          newFile("pkg/foo/e\u0301te\u0301.go", "100644", "x"),
		"case spelling of a tracked file": newFile("pkg/foo/FOO.go", "100644", "x"),
	}
	for name, diff := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, healthband.PatchCodeCaseCollision, evaluate(t, repo, healthband.PatchPolicy{}, replyWith(diff)).Code)
		})
	}
}

// S5: the C-quoted spelling of a tracked UTF-8 path decodes to that path
// and passes every path check.
func TestPatchPolicy_CQuotedUTF8Path_DecodesAndPasses(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	diff := "diff --git \"a/pkg/foo/\\303\\251t\\303\\251.go\" \"b/pkg/foo/\\303\\251t\\303\\251.go\"\n" +
		"--- \"a/pkg/foo/\\303\\251t\\303\\251.go\"\n+++ \"b/pkg/foo/\\303\\251t\\303\\251.go\"\n" +
		"@@ -1 +1,2 @@\n package foo\n+// accent\n"

	verdict := evaluate(t, repo, healthband.PatchPolicy{}, replyWith(diff))

	require.True(t, verdict.Accepted(), "code %q", verdict.Code)
	assert.Equal(t, []healthband.PatchFile{{Path: "pkg/foo/\u00e9t\u00e9.go", Added: 1}}, verdict.Files)
}

// S5: a touched path whose filter attribute is set at the base, lfs
// included, is refused before any content check (CD-3 F-009).
func TestPatchPolicy_FilterAttributeAtBase_RefusesPathDeniedFilter(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	diff := "diff --git a/pkg/foo/model.go b/pkg/foo/model.go\n--- a/pkg/foo/model.go\n+++ b/pkg/foo/model.go\n@@ -1 +1,2 @@\n package foo\n+// raw\n"
	git := &recordingGit{home: repo.home}

	verdict := evaluate(t, repo, healthband.PatchPolicy{Git: git}, replyWith(diff))

	assert.Equal(t, healthband.PatchCodeFilter, verdict.Code)
	assert.True(t, git.invoked("check-attr", "-z", "--source="+repo.base, "filter", "--", "pkg/foo/model.go"))
	assert.False(t, git.invoked("apply"), "the policy stops before git apply")
}

// New files and edits outside every rule are accepted: an added file is
// reported by git's summary as create mode 100644, a new empty file has no
// hunk, and a traditional ---/+++ section reads like a git one.
func TestPatchPolicy_AllowedShapes_Accept(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	cases := map[string]struct {
		diff  string
		files []healthband.PatchFile
	}{
		"new file": {newFile("pkg/foo/new.go", "100644", "package foo", "// new"), []healthband.PatchFile{{Path: "pkg/foo/new.go", Added: 2}}},
		"new empty file": {"diff --git a/pkg/foo/empty.go b/pkg/foo/empty.go\nnew file mode 100644\nindex 0000000..e69de29\n",
			[]healthband.PatchFile{{Path: "pkg/foo/empty.go"}}},
		"traditional edit": {"--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -3 +3 @@\n-func Foo() int { return 1 }\n+func Foo() int { return 2 }\n",
			[]healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 1, Removed: 1}}},
		"two files and an index line": {"diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\nindex b7b2d7d..1111111 100644\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n" +
			"@@ -1,3 +1,3 @@ package foo\n package foo\n-\n+// gap\n func Foo() int { return 1 }\n" + newFile("pkg/bar/bar.go", "100644", "package bar"),
			[]healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 1, Removed: 1}, {Path: "pkg/bar/bar.go", Added: 1}}},
		"empty context line and blank tail": {"diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n" +
			"@@ -1,3 +1,4 @@\n package foo\n\n func Foo() int { return 1 }\n+// tail\n\n\n",
			[]healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 1}}},
		"no newline marker": {"diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n" +
			"@@ -3 +3,2 @@\n func Foo() int { return 1 }\n+// end\n\\ No newline at end of file\n",
			[]healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 1}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			verdict := evaluate(t, repo, healthband.PatchPolicy{}, replyWith(tc.diff))
			require.True(t, verdict.Accepted(), "code %q", verdict.Code)
			assert.Equal(t, tc.files, verdict.Files)
		})
	}
}
