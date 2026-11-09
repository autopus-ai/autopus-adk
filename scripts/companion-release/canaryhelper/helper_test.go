//go:build darwin

package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The helper must name the same account the shell library derives, or the
// shell would verify one account while the helper acts on another.
func TestAccountMatchesTheShellDerivation(t *testing.T) {
	r, err := newRun("0000c351ffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.account("1")
	if err != nil {
		t.Fatal(err)
	}
	// seed 0xc351 = 50001; 50000 + (50001 + 0) % 10000 = 50001
	if a.uid != 50001 || a.name != "_autopus_v110_0000c351_01" ||
		a.marker != "Autopus Release Canary 0000c351ffffffffffffffffffffffff attempt 01 uid 50001" ||
		a.home != "/private/tmp/autopus-adk-release-prep-0000c351ffffffffffffffffffffffff-final/home" {
		t.Fatalf("unexpected account %+v", a)
	}
	for _, bad := range []string{"0", "65", "01", "-1", "x"} {
		if _, err := r.account(bad); err == nil {
			t.Fatalf("attempt %q was accepted", bad)
		}
	}
	for _, bad := range []string{"", "../etc", "0000C351FFFFFFFFFFFFFFFFFFFFFFFF", "abc"} {
		if _, err := newRun(bad); err == nil {
			t.Fatalf("nonce %q was accepted", bad)
		}
	}
}

func tarOf(t *testing.T, entries ...tar.Header) *tar.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range entries {
		h := h
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write(bytes.Repeat([]byte("x"), int(h.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return tar.NewReader(&buf)
}

// An archived symlink cannot redirect a later entry outside the destination,
// and no name may climb out of it.
func TestExtractRefusesEscapes(t *testing.T) {
	outside := t.TempDir()
	for name, entries := range map[string][]tar.Header{
		"dot-dot":       {{Typeflag: tar.TypeReg, Name: "../escape", Size: 1, Mode: 0o644}},
		"absolute":      {{Typeflag: tar.TypeReg, Name: "/etc/escape", Size: 1, Mode: 0o644}},
		"through-link":  {{Typeflag: tar.TypeSymlink, Name: "a", Linkname: outside}, {Typeflag: tar.TypeReg, Name: "a/escape", Size: 1, Mode: 0o644}},
		"dir-over-link": {{Typeflag: tar.TypeSymlink, Name: "b", Linkname: outside}, {Typeflag: tar.TypeDir, Name: "b/", Mode: 0o755}},
		"hard-link":     {{Typeflag: tar.TypeLink, Name: "c", Linkname: "/etc/hosts"}},
	} {
		dest := t.TempDir()
		if err := extract(tarOf(t, entries...), dest); err == nil {
			t.Errorf("%s: extraction was accepted", name)
		}
		if _, err := os.Lstat(filepath.Join(outside, "escape")); err == nil {
			t.Fatalf("%s: a file was written outside the destination", name)
		}
	}
}

// From an untrusted tree a hard-linked file is left out, so root never reads
// a file linked in from elsewhere; regular files, dirs, and links copy over.
func TestPackSkipsHardLinksFromAnUntrustedTree(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "d", "plain.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(secret, filepath.Join(src, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("d/plain.txt", filepath.Join(src, "alias")); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	skipped, err := pack(tw, src, os.Getuid())
	if err != nil || tw.Close() != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Fatalf("want the hard link skipped, skipped=%d", skipped)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := extract(tar.NewReader(&buf), dest); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "d", "plain.txt")); err != nil || string(data) != "ok" {
		t.Fatalf("plain file did not copy: %q %v", data, err)
	}
	if target, err := os.Readlink(filepath.Join(dest, "alias")); err != nil || target != "d/plain.txt" {
		t.Fatalf("symlink did not copy as a link: %q %v", target, err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "linked")); err == nil {
		t.Fatal("the hard-linked file was copied")
	}
}

func TestChownTreeSkipsHardLinks(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(other, filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	skipped, err := chownTree(dir, os.Getuid(), os.Getuid(), os.Getgid())
	if err != nil || skipped != 1 {
		t.Fatalf("want one skipped hard link, skipped=%d err=%v", skipped, err)
	}
}

func TestPrivilegedCommandsRefuseWithoutSudo(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root")
	}
	if err := dispatch([]string{"prepare-root", "0000c351ffffffffffffffffffffffff"}); err == nil {
		t.Fatal("a privileged command ran without root")
	}
}

// A file owned by someone other than the tree's expected owner is never
// read or handed over, even with a single link.
func TestUntrustedTreeRequiresTheExpectedOwner(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "mine"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	skipped, err := pack(tw, src, os.Getuid()+1)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Fatalf("want the foreign-owned file skipped, skipped=%d", skipped)
	}
	if _, err := chownTree(src, os.Getuid()+1, os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("a tree not owned by the expected identity was handed over")
	}
}

// chownTree clears setuid and setgid while it hands a file over.
func TestChownTreeClearsSetIDBits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755|os.ModeSetuid|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	if _, err := chownTree(dir, os.Getuid(), os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("set-id bits survived: %v", info.Mode())
	}
}

// nobody's ID arrives as -2 from one call and 4294967294 from another.
func TestSameIDComparesAsUint32(t *testing.T) {
	if !sameID(-2, 4294967294) || !sameID(50001, 50001) || sameID(-2, 4294967293) {
		t.Fatal("IDs must compare as 32-bit unsigned values")
	}
}
