//go:build darwin

package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// FuzzExtractNeverFollowsALink builds a small archive from fuzzed names,
// link targets, and entry kinds, including links that point at an absolute
// directory outside the destination. Whatever extract accepts, nothing may
// appear in that outside directory or next to the destination, and no
// set-id bit may survive.
func FuzzExtractNeverFollowsALink(f *testing.F) {
	f.Add("l", "OUT", byte(2), "l/x", "", byte(0), "d", "", byte(1))
	f.Add("../x", "", byte(0), "a", "../OUT", byte(2), "a/y", "", byte(0))
	f.Add("d", "", byte(1), "d/s", "/", byte(2), "d/s/z", "", byte(0))
	f.Fuzz(func(t *testing.T, n1, l1 string, k1 byte, n2, l2 string, k2 byte, n3, l3 string, k3 byte) {
		base := t.TempDir()
		dest := filepath.Join(base, "dest")
		outside := filepath.Join(base, "outside")
		for _, dir := range []string{dest, outside} {
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		for _, e := range []struct {
			name, link string
			kind       byte
		}{{n1, l1, k1}, {n2, l2, k2}, {n3, l3, k3}} {
			link := strings.ReplaceAll(e.link, "OUT", outside)
			h := &tar.Header{Name: e.name, Mode: 0o6777}
			switch e.kind % 3 {
			case 0:
				h.Typeflag, h.Size = tar.TypeReg, 1
			case 1:
				h.Typeflag = tar.TypeDir
			default:
				h.Typeflag, h.Linkname = tar.TypeSymlink, link
			}
			if tw.WriteHeader(h) != nil {
				return
			}
			if h.Size > 0 {
				_, _ = tw.Write([]byte("x"))
			}
		}
		_ = tw.Close()
		_ = extract(tar.NewReader(&buf), dest)
		if entries, _ := os.ReadDir(base); len(entries) != 2 {
			t.Fatalf("extract wrote next to the destination: %v", entries)
		}
		if inside, _ := os.ReadDir(outside); len(inside) != 0 {
			t.Fatalf("extract wrote through a link into %s: %v", outside, inside)
		}
		_ = filepath.WalkDir(dest, func(path string, d os.DirEntry, err error) error {
			if info, ierr := os.Lstat(path); err == nil && ierr == nil && info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
				t.Fatalf("extract kept a set-id bit on %s", path)
			}
			return nil
		})
	})
}

// FuzzAccountDerivation checks that no nonce or attempt string derives an
// account outside the one-shot range, or anything at all from a malformed
// input.
func FuzzAccountDerivation(f *testing.F) {
	f.Add("0000c351ffffffffffffffffffffffff", "1")
	f.Add("ffffffffffffffffffffffffffffffff", "64")
	f.Add("../../etc", "01")
	f.Fuzz(func(t *testing.T, nonce, attempt string) {
		r, err := newRun(nonce)
		if err != nil {
			return
		}
		a, err := r.account(attempt)
		if err != nil {
			return
		}
		if a.uid < canaryUIDMin || a.uid > canaryUIDMax || !accountName.MatchString(a.name) ||
			!accountMarker.MatchString(a.marker) || filepath.Dir(r.root) != "/private/tmp" {
			t.Fatalf("derived an unsafe account %+v for %q %q", a, nonce, attempt)
		}
		// sweep must recognize exactly what account-create made.
		target, ok := parseSweepTarget(a.name, a.marker, strconv.Itoa(a.uid), "20", "/usr/bin/false", a.home)
		if !ok || target.uid != a.uid {
			t.Fatalf("sweep does not recognize the account it should: %+v", a)
		}
	})
}
