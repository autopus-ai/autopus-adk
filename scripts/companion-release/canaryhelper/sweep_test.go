//go:build darwin

package main

import (
	"testing"

	"golang.org/x/sys/unix"
)

// parseSweepTarget accepts only accounts whose name, UID, group, shell, and
// home all derive from one nonce prefix, so a real account is never swept;
// a changed marker alone does not hide one of ours.
func TestParseSweepTarget(t *testing.T) {
	r, _ := newRun("0000c351ffffffffffffffffffffffff")
	a, _ := r.account("1")
	ok := func(name, marker, uid, gid, shell, home string) bool {
		_, got := parseSweepTarget(name, marker, uid, gid, shell, home)
		return got
	}
	if !ok(a.name, a.marker, "50001", "20", "/usr/bin/false", a.home) {
		t.Fatal("a complete account of ours was not recognized")
	}
	if !ok(a.name, "changed by the canary", "50001", "20", "/usr/bin/false", a.home) {
		t.Fatal("a changed marker hid one of our accounts")
	}
	if !ok(a.name, a.marker, "", "", "", "") {
		t.Fatal("a record interrupted after the marker was not recognized")
	}
	for name, args := range map[string][6]string{
		"real account":       {"bitgapnam", a.marker, "501", "20", "/bin/zsh", "/Users/bitgapnam"},
		"wrong uid":          {a.name, a.marker, "501", "20", "/usr/bin/false", a.home},
		"login shell":        {a.name, a.marker, "50001", "20", "/bin/zsh", a.home},
		"other home":         {a.name, a.marker, "50001", "20", "/usr/bin/false", "/Users/x"},
		"other group":        {a.name, a.marker, "50001", "0", "/usr/bin/false", a.home},
		"marker other nonce": {a.name, "Autopus Release Canary 1111c351ffffffffffffffffffffffff attempt 01 uid 50001", "50001", "20", "/usr/bin/false", a.home},
		"no uid, no marker":  {a.name, "", "", "", "", ""},
		"attempt 00":         {"_autopus_v110_0000c351_00", "", "50000", "20", "/usr/bin/false", a.home},
	} {
		if ok(args[0], args[1], args[2], args[3], args[4], args[5]) {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestCanaryAccountCount(t *testing.T) {
	names := []string{"root", "_autopus_v110_0000c351_01", "_autopus_v110_0000c351_02x", "_autopus_v110_abcdef01_64"}
	if n := canaryAccountCount(names); n != 2 {
		t.Fatalf("want 2 one-shot accounts, got %d", n)
	}
}

func TestRuleIsOurs(t *testing.T) {
	root := unix.Stat_t{Mode: unix.S_IFREG | 0o440, Uid: 0}
	rule := "bitgapnam ALL=(root) NOPASSWD: " + installedHelper + "\n"
	if !ruleIsOurs(root, rule, "bitgapnam") {
		t.Fatal("the installer's rule was not recognized")
	}
	userOwned := root
	userOwned.Uid = 501
	link := root
	link.Mode = unix.S_IFLNK | 0o777
	for name, c := range map[string]struct {
		st      unix.Stat_t
		content string
		account string
	}{
		"other account": {root, rule, "someone"},
		"extra line":    {root, rule + "someone ALL=(ALL) ALL\n", "bitgapnam"},
		"other command": {root, "bitgapnam ALL=(root) NOPASSWD: /bin/sh\n", "bitgapnam"},
		"not root's":    {userOwned, rule, "bitgapnam"},
		"a link":        {link, rule, "bitgapnam"},
	} {
		if ruleIsOurs(c.st, c.content, c.account) {
			t.Errorf("%s was accepted", name)
		}
	}
}
