package cli

// initMachineOps is the machine_ops value init writes. A project that
// already has autopus.yaml, here or in a directory above (a monorepo
// package), keeps that file's value, so a re-init or a nested init does
// not turn the shared-machine helpers on or off behind the user's back. A
// new project starts with the verification core only.
func initMachineOps(dir string) *bool {
	if existing := nearestConfig(dir); existing != nil {
		return existing.MachineOps
	}
	off := false
	return &off
}
