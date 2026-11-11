package diskspace

import (
	"strconv"
	"strings"
)

// parseProcSample reads /proc/loadavg and /proc/meminfo text. It lives
// outside the Linux file so every platform's tests cover the parsing.
func parseProcSample(loadavg, meminfo string) MachineSample {
	var s MachineSample
	if fields := strings.Fields(loadavg); len(fields) > 0 {
		if load, err := strconv.ParseFloat(fields[0], 64); err == nil && load >= 0 {
			s.Load1, s.HasLoad = load, true
		}
	}
	var total, avail int64
	var haveAvail bool
	for _, line := range strings.Split(meminfo, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		kib, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || kib < 0 || kib > MaxLeaseGiB*GiB/KiB {
			continue
		}
		switch key {
		case "MemTotal":
			total = kib * KiB
		case "MemAvailable":
			avail, haveAvail = kib*KiB, true
		}
	}
	if total > 0 && haveAvail {
		s.TotalMemBytes, s.AvailMemBytes, s.HasMem = total, min(avail, total), true
	}
	return s
}
