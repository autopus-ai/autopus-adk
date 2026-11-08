package loop

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearIdentityEnv removes identity variables for this test only.
func clearIdentityEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
}

// A machine with no git identity still gets its loop commits, signed as the
// tool; a configured identity is always the one used.
func TestCommit_FallsBackToAToolIdentityOnlyWhenNoneResolves(t *testing.T) {
	clearIdentityEnv(t)
	r := newRepo(t, seed)
	r.git("config", "user.useConfigOnly", "true")
	g := gitRepo{top: r.dir, env: []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}}

	r.write("fixed.txt", "first\n")
	_, err := g.commit([]string{"fixed.txt"}, "fix: first\n")
	require.NoError(t, err)
	assert.Equal(t, "Autopus QA loop <noreply@autopus.co>", r.git("log", "-1", "--format=%an <%ae>"))
	assert.Equal(t, "Autopus QA loop <noreply@autopus.co>", r.git("log", "-1", "--format=%cn <%ce>"))
	_, err = runGit(r.dir, nil, "config", "--get", "user.email")
	assert.Error(t, err, "the person's config is never written")

	r.git("config", "user.name", "Dana")
	r.git("config", "user.email", "dana@example.invalid")
	r.write("fixed.txt", "second\n")
	_, err = g.commit([]string{"fixed.txt"}, "fix: second\n")
	require.NoError(t, err)
	assert.Equal(t, "Dana <dana@example.invalid>", r.git("log", "-1", "--format=%an <%ae>"))
}

// The kept tail is bounded and never splits a multibyte rune.
func TestOutputTail_BoundsAndKeepsRunes(t *testing.T) {
	assert.Equal(t, "short", outputTail("  short \n"))
	long := strings.Repeat("가", maxOutputTail)
	got := outputTail(long)
	assert.True(t, utf8.ValidString(got))
	assert.LessOrEqual(t, len(got), maxOutputTail+len("…"))
	assert.True(t, strings.HasPrefix(got, "…"))
}
