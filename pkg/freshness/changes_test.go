package freshness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		writeRel(t, dir, rel, body)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "base", "--allow-empty"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dir
}

func writeRel(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

func changedSpecs(t *testing.T, dir string) map[string]string {
	t.Helper()
	_, changes, ok := Changed(context.Background(), dir)
	require.True(t, ok)
	out := map[string]string{}
	for _, c := range changes {
		out[c.Item.Key()] = c.Previous + "->" + c.Item.Spec
	}
	return out
}

// Only what the work tree adds or moves is reported; a pre-existing old pin
// stays quiet even when the edit reorders its file.
func TestChanged_OnlyNewAndBumpedPins(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"package.json":       `{"dependencies":{"lodash":"^3.0.0","react":"^17.0.2"}}`,
		"docker-compose.yml": "services:\n  db:\n    image: postgres:12\n",
	})
	writeRel(t, dir, "package.json", "{\n  \"dependencies\": {\n    \"axios\": \"^0.27.0\",\n    \"react\": \"^18.2.0\",\n    \"lodash\": \"^3.0.0\"\n  }\n}\n")
	writeRel(t, dir, "docker-compose.yml", "services:\n  cache:\n    image: redis:6\n  db:\n    image: postgres:12\n")
	writeRel(t, dir, "api/requirements.txt", "django==3.2\n")
	writeRel(t, dir, "testdata/docker-compose.yml", "services:\n  old:\n    image: mysql:5.7\n")
	writeRel(t, dir, "README.md", "postgres:9\n")
	assert.Equal(t, map[string]string{
		"npm:axios@dependencies": "->^0.27.0",
		"npm:react@dependencies": "^17.0.2->^18.2.0",
		"image:redis":            "->6",
		"pypi:django":            "->==3.2",
	}, changedSpecs(t, dir))
}

func TestChanged_CleanTreeNonRepoAndNoHead(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": "module x\n\ngo 1.20\n"})
	assert.Empty(t, changedSpecs(t, dir))
	_, _, ok := Changed(context.Background(), t.TempDir())
	assert.False(t, ok)

	fresh := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = fresh
	require.NoError(t, cmd.Run())
	writeRel(t, fresh, "go.mod", "module x\n\ngo 1.20\n")
	assert.Equal(t, map[string]string{"toolchain:go@go": "->1.20"}, changedSpecs(t, fresh), "a repository with no commit yet: every pin is new")
}
