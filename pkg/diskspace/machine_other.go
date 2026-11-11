//go:build !darwin && !linux

package diskspace

// SystemSampler knows nothing about the machine here: CPU leases fall back
// to slot-only admission (and on Windows run unleased).
func SystemSampler() MachineSampler {
	return SamplerFunc(func() (MachineSample, error) { return MachineSample{}, nil })
}
