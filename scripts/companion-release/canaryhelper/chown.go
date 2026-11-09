//go:build darwin

package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// chownCommand hands the three private directories from root to the verified
// canary account before launch, or from that account to nobody after it.
// There is no hand-back to root: a canary may have written a setuid file, and
// making root its owner must never be one command away.
func (r *run) chownCommand(rest []string) error {
	if err := r.checkUntainted(); err != nil {
		return err
	}
	if len(rest) != 2 || (rest[0] != "canary" && rest[0] != "nobody") {
		return errors.New(usage)
	}
	a, err := r.account(rest[1])
	if err != nil {
		return err
	}
	var from, uid, gid int
	if rest[0] == "canary" {
		if err := verifyOwned(a); err != nil {
			return err
		}
		from, uid, gid = 0, a.uid, canaryGID
	} else {
		// Every canary process must be gone: a live one could set a set-id
		// bit again between the clear and the hand-over.
		if free, err := uidProcessFree(a.uid); err != nil || !free {
			return errors.New("the canary UID still runs processes")
		}
		from = a.uid
		if uid, gid, err = nobodyIdentity(); err != nil {
			return err
		}
	}
	skipped := 0
	for _, dir := range r.privateDirs() {
		n, err := chownTree(dir, from, uid, gid)
		skipped += n
		if err != nil {
			return errors.Join(err, r.taint())
		}
	}
	if skipped > 0 {
		return errors.Join(fmt.Errorf("left %d file(s) unchanged that the tree may not own; this root is now refused until remove-root", skipped), r.taint())
	}
	return nil
}

// chownTree hands every directory and single-link regular file owned by from
// under dir to uid:gid, through descriptors (see fdwalk.go), and clears the
// setuid and setgid bits on the way. Anything else is skipped and counted.
// Symlinks keep their owner: changing it would act on a name.
func chownTree(dir string, from, uid, gid int) (int, error) {
	fd, st, err := openRootDir(dir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = unix.Close(fd) }()
	if int(st.Uid) != from {
		return 1, fmt.Errorf("%s is not owned by the expected identity", dir)
	}
	if err := handOver(fd, st, uid, gid); err != nil {
		return 0, err
	}
	return chownBelow(fd, st.Dev, uint32(from), uid, gid)
}

func chownBelow(dirfd int, dev int32, from uint32, uid, gid int) (int, error) {
	skipped := 0
	err := eachEntry(dirfd, func(e entry) error {
		switch {
		case e.kind == unix.S_IFLNK:
			return nil
		case !e.trusted(dev, from):
			skipped++
			return nil
		}
		reset, err := handOverChecked(e.fd, e.stat, uid, gid)
		if err != nil {
			return err
		}
		if reset {
			skipped++ // a set-id bit came back during the hand-over
		}
		if e.kind == unix.S_IFDIR {
			n, err := chownBelow(e.fd, dev, from, uid, gid)
			skipped += n
			return err
		}
		return nil
	})
	return skipped, err
}

// handOver clears setuid and setgid before the owner changes, so no moment
// exists where the new owner holds a set-id file.
func handOver(fd int, st unix.Stat_t, uid, gid int) error {
	if err := unix.Fchmod(fd, uint32(st.Mode)&0o777); err != nil {
		return err
	}
	return unix.Fchown(fd, uid, gid)
}

// handOverChecked hands over and then looks again: if a set-id bit was set
// back in the gap, it is cleared now that the old owner no longer owns the
// file, and reported so the caller taints the root.
func handOverChecked(fd int, st unix.Stat_t, uid, gid int) (bool, error) {
	if err := handOver(fd, st, uid, gid); err != nil {
		return false, err
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return false, err
	}
	if after.Mode&0o6000 == 0 {
		return false, nil
	}
	return true, unix.Fchmod(fd, uint32(after.Mode)&0o777)
}
