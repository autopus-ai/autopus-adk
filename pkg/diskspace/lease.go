package diskspace

import (
	"math"
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/oslock"
)

// Lease files (SPEC-DISK-002 REQ-DISK2-LEASE-01).
const (
	leaseLockFile = "lease.lock"
	leaseSuffix   = ".lease"
	leaseLockWait = 30 * time.Second

	// MaxLeaseGiB bounds one lease's need, 1 PiB, so no sum or difference of
	// needs and free space can overflow int64 (review F-11).
	MaxLeaseGiB = 1 << 20

	// releaseProbeTries × releaseProbeWait bounds how long Release waits out
	// a transient holder before leaving the file to the next admission.
	releaseProbeTries = 50
	releaseProbeWait  = 5 * time.Millisecond
)

// LeaseInfo is what a lease file records about its holder.
type LeaseInfo struct {
	PID       int       `json:"pid"`
	Label     string    `json:"label,omitempty"`
	NeedBytes int64     `json:"need_bytes"`
	Started   time.Time `json:"started"`
	// Orphaned marks a live lease whose wrapper process is gone: a process
	// the command started still holds it (security review M2). It still
	// reserves, because that process may still be writing.
	Orphaned bool `json:"orphaned"`
	// CPUs is the number of CPU slots the lease holds, or the waiter asks
	// for; 0 for a disk-only lease.
	CPUs int `json:"cpus,omitempty"`
	// Seq orders the waiters of the CPU queue; 0 on a lease.
	Seq int64 `json:"seq,omitempty"`
}

// Lease is a held lease: its file stays flocked until Release, or until the
// holding process dies, which frees the reservation with no bookkeeping.
type Lease struct {
	file *os.File
	path string
}

// File is the locked lease file. A command run under the lease inherits it,
// so the reservation lives as long as either holder: the wrapper dying
// alone does not free space the command is still filling.
func (l *Lease) File() *os.File { return l.file }

// Release closes this process's descriptor and removes the lease file only
// when nobody else holds its lock. It never unlocks explicitly: the flock
// belongs to an open file description the command and its children may
// still share, and an explicit unlock would free a reservation an orphaned
// build is still filling. A file left behind is discarded by the next
// admission once its last holder exits. It is safe on a nil Lease.
//
// The probe retries for a short while before leaving the file: a fork by
// another goroutine of this process copies every descriptor, close-on-exec
// ones included, and holds them until its exec, so a lock can look held for
// that instant even though no command still runs under it.
func (l *Lease) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
	l.file = nil
	// Read-only: a flock needs no write access, and in a shared lease dir
	// the file may be readable only.
	probe, err := os.OpenFile(l.path, os.O_RDONLY, 0)
	if err != nil {
		return
	}
	defer probe.Close()
	for try := 0; try < releaseProbeTries; try++ {
		busy, err := oslock.TryLock(probe)
		if err != nil {
			return
		}
		if !busy {
			_ = os.Remove(l.path)
			_ = oslock.Unlock(probe)
			return
		}
		time.Sleep(releaseProbeWait)
	}
}

// satAdd adds two non-negative int64 values, saturating at math.MaxInt64.
func satAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// CleanLabel drops control characters from a lease label and cuts it to 200
// bytes, so a label is safe to print and never floods the event log
// (security review L4).
func CleanLabel(label string) string { return cleanOwner(label) }
