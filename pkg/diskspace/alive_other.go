//go:build !darwin && !linux

package diskspace

// processAlive cannot ask the system here; a live lease is assumed to have
// its wrapper.
func processAlive(int) bool { return true }
