package promptlayer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Guards the fail-closed admission boundary of the persisted evidence store:
// every rejection below must surface as an error, never as an empty-but-ok load.

func writeEvidenceFixture(t *testing.T, checkedAt time.Time) (string, promptlayer.OMPContextEvidenceStoreBindingV1) {
	t.Helper()
	root := t.TempDir()
	binding := evidenceStoreBinding(checkedAt)
	require.NoError(t, promptlayer.WriteOMPContextEvidenceStoreV1(root, promptlayer.OMPContextEvidenceStoreV1{
		Binding: binding, Policy: evidenceStorePolicy(), CanaryRows: promotionRows(),
	}))
	return root, binding
}

func evidenceSubject() promptlayer.OMPContextPromotionSubjectV1 {
	return promptlayer.OMPContextPromotionSubjectV1{
		WorkspaceID: "workspace-1", SpecID: "SPEC-OMP-004", TaskID: "T5",
		Phase: "implementation", SessionID: "session-1",
		BindingHash: "sha256:" + strings.Repeat("e", 64),
	}
}

func TestOMPContextEvidenceVerify_RequiresVerificationClock(t *testing.T) {
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root, binding := writeEvidenceFixture(t, checkedAt)

	_, err := promptlayer.LoadOMPContextEvidenceStoreV1(
		root, binding, evidenceSubject(), evidenceStorePolicy(), time.Time{},
	)
	require.ErrorContains(t, err, "verification time is required")
}

func TestOMPContextEvidenceVerify_RejectsUnsafeRootAndDirectoryModes(t *testing.T) {
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	now := checkedAt.Add(time.Minute)
	binding := evidenceStoreBinding(checkedAt)
	policy := evidenceStorePolicy()
	subject := evidenceSubject()

	load := func(root string) error {
		_, err := promptlayer.LoadOMPContextEvidenceStoreV1(root, binding, subject, policy, now)
		return err
	}

	require.ErrorContains(t, load(""), "root is required")
	require.ErrorContains(t, load("."), "root is required")
	require.ErrorContains(t, load(filepath.Join(t.TempDir(), "absent")), "must be a regular directory")

	fileRoot := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(fileRoot, []byte("x"), 0o600))
	require.ErrorContains(t, load(fileRoot), "must be a regular directory")

	worldWritable, _ := writeEvidenceFixture(t, checkedAt)
	require.NoError(t, os.Chmod(worldWritable, 0o777))
	t.Cleanup(func() { _ = os.Chmod(worldWritable, 0o700) })
	require.ErrorContains(t, load(worldWritable), "root permissions are unsafe")

	loosened, _ := writeEvidenceFixture(t, checkedAt)
	leaf := filepath.Dir(promptlayer.OMPContextEvidenceStorePath(loosened))
	require.NoError(t, os.Chmod(leaf, 0o750))
	t.Cleanup(func() { _ = os.Chmod(leaf, 0o700) })
	require.ErrorContains(t, load(loosened), "directory permissions are unsafe")
}

func TestOMPContextEvidenceVerify_RejectsEscapingEvidenceDirectory(t *testing.T) {
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root, binding := writeEvidenceFixture(t, checkedAt)

	leaf := filepath.Dir(promptlayer.OMPContextEvidenceStorePath(root))
	outside := t.TempDir()
	require.NoError(t, os.Chmod(outside, 0o700))
	require.NoError(t, os.RemoveAll(leaf))
	require.NoError(t, os.Symlink(outside, leaf))

	_, err := promptlayer.LoadOMPContextEvidenceStoreV1(
		root, binding, evidenceSubject(), evidenceStorePolicy(), checkedAt.Add(time.Minute),
	)
	require.ErrorContains(t, err, "unsafe")
}

