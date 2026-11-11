package diskspace

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAging = 10 * time.Minute

var priorityT0 = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

// admitAt is one CPU attempt with an explicit class and an injected clock.
func admitAt(t *testing.T, dir LeaseDir, cpus int, class string, ticket *Ticket, now time.Time) Admission {
	t.Helper()
	a, err := AdmitIn(context.Background(), dir, AdmitRequest{CPUs: cpus, CPUSlots: 4, Ticket: ticket,
		Priority: class, AgingInterval: testAging, Label: class, PID: os.Getpid(), Now: now})
	require.NoError(t, err)
	return a
}

func TestParsePriority(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"": PriorityCI, "ci": PriorityCI, "Interactive": PriorityInteractive, " background ": PriorityBackground} {
		got, err := ParsePriority(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}
	_, err := ParsePriority("urgent")
	assert.Error(t, err)
}

// A background waiter queued before an interactive one is served after it.
func TestPriority_InteractivePassesEarlierBackground(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	holder := admitAt(t, dir, 4, PriorityCI, nil, priorityT0)
	require.True(t, holder.Admitted())
	bg := admitAt(t, dir, 2, PriorityBackground, nil, priorityT0)
	require.NotNil(t, bg.Ticket)
	fg := admitAt(t, dir, 2, PriorityInteractive, nil, priorityT0.Add(time.Minute))
	require.NotNil(t, fg.Ticket)
	assert.Zero(t, fg.Waiting, "the interactive waiter is ahead of the background one")
	holder.Lease.Release()

	now := priorityT0.Add(2 * time.Minute)
	blocked := admitAt(t, dir, 2, PriorityBackground, bg.Ticket, now)
	assert.False(t, blocked.Admitted())
	assert.Equal(t, 1, blocked.Waiting)
	served := admitAt(t, dir, 2, PriorityInteractive, fg.Ticket, now)
	require.True(t, served.Admitted())
	next := admitAt(t, dir, 2, PriorityBackground, bg.Ticket, now)
	require.True(t, next.Admitted(), "2 + 2 slots fit once the interactive waiter is served")
	served.Lease.Release()
	next.Lease.Release()
}

// Aging raises a waiter one class per interval: after one interval a
// background waiter is ci and still behind interactive; after two it is
// interactive, and as the earlier waiter of that class it goes first.
func TestPriority_AgingPreventsStarvation(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	holder := admitAt(t, dir, 4, PriorityCI, nil, priorityT0)
	require.True(t, holder.Admitted())
	bg := admitAt(t, dir, 4, PriorityBackground, nil, priorityT0)
	require.NotNil(t, bg.Ticket)
	fg := admitAt(t, dir, 4, PriorityInteractive, nil, priorityT0.Add(14*time.Minute))
	require.NotNil(t, fg.Ticket)
	holder.Lease.Release()

	oneInterval := priorityT0.Add(15 * time.Minute)
	early := admitAt(t, dir, 4, PriorityBackground, bg.Ticket, oneInterval)
	assert.False(t, early.Admitted(), "background aged once is ci, still behind interactive")
	waiters, err := LiveWaiters(dir)
	require.NoError(t, err)
	ranked := RankWaiters(waiters, oneInterval, testAging)
	require.Len(t, ranked, 2)
	assert.Equal(t, []string{PriorityInteractive, PriorityBackground}, []string{ranked[0].Priority, ranked[1].Priority})
	assert.Equal(t, []string{PriorityInteractive, PriorityCI}, []string{ranked[0].EffectivePriority, ranked[1].EffectivePriority})

	twoIntervals := priorityT0.Add(21 * time.Minute)
	newer := admitAt(t, dir, 4, PriorityInteractive, fg.Ticket, twoIntervals)
	assert.False(t, newer.Admitted(), "the aged background waiter is now interactive and earlier")
	aged := admitAt(t, dir, 4, PriorityBackground, bg.Ticket, twoIntervals)
	require.True(t, aged.Admitted())
	aged.Lease.Release()
	fg.Ticket.Release()
}

// Within one class the queue stays first in, first out, and a newcomer
// without a ticket waits behind every live waiter of its class or above.
func TestPriority_FIFOWithinClass(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	holder := admitAt(t, dir, 4, PriorityBackground, nil, priorityT0)
	require.True(t, holder.Admitted())
	first := admitAt(t, dir, 1, PriorityInteractive, nil, priorityT0)
	second := admitAt(t, dir, 1, PriorityInteractive, nil, priorityT0)
	require.NotNil(t, first.Ticket)
	require.NotNil(t, second.Ticket)
	assert.Equal(t, 1, second.Waiting)
	holder.Lease.Release()

	jump := admitAt(t, dir, 1, PriorityInteractive, second.Ticket, priorityT0)
	assert.False(t, jump.Admitted())
	newcomer := admitAt(t, dir, 1, PriorityInteractive, nil, priorityT0)
	assert.False(t, newcomer.Admitted())
	assert.Equal(t, 2, newcomer.Waiting)
	newcomer.Ticket.Release()
	a := admitAt(t, dir, 1, PriorityInteractive, first.Ticket, priorityT0)
	b := admitAt(t, dir, 1, PriorityInteractive, second.Ticket, priorityT0)
	require.True(t, a.Admitted())
	require.True(t, b.Admitted())
	a.Lease.Release()
	b.Lease.Release()
}
