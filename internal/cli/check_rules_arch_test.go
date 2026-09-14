package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckArch_WarnRangeQuiet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFileWithCodeLines(t, dir, "warn.go", 200)

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, false)
	assert.True(t, result, "warn-range file must pass in quiet mode")
	assert.Empty(t, buf.String(), "quiet mode must suppress warn-range output")
}

func TestCheckArchStaged_OnlyChecksStaged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initTestGitRepo(t, dir)
	writeGoFileWithCodeLines(t, dir, "big.go", 300)
	writeTestFile(t, dir, "small.go", "package dummy\n")

	runGitCommand(t, dir, "add", "small.go")

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, true)
	assert.True(t, result, "staged mode should pass when only small.go is staged")
}

func TestCheckArchStaged_FailsOnOversizedStaged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFileSizePolicy(t, dir, 300)
	initTestGitRepo(t, dir)
	writeGoFileWithCodeLines(t, dir, "big.go", 300)

	runGitCommand(t, dir, "add", "big.go")

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, true)
	assert.False(t, result, "staged mode should fail when big.go is staged")
	assert.Contains(t, buf.String(), "big.go")
}

func TestCheckArchStaged_FailsOnOversizedStagedSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFileSizePolicy(t, dir, 300)
	initTestGitRepo(t, dir)
	writeSourceFileWithLines(t, dir, "panel.tsx", 301)

	runGitCommand(t, dir, "add", "panel.tsx")

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, true)
	assert.False(t, result, "staged mode should fail when a large TypeScript source file is staged")
	assert.Contains(t, buf.String(), "panel.tsx")
}

func TestCheckArchStaged_UsesStagedContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFileSizePolicy(t, dir, 300)
	initTestGitRepo(t, dir)
	writeSourceFileWithLines(t, dir, "staged.ts", 301)
	runGitCommand(t, dir, "add", "staged.ts")
	writeTestFile(t, dir, "staged.ts", "const ok = true\n")

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, true)
	assert.False(t, result, "staged mode should inspect the index rather than the working tree")
	assert.Contains(t, buf.String(), "staged.ts")
}

func TestCheckArchWalk_ChecksNonGoSourceFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFileSizePolicy(t, dir, 300)
	writeSourceFileWithLines(t, dir, "view.tsx", 301)
	writeSourceFileWithLines(t, dir, "native.rs", 301)

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, false)
	assert.False(t, result, "walk should fail on oversized non-Go source files")
	assert.Contains(t, buf.String(), "view.tsx")
	assert.Contains(t, buf.String(), "native.rs")
}

func TestCheckArchWalk_SkipsSubmodule(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	subDir := filepath.Join(dir, "mysubmodule")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(subDir, ".git"), []byte("gitdir: ../.git/modules/mysubmodule"), 0o644),
	)
	writeGoFileWithCodeLines(t, subDir, "big.go", 300)

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, false)
	assert.True(t, result, "walk should skip submodule directories")
}

func TestCheckArchWalk_SkipsNestedGitRepository(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "child-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(nestedDir, ".git"), 0o755))
	writeGoFileWithCodeLines(t, nestedDir, "big.go", 300)

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, false)
	assert.True(t, result, "walk should skip sibling repositories in a meta workspace")
	assert.Empty(t, buf.String())
}

func TestCheckArchWalk_SkipsRuntimeCacheDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".autopus", "qa", "cache", "gopath", "pkg", "mod", "example")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	writeGoFileWithCodeLines(t, cacheDir, "vendor_big.go", 300)

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, false)
	assert.True(t, result, "walk should skip runtime and generated cache directories")
	assert.Empty(t, buf.String())
}

func TestCheckArchWalk_SkipsGeneratedSourceFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSourceFileWithLines(t, dir, "client.generated.ts", 301)
	writeSourceFileWithLines(t, dir, "types.d.ts", 301)
	writeSourceFileWithLines(t, dir, "bundle.min.js", 301)

	var buf bytes.Buffer
	result := checkArch(dir, &buf, true, false)
	assert.True(t, result, "walk should skip generated source files")
	assert.Empty(t, buf.String())
}

func TestCountLines_ReturnsCorrectCount(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "test.go", "line1\nline2\nline3\n")

	n, err := countLines(path)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestCountLines_NonExistentFile(t *testing.T) {
	t.Parallel()

	_, err := countLines("/nonexistent/path/file.go")
	assert.Error(t, err)
}

func writeSourceFileWithLines(t *testing.T, dir, name string, lines int) string {
	t.Helper()

	return writeTestFile(t, dir, name, strings.Repeat("let value = 1;\n", lines))
}