func TestOMPContextEvidenceVerify_RejectsPolicyAndSubjectDivergence(t *testing.T) {
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	now := checkedAt.Add(time.Minute)
	root, binding := writeEvidenceFixture(t, checkedAt)

	otherPolicy := evidenceStorePolicy()
	otherPolicy.HistoryTargetTokens = 4242
	_, err := promptlayer.LoadOMPContextEvidenceStoreV1(root, binding, evidenceSubject(), otherPolicy, now)
	require.ErrorContains(t, err, "policy mismatch")

	invalidPolicy := evidenceStorePolicy()
	invalidPolicy.Profile = ""
	_, err = promptlayer.LoadOMPContextEvidenceStoreV1(root, binding, evidenceSubject(), invalidPolicy, now)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "policy mismatch")

	foreignSubject := evidenceSubject()
	foreignSubject.WorkspaceID = "workspace-2"
	_, err = promptlayer.LoadOMPContextEvidenceStoreV1(root, binding, foreignSubject, evidenceStorePolicy(), now)
	require.ErrorContains(t, err, "subject mismatch")

	foreignSpec := evidenceSubject()
	foreignSpec.SpecID = "SPEC-OMP-999"
	_, err = promptlayer.LoadOMPContextEvidenceStoreV1(root, binding, foreignSpec, evidenceStorePolicy(), now)
	require.ErrorContains(t, err, "subject mismatch")
}

func TestOMPContextEvidenceVerify_RejectsMalformedExpectedBinding(t *testing.T) {
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root, binding := writeEvidenceFixture(t, checkedAt)
	now := checkedAt.Add(time.Minute)

	// An expected binding that cannot itself validate must never be treated as authority,
	// even when the stored document is otherwise intact.
	malformed := binding
	malformed.GitCommitHash = "not-a-git-hash"
	_, err := promptlayer.LoadOMPContextEvidenceStoreV1(root, malformed, evidenceSubject(), evidenceStorePolicy(), now)
	require.ErrorContains(t, err, "binding mismatch")

	shifted := binding
	shifted.CheckedAt = checkedAt.Add(time.Second)
	_, err = promptlayer.LoadOMPContextEvidenceStoreV1(root, shifted, evidenceSubject(), evidenceStorePolicy(), now)
	require.ErrorContains(t, err, "binding mismatch")

	// PolicyDigest is always re-derived from the expected policy, so a caller-supplied
	// digest neither helps nor hurts a matching binding.
	forged := binding
	forged.PolicyDigest = "sha256:" + strings.Repeat("0", 64)
	_, err = promptlayer.LoadOMPContextEvidenceStoreV1(root, forged, evidenceSubject(), evidenceStorePolicy(), now)
	require.NoError(t, err)
}

func TestOMPContextEvidenceVerify_EnforcesFreshnessWindowBoundaries(t *testing.T) {
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root, binding := writeEvidenceFixture(t, checkedAt)
	load := func(now time.Time) error {
		_, err := promptlayer.LoadOMPContextEvidenceStoreV1(
			root, binding, evidenceSubject(), evidenceStorePolicy(), now,
		)
		return err
	}

	require.NoError(t, load(checkedAt.Add(-4*time.Minute))) // inside future skew
	require.ErrorContains(t, load(checkedAt.Add(-6*time.Minute)), "future-dated")
	require.NoError(t, load(checkedAt.Add(24*time.Hour-time.Second)))    // last instant of validity
	require.ErrorContains(t, load(checkedAt.Add(24*time.Hour)), "stale") // expiry is exclusive
}

func TestOMPContextEvidenceVerify_RejectsUnsafeEvidenceFile(t *testing.T) {
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	now := checkedAt.Add(time.Minute)

	loosened, binding := writeEvidenceFixture(t, checkedAt)
	path := promptlayer.OMPContextEvidenceStorePath(loosened)
	require.NoError(t, os.Chmod(path, 0o644))
	_, err := promptlayer.LoadOMPContextEvidenceStoreV1(loosened, binding, evidenceSubject(), evidenceStorePolicy(), now)
	require.ErrorContains(t, err, "file is unsafe")

	emptied, binding2 := writeEvidenceFixture(t, checkedAt)
	require.NoError(t, os.WriteFile(promptlayer.OMPContextEvidenceStorePath(emptied), nil, 0o600))
	_, err = promptlayer.LoadOMPContextEvidenceStoreV1(emptied, binding2, evidenceSubject(), evidenceStorePolicy(), now)
	require.ErrorContains(t, err, "size is invalid")

	missing := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Dir(promptlayer.OMPContextEvidenceStorePath(missing)), 0o700))
	_, err = promptlayer.LoadOMPContextEvidenceStoreV1(missing, binding, evidenceSubject(), evidenceStorePolicy(), now)
	require.ErrorContains(t, err, "file is unsafe")
}
