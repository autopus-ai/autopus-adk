package diskspace

import (
	"context"
	"fmt"
	"math"
	"time"
)

// MaxLeaseCPUs bounds one lease's CPU slots, so no sum of slots overflows.
const MaxLeaseCPUs = 1 << 16

// AdmitRequest is one admission attempt. Free is measured inside the
// admission lock, so two waiters never admit against the same reading. A
// request with NeedBytes 0 is CPU-only and never measures free space.
type AdmitRequest struct {
	Free          func() (int64, error)
	FloorBytes    int64
	NeedBytes     int64
	MaxConcurrent int
	// CPUs is how many of CPUSlots the command reserves; 0 asks for none.
	CPUs     int
	CPUSlots int
	// Ticket is the caller's place in the CPU queue from an earlier refused
	// attempt; nil on the first attempt. An admission consumes it.
	Ticket *Ticket
	// Priority is the class of a CPU request (ParsePriority); empty is ci.
	// AgingInterval raises a waiter one class per interval; 0 disables it.
	Priority      string
	AgingInterval time.Duration
	// MemBytes is the memory a CPU request reserves (--mem); Load makes
	// the head waiter also wait for the machine's load and memory.
	MemBytes int64
	Load     LoadPolicy
	Label    string
	PID      int
	Now      time.Time
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
	// DiskShort means free space above the floor refused the attempt.
	DiskShort bool
	// ReservedCPUs is the slots the live leases hold; CPULimited means they
	// leave too few for the request.
	ReservedCPUs int
	CPULimited   bool
	// Waiting is how many live waiters queued before this request.
	Waiting int
	// Machine is the machine check of a head waiter whose slots were free;
	// nil when it was not reached or the policy has no sampler.
	Machine *MachineGate
	// Forced means the lease was admitted despite load: no lease ran and
	// the waiter had waited LoadPolicy.MaxLoadWait.
	Forced bool
	// Ticket is set when a refused CPU request without a ticket was queued;
	// the caller keeps it and passes it to its next attempts.
	Ticket *Ticket
}

// Admitted reports whether the attempt holds a lease.
func (a Admission) Admitted() bool { return a.Lease != nil }

// Admit admits a request in the per-user state dir; see AdmitIn.
func Admit(ctx context.Context, state StateDir, req AdmitRequest) (Admission, error) {
	return AdmitIn(ctx, state.Leases(), req)
}

// AdmitIn discards dead leases and admits the request when both budgets
// allow it (REQ-DISK2-LEASE-01). Disk: free space minus the live
// reservations minus the need stays at or above the floor, and the count
// limit allows one more disk lease. CPU: the live leases leave enough slots
// and no live waiter queued earlier, so CPU waiters are served first in,
// first out. The lease file is created and locked before the admission
// lock is released.
func AdmitIn(ctx context.Context, d LeaseDir, req AdmitRequest) (Admission, error) {
	unlock, err := d.lock(ctx)
	if err != nil {
		return Admission{}, fmt.Errorf("disk lease: admission lock: %w", err)
	}
	defer unlock()
	live, err := LiveLeasesIn(d, true)
	if err != nil {
		return Admission{}, err
	}
	a := Admission{Live: live}
	diskLeases := 0
	for _, info := range live {
		a.ReservedBytes = satAdd(a.ReservedBytes, info.NeedBytes)
		a.ReservedCPUs += info.CPUs
		if info.NeedBytes > 0 {
			diskLeases++
		}
	}
	fits := true
	if req.NeedBytes > 0 {
		if fits, err = admitDisk(&a, req, diskLeases); err != nil {
			return Admission{}, err
		}
	}
	if req.CPUs > 0 {
		waiters, err := liveIn(d, waitSuffix, true)
		if err != nil {
			return Admission{}, err
		}
		var lastSeq int64
		for _, w := range waiters {
			lastSeq = max(lastSeq, w.Seq)
		}
		// A request without a ticket stands where it would be queued now.
		self := LeaseInfo{PID: req.PID, Label: req.Label, NeedBytes: max(req.NeedBytes, 0), CPUs: req.CPUs,
			Seq: lastSeq + 1, Priority: cpuPriority(req), Started: req.Now.UTC(), MemBytes: max(req.MemBytes, 0)}
		if req.Ticket != nil {
			self = req.Ticket.info
		}
		for _, w := range waiters {
			if w.Seq != self.Seq && grantedBefore(w, self, req.Now, req.AgingInterval) {
				a.Waiting++
			}
		}
		a.CPULimited = a.ReservedCPUs+req.CPUs > req.CPUSlots
		if a.CPULimited || a.Waiting > 0 {
			fits = false
		} else if fits {
			fits = admitMachine(&a, req, live, self.Started)
		}
		if !fits && req.Ticket == nil {
			a.Ticket, err = enqueue(d, self)
			return a, err
		}
	}
	if !fits {
		return a, nil
	}
	a.Lease, err = createLease(d, LeaseInfo{PID: req.PID, Label: req.Label, NeedBytes: req.NeedBytes,
		CPUs: req.CPUs, Priority: cpuPriority(req), Started: req.Now.UTC(), MemBytes: cpuMem(req)})
	if err == nil {
		req.Ticket.Release()
	}
	return a, err
}

// admitMachine applies the load policy to the head waiter, recording the
// check in a; it reports whether the machine lets the waiter in. Load alone
// gives way once no lease runs and the waiter has queued MaxLoadWait, so
// unrelated load never blocks heavy work forever; the forced admissions go
// one at a time because the next one again needs no lease running.
func admitMachine(a *Admission, req AdmitRequest, live []LeaseInfo, queued time.Time) bool {
	s, ok := req.Load.sample()
	if !ok {
		return true
	}
	gate := req.Load.Gate(s, live, req.MemBytes, req.Now)
	a.Machine = &gate
	if !gate.Blocked() {
		return true
	}
	if gate.onlyLoad() && len(live) == 0 && req.Load.MaxLoadWait > 0 && req.Now.Sub(queued) >= req.Load.MaxLoadWait {
		a.Forced = true
		return true
	}
	return false
}

// cpuMem is the memory a lease records: none for a disk-only lease.
func cpuMem(req AdmitRequest) int64 {
	if req.CPUs <= 0 {
		return 0
	}
	return max(req.MemBytes, 0)
}

// cpuPriority is the class a lease records: none for a disk-only lease.
func cpuPriority(req AdmitRequest) string {
	if req.CPUs <= 0 {
		return ""
	}
	if req.Priority == "" {
		return PriorityCI
	}
	return req.Priority
}

// admitDisk measures free space and applies the disk budget, recording its
// numbers in a; it reports whether the disk part fits.
func admitDisk(a *Admission, req AdmitRequest, diskLeases int) (bool, error) {
	free, err := req.Free()
	if err != nil {
		return false, err
	}
	a.FreeBytes = free
	floor := req.FloorBytes
	if floor < 0 {
		// An overflowed floor means one beyond any disk: admit nothing.
		floor = math.MaxInt64
	}
	a.AvailableBytes = max(free-min(a.ReservedBytes, free)-min(floor, free), 0)
	if req.MaxConcurrent > 0 && diskLeases >= req.MaxConcurrent {
		a.CountLimited = true
		return false, nil
	}
	// need + reserved + floor <= free, in saturating arithmetic so no sum of
	// bounded needs and no configured floor can wrap negative (review r2).
	if satAdd(satAdd(max(req.NeedBytes, 0), a.ReservedBytes), floor) > free {
		a.DiskShort = true
		return false, nil
	}
	return true, nil
}
