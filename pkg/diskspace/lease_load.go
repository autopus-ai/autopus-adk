package diskspace

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// The 1-minute load average is an exponential moving average with a
// 60-second time constant: a process started t seconds ago shows in it at
// only 1-e^(-t/60) of its weight. Admission counts the unseen rest of every
// recent lease so a burst of admissions cannot overshoot before the load
// average catches up. Beyond the horizon the unseen part is negligible.
const (
	loadTimeConstant = 60 * time.Second
	inFlightHorizon  = 5 * time.Minute
)

// MachineSample is one reading of the machine's load and memory. A field
// whose Has flag is false could not be read and its check is skipped.
type MachineSample struct {
	Load1         float64
	HasLoad       bool
	AvailMemBytes int64
	TotalMemBytes int64
	HasMem        bool
	// PressureCritical is the kernel's critical memory pressure level, read
	// on macOS only; elsewhere it stays false.
	PressureCritical bool
}

// MachineSampler reads the machine; tests inject fixed readings.
type MachineSampler interface {
	Sample() (MachineSample, error)
}

// SamplerFunc adapts a function to MachineSampler.
type SamplerFunc func() (MachineSample, error)

// Sample calls f.
func (f SamplerFunc) Sample() (MachineSample, error) { return f() }

// LoadPolicy is the load- and memory-aware part of CPU admission. A zero
// policy, or one without a Sampler, admits on slots alone.
type LoadPolicy struct {
	Sampler MachineSampler
	// MaxLoadRatio × LogicalCPUs is the highest 1-minute load, in-flight
	// grants included, at which a waiter is admitted; 0 turns the check off.
	MaxLoadRatio float64
	LogicalCPUs  int
	// MinFreeMemBytes is the memory that stays free after every --mem
	// reservation, the waiter's included.
	MinFreeMemBytes int64
	// MaxLoadWait admits the head waiter despite load once it has waited
	// this long while no lease runs; 0 turns forced admission off.
	MaxLoadWait time.Duration
}

// MachineGate is the machine check of one waiter at one instant.
type MachineGate struct {
	Load        float64 `json:"load"`
	InFlight    float64 `json:"in_flight"`
	LoadLimit   float64 `json:"load_limit"`
	LoadLimited bool    `json:"load_limited"`
	// FreeMemBytes is the available memory minus the live reservations.
	FreeMemBytes     int64 `json:"free_mem_bytes"`
	ReservedMemBytes int64 `json:"reserved_mem_bytes"`
	FloorMemBytes    int64 `json:"floor_mem_bytes"`
	NeedMemBytes     int64 `json:"need_mem_bytes"`
	MemLimited       bool  `json:"mem_limited"`
	PressureCritical bool  `json:"pressure_critical"`
}

// Blocked reports whether any machine check refuses the waiter.
func (g MachineGate) Blocked() bool { return g.LoadLimited || g.MemLimited || g.PressureCritical }

// onlyLoad reports whether load is the one check that refuses the waiter.
func (g MachineGate) onlyLoad() bool { return g.LoadLimited && !g.MemLimited && !g.PressureCritical }

// Reason says why the waiter is held, as "load 22.1 > 20.4" or
// "mem 6.2 GiB < 8+4 GiB"; empty when nothing holds it.
func (g MachineGate) Reason() string {
	var parts []string
	if g.LoadLimited {
		load := strconv.FormatFloat(g.Load, 'f', 1, 64)
		if g.InFlight >= 0.05 {
			load = strconv.FormatFloat(g.Load-g.InFlight, 'f', 1, 64) + "+" +
				strconv.FormatFloat(g.InFlight, 'f', 1, 64) + " in flight"
		}
		parts = append(parts, fmt.Sprintf("load %s > %.1f", load, g.LoadLimit))
	}
	if g.MemLimited {
		want := compactGiB(g.FloorMemBytes)
		if g.NeedMemBytes > 0 {
			want += "+" + compactGiB(g.NeedMemBytes)
		}
		parts = append(parts, fmt.Sprintf("mem %s GiB < %s GiB", GiBString(max(g.FreeMemBytes, 0)), want))
	}
	if g.PressureCritical {
		parts = append(parts, "memory pressure critical")
	}
	return strings.Join(parts, "; ")
}

