//go:build darwin

package diskspace

import (
	"encoding/binary"
	"errors"

	"golang.org/x/sys/unix"
)

// darwinPressureCritical is kern.memorystatus_vm_pressure_level's critical
// value (1 normal, 2 warning, 4 critical).
const darwinPressureCritical = 4

// SystemSampler reads macOS through sysctl, with no process spawned: the
// 1-minute load from vm.loadavg, the total from hw.memsize, the available
// memory from kern.memorystatus_level (the free percentage memory_pressure
// prints, which counts inactive and purgeable pages as available), and the
// kernel's memory pressure level.
func SystemSampler() MachineSampler { return SamplerFunc(sampleDarwin) }

func sampleDarwin() (MachineSample, error) {
	var s MachineSample
	if load, err := darwinLoad1(); err == nil {
		s.Load1, s.HasLoad = load, true
	}
	if total, err := unix.SysctlUint64("hw.memsize"); err == nil && total > 0 {
		s.TotalMemBytes = int64(min(total, uint64(MaxLeaseGiB*GiB)))
		if avail, ok := darwinAvailable(s.TotalMemBytes); ok {
			s.AvailMemBytes, s.HasMem = avail, true
		}
	}
	if level, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level"); err == nil {
		s.PressureCritical = level >= darwinPressureCritical
	}
	return s, nil
}

// darwinLoad1 decodes struct loadavg { fixpt_t ldavg[3]; long fscale; }.
func darwinLoad1() (float64, error) {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil {
		return 0, err
	}
	if len(raw) < 24 {
		return 0, errors.New("vm.loadavg: short read")
	}
	fscale := binary.LittleEndian.Uint64(raw[16:24])
	if fscale == 0 {
		return 0, errors.New("vm.loadavg: zero fscale")
	}
	return float64(binary.LittleEndian.Uint32(raw[0:4])) / float64(fscale), nil
}

// darwinAvailable is total × kern.memorystatus_level / 100, or, where that
// key is missing, the free, speculative, and purgeable pages: a lower bound
// that leaves out inactive pages, which no sysctl reports.
func darwinAvailable(total int64) (int64, bool) {
	if pct, err := unix.SysctlUint32("kern.memorystatus_level"); err == nil && pct <= 100 {
		return total / 100 * int64(pct), true
	}
	page, err := unix.SysctlUint32("hw.pagesize")
	if err != nil || page == 0 {
		return 0, false
	}
	var pages uint64
	for _, key := range []string{"vm.page_free_count", "vm.page_speculative_count", "vm.page_purgeable_count"} {
		n, err := unix.SysctlUint32(key)
		if err != nil {
			return 0, false
		}
		pages += uint64(n)
	}
	return int64(min(pages*uint64(page), uint64(total))), true
}
