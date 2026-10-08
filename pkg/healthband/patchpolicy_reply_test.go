package healthband_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S4 (policy part): a well-formed proposal is accepted with the diff and the
// git apply --numstat counts that the result record's files[] carries.
func TestPatchPolicy_ModificationOfTrackedFile_AcceptsWithNumstatFiles(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	diff := modifyFoo("", "func Bar() int { return 2 }")

	verdict := evaluate(t, repo, healthband.PatchPolicy{}, replyWith(diff))

	require.True(t, verdict.Accepted(), "code %q", verdict.Code)
	assert.Equal(t, diff, verdict.Diff)
	assert.Equal(t, []healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 2, Removed: 0}}, verdict.Files)
}

// S5 (item 1): a capture past 1 MiB, a dropped capture, more than 64 KiB of
// diff, and zero, two, or an unclosed diff fence stop before any git command.
func TestPatchPolicy_ReplyShape_RefusesWithItemOneCodes(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	ok := modifyFoo("// ok")
	var big []string
	for i := 0; len(strings.Join(big, "\n")) < 70<<10; i++ {
		big = append(big, fmt.Sprintf("// line %d of a seventy KiB diff", i))
	}
	cases := []struct {
		name  string
		reply healthband.PatchReply
		code  string
	}{
		{"capture past 1 MiB", healthband.PatchReply{Text: replyWith(ok).Text + strings.Repeat("x", healthband.ProviderCaptureBytes)}, healthband.PatchCodeTooLarge},
		{"dropped capture", healthband.PatchReply{Text: replyWith(ok).Text, Dropped: true}, healthband.PatchCodeTooLarge},
		{"70 KiB diff", replyWith(newFile("pkg/foo/big.go", "100644", big...)), healthband.PatchCodeTooLarge},
		{"zero fences", healthband.PatchReply{Text: "No safe change exists.\n"}, healthband.PatchCodeNoPatch},
		{"two diff fences", healthband.PatchReply{Text: replyWith(ok).Text + replyWith(ok).Text}, healthband.PatchCodeNoPatch},
		{"unclosed diff fence", healthband.PatchReply{Text: "```diff\n" + ok}, healthband.PatchCodeNoPatch},
		{"diff fence only inside another fence", healthband.PatchReply{Text: "````markdown\n```diff\n" + ok + "```\n````\n"}, healthband.PatchCodeNoPatch},
		{"go fence is no diff fence", healthband.PatchReply{Text: "```go\n" + ok + "```\n"}, healthband.PatchCodeNoPatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			git := &recordingGit{home: repo.home}
			verdict := evaluate(t, repo, healthband.PatchPolicy{Git: git}, tc.reply)
			assert.Equal(t, tc.code, verdict.Code)
			assert.Empty(t, verdict.Diff)
			assert.Empty(t, git.calls, "item 1 runs no git command")
		})
	}
}

// Item 1: the fence may be longer than three backticks, use tildes, carry
// text after diff, and end its lines with CRLF; the content is the diff.
func TestPatchPolicy_FenceVariants_TakeTheFenceContentAsTheDiff(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	diff := modifyFoo("// ok")
	for name, text := range map[string]string{
		"four backticks": "````diff\n" + diff + "````\n",
		"tildes":         "~~~diff\n" + diff + "~~~\n",
		"info words":     "```diff title=fix\n" + diff + "```   \n",
		"crlf fences":    "```diff\r\n" + diff + "```\r\n",
		"other fence":    "```text\nplan\n```\n```diff\n" + diff + "```\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			verdict := evaluate(t, repo, healthband.PatchPolicy{}, healthband.PatchReply{Text: text})
			require.True(t, verdict.Accepted(), "code %q", verdict.Code)
			assert.Equal(t, diff, verdict.Diff)
		})
	}
}

// A context that ends while git runs is an error, never a verdict, so the
// executor can tell a stopped step from a refused diff.
func TestPatchPolicy_ContextEnded_ReturnsTheContextError(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := healthband.PatchPolicy{Git: &recordingGit{home: repo.home}}.Evaluate(ctx, healthband.PatchPolicyInput{
		Reply: replyWith(modifyFoo("// ok")), BaseSHA: repo.base, Worktree: repo.dir, Checkout: repo.dir,
	})

	require.ErrorIs(t, err, context.Canceled)
}

// Invalid input is a programming error of the caller, never a verdict.
func TestPatchPolicy_InvalidInput_ReturnsAnError(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, policyBase)
	valid := healthband.PatchPolicyInput{Reply: replyWith(modifyFoo("// ok")), BaseSHA: repo.base, Worktree: repo.dir, Checkout: repo.dir}
	for name, mutate := range map[string]func(*healthband.PatchPolicyInput){
		"short base":       func(in *healthband.PatchPolicyInput) { in.BaseSHA = "abc123" },
		"upper base":       func(in *healthband.PatchPolicyInput) { in.BaseSHA = strings.ToUpper(repo.base) },
		"relative tree":    func(in *healthband.PatchPolicyInput) { in.Worktree = "worktree" },
		"missing checkout": func(in *healthband.PatchPolicyInput) { in.Checkout = "" },
	} {
		in := valid
		mutate(&in)
		_, err := healthband.PatchPolicy{Git: &recordingGit{home: repo.home}}.Evaluate(context.Background(), in)
		assert.Error(t, err, name)
	}
	_, err := healthband.PatchPolicy{}.Evaluate(context.Background(), valid)
	assert.Error(t, err, "nil runner")
}