// compactGiB prints whole GiB without a decimal: "8", "4.5".
func compactGiB(bytes int64) string {
	return strings.TrimSuffix(GiBString(bytes), ".0")
}

// InFlightCPUs is the part of the live leases' CPUs the 1-minute load
// average does not show yet.
func InFlightCPUs(live []LeaseInfo, now time.Time) float64 {
	var sum float64
	for _, info := range live {
		age := now.Sub(info.Started)
		if info.CPUs <= 0 || age >= inFlightHorizon {
			continue
		}
		age = max(age, 0)
		sum += float64(info.CPUs) * math.Exp(-age.Seconds()/loadTimeConstant.Seconds())
	}
	return sum
}

// Gate applies the policy to a sample: the load, in-flight grants
// included, must stay at or under the limit; the available memory minus
// the live --mem reservations must cover the floor and the waiter's own
// --mem; the kernel must not report critical pressure.
func (p LoadPolicy) Gate(s MachineSample, live []LeaseInfo, memBytes int64, now time.Time) MachineGate {
	var g MachineGate
	if s.HasLoad && p.MaxLoadRatio > 0 && p.LogicalCPUs > 0 {
		g.InFlight = InFlightCPUs(live, now)
		g.Load = s.Load1 + g.InFlight
		g.LoadLimit = p.MaxLoadRatio * float64(p.LogicalCPUs)
		g.LoadLimited = g.Load > g.LoadLimit
	}
	if s.HasMem {
		for _, info := range live {
			g.ReservedMemBytes = satAdd(g.ReservedMemBytes, info.MemBytes)
		}
		g.FreeMemBytes = s.AvailMemBytes - g.ReservedMemBytes
		g.FloorMemBytes, g.NeedMemBytes = max(p.MinFreeMemBytes, 0), max(memBytes, 0)
		g.MemLimited = g.FreeMemBytes < satAdd(g.FloorMemBytes, g.NeedMemBytes)
	}
	g.PressureCritical = s.PressureCritical
	return g
}

// sample reads the machine; a sampler error reads as nothing known, so a
// broken reading falls back to slot-only admission instead of blocking.
func (p LoadPolicy) sample() (MachineSample, bool) {
	if p.Sampler == nil {
		return MachineSample{}, false
	}
	s, err := p.Sampler.Sample()
	if err != nil {
		return MachineSample{}, false
	}
	return s, true
}

// NeverFits names why a request could never be admitted on this machine
// alone: its --mem and the free-memory floor exceed the total memory.
// It returns "" when the request can fit or the memory is unknown.
func (p LoadPolicy) NeverFits(memBytes int64) string {
	s, ok := p.sample()
	if !ok || !s.HasMem || s.TotalMemBytes <= 0 || memBytes <= 0 {
		return ""
	}
	if want := satAdd(memBytes, max(p.MinFreeMemBytes, 0)); want > s.TotalMemBytes {
		return fmt.Sprintf("--mem %s GiB plus the %s GiB free-memory floor exceeds the machine's %s GiB of memory",
			compactGiB(memBytes), compactGiB(p.MinFreeMemBytes), compactGiB(s.TotalMemBytes))
	}
	return ""
}

// ExplainHead sets Blocked on the first of the ranked waiters when its
// slots are free and only the machine holds it, for `auto disk status`.
// The waiters behind it wait for it, so only the head is explained.
func (p LoadPolicy) ExplainHead(ranked, live []LeaseInfo, slots int, now time.Time) {
	if len(ranked) == 0 {
		return
	}
	reserved := 0
	for _, info := range live {
		reserved += info.CPUs
	}
	if reserved+ranked[0].CPUs > slots {
		return
	}
	if s, ok := p.sample(); ok {
		ranked[0].Blocked = p.Gate(s, live, ranked[0].MemBytes, now).Reason()
	}
}
