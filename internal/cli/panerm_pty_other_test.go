//go:build !darwin && !linux

package cli_test

import (
	"errors"
	"os"
)

// openPanermPTY has no portable implementation here; the S6 oracle skips.
func openPanermPTY() (master, slave *os.File, err error) {
	return nil, nil, errors.New("pseudo-terminals are opened only on darwin and linux")
}
