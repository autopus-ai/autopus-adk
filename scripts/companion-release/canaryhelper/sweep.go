//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// maxCanaryAccounts bounds how many one-shot accounts may exist at once. A
// release prep holds one at a time, so more means leftovers or someone
// calling account-create in a loop.
const maxCanaryAccounts = 2

// staleAfter is how old a root must be before sweep treats it, and the
// account named for its nonce, as left behind. A release prep finishes well
// within it.
const staleAfter = 6 * time.Hour

var (
	accountName   = regexp.MustCompile(`^_autopus_v110_([0-9a-f]{8})_([0-9]{2})$`)
	accountMarker = regexp.MustCompile(`^Autopus Release Canary ([0-9a-f]{32}) attempt ([0-9]{2}) uid ([0-9]+)$`)
	rootName      = regexp.MustCompile(`^autopus-adk-release-prep-([0-9a-f]{32})-final$`)
)

func canaryAccountCount(names []string) int {
	n := 0
	for _, name := range names {
		if accountName.MatchString(name) {
			n++
		}
	}
	return n
}

// sweepTarget is a one-shot account sweep may remove. Only the helper, as
// root, can create an account, and the name carries the nonce prefix and
// attempt the UID derives from, so name and UID together identify one of ours
// even if its marker was changed. The marker, when present, must name the
// same nonce prefix and attempt.
type sweepTarget struct {
	name     string
	uid      int // -1 when the record never got a UniqueID
	rootGlob string
}

// parseSweepTarget decides from an account's attributes whether it is a
// one-shot account this helper made. It is pure, so it is tested directly.
func parseSweepTarget(name, marker, uniqueID, gid, shell, home string) (sweepTarget, bool) {
	m := accountName.FindStringSubmatch(name)
	if m == nil {
		return sweepTarget{}, false
	}
	seed, err := strconv.ParseUint(m[1], 16, 32)
	if err != nil {
		return sweepTarget{}, false
	}
	attempt, _ := strconv.Atoi(m[2])
	if attempt < 1 || attempt > canaryUIDAttempts {
		return sweepTarget{}, false
	}
	uid := canaryUIDMin + int((seed+uint64(attempt)-1)%uint64(canaryUIDMax-canaryUIDMin+1))
	if mm := accountMarker.FindStringSubmatch(marker); mm != nil && (mm[1][:8] != m[1] || mm[2] != m[2]) {
		return sweepTarget{}, false
	}
	rootGlob := "/private/tmp/autopus-adk-release-prep-" + m[1] + "*-final"
	if uniqueID == "" {
		// Interrupted right after the marker: nothing else was set yet.
		if accountMarker.MatchString(marker) {
			return sweepTarget{name: name, uid: -1, rootGlob: rootGlob}, true
		}
		return sweepTarget{}, false
	}
	homeOK, _ := filepath.Match("/private/tmp/autopus-adk-release-prep-"+m[1]+"*-final/home", home)
	if uniqueID != strconv.Itoa(uid) || gid != strconv.Itoa(canaryGID) || shell != "/usr/bin/false" || !homeOK {
		return sweepTarget{}, false
	}
	return sweepTarget{name: name, uid: uid, rootGlob: rootGlob}, true
}

// sweep removes one-shot accounts and isolation roots a release prep left
// behind. A root younger than staleAfter, or one another helper call holds
// locked, is a run in progress and is left alone, and so is its account.
func sweep() error {
	if err := checkPrivateTmp(); err != nil {
		return err
	}
	unlock, err := globalLock()
	if err != nil {
		return err
	}
	defer unlock()
	names, err := accountNames()
	if err != nil {
		return err
	}
	var problems []error
	for _, name := range names {
		if err := sweepAccount(name); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", name, err))
		}
	}
	roots, _ := filepath.Glob("/private/tmp/autopus-adk-release-prep-*-final")
	for _, root := range roots {
		if err := sweepRoot(root); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", root, err))
		}
	}
	return errors.Join(problems...)
}

