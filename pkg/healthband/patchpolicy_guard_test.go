package healthband_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/editguard"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// editFooTest is an edit of the tracked pkg/foo/foo_test.go.
const editFooTest = "diff --git a/pkg/foo/foo_test.go b/pkg/foo/foo_test.go\n--- a/pkg/foo/foo_test.go\n" +
	"+++ b/pkg/foo/foo_test.go\n@@ -1 +1,2 @@\n package foo\n+// weakened\n"

// S5 (item 9): an active auto fix lock in the user's checkout denies the
// locked test with path_denied:fix_lock, although no hook sees band's write.
func TestPatchPolicy_FixLockInUserCheckout_RefusesFixLock(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	require.NoError(t, os.WriteFile(filepath.Join(repo.dir, "autopus.yaml"), []byte("project:\n  name: band-fixture\n"), 0o644))
	store, err := editguard.OpenStore(repo.dir)
	require.NoError(t, err)
	require.NoError(t, store.Lock([]string{"pkg/foo/foo_test.go"}, 0))

	verdict := evaluate(t, repo, healthband.PatchPolicy{}, replyWith(editFooTest))

	assert.Equal(t, "path_denied:fix_lock", verdict.Code)
}

// S5 (item 9, CD-3 L1): a guard decision that allows with a Diagnostic is a
// guard fault and denies, as does a panic inside the guard; a deny carries
// its class; the call names the user's checkout and the decoded paths.
func TestPatchPolicy_GuardDecisions_MapToPathDeniedCodes(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	cases := map[string]struct {
		decision editguard.Decision
		panics   bool
		code     string
	}{
		"fault allows with a diagnostic": {decision: editguard.Decision{Diagnostic: "autopus edit-guard: allow (manifest unreadable: x)"}, code: healthband.PatchCodeGuardFault},
		"guard state deny":               {decision: editguard.Decision{Deny: true, Class: editguard.ClassGuardState, Reason: "r"}, code: "path_denied:guard_state"},
		"generated surface deny":         {decision: editguard.Decision{Deny: true, Class: editguard.ClassGeneratedSurface, Reason: "r"}, code: "path_denied:generated_surface"},
		"deny without a class":           {decision: editguard.Decision{Deny: true}, code: healthband.PatchCodeGuardFault},
		"panic":                          {panics: true, code: healthband.PatchCodeGuardFault},
		"allow":                          {code: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var seen editguard.Call
			decide := func(call editguard.Call, _ editguard.Options) editguard.Decision {
				seen = call
				if tc.panics {
					panic("guard bug")
				}
				return tc.decision
			}
			verdict := evaluate(t, repo, healthband.PatchPolicy{Decide: decide}, replyWith(editFooTest))
			assert.Equal(t, tc.code, verdict.Code)
			assert.Equal(t, editguard.Call{Cwd: repo.dir, Targets: []string{"pkg/foo/foo_test.go"}}, seen)
		})
	}
}

// S5 (item 2): a header path set or counts that differ from what git apply
// --numstat --summary -z --check reports refuse the diff, and so does any
// summary line but one create mode 100644 per added file (test seams over
// git's output).
func TestPatchPolicy_GitApplyDisagrees_RefusesPatchInvalid(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	newGo := newFile("pkg/foo/new.go", "100644", "package foo")
	cases := map[string]struct {
		diff    string
		rewrite func(out []byte) []byte
	}{
		"other path": {modifyFoo("// x"), func(out []byte) []byte {
			return bytes.Replace(out, []byte("pkg/foo/foo.go"), []byte("pkg/foo/other.go"), 1)
		}},
		"other counts":   {modifyFoo("// x"), func([]byte) []byte { return []byte("2\t0\tpkg/foo/foo.go\x00") }},
		"binary counts":  {modifyFoo("// x"), func([]byte) []byte { return []byte("-\t-\tpkg/foo/foo.go\x00") }},
		"extra record":   {modifyFoo("// x"), func(out []byte) []byte { return append(out, []byte("1\t0\tpkg/foo/x.go\x00")...) }},
		"no record":      {modifyFoo("// x"), func([]byte) []byte { return nil }},
		"short record":   {modifyFoo("// x"), func([]byte) []byte { return []byte("1\tpkg/foo/foo.go\x00") }},
		"delete summary": {modifyFoo("// x"), func(out []byte) []byte { return append(out, []byte(" delete mode 100644 pkg/foo/foo.go\n")...) }},
		"create 100755": {newGo, func(out []byte) []byte {
			return bytes.Replace(out, []byte("create mode 100644"), []byte("create mode 100755"), 1)
		}},
		"missing create": {newGo, func(out []byte) []byte { return out[:bytes.IndexByte(out, 0)+1] }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			git := &recordingGit{home: repo.home, rewrite: func(args []string, out []byte) []byte {
				if args[0] == "apply" {
					return tc.rewrite(out)
				}
				return out
			}}
			verdict := evaluate(t, repo, healthband.PatchPolicy{Git: git}, replyWith(tc.diff))
			assert.Equal(t, healthband.PatchCodeInvalid, verdict.Code)
			assert.True(t, git.invoked("apply", "--numstat", "--summary", "-z", "--check"))
		})
	}
}

// A git fault before the cross-check, or output git never prints, refuses
// the diff patch_invalid, so no fault accepts one.
func TestPatchPolicy_GitFaults_RefusePatchInvalid(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	for name, rewrite := range map[string]func(args []string, out []byte) []byte{
		"listing garbage": func(args []string, out []byte) []byte { return pick(args, "-r", out, []byte("garbage\x00")) },
		"check-attr short": func(args []string, out []byte) []byte {
			return pick(args, "check-attr", out, []byte("pkg/foo/foo.go\x00filter\x00"))
		},
		"check-attr other": func(args []string, out []byte) []byte {
			return pick(args, "check-attr", out, []byte("x\x00filter\x00unset\x00"))
		},
		"check-attr wrong": func(args []string, out []byte) []byte {
			return pick(args, "check-attr", out, []byte("x\x00diff\x00unset\x00"))
		},
		"check-attr set bit": func(args []string, out []byte) []byte {
			return pick(args, "check-attr", out, []byte("pkg/foo/foo.go\x00filter\x00set\x00"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			git := &recordingGit{home: repo.home, rewrite: rewrite}
			want := healthband.PatchCodeInvalid
			if name == "check-attr set bit" {
				want = healthband.PatchCodeFilter
			}
			assert.Equal(t, want, evaluate(t, repo, healthband.PatchPolicy{Git: git}, replyWith(modifyFoo("// x"))).Code)
		})
	}
	failing := healthband.GitRunnerFunc(func(_ context.Context, _ string, _ []byte, args ...string) ([]byte, error) {
		return nil, errors.New("git " + strings.Join(args, " ") + ": exit status 128")
	})
	assert.Equal(t, healthband.PatchCodeInvalid, evaluate(t, repo, healthband.PatchPolicy{Git: failing}, replyWith(modifyFoo("// x"))).Code)
}

// pick returns replacement for the command whose argv holds marker.
func pick(args []string, marker string, out, replacement []byte) []byte {
	for _, arg := range args {
		if arg == marker {
			return replacement
		}
	}
	return out
}
