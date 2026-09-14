package skillevolve

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A candidate that fails mid-apply must leave the workspace exactly as it was:
// a half-applied promotion ships a source of truth nobody reviewed together.
func TestPromoteCandidate_RestoresEarlierTargetsWhenALaterWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not block writes the same way on windows")
	}
	t.Parallel()

	projectDir := t.TempDir()
	firstRel := "autopus-adk/content/skills/testing-strategy.md"
	secondRel := "autopus-adk/content/skills/locked/verification.md"
	oldFirst := validSkillContent("testing-strategy") + "old body\n"
	writeWorkspaceFile(t, projectDir, firstRel, oldFirst)

	// The second target's parent exists but denies writes, so the write fails
	// after the first target has already been replaced.
	lockedDir := filepath.Join(projectDir, "autopus-adk", "content", "skills", "locked")
	require.NoError(t, os.MkdirAll(lockedDir, 0o755))
	require.NoError(t, os.Chmod(lockedDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) })

	candidate := promotionReadyCandidate(validSkillContent("testing-strategy") + "new body\n")
	candidate.ProposedFiles = append(candidate.ProposedFiles, ProposedFile{
		Path:    secondRel,
		Content: validSkillContent("verification") + "second body\n",
	})
	candidate.Provenance.AffectedSourceOfTruths = append(
		candidate.Provenance.AffectedSourceOfTruths, secondRel,
	)

	result, err := PromoteCandidate(context.Background(), PromotionOptions{
		ProjectDir: projectDir,
		Candidate:  candidate,
		Approval:   HumanApproval{ApprovedBy: "human-reviewer"},
		Apply:      true,
	})

	require.Error(t, err)
	assert.False(t, result.Applied)
	assert.Equal(t, oldFirst, readWorkspaceFiles(t, projectDir, []string{firstRel})[firstRel])
}

// rollbackPromotionTargets deletes files that did not exist before the apply
// and restores both the bytes and the mode of files that did.
func TestRollbackPromotionTargets_RemovesNewFilesAndRestoresModes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.md")
	created := filepath.Join(dir, "created.md")
	require.NoError(t, os.WriteFile(existing, []byte("original\n"), 0o600))
	require.NoError(t, os.WriteFile(existing, []byte("overwritten\n"), 0o644))
	require.NoError(t, os.WriteFile(created, []byte("brand new\n"), 0o644))

	targets := []promotionTarget{{Path: existing}, {Path: created}}
	snapshots := map[string]fileSnapshot{
		existing: {Exists: true, Mode: 0o600, Body: []byte("original\n")},
		created:  {},
	}

	rollbackPromotionTargets(targets, snapshots)

	body, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(body))
	info, err := os.Stat(existing)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	_, statErr := os.Stat(created)
	assert.True(t, os.IsNotExist(statErr), "a file the apply created must not survive rollback")
}

// A snapshot recorded with a zero mode still restores a readable file rather
// than one nothing can open.
func TestRollbackPromotionTargets_UsesDefaultModeForZeroModeSnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "zero-mode.md")
	require.NoError(t, os.WriteFile(path, []byte("replaced\n"), 0o644))

	rollbackPromotionTargets(
		[]promotionTarget{{Path: path}},
		map[string]fileSnapshot{path: {Exists: true, Body: []byte("restored\n")}},
	)

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "restored\n", string(body))
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	}
}

// Snapshotting refuses a symlinked target: following it would capture and later
// restore a file outside the reviewed set.
func TestSnapshotPromotionTargets_RejectsSymlinkedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevation on windows")
	}
	t.Parallel()

	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside\n"), 0o644))
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(outside, link))

	_, err := snapshotPromotionTargets([]promotionTarget{{Path: link}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

// An absent target is snapshotted as "did not exist" so rollback deletes it
// instead of leaving a written file behind.
func TestSnapshotPromotionTargets_RecordsAbsentTargetAsNonExistent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "absent.md")

	snapshots, err := snapshotPromotionTargets([]promotionTarget{{Path: path}})

	require.NoError(t, err)
	require.Contains(t, snapshots, path)
	assert.False(t, snapshots[path].Exists)
}

// The write is atomic: the target is replaced by a rename, so a reader never
// observes a partially written source of truth, and no temp file survives.
func TestWritePromotionTarget_ReplacesAtomicallyWithoutLeavingTempFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "skill.md")
	require.NoError(t, os.WriteFile(path, []byte("before\n"), 0o644))

	require.NoError(t, writePromotionTarget(promotionTarget{Path: path, Content: "after\n"}))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "after\n", string(body))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "skill.md", entries[0].Name())
}

func TestPromoteCandidate_RejectsDuplicateProposedTarget(t *testing.T) {
	t.Parallel()

	body := validSkillContent("testing-strategy") + "body\n"
	candidate := promotionReadyCandidate(body)
	duplicate := candidate.ProposedFiles[0]
	candidate.ProposedFiles = append(candidate.ProposedFiles, duplicate)

	_, err := PromoteCandidate(context.Background(), PromotionOptions{
		ProjectDir: t.TempDir(),
		Candidate:  candidate,
		Approval:   HumanApproval{ApprovedBy: "human-reviewer"},
		Apply:      true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}
