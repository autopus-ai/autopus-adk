package diskspace

import (
	"context"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Automatic reclaim rate-limit files.
const (
	autoReclaimStampFile = "auto-reclaim.stamp"
	autoReclaimLockFile  = "auto-reclaim.lock"
	// autoReclaimLockWait keeps the claim from ever holding up a turn.
	autoReclaimLockWait = 200 * time.Millisecond
)

// ClaimAutoReclaim reports whether an automatic reclaim may start now, and
// when it may, records now as the last start before it returns, under a
// lock, so concurrent sessions start at most one reclaim per interval. A
// stamp in the future (a clock moved back) does not block forever: it is
// read as due.
func ClaimAutoReclaim(state StateDir, now time.Time, interval time.Duration) (bool, error) {
	lock, err := filelock.Acquire(context.Background(), state.Join(autoReclaimLockFile), autoReclaimLockWait)
	if err != nil {
		return false, err
	}
	defer func() { _ = lock.Unlock() }()
	if data, err := state.ReadFile(autoReclaimStampFile); err == nil {
		last, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
		if since := now.Sub(last); err == nil && since >= 0 && since < interval {
			return false, nil
		}
	}
	if err := state.WriteFileAtomic(autoReclaimStampFile, []byte(now.UTC().Format(time.RFC3339Nano)+"\n")); err != nil {
		return false, err
	}
	return true, nil
}