func sweepAccount(name string) error {
	if !accountName.MatchString(name) {
		return nil
	}
	value := func(attr string) string { v, _ := accountValue(name, attr); return v }
	t, ok := parseSweepTarget(name, value("RealName"), value("UniqueID"), value("PrimaryGroupID"),
		value("UserShell"), value("NFSHomeDirectory"))
	if !ok || !rootsStale(t.rootGlob) {
		return nil
	}
	if t.uid >= 0 {
		// Never signal a UID another record also uses.
		inventory, err := accountInventory()
		if err != nil {
			return err
		}
		for other, uid := range inventory {
			if uid == t.uid && other != t.name {
				return errors.New("another account shares its UID; left alone")
			}
		}
		if err := killUID(t.uid, "KILL"); err != nil {
			return err
		}
		if free, err := uidProcessFree(t.uid); err != nil || !free {
			return errors.New("its processes did not stop")
		}
	}
	return runQuiet(dscl, ".", "-delete", "/Users/"+t.name)
}

// rootsStale reports that no root for the account's nonce prefix is a run in
// progress: each is older than staleAfter and not locked, or none exists.
func rootsStale(glob string) bool {
	roots, _ := filepath.Glob(glob)
	for _, root := range roots {
		m := rootName.FindStringSubmatch(filepath.Base(root))
		if m == nil {
			continue
		}
		r, err := newRun(m[1])
		if err != nil || r.checkRoot() != nil {
			continue // only a root this helper made counts as a run in progress
		}
		if !r.stale() || r.locked() {
			return false
		}
	}
	return true
}

func sweepRoot(root string) error {
	m := rootName.FindStringSubmatch(filepath.Base(root))
	if m == nil || filepath.Dir(root) != "/private/tmp" {
		return nil
	}
	r, err := newRun(m[1])
	if err != nil || r.checkRoot() != nil || !r.stale() {
		return nil
	}
	if fd, err := r.openLock(); err == nil {
		defer func() { _ = unix.Close(fd) }()
		if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
			return nil // a helper call is working on it
		}
	}
	return r.removeTree()
}

// stale reports a root older than staleAfter, or a nonce whose root is gone.
func (r *run) stale() bool {
	st, err := lstat(r.root)
	if errors.Is(err, unix.ENOENT) {
		return true
	}
	if err != nil {
		return false
	}
	return time.Since(time.Unix(st.Ctimespec.Sec, 0)) > staleAfter
}

// locked reports whether another helper call holds the root's lock.
func (r *run) locked() bool {
	fd, err := r.openLock()
	if err != nil {
		return false
	}
	defer func() { _ = unix.Close(fd) }()
	if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		return true
	}
	_ = unix.Flock(fd, unix.LOCK_UN)
	return false
}

// globalLock serializes account creation and sweep across every nonce, so
// the account limit holds under concurrent calls. The lock file is fixed,
// root-owned, and never followed through a link.
func globalLock() (func(), error) {
	fd, err := unix.Open("/var/run/autopus-release-canary.lock",
		unix.O_RDONLY|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the helper lock: %w", err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Uid != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = unix.Close(fd)
		return nil, errors.New("the helper lock is not a root-owned file")
	}
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return func() { _ = unix.Close(fd) }, nil
}

// clearFlags removes the flags a canary could set (immutable, append-only)
// from the tree under dirfd, through descriptors, so it cannot make its tree
// undeletable. Only entries owned by a one-shot canary UID or nobody, on the
// tree's device, are touched: their owner could clear those flags itself, so
// root clearing them grants nothing, and a file linked in from elsewhere with
// another owner keeps its flags.
func clearFlags(dirfd int, dev int32) {
	nobody, _, _ := nobodyIdentity()
	_ = eachEntry(dirfd, func(e entry) error {
		owner := int(e.stat.Uid)
		canaryOwned := (owner >= canaryUIDMin && owner <= canaryUIDMax) || sameID(owner, nobody)
		if e.fd < 0 || e.stat.Dev != dev || !canaryOwned {
			return nil
		}
		if e.stat.Flags != 0 {
			_ = unix.Fchflags(e.fd, 0)
		}
		if e.kind == unix.S_IFDIR {
			clearFlags(e.fd, dev)
		}
		return nil
	})
}

// removeTree clears flags and then removes the whole root. RemoveAll unlinks
// symlinks and hard links instead of following them.
func (r *run) removeTree() error {
	if fd, st, err := openRootDir(r.root); err == nil {
		clearFlags(fd, st.Dev)
		_ = unix.Close(fd)
	}
	if err := os.RemoveAll(r.root); err != nil {
		return fmt.Errorf("remove the isolation root: %w", err)
	}
	return nil
}
