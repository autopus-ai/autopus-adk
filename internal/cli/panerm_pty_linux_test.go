//go:build linux

package cli_test

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// openPanermPTY opens a pseudo-terminal pair through /dev/ptmx so the S6
// oracle can attach a child's stderr to a terminal without a pty dependency.
func openPanermPTY() (master, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := int(master.Fd())
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		_ = master.Close()
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}
	index, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		_ = master.Close()
		return nil, nil, fmt.Errorf("pty number: %w", err)
	}
	slave, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", index), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
