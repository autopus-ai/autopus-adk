//go:build !darwin && !linux

package diskspace

import "runtime"

// Free has no reading on this operating system (REQ-DISK-FREE-02).
func Free(string) (Space, error) {
	return Space{}, UnsupportedOn(runtime.GOOS)
}
