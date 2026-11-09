//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// runCanary execs argv as the verified one-shot canary account; runNobody
// execs it as nobody. The helper drops root before exec, so argv never runs
// with more than that identity, whoever chose it.
func (r *run) runCanary(a canaryAccount, argv []string) error {
	if err := r.checkUntainted(); err != nil {
		return err
	}
	if err := verifyOwned(a); err != nil {
		return err
	}
	return dropAndExec(a.uid, canaryGID, argv, "PATH=/usr/bin:/bin:/usr/sbin:/sbin")
}

// runNobody runs the candidate installed root-owned in this root as nobody,
// with arguments only: the helper sets the environment itself and execs the
// candidate directly. nobody owns the private directories after the canary,
// so a free-form command or environment would give the operator another
// writer on that tree.
func (r *run) runNobody(args []string) error {
	if err := r.checkUntainted(); err != nil {
		return err
	}
	candidate := filepath.Join(r.root, "auto")
	if st, err := lstat(candidate); err != nil || st.Uid != 0 || st.Mode&0o7777 != 0o555 {
		return errors.New("the candidate is not the root-owned 0555 file install-exec made")
	}
	uid, gid, err := nobodyIdentity()
	if err != nil {
		return err
	}
	return dropAndExec(uid, gid, append([]string{candidate}, args...),
		"HOME="+r.home, "TMPDIR="+r.tmp, "PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C")
}

func dropAndExec(uid, gid int, argv []string, env ...string) error {
	if len(argv) == 0 || !filepath.IsAbs(argv[0]) {
		return errors.New("the program to run must be an absolute path")
	}
	if uid == 0 || gid == 0 {
		return errors.New("refusing to run as root")
	}
	if err := dropTo(uid, gid); err != nil {
		return err
	}
	// The caller's working directory is usually unreadable to the new
	// identity; start from / instead of inheriting it.
	if err := unix.Chdir("/"); err != nil {
		return err
	}
	syscall.Umask(0o077)
	return syscall.Exec(argv[0], argv, env)
}

// dropTo leaves root for good: supplementary groups, then group, then user,
// and verifies nothing of root is left. The supplementary list is set to
// exactly the primary group: on macOS an empty list lets the kernel resolve
// group membership dynamically again (nobody would regain everyone,
// localaccounts, and more), while a non-empty list opts the process out.
func dropTo(uid, gid int) error {
	if err := unix.Setgroups([]int{gid}); err != nil {
		return errors.New("set supplementary groups")
	}
	if err := unix.Setgid(gid); err != nil {
		return errors.New("set group")
	}
	if err := unix.Setuid(uid); err != nil {
		return errors.New("set user")
	}
	groups, err := unix.Getgroups()
	if err != nil || len(groups) != 1 || !sameID(groups[0], gid) ||
		!sameID(os.Getuid(), uid) || !sameID(os.Geteuid(), uid) || !sameID(os.Getgid(), gid) || !sameID(os.Getegid(), gid) {
		return fmt.Errorf("identity verification failed after dropping root: uid=%d euid=%d gid=%d egid=%d groups=%v err=%v",
			os.Getuid(), os.Geteuid(), os.Getgid(), os.Getegid(), groups, err)
	}
	if unix.Setuid(0) == nil {
		return errors.New("root could be regained after dropping it")
	}
	return nil
}

// sameID compares two IDs as the kernel's 32-bit uid_t/gid_t. Go reports
// nobody's -2 as -2 from os.Getuid but as 4294967294 from the account
// database and getgroups, so a plain int comparison calls them different.
func sameID(a, b int) bool { return uint32(a) == uint32(b) }
