//go:build unix

package secretscan

import (
	"syscall"
	"time"
)

// processTime is the user and system CPU time of this process. Unlike wall
// time it does not grow while the process waits for a CPU that other
// processes hold, so a timing ratio survives a loaded runner.
func processTime() time.Duration {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		panic("getrusage: " + err.Error())
	}
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
}
