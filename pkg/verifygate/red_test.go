package verifygate

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	redModule  = "module redgate\n\ngo 1.22\n"
	redAddBase = "package redgate\n\nfunc Add(a, b int) int { return a - b }\n"
	redAddFix  = "package redgate\n\nfunc Add(a, b int) int { return a + b }\n"
	// redTestNew fails against redAddBase and passes against redAddFix.
	redTestNew = "package redgate\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add(1, 2) != 3\")\n\t}\n}\n"
	// redTestWeak passes against both.
	redTestWeak = "package redgate\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 0) != 2 {\n\t\tt.Fatal(\"Add(2, 0) != 2\")\n\t}\n}\n"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

// redRepo commits the base module, then applies the uncommitted edit.
func redRepo(t *testing.T, base, edit map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, base)
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "base")
	writeFiles(t, root, edit)
	return root
}

func checkRed(t *testing.T, root string) RedResult {
	t.Helper()
	_, changed, ok := ChangedFiles(context.Background(), root)
	require.True(t, ok)
	return CheckRed(context.Background(), root, changed, 3*time.Minute)
}

func TestCheckRed_GoVerdicts(t *testing.T) {
	base := map[string]string{"go.mod": redModule, "add.go": redAddBase}
	cases := map[string]struct {
		base, edit map[string]string
		want       RedVerdict
	}{
		"a new test that fails before the fix": {base, map[string]string{"add.go": redAddFix, "add_test.go": redTestNew}, RedProven},
		"a test that passes before the fix":    {base, map[string]string{"add.go": redAddFix, "add_test.go": redTestWeak}, RedPassesBefore},
		"a fix with no test":                   {base, map[string]string{"add.go": redAddFix}, RedNoTest},
		"a test-only change":                   {base, map[string]string{"add_test.go": redTestWeak}, RedNotApplicable},
		"a docs-only change":                   {base, map[string]string{"README.md": "# notes\n"}, RedNotApplicable},
		"a test of a function that did not exist": {
			map[string]string{"go.mod": redModule, "doc.go": "package redgate\n"},
			map[string]string{"add.go": redAddFix, "add_test.go": redTestNew}, RedProven,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, checkRed(t, redRepo(t, tc.base, tc.edit)).Verdict)
		})
	}
}

// The before-the-change run happens in a scratch copy: the repository keeps
// no worktree, the work tree keeps its edit, and HEAD does not move.
func TestCheckRed_LeavesTheRepositoryAlone(t *testing.T) {
	root := redRepo(t, map[string]string{"go.mod": redModule, "add.go": redAddBase},
		map[string]string{"add.go": redAddFix, "add_test.go": redTestNew})
	head := gitRun(t, root, "rev-parse", "HEAD")
	status := gitRun(t, root, "status", "--porcelain")

	require.Equal(t, RedProven, checkRed(t, root).Verdict)
	assert.Equal(t, head, gitRun(t, root, "rev-parse", "HEAD"))
	assert.Equal(t, status, gitRun(t, root, "status", "--porcelain"))
	assert.Equal(t, 1, strings.Count(gitRun(t, root, "worktree", "list"), "\n"))
	data, err := os.ReadFile(root + "/add.go")
	require.NoError(t, err)
	assert.Equal(t, redAddFix, string(data))
}

func TestIsTest(t *testing.T) {
	for _, f := range []string{"a/b_test.go", "a/testdata/x.json", "web/__tests__/x.ts", "web/x.test.tsx", "web/x.spec.js", "py/test_x.py", "py/x_test.py"} {
		assert.True(t, IsTest(f), f)
	}
	for _, f := range []string{"a/b.go", "web/x.tsx", "py/x.py", "py/testing.py", "README.md"} {
		assert.False(t, IsTest(f), f)
	}
}
