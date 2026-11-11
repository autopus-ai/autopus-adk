package diskspace

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func fixedSampler(s MachineSample) MachineSampler {
	return SamplerFunc(func() (MachineSample, error) { return s, nil })
}

func idleSample(availGiB, totalGiB int64) MachineSample {
	return MachineSample{HasLoad: true, HasMem: true, AvailMemBytes: availGiB * GiB, TotalMemBytes: totalGiB * GiB}
}

// The load limit is ratio × logical CPUs, and the waiter is held only when
// the load, in-flight grants included, is above it.
func TestLoadGate_ThresholdMaths(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	p := LoadPolicy{MaxLoadRatio: 0.85, LogicalCPUs: 24}
	for _, tc := range []struct {
		name    string
		load    float64
		live    []LeaseInfo
		limited bool
		reason  string
	}{
		{"idle", 9, nil, false, ""},
		{"at the limit", 20.4, nil, false, ""},
		{"above", 22.1, nil, true, "load 22.1 > 20.4"},
		{"old lease is already visible", 18, []LeaseInfo{{CPUs: 8, Started: now.Add(-10 * time.Minute)}}, false, ""},
		{"fresh lease counts in full", 18, []LeaseInfo{{CPUs: 4, Started: now}}, true, "load 18.0+4.0 in flight > 20.4"},
		{"half-seen lease", 18, []LeaseInfo{{CPUs: 4, Started: now.Add(-time.Minute)}}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := p.Gate(MachineSample{Load1: tc.load, HasLoad: true}, tc.live, 0, now)
			assert.InDelta(t, 20.4, g.LoadLimit, 1e-9)
			assert.Equal(t, tc.limited, g.LoadLimited)
			assert.Equal(t, tc.reason, g.Reason())
		})
	}
	off := LoadPolicy{LogicalCPUs: 24}.Gate(MachineSample{Load1: 99, HasLoad: true}, nil, 0, now)
	assert.False(t, off.LoadLimited, "a zero ratio turns the load check off")
	unknown := p.Gate(MachineSample{Load1: 99}, nil, 0, now)
	assert.False(t, unknown.LoadLimited, "an unread load is not a reason to wait")
}

// A lease started t ago still hides e^(-t/60s) of its CPUs from the load
// average; one past the horizon or without CPUs hides nothing.
func TestInFlightCPUs(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		age  time.Duration
		cpus int
		want float64
	}{
		{0, 4, 4},
		{time.Minute, 4, 4 * math.Exp(-1)},
		{2 * time.Minute, 6, 6 * math.Exp(-2)},
		{5 * time.Minute, 8, 0},
		{-time.Second, 2, 2},
		{0, 0, 0},
	} {
		got := InFlightCPUs([]LeaseInfo{{CPUs: tc.cpus, Started: now.Add(-tc.age)}}, now)
		assert.InDelta(t, tc.want, got, 1e-9, "age %s", tc.age)
	}
}

// Memory: available minus the live --mem reservations must cover the floor
// plus the waiter's own --mem.
func TestLoadGate_MemAccounting(t *testing.T) {
	t.Parallel()
	now := time.Now()
	p := LoadPolicy{MinFreeMemBytes: 8 * GiB}
	live := []LeaseInfo{{MemBytes: 10 * GiB}, {MemBytes: 6 * GiB}, {NeedBytes: 50 * GiB}}
	for _, tc := range []struct {
		name    string
		avail   int64
		mem     int64
		limited bool
		reason  string
	}{
		{"fits exactly", 28 * GiB, 4 * GiB, false, ""},
		{"short by the reservation", 28 * GiB, 5 * GiB, true, "mem 12.0 GiB < 8+5 GiB"},
		{"floor only", 23 * GiB, 0, true, "mem 7.0 GiB < 8 GiB"},
		{"reservations exceed available", 10 * GiB, 0, true, "mem 0.0 GiB < 8 GiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := p.Gate(MachineSample{HasMem: true, AvailMemBytes: tc.avail, TotalMemBytes: 128 * GiB}, live, tc.mem, now)
			assert.Equal(t, 16*GiB, g.ReservedMemBytes)
			assert.Equal(t, tc.limited, g.MemLimited)
			assert.Equal(t, tc.reason, g.Reason())
		})
	}
}

