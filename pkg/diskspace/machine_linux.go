//go:build linux

package diskspace

import "os"

// SystemSampler reads Linux from /proc: the 1-minute load from
// /proc/loadavg and MemTotal and MemAvailable from /proc/meminfo. Linux has
// no pressure level comparable to macOS's, so that check is skipped.
func SystemSampler() MachineSampler {
	return SamplerFunc(func() (MachineSample, error) {
		loadavg, _ := os.ReadFile("/proc/loadavg")
		meminfo, _ := os.ReadFile("/proc/meminfo")
		return parseProcSample(string(loadavg), string(meminfo)), nil
	})
}
