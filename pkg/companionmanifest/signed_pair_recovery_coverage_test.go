package companionmanifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRecoveryTransaction materializes a crash-residue transaction directory
// with the given entries so recovery can be exercised without crashing a
// helper process.
func writeRecoveryTransaction(
	t *testing.T,
	parent, name string,
	perm os.FileMode,
	files map[string]string,
) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(path, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func canonicalRecoveryState(t *testing.T, manifestExisted, signatureExisted bool) string {
	t.Helper()
	existed := func(value bool) string {
		if value {
			return "true"
		}
		return "false"
	}
	return `{"schema_version":"` + signedPairStateSchema +
		`","manifest_name":"manifest.json","signature_name":"manifest.sig"` +
		`,"manifest_existed":` + existed(manifestExisted) +
		`,"signature_existed":` + existed(signatureExisted) + `}`
}

// TestRecoverSignedFileTransactions_RejectsMalformedResidue guards the
// fail-closed recovery gate: a residue directory whose entries, markers, or
// state file do not match the transaction schema must abort recovery with a
// reason instead of silently deleting or publishing half-written outputs.
func TestRecoverSignedFileTransactions_RejectsMalformedResidue(t *testing.T) {
	valid := canonicalRecoveryState(t, true, true)

	tests := []struct {
		name    string
		perm    os.FileMode
		files   map[string]string
		wantErr string
	}{
		{
			name:    "unexpected entry",
			perm:    0o700,
			files:   map[string]string{signedPairLockName: "", "stray.txt": "x"},
			wantErr: "unexpected signed pair transaction entry",
		},
		{
			name:    "loose transaction directory permissions",
			perm:    0o755,
			files:   map[string]string{signedPairLockName: ""},
			wantErr: "invalid signed pair recovery transaction",
		},
		{
			name: "unprepared transaction holding published backup",
			perm: 0o700,
			files: map[string]string{
				signedPairLockName:    "",
				signedPairStateName:   valid,
				signedPairManifestOld: "old-manifest",
			},
			wantErr: "unprepared signed pair transaction contains published state",
		},
		{
			name: "corrupt prepared marker",
			perm: 0o700,
			files: map[string]string{
				signedPairLockName:     "",
				signedPairStateName:    valid,
				signedPairPreparedName: "tampered\n",
			},
			wantErr: "invalid signed pair transaction marker",
		},
		{
			name: "state with unknown field",
			perm: 0o700,
			files: map[string]string{
				signedPairLockName:     "",
				signedPairPreparedName: "prepared\n",
				signedPairStateName:    `{"schema_version":"x","extra":1}`,
			},
			wantErr: "validate signed pair recovery transaction",
		},
		{
			name: "state with trailing data",
			perm: 0o700,
			files: map[string]string{
				signedPairLockName:     "",
				signedPairPreparedName: "prepared\n",
				signedPairStateName:    valid + "\n{}",
			},
			wantErr: "validate signed pair recovery transaction",
		},
		{
			name: "non-canonical state encoding",
			perm: 0o700,
			files: map[string]string{
				signedPairLockName:     "",
				signedPairPreparedName: "prepared\n",
				signedPairStateName: `{"manifest_name":"manifest.json","schema_version":"` +
					signedPairStateSchema + `","signature_name":"manifest.sig",` +
					`"manifest_existed":true,"signature_existed":true}`,
			},
			wantErr: "validate signed pair recovery transaction",
		},
		{
			name: "committed pair missing published outputs",
			perm: 0o700,
			files: map[string]string{
				signedPairLockName:     "",
				signedPairPreparedName: "prepared\n",
				signedPairCommitName:   "committed\n",
				signedPairStateName:    valid,
			},
			wantErr: "committed signed pair is incomplete during recovery",
		},
		{
			name: "rollback without backup or original",
			perm: 0o700,
			files: map[string]string{
				signedPairLockName:     "",
				signedPairPreparedName: "prepared\n",
				signedPairStateName:    valid,
			},
			wantErr: "signed pair backup and original are both absent",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRecoveryTransaction(
				t,
				dir,
				signedPairTransactionPrefix+"residue",
				test.perm,
				test.files,
			)

			err := recoverSignedFileTransactions(dir)

			if err == nil {
				t.Fatalf("recoverSignedFileTransactions() error = nil, want %q", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("recoverSignedFileTransactions() error = %v, want %q", err, test.wantErr)
			}
			if _, statErr := os.Stat(
				filepath.Join(dir, signedPairTransactionPrefix+"residue"),
			); statErr != nil {
				t.Fatalf("rejected residue was removed: %v", statErr)
			}
		})
	}
}

// TestRecoverSignedFileTransactions_UnpreparedResidueIsDiscarded verifies the
// staged-but-never-prepared crash window is reclaimed and the pre-existing
// pair is left untouched.
func TestRecoverSignedFileTransactions_UnpreparedResidueIsDiscarded(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	signaturePath := filepath.Join(dir, "manifest.sig")
	writeSignedPairFixture(t, manifestPath, signaturePath, "old-manifest", "old-signature")
	writeRecoveryTransaction(t, dir, signedPairTransactionPrefix+"staged", 0o700, map[string]string{
		signedPairLockName:     "",
		signedPairStateName:    canonicalRecoveryState(t, true, true),
		signedPairManifestNew:  "new-manifest",
		signedPairSignatureNew: "new-signature",
	})

	if err := recoverSignedFileTransactions(dir); err != nil {
		t.Fatalf("recoverSignedFileTransactions() error = %v", err)
	}

	assertSignedPair(t, manifestPath, signaturePath, "old-manifest", "old-signature")
	assertNoSignedPairTransactions(t, dir)
}

// TestRecoverSignedFileTransactions_RollsBackPreparedResidue verifies a
// prepared-but-uncommitted transaction restores the backed-up pair rather
// than leaving the half-published new manifest in place.
func TestRecoverSignedFileTransactions_RollsBackPreparedResidue(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	signaturePath := filepath.Join(dir, "manifest.sig")
	writeSignedPairFixture(t, manifestPath, signaturePath, "new-manifest", "old-signature")
	writeRecoveryTransaction(t, dir, signedPairTransactionPrefix+"rollback", 0o700, map[string]string{
		signedPairLockName:     "",
		signedPairPreparedName: "prepared\n",
		signedPairStateName:    canonicalRecoveryState(t, true, true),
		signedPairManifestOld:  "old-manifest",
		signedPairSignatureOld: "old-signature",
	})

	if err := recoverSignedFileTransactions(dir); err != nil {
		t.Fatalf("recoverSignedFileTransactions() error = %v", err)
	}

	assertSignedPair(t, manifestPath, signaturePath, "old-manifest", "old-signature")
	assertNoSignedPairTransactions(t, dir)
}

// TestRecoverSignedPairCleanup_RemovesOrRejectsResidue verifies detached
// cleanup directories are reclaimed only when their contents still match the
// transaction schema.
func TestRecoverSignedPairCleanup_RemovesOrRejectsResidue(t *testing.T) {
	t.Run("valid cleanup is removed", func(t *testing.T) {
		dir := t.TempDir()
		writeRecoveryTransaction(t, dir, signedPairCleanupPrefix+"done", 0o700, map[string]string{
			signedPairStateName:  canonicalRecoveryState(t, false, false),
			signedPairCommitName: "committed\n",
		})

		if err := recoverSignedFileTransactions(dir); err != nil {
			t.Fatalf("recoverSignedFileTransactions() error = %v", err)
		}
		assertNoSignedPairTransactions(t, dir)
	})

	t.Run("foreign entry blocks cleanup removal", func(t *testing.T) {
		dir := t.TempDir()
		name := signedPairCleanupPrefix + "dirty"
		writeRecoveryTransaction(t, dir, name, 0o700, map[string]string{"payload": "x"})

		err := recoverSignedFileTransactions(dir)

		if err == nil || !strings.Contains(err.Error(), "unexpected signed pair transaction entry") {
			t.Fatalf("recoverSignedFileTransactions() error = %v, want unexpected-entry rejection", err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
			t.Fatalf("rejected cleanup residue was removed: %v", statErr)
		}
	})
}

// TestRecoverSignedFileTransactions_RejectsWorldReadableEntry keeps recovery
// from trusting transaction files whose permissions were widened after the
// crash.
func TestRecoverSignedFileTransactions_RejectsWorldReadableEntry(t *testing.T) {
	dir := t.TempDir()
	name := signedPairTransactionPrefix + "widened"
	path := writeRecoveryTransaction(t, dir, name, 0o700, map[string]string{
		signedPairLockName:  "",
		signedPairStateName: canonicalRecoveryState(t, false, false),
	})
	if err := os.Chmod(filepath.Join(path, signedPairStateName), 0o644); err != nil {
		t.Fatal(err)
	}

	err := recoverSignedFileTransactions(dir)

	if err == nil || !strings.Contains(err.Error(), "invalid signed pair transaction entry") {
		t.Fatalf("recoverSignedFileTransactions() error = %v, want entry-permission rejection", err)
	}
}

// TestRecoverSignedFileTransactions_RejectsNonDirectoryParent keeps recovery
// from operating on a path that is not a plain directory.
func TestRecoverSignedFileTransactions_RejectsNonDirectoryParent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := recoverSignedFileTransactions(file)

	if err == nil || !strings.Contains(err.Error(), "signed output directory must be a regular directory") {
		t.Fatalf("recoverSignedFileTransactions() error = %v, want directory rejection", err)
	}
}