// Critical pressure holds a waiter whatever the numbers say, and every
// reason is listed.
func TestLoadGate_PressureAndCombinedReasons(t *testing.T) {
	t.Parallel()
	p := LoadPolicy{MaxLoadRatio: 0.5, LogicalCPUs: 4, MinFreeMemBytes: 8 * GiB}
	s := MachineSample{Load1: 3, HasLoad: true, HasMem: true, AvailMemBytes: 2 * GiB, PressureCritical: true}
	g := p.Gate(s, nil, 0, time.Now())
	assert.True(t, g.Blocked())
	assert.False(t, g.onlyLoad())
	assert.Equal(t, "load 3.0 > 2.0; mem 2.0 GiB < 8 GiB; memory pressure critical", g.Reason())
}

// A --mem that cannot fit beside the floor even on an empty machine is
// named; an unknown total or no --mem names nothing.
func TestLoadPolicy_NeverFits(t *testing.T) {
	t.Parallel()
	p := LoadPolicy{Sampler: fixedSampler(idleSample(100, 128)), MinFreeMemBytes: 8 * GiB}
	assert.Empty(t, p.NeverFits(120*GiB))
	assert.Equal(t, "--mem 121 GiB plus the 8 GiB free-memory floor exceeds the machine's 128 GiB of memory",
		p.NeverFits(121*GiB))
	assert.Empty(t, p.NeverFits(0))
	assert.Empty(t, LoadPolicy{}.NeverFits(1<<40))
	broken := LoadPolicy{Sampler: SamplerFunc(func() (MachineSample, error) { return MachineSample{}, errors.New("x") })}
	assert.Empty(t, broken.NeverFits(1<<40))
}

// Status explains the head waiter only, and only when its slots are free.
func TestLoadPolicy_ExplainHead(t *testing.T) {
	t.Parallel()
	now := time.Now()
	p := LoadPolicy{Sampler: fixedSampler(MachineSample{Load1: 30, HasLoad: true}), MaxLoadRatio: 0.85, LogicalCPUs: 24}
	ranked := []LeaseInfo{{CPUs: 4}, {CPUs: 2}}
	p.ExplainHead(ranked, nil, 24, now)
	assert.Equal(t, "load 30.0 > 20.4", ranked[0].Blocked)
	assert.Empty(t, ranked[1].Blocked)

	full := []LeaseInfo{{CPUs: 4}}
	p.ExplainHead(full, []LeaseInfo{{CPUs: 22, Started: now.Add(-time.Hour)}}, 24, now)
	assert.Empty(t, full[0].Blocked, "a waiter short of slots waits for slots, not the machine")
}

// /proc parsing: the first loadavg field and MemTotal/MemAvailable in KiB.
func TestParseProcSample(t *testing.T) {
	t.Parallel()
	s := parseProcSample("22.10 9.0 4.0 3/900 1234\n",
		"MemTotal:       131072000 kB\nMemFree:  1000 kB\nMemAvailable:   65536000 kB\n")
	assert.True(t, s.HasLoad)
	assert.InDelta(t, 22.1, s.Load1, 1e-9)
	assert.True(t, s.HasMem)
	assert.Equal(t, int64(131072000)*KiB, s.TotalMemBytes)
	assert.Equal(t, int64(65536000)*KiB, s.AvailMemBytes)

	old := parseProcSample("garbage", "MemTotal: 1000 kB\n")
	assert.False(t, old.HasLoad)
	assert.False(t, old.HasMem, "a kernel without MemAvailable leaves the memory check off")
}

// The real sampler answers on this machine without error.
func TestSystemSampler_Answers(t *testing.T) {
	t.Parallel()
	s, err := SystemSampler().Sample()
	assert.NoError(t, err)
	if s.HasMem {
		assert.Positive(t, s.TotalMemBytes)
		assert.LessOrEqual(t, s.AvailMemBytes, s.TotalMemBytes)
	}
	if s.HasLoad {
		assert.GreaterOrEqual(t, s.Load1, 0.0)
	}
}
