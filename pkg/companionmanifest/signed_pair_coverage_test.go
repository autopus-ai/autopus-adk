package companionmanifest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func faultAtStep(step string) func(string) error {
	return func(current string) error {
		if current == step {
			return errors.New("injected fault at " + current)
		}
		return nil
	}
}

// TestWriteSignedFiles_FaultBeforePublication_KeepsExistingPair verifies every
// pre-publication abort point rolls the pair back to its previous contents and
// leaves no transaction residue, so a failed write never splits the pair.
func TestWriteSignedFiles_FaultBeforePublication_KeepsExistingPair(t *testing.T) {
	for _, step := range []string{"staged", "backed_up", "manifest_published"} {
		t.Run(step, func(t *testing.T) {
			dir := t.TempDir()
			manifestPath := filepath.Join(dir, "manifest.json")
			signaturePath := filepath.Join(dir, "manifest.sig")
			writeSignedPairFixture(t, manifestPath, signaturePath, "old-manifest", "old-signature")

			err := writeSignedFilesWithFault(
				manifestPath,
				signaturePath,
				[]byte("new-manifest"),
				[]byte("new-signature"),
				faultAtStep(step),
			)

			if err == nil {
				t.Fatal("writeSignedFilesWithFault() error = nil, want fault")
			}
			if !strings.Contains(err.Error(), "signed pair transaction fault") {
				t.Fatalf("writeSignedFilesWithFault() error = %v, want fault reason", err)
			}
			assertSignedPair(t, manifestPath, signaturePath, "old-manifest", "old-signature")
			assertNoSignedPairTransactions(t, dir)
		})
	}
}

// TestWriteSignedFiles_FaultAfterSignaturePublication_RollsBackBothOutputs
// verifies that a fault raised once both files are on disk still restores the
// previous pair instead of leaving the new manifest paired with the old
// signature.
func TestWriteSignedFiles_FaultAfterSignaturePublication_RollsBackBothOutputs(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	signaturePath := filepath.Join(dir, "manifest.sig")
	writeSignedPairFixture(t, manifestPath, signaturePath, "old-manifest", "old-signature")

	err := writeSignedFilesWithFault(
		manifestPath,
		signaturePath,
		[]byte("new-manifest"),
		[]byte("new-signature"),
		faultAtStep("signature_published"),
	)

	if err == nil {
		t.Fatal("writeSignedFilesWithFault() error = nil, want fault")
	}
	assertSignedPair(t, manifestPath, signaturePath, "old-manifest", "old-signature")
	assertNoSignedPairTransactions(t, dir)
}

// TestWriteSignedFiles_FaultAfterCommit_KeepsNewPair verifies the commit marker
// is the point of no return: faults raised after it must surface an error while
// the new pair stays published and the transaction directory is reclaimed.
func TestWriteSignedFiles_FaultAfterCommit_KeepsNewPair(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	signaturePath := filepath.Join(dir, "manifest.sig")
	writeSignedPairFixture(t, manifestPath, signaturePath, "old-manifest", "old-signature")

	err := writeSignedFilesWithFault(
		manifestPath,
		signaturePath,
		[]byte("new-manifest"),
		[]byte("new-signature"),
		faultAtStep("pair_committed"),
	)

	if err == nil {
		t.Fatal("writeSignedFilesWithFault() error = nil, want fault")
	}
	assertSignedPair(t, manifestPath, signaturePath, "new-manifest", "new-signature")
	assertNoSignedPairTransactions(t, dir)
}

// TestWriteSignedFiles_FaultAfterDetach_LeavesRecoverableCleanup verifies a
// fault between detach and removal keeps the committed pair and leaves only
// cleanup residue that the next recovery pass reclaims.
func TestWriteSignedFiles_FaultAfterDetach_LeavesRecoverableCleanup(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	signaturePath := filepath.Join(dir, "manifest.sig")
	writeSignedPairFixture(t, manifestPath, signaturePath, "old-manifest", "old-signature")

	err := writeSignedFilesWithFault(
		manifestPath,
		signaturePath,
		[]byte("new-manifest"),
		[]byte("new-signature"),
		faultAtStep("transaction_detached"),
	)

	if err == nil {
		t.Fatal("writeSignedFilesWithFault() error = nil, want fault")
	}
	assertSignedPair(t, manifestPath, signaturePath, "new-manifest", "new-signature")
	if err := recoverSignedFileTransactions(dir); err != nil {
		t.Fatalf("recoverSignedFileTransactions() error = %v", err)
	}
	assertSignedPair(t, manifestPath, signaturePath, "new-manifest", "new-signature")
	assertNoSignedPairTransactions(t, dir)
}

// TestWriteSignedFiles_RejectsUnusableOutputLocations verifies the writer
// refuses targets it cannot own atomically instead of partially writing.
func TestWriteSignedFiles_RejectsUnusableOutputLocations(t *testing.T) {
	t.Run("missing parent directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "absent")

		err := writeSignedFilesWithFault(
			filepath.Join(dir, "manifest.json"),
			filepath.Join(dir, "manifest.sig"),
			[]byte("m"),
			[]byte("s"),
			nil,
		)

		if err == nil ||
			!strings.Contains(err.Error(), "signed output directory must be a regular directory") {
			t.Fatalf("writeSignedFilesWithFault() error = %v, want directory rejection", err)
		}
	})

	t.Run("output path occupied by directory", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "manifest.json")
		if err := os.Mkdir(manifestPath, 0o700); err != nil {
			t.Fatal(err)
		}

		err := writeSignedFilesWithFault(
			manifestPath,
			filepath.Join(dir, "manifest.sig"),
			[]byte("m"),
			[]byte("s"),
			nil,
		)

		if err == nil || !strings.Contains(err.Error(), "signed output must be a regular file") {
			t.Fatalf("writeSignedFilesWithFault() error = %v, want regular-file rejection", err)
		}
		assertNoSignedPairTransactions(t, dir)
	})

	t.Run("outputs reserved for transaction state", func(t *testing.T) {
		dir := t.TempDir()

		err := writeSignedFilesWithFault(
			filepath.Join(dir, signedPairTransactionPrefix+"manifest.json"),
			filepath.Join(dir, "manifest.sig"),
			[]byte("m"),
			[]byte("s"),
			nil,
		)

		if err == nil ||
			!strings.Contains(err.Error(), "signed outputs must be distinct files in one directory") {
			t.Fatalf("writeSignedFilesWithFault() error = %v, want reserved-name rejection", err)
		}
	})
}
