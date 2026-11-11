//go:build !darwin && !linux

package diskspace

// leaseNoFollow has no portable equivalent here; the regular-file check
// after opening still refuses a non-file.
const leaseNoFollow = 0
