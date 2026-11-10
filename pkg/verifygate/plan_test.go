package verifygate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
}

func TestPlan_GoPackagesOfChangedFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod": "module x\n", "a.go": "package x\n", "pkg/b/b.go": "package b\n",
		"pkg/b/testdata/c.go": "package c\n",
	})

	steps := Plan(root, []string{"pkg/b/b.go", "a.go", "pkg/b/testdata/c.go", "gone/d.go", "README.md"})

	require.Len(t, steps, 2)
	assert.Equal(t, []string{"go", "build", "./..."}, steps[0].Argv)
	assert.Equal(t, []string{"go", "test", ".", "./pkg/b"}, steps[1].Argv, "testdata and deleted dirs are not packages")
}

func TestPlan_NothingForDocsOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module x\n", "package.json": `{"scripts":{"test":"x"}}`})

	assert.Empty(t, Plan(root, []string{"README.md", "docs/a.md"}))
}

func TestPlan_NodeScriptsWithLockfileManager(t *testing.T) {
	onPath(t, "pnpm")
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"package.json": `{"scripts":{"test":"vitest run","typecheck":"tsc","lint":"eslint"}}`, "pnpm-lock.yaml": "",
	})

	steps := Plan(root, []string{"src/a.ts"})

	require.Len(t, steps, 2)
	assert.Equal(t, []string{"pnpm", "run", "typecheck"}, steps[0].Argv)
	assert.Equal(t, []string{"pnpm", "run", "test"}, steps[1].Argv)
}

func TestParsePorcelainZ(t *testing.T) {
	t.Parallel()
	out := []byte(" M a.go\x00?? new.go\x00R  b.go\x00old.go\x00 D gone.go\x00D  gone2.go\x00")

	assert.Equal(t, []string{"a.go", "new.go", "b.go"}, parsePorcelainZ(out))
}

func TestWithoutHarnessPaths(t *testing.T) {
	t.Parallel()
	got := withoutHarnessPaths([]string{"main.go", ".autopus/qa/hunt/findings/x/repro.sh", ".claude/settings.json", "docs/a.md"})

	assert.Equal(t, []string{"main.go", "docs/a.md"}, got)
}
