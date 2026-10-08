package healthband_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Item 2 name forms: each side of a `diff --git` line and each `---` and
// `+++` name may be C-quoted; a well-formed spelling of the tracked path is
// accepted, as git reads it, and a malformed quote refuses the diff.
func TestPatchPolicy_HeaderNameForms_DecodeLikeGit(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	body := "--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -3 +3 @@\n-func Foo() int { return 1 }\n+func Foo() int { return 2 }\n"
	accepted := map[string]string{
		"second side quoted": "diff --git a/pkg/foo/foo.go \"b/pkg/foo/foo.go\"\n" + body,
		"first side quoted":  "diff --git \"a/pkg/foo/foo.go\" b/pkg/foo/foo.go\n" + body,
		"octal escapes":      "diff --git \"a/pkg/foo/\\146oo.go\" \"b/pkg/foo/\\146oo.go\"\n" + body,
		"quoted old and new": "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- \"a/pkg/foo/foo.go\"\n+++ \"b/pkg/foo/foo.go\"\n" + body[len("--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n"):],
	}
	for name, diff := range accepted {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			verdict := evaluate(t, repo, healthband.PatchPolicy{}, replyWith(diff))
			assert.True(t, verdict.Accepted(), "code %q", verdict.Code)
		})
	}
	refused := map[string]string{
		"no space after quote":   "diff --git \"a/pkg/foo/foo.go\"b/pkg/foo/foo.go\n" + body,
		"second quote unclosed":  "diff --git a/pkg/foo/foo.go \"b/pkg/foo/foo.go\n" + body,
		"second quote trailing":  "diff --git a/pkg/foo/foo.go \"b/pkg/foo/foo.go\"x\n" + body,
		"first quote unclosed":   "diff --git \"a/pkg/foo/foo.go\n" + body,
		"backslash at the end":   "diff --git \"a/pkg/foo/foo.go\\\n" + body,
		"octal out of range":     "diff --git \"a/pkg/foo/\\400.go\" \"b/pkg/foo/\\400.go\"\n" + body,
		"short octal":            "diff --git \"a/pkg/foo/\\08.go\" \"b/pkg/foo/\\08.go\"\n" + body,
		"uneven unquoted halves": "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.goo\n" + body,
		"too short":              "diff --git a/x\n" + body,
		"quoted name trailing":   "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- \"a/pkg/foo/foo.go\"x\n+++ b/pkg/foo/foo.go\n@@ -3 +3 @@\n-x\n+y\n",
		"empty path below b/":    "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- a/pkg/foo/foo.go\n+++ b/\n@@ -3 +3 @@\n-x\n+y\n",
	}
	for name, diff := range refused {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, healthband.PatchCodeInvalid, evaluate(t, repo, healthband.PatchPolicy{}, replyWith(diff)).Code)
		})
	}
}

// Item 1: an info string holding a backtick does not open a backtick fence,
// as in CommonMark, so the diff below it is no diff fence.
func TestPatchPolicy_BacktickInInfoString_OpensNoFence(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	reply := healthband.PatchReply{Text: "```diff `x`\n" + modifyFoo("// x") + "```\n"}

	assert.Equal(t, healthband.PatchCodeNoPatch, evaluate(t, repo, healthband.PatchPolicy{}, reply).Code)
}
