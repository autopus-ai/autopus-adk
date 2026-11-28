package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harnessfb"
)

// encodeRepo is a repository with one committed source file, an approved
// lesson HL-001, and an uncommitted, untracked lint script: the check a
// work agent has just written and not yet committed.
func encodeRepo(t *testing.T) string {
	t.Helper()
	dir := harnessRepo(t)
	t.Setenv(harnessFeedbackEnv, "")
	for _, kv := range [][2]string{{"GIT_AUTHOR_NAME", "t"}, {"GIT_AUTHOR_EMAIL", "t@t"}, {"GIT_COMMITTER_NAME", "t"}, {"GIT_COMMITTER_EMAIL", "t@t"}} {
		t.Setenv(kv[0], kv[1])
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.js"), []byte("render()\n"), 0o644))
	encodeGit(t, dir, "add", "src")
	encodeGit(t, dir, "commit", "-q", "-m", "init")

	stubHarnessProviders(t, "survived")
	tdir := t.TempDir()
	for _, id := range []string{"s1", "s2", "s3"} {
		_, err := runAuto(t, "harness", "observe", "--session-id", id, "--transcript", reviewHeavyTranscript(t, tdir, id))
		require.NoError(t, err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lint.sh"), []byte("! grep -rn setTimeout src\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.js"), []byte("render()\nupdate()\n"), 0o644))
	return dir
}

func encodeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func writePatch(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "repro.patch")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

// reproPatch puts the lesson's mistake into the uncommitted app.js.
const reproPatch = `--- a/src/app.js
+++ b/src/app.js
@@ -1,2 +1,3 @@
 render()
 update()
+setTimeout(update, 0)
`

func ruleText(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, harnessfb.RuleFile))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

func TestHarnessEncode_ApproveBriefsAndKeepsATemporaryRule(t *testing.T) {
	dir := encodeRepo(t)
	out, err := runAuto(t, "harness", "approve", "HL-001")
	require.NoError(t, err)
	for _, want := range []string{"api(", "> lint(", "> test >", "> hook(", "> text", "--repro", "--check", "auto harness encode HL-001"} {
		assert.Contains(t, out, want)
	}
	assert.Contains(t, ruleText(t, dir), "HL-001 (review-heavy, 3 sessions; temporary until a check enforces it)")
	status, err := runAuto(t, "harness", "status")
	require.NoError(t, err)
	assert.Contains(t, status, "구조화: 대기 (임시 글 규칙)")
}

func TestHarnessEncode_ProvenCheckLeavesTheRuleFileAndTheTreeUntouched(t *testing.T) {
	dir := encodeRepo(t)
	_, err := runAuto(t, "harness", "approve", "HL-001")
	require.NoError(t, err)
	// The rule file under .claude/ is meant to go; everything else stays.
	beforeStatus := encodeGit(t, dir, "status", "--porcelain", "--", ".", ":!.claude")
	beforeApp, _ := os.ReadFile(filepath.Join(dir, "src", "app.js"))
	beforeWT := encodeGit(t, dir, "worktree", "list")

	out, err := runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", "sh lint.sh", "--repro", writePatch(t, reproPatch))
	require.NoError(t, err, out)
	assert.Contains(t, out, "HL-001 증명됨: lint 층, 검사 `sh lint.sh`, patch가 바꾼 파일 src/app.js")
	assert.Empty(t, ruleText(t, dir), "the only lesson is enforced by a check: no text rule is left")

	status, err := runAuto(t, "harness", "status")
	require.NoError(t, err)
	assert.Contains(t, status, "구조화: lint 층, 검사 `sh lint.sh`")

	assert.Equal(t, beforeStatus, encodeGit(t, dir, "status", "--porcelain", "--", ".", ":!.claude"), "uncommitted and untracked files are as they were")
	afterApp, _ := os.ReadFile(filepath.Join(dir, "src", "app.js"))
	assert.Equal(t, string(beforeApp), string(afterApp))
	assert.Equal(t, beforeWT, encodeGit(t, dir, "worktree", "list"), "the proof's worktree is gone")
}

func TestHarnessEncode_RefusedProofsKeepTheRule(t *testing.T) {
	cases := []struct {
		name, check, patch, want string
	}{
		{"misses the mistake", "true", reproPatch, "재현 patch를 적용해도 검사가 통과합니다"},
		{"fails on the current tree", "false", reproPatch, "검사가 지금 트리에서 통과하지 않습니다"},
		{"patch does not apply", "sh lint.sh", strings.Replace(reproPatch, " update()", " missing()", 1), "재현 patch를 지금 트리에 적용할 수 없습니다"},
		{"depends on an ignored file", "test -f build.out", reproPatch, "복사본에서 통과하지 않습니다"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := encodeRepo(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build.out\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "build.out"), nil, 0o644))
			_, err := runAuto(t, "harness", "approve", "HL-001")
			require.NoError(t, err)
			out, err := runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", tc.check, "--repro", writePatch(t, tc.patch))
			require.Error(t, err)
			assert.Contains(t, out+err.Error(), tc.want)
			assert.Contains(t, ruleText(t, dir), "HL-001", "the temporary rule stays")
			status, _ := runAuto(t, "harness", "status")
			assert.Contains(t, status, "구조화: 대기")
		})
	}
}

func TestHarnessEncode_TextNeedsAReason(t *testing.T) {
	dir := encodeRepo(t)
	_, err := runAuto(t, "harness", "approve", "HL-001")
	require.NoError(t, err)

	_, err = runAuto(t, "harness", "encode", "HL-001", "--layer", "text")
	require.ErrorIs(t, err, harnessfb.ErrReasonRequired)
	status, _ := runAuto(t, "harness", "status")
	assert.Contains(t, status, "구조화: 대기")

	_, err = runAuto(t, "harness", "encode", "HL-001", "--layer", "text", "--reason", "judgement about review timing")
	require.NoError(t, err)
	assert.Contains(t, ruleText(t, dir), "no check fits: judgement about review timing")
	status, _ = runAuto(t, "harness", "status")
	assert.Contains(t, status, "구조화: 글 규칙 유지 — judgement about review timing")
}

func TestHarnessEncode_OnlyApprovedLessons(t *testing.T) {
	encodeRepo(t)
	_, err := runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", "sh lint.sh", "--repro", writePatch(t, reproPatch))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "승인된 교훈이 아닙니다")
}

// A proof is bound to the lesson text it was run for, and nothing in the
// working tree can claim one: an edited lesson is unproven again.
func TestHarnessEncode_ProofIsBoundToTheLessonText(t *testing.T) {
	dir := encodeRepo(t)
	_, err := runAuto(t, "harness", "approve", "HL-001")
	require.NoError(t, err)
	_, err = runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", "sh lint.sh", "--repro", writePatch(t, reproPatch))
	require.NoError(t, err)

	c, err := harnessfb.LoadCandidate(dir, "HL-001")
	require.NoError(t, err)
	_, ok := harnessfb.LoadEncoding(dir, c)
	require.True(t, ok)
	c.Lesson = "Something else."
	_, ok = harnessfb.LoadEncoding(dir, c)
	assert.False(t, ok, "a proof recorded for another rule text does not count")
}
