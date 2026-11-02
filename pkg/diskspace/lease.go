package diskspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
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
	probe, err := os.OpenFile(l.path, os.O_RDWR, 0)
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

// AdmitRequest is one admission attempt. Free is measured inside the
// admission lock, so two waiters never admit against the same reading.
type AdmitRequest struct {
	Free          func() (int64, error)
	FloorBytes    int64
	NeedBytes     int64
	MaxConcurrent int
	Label         string
	PID           int
	Now           time.Time
}

// Admission is the outcome of one attempt. Lease is set only when admitted.
type Admission struct {
	Lease          *Lease
	Live           []LeaseInfo
	FreeBytes      int64
	ReservedBytes  int64
	AvailableBytes int64
	// CountLimited means max_concurrent, not space, refused the attempt.
	CountLimited bool
}

// Admitted reports whether the attempt holds a lease.
func (a Admission) Admitted() bool { return a.Lease != nil }

// Admit discards dead leases and admits the request when free space minus
// the live reservations minus the need stays at or above the floor and the
// count limit allows one more (REQ-DISK2-LEASE-01). The lease file is created
// and locked before the admission lock is released.
func Admit(ctx context.Context, state StateDir, req AdmitRequest) (Admission, error) {
	lock, err := filelock.Acquire(ctx, state.Join(leaseLockFile), leaseLockWait)
	if err != nil {
		return Admission{}, fmt.Errorf("disk lease: admission lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	live, err := LiveLeases(state, true)
	if err != nil {
		return Admission{}, err
	}
	free, err := req.Free()
	if err != nil {
		return Admission{}, err
	}
	a := Admission{Live: live, FreeBytes: free}
	for _, info := range live {
		a.ReservedBytes = satAdd(a.ReservedBytes, info.NeedBytes)
	}
	floor := req.FloorBytes
	if floor < 0 {
		// An overflowed floor means one beyond any disk: admit nothing.
		floor = math.MaxInt64
	}
	a.AvailableBytes = max(free-min(a.ReservedBytes, free)-min(floor, free), 0)
	if req.MaxConcurrent > 0 && len(live) >= req.MaxConcurrent {
		a.CountLimited = true
		return a, nil
	}
	// need + reserved + floor <= free, in saturating arithmetic so no sum of
	// bounded needs and no configured floor can wrap negative (review r2).
	if satAdd(satAdd(max(req.NeedBytes, 0), a.ReservedBytes), floor) > free {
		return a, nil
	}
	a.Lease, err = createLease(state, LeaseInfo{PID: req.PID, Label: req.Label, NeedBytes: req.NeedBytes, Started: req.Now.UTC()})
	return a, err
}

func createLease(state StateDir, info LeaseInfo) (*Lease, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return nil, err
	}
	path := filepath.Join(state.LeasesDir(), fmt.Sprintf("%d-%s%s", info.PID, hex.EncodeToString(suffix[:]), leaseSuffix))
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	lease := &Lease{file: file, path: path}
	// A status probe may hold the fresh file's lock for a moment; it never
	// keeps it, so a few short retries ride it out (security review I1).
	busy, err := oslock.TryLock(file)
	for try := 0; err == nil && busy && try < 50; try++ {
		time.Sleep(2 * time.Millisecond)
		busy, err = oslock.TryLock(file)
	}
	if err != nil || busy {
		_ = file.Close()
		_ = os.Remove(path)
		if err == nil {
			err = fmt.Errorf("disk lease: %s is locked by another process", path)
		}
		return nil, err
	}
	data, _ := json.Marshal(info)
	if _, err := file.Write(data); err != nil {
		lease.Release()
		return nil, err
	}
	return lease, nil
}

// LiveLeases lists the leases whose holder still holds their lock. With
// discard, a lease file whose lock could be taken is removed: its holder is
// gone. A live lease whose content does not parse counts with no need.
func LiveLeases(state StateDir, discard bool) ([]LeaseInfo, error) {
	entries, err := os.ReadDir(state.LeasesDir())
	if err != nil {
		return nil, err
	}
	var live []LeaseInfo
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), leaseSuffix) {
			continue
		}
		path := filepath.Join(state.LeasesDir(), entry.Name())
		info, alive := probeLease(path, discard)
		if alive {
			live = append(live, info)
		}
	}
	return live, nil
}

func probeLease(path string, discard bool) (LeaseInfo, bool) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return LeaseInfo{}, false
	}
	defer file.Close()
	busy, err := oslock.TryLock(file)
	if err != nil {
		return LeaseInfo{}, false
	}
	if !busy {
		if discard {
			_ = os.Remove(path)
		}
		_ = oslock.Unlock(file)
		return LeaseInfo{}, false
	}
	var info LeaseInfo
	data, _ := os.ReadFile(path)
	_ = json.Unmarshal(data, &info)
	// A live lease with a need outside the bound is counted at the bound: it
	// is held, so it reserves, but it cannot overflow the sum.
	info.NeedBytes = min(max(info.NeedBytes, 0), MaxLeaseGiB*GiB)
	info.Orphaned = info.PID > 0 && !processAlive(info.PID)
	info.Label = CleanLabel(info.Label)
	return info, true
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
