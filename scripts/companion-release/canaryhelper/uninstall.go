//go:build darwin

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
)

const (
	installedHelper = "/Library/PrivilegedHelperTools/co.autopus.release-canary"
	sudoersRule     = "/etc/sudoers.d/autopus-release-canary"
)

// uninstall removes the sudoers rule and then the helper itself, so the
// passwordless root path exists only while a release needs it. It removes
// only the rule this helper's installer wrote: a root-owned regular file
// whose one line names this helper for the caller's account.
func uninstall(op operator) error {
	caller, err := user.LookupId(strconv.Itoa(op.uid))
	if err != nil {
		return errors.New("the calling account does not exist")
	}
	account := caller.Username
	fd, err := unix.Open(sudoersRule, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open the sudoers rule: %w", err)
	}
	f := os.NewFile(uintptr(fd), sudoersRule)
	var st unix.Stat_t
	statErr := unix.Fstat(fd, &st)
	var buf bytes.Buffer
	_, readErr := buf.ReadFrom(f)
	_ = f.Close()
	if statErr != nil || readErr != nil || !ruleIsOurs(st, buf.String(), account) {
		return errors.New("the sudoers rule is not the one install-canary-helper.sh wrote; remove it by hand")
	}
	if err := os.Remove(sudoersRule); err != nil {
		return fmt.Errorf("remove the sudoers rule: %w", err)
	}
	if err := os.Remove(installedHelper); err != nil {
		return fmt.Errorf("the rule is gone but the helper stayed: %w", err)
	}
	return nil
}

// ruleIsOurs reports whether a sudoers file is exactly the one-line rule the
// installer writes for account: a root-owned regular file and nothing more.
func ruleIsOurs(st unix.Stat_t, content, account string) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFREG && st.Uid == 0 &&
		content == account+" ALL=(root) NOPASSWD: "+installedHelper+"\n"
}
