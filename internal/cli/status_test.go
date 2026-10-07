// Package cli_test contains tests for the status command.
package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSpecFile creates a spec.md file with the given content inside dir/SPEC-ID/spec.md.
func writeSpecFile(t *testing.T, specsDir, specID, content string) {
	t.Helper()
	dir := filepath.Join(specsDir, specID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.md"), []byte(content), 0o644))
}

func TestStatusCmd_NoSpecsDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", dir})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "SPEC이 없습니다")
}

func TestStatusCmd_EmptySpecsDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	specsDir := filepath.Join(dir, ".autopus", "specs")
	require.NoError(t, os.MkdirAll(specsDir, 0o755))

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", dir})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "SPEC이 없습니다")
}

func TestStatusCmd_MultipleSpecs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specsDir := filepath.Join(dir, ".autopus", "specs")

	writeSpecFile(t, specsDir, "SPEC-CLI-001", `# SPEC-CLI-001: 단일 라우터 통합

**Status**: done
**Created**: 2026-01-01
**Domain**: CLI
`)
	writeSpecFile(t, specsDir, "SPEC-QUAL-001", `# SPEC-QUAL-001: 품질 모드 선택

**Status**: done
**Created**: 2026-01-02
**Domain**: Quality
`)
	writeSpecFile(t, specsDir, "SPEC-UX-001", `# SPEC-UX-001: UX 브랜딩 개선

**Status**: draft
**Created**: 2026-01-03
**Domain**: UX
`)

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", dir})
	require.NoError(t, cmd.Execute())

	output := out.String()
	assert.Contains(t, output, "SPEC-CLI-001")
	assert.Contains(t, output, "SPEC-QUAL-001")
	assert.Contains(t, output, "SPEC-UX-001")
	assert.Contains(t, output, "2/3 완료")
}

func TestStatusCmd_ApprovedStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specsDir := filepath.Join(dir, ".autopus", "specs")

	writeSpecFile(t, specsDir, "SPEC-FOO-001", `# SPEC-FOO-001: Approved Feature

**Status**: approved
**Created**: 2026-01-01
**Domain**: Core
`)

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", dir})
	require.NoError(t, cmd.Execute())

	output := out.String()
	assert.Contains(t, output, "SPEC-FOO-001")
	// approved should show arrow icon direction (not done)
	assert.Contains(t, output, "0/1 완료")
}

func TestStatusCmd_MalformedSpecFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specsDir := filepath.Join(dir, ".autopus", "specs")

	// Malformed: missing status and title lines
	writeSpecFile(t, specsDir, "SPEC-BAD-001", `no heading here
just random text
nothing useful
`)

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", dir})
	require.NoError(t, cmd.Execute())

	// Should still show the SPEC entry with fallback status
	output := out.String()
	assert.Contains(t, output, "SPEC-BAD-001")
}

func TestStatusCmd_NonSpecDirectoriesIgnored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specsDir := filepath.Join(dir, ".autopus", "specs")

	// Add a non-SPEC directory that should be ignored
	require.NoError(t, os.MkdirAll(filepath.Join(specsDir, "archive"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(specsDir, "templates"), 0o755))

	writeSpecFile(t, specsDir, "SPEC-REAL-001", `# SPEC-REAL-001: Real SPEC

**Status**: draft
**Created**: 2026-01-01
**Domain**: Test
`)

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", dir})
	require.NoError(t, cmd.Execute())

	output := out.String()
	assert.Contains(t, output, "SPEC-REAL-001")
	// Non-SPEC dirs should not appear
	assert.NotContains(t, output, "archive")
	assert.NotContains(t, output, "templates")
}

func TestStatusCmd_ImplementedStatusCountsAsDone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specsDir := filepath.Join(dir, ".autopus", "specs")

	writeSpecFile(t, specsDir, "SPEC-IMPL-001", `# SPEC-IMPL-001: Implemented Feature

**Status**: implemented
**Created**: 2026-01-01
**Domain**: Core
`)

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", dir})
	require.NoError(t, cmd.Execute())

	output := out.String()
	assert.Contains(t, output, "1/1 완료")
}

// TestStatusCmd_DefaultDir verifies that when --dir is omitted, the command
// falls back to the current working directory without error. The working
// directory is process-wide, so the test does not run in parallel: t.Chdir
// refuses a parallel test and restores the directory afterwards.
func TestStatusCmd_DefaultDir(t *testing.T) {
	// Create a temp dir that has no .autopus/specs so the "no specs" path runs.
	dir := t.TempDir()
	t.Chdir(dir)

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	// No --dir flag: command must derive dir from os.Getwd()
	cmd.SetArgs([]string{"status"})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "SPEC이 없습니다")
}

// TestParseSpecFile_UnreadableFile verifies that parseSpecFile returns empty
// strings gracefully when the file cannot be opened (e.g. does not exist).
func TestParseSpecFile_UnreadableFile(t *testing.T) {
	t.Parallel()

	// Given: a root layout where spec.md is absent inside the SPEC directory.
	// scanSpecs should still return the entry with the "draft" fallback status.
	root := t.TempDir()
	rootSpecsDir := filepath.Join(root, ".autopus", "specs")
	require.NoError(t, os.MkdirAll(filepath.Join(rootSpecsDir, "SPEC-MISSING-001"), 0o755))
	// spec.md is absent — parseSpecFile will fail to open and return ("", "")

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", root})
	require.NoError(t, cmd.Execute())

	// The SPEC should still be listed with fallback "draft" status.
	output := out.String()
	assert.Contains(t, output, "SPEC-MISSING-001")
}

// TestScanSpecs_NonDirectoryEntriesIgnored verifies that regular files inside
// the specs directory are not mistakenly treated as SPEC entries.
func TestScanSpecs_NonDirectoryEntriesIgnored(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	specsDir := filepath.Join(root, ".autopus", "specs")
	require.NoError(t, os.MkdirAll(specsDir, 0o755))

	// Place a regular file (not a directory) whose name starts with "SPEC-".
	// scanSpecs must skip it because IsDir() returns false.
	regularFile := filepath.Join(specsDir, "SPEC-FILE-001")
	require.NoError(t, os.WriteFile(regularFile, []byte("not a dir"), 0o644))

	// Also add a valid SPEC directory so the output contains at least one entry.
	writeSpecFile(t, specsDir, "SPEC-REAL-002", `# SPEC-REAL-002: Real Entry

**Status**: draft
**Created**: 2026-01-01
**Domain**: Test
`)

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--dir", root})
	require.NoError(t, cmd.Execute())

	output := out.String()
	assert.Contains(t, output, "SPEC-REAL-002")
	// The plain file must not appear as a SPEC entry.
	assert.NotContains(t, output, "SPEC-FILE-001")
}
