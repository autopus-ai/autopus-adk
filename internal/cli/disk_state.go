package cli

import (
	"os"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// diskStatePath is the state dir of SPEC-DISK-002, a seam so tests never
// touch the real one.
var diskStatePath = func() string {
	return diskspace.StateDirPath(diskHome(), os.Getenv("XDG_STATE_HOME"))
}

// openDiskState opens the state dir under the current uid.
func openDiskState() (diskspace.StateDir, error) {
	return diskspace.OpenStateDir(diskStatePath(), diskUID())
}

// recordDiskEvent appends an event when the state dir can be opened. It
// never fails its caller (REQ-DISK2-LOG-01).
func recordDiskEvent(event diskspace.Event) {
	state, err := openDiskState()
	if err != nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = diskNow().UTC()
	}
	_ = diskspace.AppendEvent(state, event)
}

// recordDiskSample appends a free-space sample when the state dir can be
// opened, and returns the samples read before it for a trend.
func recordDiskSample(space diskspace.Space) []diskspace.Sample {
	state, err := openDiskState()
	if err != nil {
		return nil
	}
	samples := diskspace.LoadSamples(state)
	_ = diskspace.RecordSample(state, diskspace.Sample{Time: diskNow().UTC(), FreeBytes: space.AvailBytes, Mount: space.Mount})
	return samples
}
