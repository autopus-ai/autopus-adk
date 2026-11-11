package diskspace

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// varSampler is a sampler whose load the test moves.
type varSampler struct{ load atomic.Value }

func newVarSampler(load float64) *varSampler {
	s := &varSampler{}
	s.load.Store(load)
	return s
}

func (s *varSampler) Sample() (MachineSample, error) {
	return MachineSample{Load1: s.load.Load().(float64), HasLoad: true, HasMem: true,
		AvailMemBytes: 64 * GiB, TotalMemBytes: 128 * GiB}, nil
}

func admitLoad(t *testing.T, dir LeaseDir, cpus int, mem int64, p LoadPolicy, ticket *Ticket, now time.Time) Admission {
	t.Helper()
	a, err := AdmitIn(context.Background(), dir, AdmitRequest{CPUs: cpus, CPUSlots: 24, Ticket: ticket, MemBytes: mem,
		Load: p, Label: "load", PID: os.Getpid(), Now: now})
	require.NoError(t, err)
	return a
}

// A busy machine holds the head waiter even with every slot free, and
// those behind it wait for it; once the load drops the head goes first.
func TestAdmitLoad_HeadWaitsForLoadAndKeepsOrder(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	s := newVarSampler(22)
	p := LoadPolicy{Sampler: s, MaxLoadRatio: 0.85, LogicalCPUs: 24}
	now := time.Now()
	head := admitLoad(t, dir, 2, 0, p, nil, now)
	require.False(t, head.Admitted())
	require.NotNil(t, head.Machine)
	assert.Equal(t, "load 22.0 > 20.4", head.Machine.Reason())
	defer head.Ticket.Release()
	behind := admitLoad(t, dir, 1, 0, p, nil, now)
	require.False(t, behind.Admitted())
	defer behind.Ticket.Release()
	assert.Equal(t, 1, behind.Waiting)
	assert.Nil(t, behind.Machine, "a waiter behind the head is held by the queue, not the machine")

	s.load.Store(9.0)
	again := admitLoad(t, dir, 1, 0, p, behind.Ticket, now)
	assert.False(t, again.Admitted(), "the head still goes first")
	first := admitLoad(t, dir, 2, 0, p, head.Ticket, now)
	require.True(t, first.Admitted())
	defer first.Lease.Release()
	second := admitLoad(t, dir, 1, 0, p, behind.Ticket, now)
	require.True(t, second.Admitted(), "2 in flight leaves 9+2 under 20.4")
	second.Lease.Release()
}

// Live --mem reservations count against the next waiter.
func TestAdmitLoad_MemReservations(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	p := LoadPolicy{Sampler: newVarSampler(0), MinFreeMemBytes: 8 * GiB}
	now := time.Now()
	big := admitLoad(t, dir, 1, 40*GiB, p, nil, now)
	require.True(t, big.Admitted())
	defer big.Lease.Release()
	live, err := LiveLeasesIn(dir, false)
	require.NoError(t, err)
	require.Len(t, live, 1)
	assert.Equal(t, 40*GiB, live[0].MemBytes)

	short := admitLoad(t, dir, 1, 17*GiB, p, nil, now)
	require.False(t, short.Admitted())
	defer short.Ticket.Release()
	assert.Equal(t, "mem 24.0 GiB < 8+17 GiB", short.Machine.Reason())
	short.Ticket.Release()
	small := admitLoad(t, dir, 1, 16*GiB, p, nil, now)
	require.True(t, small.Admitted())
	small.Lease.Release()
}

// Load alone gives way once no lease runs and the head has waited
// MaxLoadWait; a running lease, a memory shortage, or a shorter wait does
// not, and the forced admissions go one at a time.
func TestAdmitLoad_ForcedAdmission(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	p := LoadPolicy{Sampler: newVarSampler(40), MaxLoadRatio: 0.85, LogicalCPUs: 24, MaxLoadWait: 10 * time.Minute}
	t0 := time.Now()
	a := admitLoad(t, dir, 2, 0, p, nil, t0)
	require.False(t, a.Admitted())
	b := admitLoad(t, dir, 2, 0, p, nil, t0)
	require.False(t, b.Admitted())
	defer b.Ticket.Release()

	early := admitLoad(t, dir, 2, 0, p, a.Ticket, t0.Add(9*time.Minute))
	require.False(t, early.Admitted())
	forced := admitLoad(t, dir, 2, 0, p, a.Ticket, t0.Add(10*time.Minute))
	require.True(t, forced.Admitted())
	assert.True(t, forced.Forced)

	next := admitLoad(t, dir, 2, 0, p, b.Ticket, t0.Add(11*time.Minute))
	assert.False(t, next.Admitted(), "a forced lease still runs: one at a time")
	forced.Lease.Release()
	next = admitLoad(t, dir, 2, 0, p, b.Ticket, t0.Add(11*time.Minute))
	require.True(t, next.Admitted())
	assert.True(t, next.Forced)
	next.Lease.Release()

	starved := LoadPolicy{Sampler: SamplerFunc(func() (MachineSample, error) {
		return MachineSample{Load1: 40, HasLoad: true, HasMem: true, AvailMemBytes: GiB, TotalMemBytes: 128 * GiB}, nil
	}), MaxLoadRatio: 0.85, LogicalCPUs: 24, MinFreeMemBytes: 8 * GiB, MaxLoadWait: time.Minute}
	c := admitLoad(t, dir, 1, 0, starved, nil, t0)
	defer c.Ticket.Release()
	late := admitLoad(t, dir, 1, 0, starved, c.Ticket, t0.Add(time.Hour))
	assert.False(t, late.Admitted(), "memory is never forced")
}

// Without a sampler, or with a failing one, admission is slot-only.
func TestAdmitLoad_NoSamplerIsSlotOnly(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	failing := SamplerFunc(func() (MachineSample, error) { return MachineSample{}, os.ErrNotExist })
	for _, p := range []LoadPolicy{{}, {Sampler: failing, MaxLoadRatio: 0.1, LogicalCPUs: 1, MinFreeMemBytes: 1 << 50}} {
		a := admitLoad(t, dir, 1, 0, p, nil, time.Now())
		require.True(t, a.Admitted())
		assert.Nil(t, a.Machine)
		a.Lease.Release()
	}
}
