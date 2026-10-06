//go:build darwin

package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// openPanermPTY opens a pseudo-terminal pair through /dev/ptmx so the S6
// oracle can attach a child's stderr to a terminal without a pty dependency.
func openPanermPTY() (master, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := master.Fd()
	name := make([]byte, 128)
	for _, step := range []struct {
		req uintptr
		arg uintptr
	}{
		{syscall.TIOCPTYGRANT, 0},
		{syscall.TIOCPTYUNLK, 0},
		{syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))},
	} {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, step.req, step.arg); errno != 0 {
			_ = master.Close()
			return nil, nil, fmt.Errorf("ptmx ioctl %#x: %w", step.req, errno)
		}
	}
	end := bytes.IndexByte(name, 0)
	if end < 0 {
		end = len(name)
	}
	slave, err = os.OpenFile(string(name[:end]), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
