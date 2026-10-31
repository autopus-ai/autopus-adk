//go:build !darwin && !linux

package diskspace

import (
	"fmt"
	"runtime"
)

// rootHandle cannot be opened here: removal needs the fd-relative no-follow
// calls of REQ-DISK-TRIM-05, and free space is unmeasured anyway, so a trim
// stops before it would remove anything.
type rootHandle struct{}

func openRootHandle(Root) (*rootHandle, error) {
	return nil, fmt.Errorf("removal is not supported on %s", runtime.GOOS)
}

func (h *rootHandle) close() {}

func (h *rootHandle) remove(Candidate, int, func(rule, rel string)) outcome { return outcomeFailed }
