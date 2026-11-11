package diskspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/oslock"
)

// waitSuffix names the queue tickets of CPU waiters.
const waitSuffix = ".wait"

// Ticket is a waiter's place in the CPU queue: a flocked file whose lock
// ends with its process, so a killed waiter leaves the queue by itself.
type Ticket struct {
	file *os.File
	path string
	info LeaseInfo
}

// Release leaves the queue. It is safe on a nil Ticket and more than once.
func (t *Ticket) Release() {
	if t == nil || t.file == nil {
		return
	}
	_ = os.Remove(t.path)
	_ = t.file.Close()
	t.file = nil
}

// createLocked creates name in the lease dir, takes its lock, and writes
// info into it. The returned file stays open and locked.
func createLocked(d LeaseDir, name string, info LeaseInfo) (*os.File, string, error) {
	path := filepath.Join(d.Path, name)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, d.leaseFileMode())
	if err != nil {
		return nil, "", err
	}
	if d.shared {
		// The umask must not hide the file from the other users who probe it.
		_ = file.Chmod(d.leaseFileMode())
	}
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
		return nil, "", err
	}
	data, _ := json.Marshal(info)
	if _, err := file.Write(data); err != nil {
		_ = os.Remove(path)
		_ = file.Close()
		return nil, "", err
	}
	return file, path, nil
}

func randomSuffix() (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(suffix[:]), nil
}

func createLease(d LeaseDir, info LeaseInfo) (*Lease, error) {
	suffix, err := randomSuffix()
	if err != nil {
		return nil, err
	}
	file, path, err := createLocked(d, fmt.Sprintf("%d-%s%s", info.PID, suffix, leaseSuffix), info)
	if err != nil {
		return nil, err
	}
	return &Lease{file: file, path: path}, nil
}

func enqueue(d LeaseDir, info LeaseInfo) (*Ticket, error) {
	suffix, err := randomSuffix()
	if err != nil {
		return nil, err
	}
	file, path, err := createLocked(d, fmt.Sprintf("%020d-%d-%s%s", info.Seq, info.PID, suffix, waitSuffix), info)
	if err != nil {
		return nil, err
	}
	return &Ticket{file: file, path: path, info: info}, nil
}

// LiveLeases lists the leases of the per-user state dir; see LiveLeasesIn.
func LiveLeases(state StateDir, discard bool) ([]LeaseInfo, error) {
	return LiveLeasesIn(state.Leases(), discard)
}

// LiveLeasesIn lists the leases whose holder still holds their lock. With
// discard, a lease file whose lock could be taken is removed: its holder is
// gone. A live lease whose content does not parse counts with no need.
func LiveLeasesIn(d LeaseDir, discard bool) ([]LeaseInfo, error) {
	return liveIn(d, leaseSuffix, discard)
}

// LiveWaiters lists the live CPU waiters in queue order, without
// discarding dead ones.
func LiveWaiters(d LeaseDir) ([]LeaseInfo, error) { return liveIn(d, waitSuffix, false) }

func liveIn(d LeaseDir, suffix string, discard bool) ([]LeaseInfo, error) {
	entries, err := os.ReadDir(d.Path)
	if err != nil {
		return nil, err
	}
	var live []LeaseInfo
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		if info, alive := probeLease(filepath.Join(d.Path, entry.Name()), discard); alive {
			live = append(live, info)
		}
	}
	sort.SliceStable(live, func(i, j int) bool { return live[i].Seq < live[j].Seq })
	return live, nil
}

func probeLease(path string, discard bool) (LeaseInfo, bool) {
	// Read-only: another user's file in a shared dir is readable only, and
	// a flock needs no write access.
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
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
			// In a sticky shared dir another user's dead file cannot be
			// removed; it is skipped as dead and its owner removes it later.
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
	info.CPUs = min(max(info.CPUs, 0), MaxLeaseCPUs)
	info.Orphaned = info.PID > 0 && !processAlive(info.PID)
	info.Label = CleanLabel(info.Label)
	return info, true
}
